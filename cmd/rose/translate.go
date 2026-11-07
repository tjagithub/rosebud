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

	"github.com/tjagithub/rosebud/internal/cache"
	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/engine"
	"github.com/tjagithub/rosebud/internal/lock"
	"github.com/tjagithub/rosebud/internal/scenes"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/translate"
)

const plamoModel = "pfnet/plamo-3-translate-0.8b-MLX-4bit"

// runTranslate writes an English translation beside each scene's text. newTr
// builds the translator for a model name; nil uses PLaMo-3.
func runTranslate(ctx context.Context, args []string, stdout, stderr io.Writer, newTr func(model, lang string, timeout time.Duration) translate.Translator) error {
	fs := flag.NewFlagSet("translate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundleDir := fs.String("bundle", "", "bundle directory (default: next to FILE, named FILE.film)")
	lang := fs.String("lang", "ja", "language code of the scene text")
	model := fs.String("model", plamoModel, "translation model")
	timeout := fs.Duration("timeout", 30*time.Minute, "longest the model may take to load and translate a batch, such as 45m")
	debug := fs.Bool("debug", false, "show the commands run and how long they took")
	fs.Usage = func() { dashed(fs, stderr) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: rose translate [flags] FILE")
	}
	file, err := filepath.Abs(fs.Arg(0))
	if err != nil {
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
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if !set["lang"] {
		*lang = cfg.Get("TRANSLATE_LANG")
	}
	if !set["model"] {
		*model = cfg.Get("TRANSLATE_MODEL")
	}
	if !set["timeout"] {
		*timeout = cfg.Duration("TRANSLATE_TIMEOUT")
	}
	if *lang == "" || *timeout <= 0 {
		return errors.New("translate: language must be set and timeout must be positive")
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

	var tr translate.Translator
	if newTr != nil {
		tr = newTr(*model, *lang, *timeout)
	} else {
		dir, err := cacheDir(cfg)
		if err != nil {
			return err
		}
		c := cache.Dir{Path: dir}
		sweepCtx, stopSweep := context.WithTimeout(ctx, 30*time.Second)
		_, err = c.Sweep(sweepCtx, int64(cfg.Int("CACHE_MAX_MB"))<<20, cfg.Duration("CACHE_MAX_AGE"))
		stopSweep()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "cache: not trimmed: %v\n", err)
		}
		ex := translate.Exec(scenes.NewExec(scenes.WithWaitDelay(5 * time.Second)))
		if *debug {
			ex = tracedExec(ex, stderr)
		}
		p := translate.Plamo{Exec: ex, Model: *model, Revision: translate.PlamoRevision, Source: *lang, Load: *timeout, PerLine: *timeout / 20}
		tr = translate.Cached{Inner: p, Dir: c, Salt: p.CacheKey(), Batch: cfg.Int("TRANSLATE_BATCH")}
	}
	stage := translate.Stage{Store: st, Translator: tr}
	eng := engine.New(st, engine.WithStage("translate", stage.Run))
	defer func() { _ = eng.Close() }()

	spec := core.JobSpec{Stage: "translate", Film: core.FilmID(stem)}
	if _, err := runJob(ctx, eng, spec, stderr); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "translations written into %s\n", bundle)
	return err
}

// tracedExec logs each command and how long it took.
func tracedExec(ex translate.Exec, w io.Writer) translate.Exec {
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		start := time.Now()
		out, err := ex(ctx, name, args...)
		_, _ = fmt.Fprintf(w, "debug: %s %s (%v, %d bytes, err %v)\n", name, strings.Join(args, " "), time.Since(start).Round(time.Millisecond), len(out), err)
		return out, err
	}
}
