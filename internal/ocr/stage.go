package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"strconv"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/limit"
	"github.com/tjagithub/rosebud/internal/subs"
	"github.com/tjagithub/rosebud/internal/subsrc"
)

// Store is the part of the catalog the stage uses.
type Store interface {
	Scenes(ctx context.Context, film core.FilmID) iter.Seq2[core.Scene, error]
	SetSubtitle(ctx context.Context, film core.FilmID, text map[core.SceneID]string) error
}

// Stage writes the on-screen subtitle text of each scene, read from a bitmap
// track. The job's "file" and "lang" params pick the file and the track.
type Stage struct {
	Exec         Exec
	Store        Store
	Reader       Reader
	ProbeTimeout time.Duration // for the quick ffprobe calls
	Timeout      time.Duration // for rendering the subtitles
	OnIssues     func([]Issue) // told what was set aside, if not nil
}

// Run is a jobs.Stage. Text is written once, at the end, so a failed or
// cancelled run leaves the earlier text alone.
func (s Stage) Run(ctx context.Context, spec core.JobSpec, progress func(done, total int64)) error {
	file, lang := spec.Params["file"], spec.Params["lang"]
	if file == "" || lang == "" {
		return errors.New("ocr: needs \"file\" and \"lang\" params")
	}
	scenes, err := s.scenes(ctx, spec.Film)
	if err != nil {
		return err
	}
	tracks, err := subsrc.Probe(ctx, s.Exec, file, s.ProbeTimeout)
	if err != nil {
		return err
	}
	var tr subsrc.Track
	var ok bool
	if lang == "auto" {
		tr, ok = subsrc.PickAuto(tracks)
	} else {
		tr, ok = subsrc.Pick(tracks, lang)
	}
	if !ok {
		return core.Skip("%s has no readable %q subtitle track to OCR", file, lang)
	}
	if tr.Kind() != subsrc.Bitmap {
		return fmt.Errorf("ocr: %s track %d is %s, not a bitmap track", file, tr.Pos, tr.Codec)
	}
	events, issues, err := s.events(ctx, file, tr.Pos)
	if err != nil {
		return err
	}
	valid, more := Partition(events)
	issues = append(issues, more...)
	frames, err := Frames(ctx, s.Exec, file, tr.Pos, s.Timeout)
	if err != nil {
		return err
	}
	if len(frames) != len(valid) {
		return fmt.Errorf("ocr: %s has %d subtitle events but %d rendered frames", file, len(valid), len(frames))
	}
	progress(0, int64(PageCount(len(frames))))
	texts, more, err := ReadEvents(WithProgress(ctx, progress), s.Reader, frames)
	if err != nil {
		return err
	}
	issues = append(issues, more...)
	spans := make([]subs.Span, len(scenes))
	for i, sc := range scenes {
		spans[i] = subs.Span{Start: sc.Start, End: sc.End}
	}
	byScene, more := Assign(spans, valid, texts)
	issues = append(issues, more...)
	out := make(map[core.SceneID]string, len(byScene))
	for i, t := range byScene {
		out[scenes[i].ID] = t
	}
	if s.OnIssues != nil {
		s.OnIssues(issues)
	}
	return s.Store.SetSubtitle(ctx, spec.Film, out)
}

func (s Stage) events(ctx context.Context, file string, sub int) ([]Event, []Issue, error) {
	out, err := limit.Run(ctx, "ffprobe", s.ProbeTimeout, func(ctx context.Context) ([]byte, error) {
		return s.Exec(ctx, "ffprobe", "-v", "error", "-select_streams", "s:"+strconv.Itoa(sub),
			"-show_entries", "packet=pts_time,duration_time", "-of", "csv=p=0", "-i", file)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("ocr: list subtitle events of %s: %w", file, err)
	}
	return ParseEvents(bytes.NewReader(out))
}

func (s Stage) scenes(ctx context.Context, film core.FilmID) ([]core.Scene, error) {
	var out []core.Scene
	for sc, err := range s.Store.Scenes(ctx, film) {
		if err != nil {
			return nil, fmt.Errorf("ocr: scenes of %q: %w", film, err)
		}
		out = append(out, sc)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ocr: film %q has no scenes; run scenes first", film)
	}
	return out, nil
}
