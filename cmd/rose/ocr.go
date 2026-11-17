package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/tjagithub/rosebud/internal/cache"
	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/engine"
	"github.com/tjagithub/rosebud/internal/limit"
	"github.com/tjagithub/rosebud/internal/lock"
	"github.com/tjagithub/rosebud/internal/ocr"
	"github.com/tjagithub/rosebud/internal/scenes"
	"github.com/tjagithub/rosebud/internal/store"
)

const nemotronModel = "mlx-community/Nemotron-Parse-2.0-8bit"

// runOCR reads a film's bitmap subtitles into its scenes. newReader builds
// the Reader for a model name; nil uses Nemotron-Parse.
func runOCR(ctx context.Context, args []string, stdout, stderr io.Writer, newReader func(model string, timeout time.Duration) ocr.Reader) error {
	return execute(ctx, ocrCmd(newReader), args, stdout, stderr)
}

func ocrCmd(newReader func(model string, timeout time.Duration) ocr.Reader) *cobra.Command {
	fs := flag.NewFlagSet("ocr", flag.ContinueOnError)
	bundleDir := fs.String("bundle", "", "bundle directory (default: next to FILE, named FILE.film)")
	lang := fs.String("lang", "auto", "language code of the subtitle track to read, or auto for the best track (a foreign one over English, text over bitmap)")
	model := fs.String("model", nemotronModel, "OCR model")
	timeout := fs.Duration("timeout", 30*time.Minute, "longest the render and OCR steps may take, such as 45m or 2h")
	verbose := fs.Bool("verbose", false, "list the events that were set aside, and why")
	debug := fs.Bool("debug", false, "show the commands run and how long they took")
	burned := fs.Bool("burned", false, "read subtitles burned into the picture instead of a subtitle track")
	fps := fs.Int("fps", 2, "with --burned, frames sampled per second")
	masks := fs.String("masks", "", `with --burned, areas to black out, such as watermarks: "w,h,x,y;..." in pixels or iw/ih expressions`)
	rng := fs.String("range", "", "with --burned, the part of the film to read as FROM:TO, such as 1m30s:4m (either may be empty)")
	cmd := &cobra.Command{Use: "ocr [flags] FILE", Short: "Read a film's subtitles into its scenes: text tracks as they are, bitmap tracks by OCR", Args: oneFile, RunE: func(c *cobra.Command, args []string) error {
		ctx, stdout, stderr := c.Context(), c.OutOrStdout(), c.ErrOrStderr()
		file, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		if err := statWithin(file, 15*time.Second, os.Stat); err != nil {
			return err
		}
		stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		bundle := *bundleDir
		if bundle == "" {
			bundle = filepath.Join(filepath.Dir(file), stem+".film")
		}
		cfg, err := loadConfig(ctx, bundle, os.Environ())
		if err != nil {
			return err
		}
		set := map[string]bool{}
		c.Flags().Visit(func(f *pflag.Flag) { set[f.Name] = true })
		if !set["lang"] {
			*lang = cfg.Get("OCR_LANG")
		}
		if !set["model"] {
			*model = cfg.Get("OCR_MODEL")
		}
		if !set["timeout"] {
			*timeout = cfg.Duration("OCR_TIMEOUT")
		}
		if *lang == "" || *timeout <= 0 {
			return errors.New("ocr: language must be set and timeout must be positive")
		}
		dbPath := filepath.Join(bundle, "rose.db")
		if err := statWithin(dbPath, 15*time.Second, os.Stat); err != nil {
			return fmt.Errorf("no scenes for %s yet: run `rose scenes` first", file)
		}
		unlock, err := lock.Acquire(bundle)
		if err != nil {
			return err
		}
		defer unlock()
		db, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		st, err := store.New(ctx, db)
		if err != nil {
			return err
		}

		ex := ocr.Exec(scenes.NewExec(scenes.WithWaitDelay(5 * time.Second)))
		if *debug {
			ex = traced(ex, stderr)
		}
		var rd ocr.Reader
		if newReader != nil {
			rd = newReader(*model, *timeout)
		} else {
			dir, err := cacheDir(cfg)
			if err != nil {
				return err
			}
			c := cache.Dir{Path: dir}
			sweepCtx, stopSweep := context.WithTimeout(ctx, 30*time.Second)
			n, err := c.Sweep(sweepCtx, int64(cfg.Int("CACHE_MAX_MB"))<<20, cfg.Duration("CACHE_MAX_AGE"))
			stopSweep()
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "cache: not trimmed: %v\n", err)
			} else if *debug {
				_, _ = fmt.Fprintf(stderr, "debug: cache trimmed, %d removed\n", n)
			}
			nem := ocr.Nemotron{Exec: ex, Model: *model, Load: *timeout, PerPage: *timeout / 4}
			rd = ocr.Cached{Inner: nem, Dir: c, Salt: nem.CacheKey(), Batch: cfg.Int("OCR_BATCH")}
		}
		issues := func(is []ocr.Issue) {
			if !*verbose && !*debug {
				return
			}
			for _, i := range is {
				_, _ = fmt.Fprintf(stderr, "set aside: %s: %s\n", i.Ref, i.Reason)
			}
		}
		run := ocr.Stage{Exec: ex, Store: st, Reader: rd, ProbeTimeout: time.Minute, Timeout: *timeout, OnIssues: issues}.Run
		if *burned {
			ms, err := parseMasks(*masks)
			if err != nil {
				return err
			}
			from, to, err := parseRange(*rng)
			if err != nil {
				return err
			}
			run = ocr.BurnedStage{
				Exec: ex, Store: st, Reader: rd, MinInk: 400, Timeout: *timeout,
				Sampling: ocr.Sampling{FPS: *fps, Masks: ms, From: from, To: to}, OnIssues: issues,
			}.Run
		}
		eng := engine.New(st, engine.WithStage("ocr", run))
		defer func() { _ = eng.Close() }()

		film := core.FilmID(stem)
		spec := core.JobSpec{Stage: "ocr", Film: film, Params: map[string]string{"file": file, "lang": *lang}}
		if _, err := runJob(ctx, eng, spec, stderr); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "subtitles read into %s\n", bundle)
		return err
	}}
	cmd.Flags().AddGoFlagSet(fs)
	_ = cmd.MarkFlagDirname("bundle")
	return cmd
}

// statWithin stats path, giving up after d in case a stalled mount hangs it.
func statWithin(path string, d time.Duration, stat func(string) (os.FileInfo, error)) error {
	done := make(chan error, 1)
	go func() { _, err := stat(path); done <- err }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		return &limit.TimeoutError{What: "checking " + path, After: d}
	}
}

// traced logs each command and how long it took.
func traced(ex ocr.Exec, w io.Writer) ocr.Exec {
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		start := time.Now()
		out, err := ex(ctx, name, args...)
		_, _ = fmt.Fprintf(w, "debug: %s %s (%v, %d bytes, err %v)\n", name, strings.Join(args, " "), time.Since(start).Round(time.Millisecond), len(out), err)
		return out, err
	}
}
