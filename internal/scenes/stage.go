package scenes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
)

// Sink receives the finished scenes for a film.
type Sink interface {
	ReplaceScenes(ctx context.Context, film core.FilmID, scenes []store.SceneText) error
}

// Stage finds the cuts in a film and extracts one frame per scene.
// The video path comes from the job's "file" param.
type Stage struct {
	Exec      Exec
	Sink      Sink
	Films     func(ctx context.Context, id core.FilmID) (core.Film, error)
	Threshold float64       // ffmpeg scene score above which a frame is a cut
	MinLen    time.Duration // shortest scene to keep
}

// Run is a jobs.Stage. Frames are named by time, so those already on disk are reused. Scenes are
// written to the Sink only after every frame exists.
func (s Stage) Run(ctx context.Context, spec core.JobSpec, progress func(done, total int64)) error {
	file := spec.Params["file"]
	if file == "" {
		return errors.New("scenes: missing \"file\" param")
	}
	film, err := s.Films(ctx, spec.Film)
	if err != nil {
		return fmt.Errorf("scenes: film %q: %w", spec.Film, err)
	}

	spans, err := s.detect(ctx, file)
	if err != nil {
		return err
	}
	total := int64(len(spans))
	progress(0, total)

	if err := os.MkdirAll(filepath.Join(film.Bundle, "frames"), 0o755); err != nil {
		return err
	}
	rows := make([]store.SceneText, len(spans))
	for i, sp := range spans {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel := fmt.Sprintf("frames/%08d.jpg", sp.Mid().Milliseconds())
		if err := s.frame(ctx, file, filepath.Join(film.Bundle, filepath.FromSlash(rel)), sp.Mid()); err != nil {
			return err
		}
		rows[i] = store.SceneText{Scene: core.Scene{Start: sp.Start, End: sp.End, Frame: rel}}
		progress(int64(i+1), total)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.Sink.ReplaceScenes(ctx, spec.Film, rows); err != nil {
		return err
	}
	prune(filepath.Join(film.Bundle, "frames"), rows)
	return nil
}

// prune deletes frames no scene refers to, left over from earlier runs.
func prune(dir string, rows []store.SceneText) {
	keep := make(map[string]bool, len(rows))
	for _, r := range rows {
		keep[filepath.Base(r.Frame)] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !keep[e.Name()] {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func (s Stage) detect(ctx context.Context, file string) ([]Span, error) {
	out, err := s.Exec(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", file)
	if err != nil {
		return nil, err
	}
	total, err := ParseDuration(string(out))
	if err != nil {
		return nil, err
	}
	filter := fmt.Sprintf("select='gt(scene,%g)',metadata=print:file=-", s.Threshold)
	out, err = s.Exec(ctx, "ffmpeg", "-hide_banner", "-nostats", "-i", file, "-vf", filter, "-an", "-f", "null", "-")
	if err != nil {
		return nil, err
	}
	cuts, err := ParseCuts(bytes.NewReader(out))
	if err != nil {
		return nil, err
	}
	return Boundaries(cuts, total, s.MinLen), nil
}

// frame extracts one JPEG at t. It writes a .part file first so a killed
// ffmpeg never leaves a truncated frame that a later run would trust.
func (s Stage) frame(ctx context.Context, file, dst string, t time.Duration) error {
	if fi, err := os.Stat(dst); err == nil && fi.Size() > 0 {
		return nil
	}
	part := dst[:len(dst)-len(".jpg")] + ".part.jpg"
	_, err := s.Exec(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-ss", fmt.Sprintf("%.3f", t.Seconds()), "-i", file, "-frames:v", "1", "-q:v", "3", part)
	if err != nil {
		_ = os.Remove(part)
		return err
	}
	return os.Rename(part, dst)
}
