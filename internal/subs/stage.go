package subs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
)

// Store is the part of the catalog the stage uses.
type Store interface {
	Scenes(ctx context.Context, film core.FilmID) iter.Seq2[core.Scene, error]
	SetText(ctx context.Context, film core.FilmID, text map[core.SceneID]string) error
}

// Stage writes the spoken text of each of a film's scenes. The audio file
// comes from the job's "file" param, and the film must already have scenes.
type Stage struct {
	Exec    Exec
	Store   Store
	Tr      Transcriber
	Max     time.Duration // longest chunk sent to the Transcriber
	Retries int           // extra tries for an empty loud chunk
	Shift   time.Duration // how far each retry moves the chunk start
	TempDir string        // where chunk WAVs go; the system default if empty
}

// Run is a jobs.Stage. Text is written once, after every chunk is done, so a
// failed or cancelled run leaves the earlier text alone.
func (s Stage) Run(ctx context.Context, spec core.JobSpec, progress func(done, total int64)) error {
	file := spec.Params["file"]
	if file == "" {
		return errors.New("subs: missing \"file\" param")
	}
	scenes, err := s.scenes(ctx, spec.Film)
	if err != nil {
		return err
	}
	total, err := Duration(ctx, s.Exec, file)
	if err != nil {
		return err
	}
	quiet, err := DetectSilences(ctx, s.Exec, file, total)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp(s.TempDir, "rose-subs-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	chunks := Chunks(total, quiet, s.Max)
	progress(0, int64(len(chunks)))
	toks, err := Transcribe(ctx, Job{
		Chunks: chunks, Quiet: quiet, Cut: FFmpegCutter(s.Exec, file, dir), Tr: s.Tr,
		Retries: s.Retries, Shift: s.Shift,
		Progress: func(done, n int) { progress(int64(done), int64(n)) },
	})
	if err != nil {
		return err
	}

	spans := make([]Span, len(scenes))
	for i, sc := range scenes {
		spans[i] = Span{sc.Start, sc.End}
	}
	text := make(map[core.SceneID]string, len(scenes))
	for i, t := range Assign(spans, toks) {
		text[scenes[i].ID] = t
	}
	return s.Store.SetText(ctx, spec.Film, text)
}

func (s Stage) scenes(ctx context.Context, film core.FilmID) ([]core.Scene, error) {
	var out []core.Scene
	for sc, err := range s.Store.Scenes(ctx, film) {
		if err != nil {
			return nil, fmt.Errorf("subs: scenes of %q: %w", film, err)
		}
		out = append(out, sc)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("subs: film %q has no scenes; run scenes first", film)
	}
	return out, nil
}
