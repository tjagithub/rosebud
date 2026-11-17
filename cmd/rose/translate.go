package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/tjagithub/rosebud/internal/cache"
	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/engine"
	"github.com/tjagithub/rosebud/internal/lock"
	"github.com/tjagithub/rosebud/internal/scenes"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/translate"
)

// runTranslate writes an English translation beside each scene's text. newTr
// builds the translator for a model name; nil uses PLaMo-3.
func runTranslate(ctx context.Context, args []string, stdout, stderr io.Writer, newTr func(model, lang string, timeout time.Duration) translate.Translator) error {
	return execute(ctx, translateCmd(newTr), args, stdout, stderr)
}

func translateCmd(newTr func(model, lang string, timeout time.Duration) translate.Translator) *cobra.Command {
	fs := flag.NewFlagSet("translate", flag.ContinueOnError)
	bundleDir := fs.String("bundle", "", "bundle directory (default: next to FILE, named FILE.film)")
	lang := fs.String("lang", "auto", "language code of the scene text, or auto to detect it")
	backend := fs.String("backend", "rikai", "how to translate: rikai (ja to en) or plamo (more languages, runs the model's own code)")
	model := fs.String("model", "", "translation model (default: the backend's)")
	revision := fs.String("revision", "", "commit of --model to download (required with a custom model)")
	timeout := fs.Duration("timeout", 30*time.Minute, "longest the model may take to load and translate a batch, such as 45m")
	debug := fs.Bool("debug", false, "show the commands run and how long they took")
	cmd := &cobra.Command{Use: "translate [flags] FILE", Short: "Translate scene text to English", Args: oneFile, RunE: func(c *cobra.Command, args []string) error {
		ctx, stdout, stderr := c.Context(), c.OutOrStdout(), c.ErrOrStderr()
		file, err := filepath.Abs(args[0])
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
		c.Flags().Visit(func(f *pflag.Flag) { set[f.Name] = true })
		if !set["lang"] {
			*lang = cfg.Get("TRANSLATE_LANG")
		}
		if !set["backend"] {
			*backend = cfg.Get("TRANSLATE_BACKEND")
		}
		if !set["model"] {
			*model = cfg.Get("TRANSLATE_MODEL")
		}
		if !set["revision"] {
			*revision = cfg.Get("TRANSLATE_REVISION")
		}
		be, ok := translate.Backends[*backend]
		if !ok {
			return fmt.Errorf("translate: unknown backend %q", *backend)
		}
		if *model == "" {
			*model = be.Model
			if *revision == "" {
				*revision = be.Revision
			}
		}
		if *revision == "" {
			return errors.New("translate: a custom model needs --revision (TRANSLATE_REVISION), the commit to download")
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

		if *lang == "auto" {
			detected, err := detectLang(ctx, st, core.FilmID(stem))
			if err != nil {
				return err
			}
			if detected == "" {
				return &skippedError{stage: "translate", reason: "no Japanese, Chinese or Korean text found"}
			}
			*lang = detected
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
			p := translate.Local{Exec: ex, Backend: be, Model: *model, Revision: *revision, Source: *lang, Load: *timeout, PerLine: *timeout / 20}
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
	}}
	cmd.Flags().AddGoFlagSet(fs)
	_ = cmd.RegisterFlagCompletionFunc("backend", cobra.FixedCompletions(slices.Sorted(maps.Keys(translate.Backends)), cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.MarkFlagDirname("bundle")
	return cmd
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

// detectLang guesses the language of a film's scene text, or "" if it has none to translate.
func detectLang(ctx context.Context, st *store.Store, film core.FilmID) (string, error) {
	var lines []string
	for l, err := range st.Texts(ctx, film) {
		if err != nil {
			return "", err
		}
		lines = append(lines, l.Subtitle, l.Text)
	}
	return translate.DetectLang(lines), nil
}
