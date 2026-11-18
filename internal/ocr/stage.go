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

// Stage writes the subtitle text of each scene, read straight from a text track
// or by OCR from a bitmap one. The job's "file" param names the video. "track"
// picks a track exactly: a subtitle stream number or a subtitle file. Otherwise
// "lang" picks by language, or "auto" by "spoken", the language spoken in the film.
type Stage struct {
	Exec         Exec
	Store        Store
	Reader       Reader
	ProbeTimeout time.Duration      // for the quick ffprobe calls
	Timeout      time.Duration      // for rendering the subtitles
	OnIssues     func([]Issue)      // told what was set aside, if not nil
	OnTrack      func(subsrc.Track) // told which track is read, if not nil
}

// Run is a jobs.Stage. Text is written once, at the end, so a failed or
// cancelled run leaves the earlier text alone.
func (s Stage) Run(ctx context.Context, spec core.JobSpec, progress func(done, total int64)) error {
	file, lang, track := spec.Params["file"], spec.Params["lang"], spec.Params["track"]
	if file == "" || lang == "" && track == "" {
		return errors.New("ocr: needs a \"file\" param, and \"lang\" or \"track\"")
	}
	scenes, err := s.scenes(ctx, spec.Film)
	if err != nil {
		return err
	}
	tr, err := s.choose(ctx, file, track, lang, spec.Params["spoken"])
	if err != nil {
		return err
	}
	read := s.readBitmap
	if tr.Kind() == subsrc.Text {
		read = s.readText
	}
	if s.OnTrack != nil {
		s.OnTrack(tr)
	}
	src, pos := file, tr.Pos
	if tr.Path != "" {
		src, pos = tr.Path, 0
	}
	events, texts, issues, err := read(ctx, src, pos, progress)
	if err != nil {
		return err
	}
	spans := make([]subs.Span, len(scenes))
	for i, sc := range scenes {
		spans[i] = subs.Span{Start: sc.Start, End: sc.End}
	}
	byScene, more := Assign(spans, events, texts)
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

// choose is the exact track asked for, or else the best for lang.
func (s Stage) choose(ctx context.Context, file, track, lang, spoken string) (subsrc.Track, error) {
	if track == "" {
		return s.track(ctx, file, lang, spoken)
	}
	pos, err := strconv.Atoi(track)
	if err != nil {
		return subsrc.File(track)
	}
	tracks, err := subsrc.Probe(ctx, s.Exec, file, s.ProbeTimeout)
	if err != nil {
		return subsrc.Track{}, err
	}
	for _, t := range tracks {
		if t.Pos == pos && t.Kind() != subsrc.Unknown {
			return t, nil
		}
	}
	return subsrc.Track{}, fmt.Errorf("ocr: %s has no readable subtitle track %d", file, pos)
}

// track picks the subtitle track for lang from file's own and the subtitle files
// beside it. For "auto" it follows spoken, or else the audio track's tag.
func (s Stage) track(ctx context.Context, file, lang, spoken string) (subsrc.Track, error) {
	tracks, err := subsrc.Probe(ctx, s.Exec, file, s.ProbeTimeout)
	if err != nil {
		return subsrc.Track{}, err
	}
	files, err := subsrc.Sidecars(file)
	if err != nil {
		return subsrc.Track{}, err
	}
	tracks = append(tracks, files...)
	if lang == "auto" {
		if spoken == "" {
			spoken = subsrc.AudioLang(ctx, s.Exec, file, s.ProbeTimeout)
		}
		if tr, ok := subsrc.PickAuto(tracks, spoken); ok {
			return tr, nil
		}
		return subsrc.Track{}, core.Skip("%s has no subtitle track to read", file)
	}
	if tr, ok := subsrc.Pick(tracks, lang); ok {
		return tr, nil
	}
	return subsrc.Track{}, core.Skip("%s has no readable %q subtitle track", file, lang)
}

// readText takes the cues of a text track as they are: no model needed.
func (s Stage) readText(ctx context.Context, file string, pos int, _ func(done, total int64)) ([]Event, []string, []Issue, error) {
	cues, err := subsrc.ReadText(ctx, s.Exec, file, pos, s.Timeout)
	if err != nil {
		return nil, nil, nil, err
	}
	events, texts := make([]Event, len(cues)), make([]string, len(cues))
	for i, c := range cues {
		events[i], texts[i] = Event{Start: c.Start, End: c.End}, c.Text
	}
	return events, texts, nil, nil
}

// readBitmap draws each event of a bitmap track and reads it with the OCR model.
func (s Stage) readBitmap(ctx context.Context, file string, pos int, progress func(done, total int64)) ([]Event, []string, []Issue, error) {
	events, issues, err := s.events(ctx, file, pos)
	if err != nil {
		return nil, nil, nil, err
	}
	valid, more := Partition(events)
	issues = append(issues, more...)
	frames, err := Frames(ctx, s.Exec, file, pos, s.Timeout)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(frames) != len(valid) {
		return nil, nil, nil, fmt.Errorf("ocr: %s has %d subtitle events but %d rendered frames", file, len(valid), len(frames))
	}
	progress(0, int64(PageCount(len(frames))))
	texts, more, err := ReadEvents(WithProgress(ctx, progress), s.Reader, frames)
	if err != nil {
		return nil, nil, nil, err
	}
	return valid, texts, append(issues, more...), nil
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
