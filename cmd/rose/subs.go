package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/engine"
	"github.com/tjagithub/rosebud/internal/lock"
	"github.com/tjagithub/rosebud/internal/scenes"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/subs"
)

// runSubs transcribes a film's speech into the text of the scenes that
// `rose scenes` found. newTr builds the Transcriber for a model name.
func runSubs(ctx context.Context, args []string, stdout, stderr io.Writer, newTr func(model string) subs.Transcriber) error {
	return execute(ctx, subsCmd(newTr), args, stdout, stderr)
}

func subsCmd(newTr func(model string) subs.Transcriber) *cobra.Command {
	fs := flag.NewFlagSet("subs", flag.ContinueOnError)
	bundleDir := fs.String("bundle", "", "bundle directory (default: next to FILE, named FILE.film)")
	lang := fs.String("lang", "auto", "spoken language code, such as en or ja; auto listens to the film")
	model := fs.String("model", "", "speech model, overriding the one chosen for -lang")
	maxChunk := fs.Duration("max-chunk", time.Minute, "longest stretch of audio sent to the model at once")
	retries := fs.Int("retries", 2, "extra tries for a stretch that comes back empty")
	shift := fs.Duration("shift", time.Second, "how far each retry moves the start of the stretch")
	cmd := &cobra.Command{Use: "subs [flags] FILE", Short: "Transcribe a film's speech into its scenes", Args: oneFile, RunE: func(c *cobra.Command, args []string) error {
		ctx, stdout, stderr := c.Context(), c.OutOrStdout(), c.ErrOrStderr()
		file, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		if _, err := os.Stat(file); err != nil {
			return err
		}
		if *lang != "auto" && *model == "" {
			if _, err := subs.ModelFor(*lang); err != nil {
				return err
			}
		}

		stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		bundle := *bundleDir
		if bundle == "" {
			bundle = filepath.Join(filepath.Dir(file), stem+".film")
		}
		dbPath := filepath.Join(bundle, "rose.db")
		if _, err := os.Stat(dbPath); err != nil {
			return fmt.Errorf("no scenes for %s yet: run `rose scenes` first", file)
		}
		spoken := *lang
		if *model == "" {
			if spoken == "auto" {
				if spoken, err = autoLanguage(ctx, file, stderr); err != nil {
					return err
				}
			}
			if *model, err = subs.ModelFor(spoken); err != nil {
				return err
			}
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

		if spoken != "auto" {
			if err := st.SetSpoken(ctx, core.FilmID(stem), spoken); err != nil {
				return err
			}
		}
		exec := scenes.NewExec()
		stage := subs.Stage{
			Exec: subs.Exec(exec), Store: st, Tr: newTr(*model),
			Max: *maxChunk, Retries: *retries, Shift: *shift,
		}
		eng := engine.New(st, engine.WithStage("subs", stage.Run))
		defer func() { _ = eng.Close() }()

		film := core.FilmID(stem)
		if _, err := runJob(ctx, eng, core.JobSpec{Stage: "subs", Film: film, Params: map[string]string{"file": file}}, stderr); err != nil {
			return err
		}
		n := 0
		for _, err := range st.Scenes(ctx, film) {
			if err != nil {
				return err
			}
			n++
		}
		_, err = fmt.Fprintf(stdout, "text for %d scenes in %s\n", n, bundle)
		return err
	}}
	cmd.Flags().AddGoFlagSet(fs)
	_ = cmd.MarkFlagDirname("bundle")
	return cmd
}

// parakeet is the transcriber `rose subs` uses unless a test supplies another.
func parakeet(model string) subs.Transcriber {
	return subs.Parakeet{Exec: subs.Exec(scenes.NewExec(scenes.WithWaitDelay(5 * time.Second))), Model: model}
}
