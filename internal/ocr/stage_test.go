package ocr_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/ocr"
)

type memStore struct {
	scenes []core.Scene
	got    map[core.SceneID]string
	set    bool
}

func (m *memStore) Scenes(context.Context, core.FilmID) iter.Seq2[core.Scene, error] {
	return func(yield func(core.Scene, error) bool) {
		for _, s := range m.scenes {
			if !yield(s, nil) {
				return
			}
		}
	}
}

func (m *memStore) SetSubtitle(_ context.Context, _ core.FilmID, t map[core.SceneID]string) error {
	m.got, m.set = t, true
	return nil
}

// ffExec fakes the ffprobe track list, ffprobe packets and ffmpeg frames.
func ffExec(events string, nFrames int) ocr.Exec {
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	draw.Draw(img, image.Rect(10, 10, 20, 20), image.NewUniform(color.White), image.Point{}, draw.Src)
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch {
		case name == "ffmpeg":
			return bytes.Repeat(b.Bytes(), nFrames), nil
		case strings.Contains(strings.Join(args, " "), "packet="):
			return []byte(events), nil
		}
		return []byte("2,dvd_subtitle,eng\n3,dvd_subtitle,jpn\n"), nil
	}
}

func newStage(ex ocr.Exec, st *memStore, rd ocr.Reader) ocr.Stage {
	return ocr.Stage{Exec: ex, Store: st, Reader: rd, ProbeTimeout: time.Minute, Timeout: time.Minute}
}

func spec(params map[string]string) core.JobSpec {
	return core.JobSpec{Film: "f", Params: params}
}

func noProgress(int64, int64) {}

func TestStageStoresTextPerScene(t *testing.T) {
	st := &memStore{scenes: []core.Scene{{ID: 1, Start: 0, End: sec(10)}, {ID: 2, Start: sec(10), End: sec(20)}}}
	// three events to read, a 34 ms stub, and a junk line
	ev := "1.0,2.0\n5.0,1.0\n12.0,1.0\n15.0,0.034\nbad\n"
	rd := &bandReader{texts: [][]string{{"one", "two", "three"}}}
	var issues []ocr.Issue
	s := newStage(ffExec(ev, 3), st, rd)
	s.OnIssues = func(i []ocr.Issue) { issues = append(issues, i...) }
	if err := s.Run(t.Context(), spec(map[string]string{"file": "f.mkv", "lang": "jpn"}), noProgress); err != nil {
		t.Fatal(err)
	}
	if st.got[1] != "one\ntwo" || st.got[2] != "three" {
		t.Errorf("stored %v", st.got)
	}
	if len(issues) != 2 {
		t.Errorf("issues = %+v, want 2", issues)
	}
}

func TestStageFailsWithoutWriting(t *testing.T) {
	st := &memStore{scenes: []core.Scene{{ID: 1, Start: 0, End: sec(10)}}}
	ok := map[string]string{"file": "f.mkv", "lang": "jpn"}
	for name, c := range map[string]struct {
		params map[string]string
		ex     ocr.Exec
		rd     ocr.Reader
	}{
		"no file":        {map[string]string{"lang": "jpn"}, ffExec("", 0), &bandReader{}},
		"no lang":        {map[string]string{"file": "f.mkv"}, ffExec("", 0), &bandReader{}},
		"no such track":  {map[string]string{"file": "f.mkv", "lang": "fra"}, ffExec("", 0), &bandReader{}},
		"frame mismatch": {ok, ffExec("1.0,1.0\n3.0,1.0\n", 1), &bandReader{texts: [][]string{{"x"}}}},
		"reader fails":   {ok, ffExec("1.0,1.0\n", 1), &bandReader{err: errors.New("boom")}},
	} {
		st.set = false
		s := newStage(c.ex, st, c.rd)
		if err := s.Run(t.Context(), spec(c.params), noProgress); err == nil {
			t.Errorf("%s: no error", name)
		}
		if st.set {
			t.Errorf("%s: wrote despite the error", name)
		}
	}
}

func TestStageNoScenes(t *testing.T) {
	s := newStage(ffExec("", 0), &memStore{}, &bandReader{})
	if err := s.Run(t.Context(), spec(map[string]string{"file": "f", "lang": "jpn"}), noProgress); err == nil {
		t.Error("no error for a film with no scenes")
	}
}

func TestStageSkipMessagesNameWhatIsMissing(t *testing.T) {
	for lang, want := range map[string]string{
		"auto": "has no bitmap subtitle track to OCR",
		"fra":  `has no readable "fra" subtitle track`,
	} {
		ex := ffExec("", 0)
		if lang == "auto" {
			ex = func(context.Context, string, ...string) ([]byte, error) { return nil, nil } // no tracks
		}
		s := newStage(ex, &memStore{scenes: []core.Scene{{ID: 1, End: sec(10)}}}, &bandReader{})
		err := s.Run(t.Context(), spec(map[string]string{"file": "f.mkv", "lang": lang}), noProgress)
		if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), `"auto"`) {
			t.Errorf("lang %s: err = %v, want %q", lang, err, want)
		}
	}
}
