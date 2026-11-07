package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/engine"
	"github.com/tjagithub/rosebud/internal/lock"
	"github.com/tjagithub/rosebud/internal/scenes"
	"github.com/tjagithub/rosebud/internal/store"
)

// runScenes cuts a film into scenes and extracts a frame for each, into a
// bundle directory next to the file.
func runScenes(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("scenes", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bundleDir := fs.String("bundle", "", "bundle directory (default: next to FILE, named FILE.film)")
	threshold := fs.Float64("threshold", 0.3, "ffmpeg scene score that counts as a cut")
	minLen := fs.Duration("min-len", 2*time.Second, "shortest scene to keep")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: rose scenes [flags] FILE")
	}
	file, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := os.Stat(file); err != nil {
		return err
	}

	stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	bundle := *bundleDir
	if bundle == "" {
		bundle = filepath.Join(filepath.Dir(file), stem+".film")
	}
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		return err
	}
	unlock, err := lock.Acquire(bundle)
	if err != nil {
		return err
	}
	defer unlock()
	db, err := store.Open(filepath.Join(bundle, "rose.db"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	st, err := store.New(ctx, db)
	if err != nil {
		return err
	}
	film := core.Film{ID: core.FilmID(stem), Title: stem, Bundle: bundle}
	if err := st.PutFilm(ctx, film); err != nil {
		return err
	}

	cfg, err := loadConfig(ctx, bundle, os.Environ())
	if err != nil {
		return err
	}
	exec := scenes.NewExec()
	hw := hwaccelFor(cfg.Get("FFMPEG_HWACCEL"), runtime.GOOS)
	if hw != "" && hw != "auto" {
		avail, err := scenes.HWAccels(ctx, exec, 15*time.Second)
		if err != nil {
			return err
		}
		if err := scenes.CheckHWAccel(hw, avail); err != nil {
			return fmt.Errorf("FFMPEG_HWACCEL: %w", err)
		}
	}
	stage := scenes.Stage{
		Exec: exec, Sink: st, Threshold: *threshold, MinLen: *minLen,
		HWAccel: hw, HWDevice: cfg.Get("FFMPEG_HWACCEL_DEVICE"),
		Films: func(context.Context, core.FilmID) (core.Film, error) { return film, nil },
	}
	eng := engine.New(st, engine.WithStage("scenes", stage.Run))
	defer func() { _ = eng.Close() }()

	info, err := runJob(ctx, eng, core.JobSpec{Stage: "scenes", Film: film.ID, Params: map[string]string{"file": file}}, stderr)
	if err != nil {
		return err
	}
	noun := "scenes"
	if info.Total == 1 {
		noun = "scene"
	}
	_, err = fmt.Fprintf(stdout, "%d %s in %s\n", info.Total, noun, bundle)
	return err
}

// runJob starts spec, prints progress, and returns the job once it ends.
// If ctx is cancelled the job is cancelled and runJob waits for it to stop.
func runJob(ctx context.Context, c core.Client, spec core.JobSpec, progress io.Writer) (core.JobInfo, error) {
	evCtx, stopEvents := context.WithCancel(context.WithoutCancel(ctx))
	defer stopEvents()
	events, err := c.Subscribe(evCtx)
	if err != nil {
		return core.JobInfo{}, err
	}
	id, err := c.StartJob(ctx, spec)
	if err != nil {
		return core.JobInfo{}, err
	}

	cancelled := ctx.Done()
	for {
		select {
		case <-cancelled:
			cancelled = nil
			_ = c.CancelJob(context.WithoutCancel(ctx), id)
		case ev, ok := <-events:
			if !ok {
				return finalState(ctx, c, id)
			}
			switch e := ev.(type) {
			case core.JobProgress:
				if e.Job == id {
					_, _ = fmt.Fprintf(progress, "%s: %d/%d\n", spec.Stage, e.Done, e.Total)
				}
			case core.JobChanged:
				if e.Info.ID == id && ended(e.Info.State) {
					return result(e.Info)
				}
			}
		}
	}
}

// finalState looks the job up directly when the event stream ended early.
func finalState(ctx context.Context, c core.Client, id core.JobID) (core.JobInfo, error) {
	list, err := c.Jobs(context.WithoutCancel(ctx))
	if err != nil {
		return core.JobInfo{}, err
	}
	for _, j := range list {
		if j.ID == id && ended(j.State) {
			return result(j)
		}
	}
	return core.JobInfo{}, errors.New("event stream closed before the job finished")
}

func ended(s core.JobState) bool {
	return s == core.JobDone || s == core.JobFailed || s == core.JobCancelled
}

func result(j core.JobInfo) (core.JobInfo, error) {
	switch j.State {
	case core.JobFailed:
		return j, fmt.Errorf("%s failed: %s", j.Spec.Stage, j.Err)
	case core.JobCancelled:
		return j, fmt.Errorf("%s cancelled", j.Spec.Stage)
	}
	return j, nil
}
