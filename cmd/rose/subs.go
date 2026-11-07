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
	fs := flag.NewFlagSet("subs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundleDir := fs.String("bundle", "", "bundle directory (default: next to FILE, named FILE.film)")
	lang := fs.String("lang", "en", "spoken language code, such as en or ja")
	model := fs.String("model", "", "speech model, overriding the one chosen for -lang")
	maxChunk := fs.Duration("max-chunk", time.Minute, "longest stretch of audio sent to the model at once")
	retries := fs.Int("retries", 2, "extra tries for a stretch that comes back empty")
	shift := fs.Duration("shift", time.Second, "how far each retry moves the start of the stretch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: rose subs [flags] FILE")
	}
	file, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := os.Stat(file); err != nil {
		return err
	}
	if *model == "" {
		if *model, err = subs.ModelFor(*lang); err != nil {
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
}

// parakeet is the transcriber `rose subs` uses unless a test supplies another.
func parakeet(model string) subs.Transcriber {
	return subs.Parakeet{Exec: subs.Exec(scenes.NewExec(scenes.WithWaitDelay(5 * time.Second))), Model: model}
}
