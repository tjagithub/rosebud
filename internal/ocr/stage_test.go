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
	"os"
	"path/filepath"
	"slices"
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

func TestStageReadsATextTrackWithoutOCR(t *testing.T) {
	srt := "1\n00:00:01,000 --> 00:00:02,000\none\n\n2\n00:00:05,000 --> 00:00:06,000\n<i>two</i>\n\n3\n00:00:12,000 --> 00:00:13,000\nthree\n\n4\n00:00:30,000 --> 00:00:31,000\nafter the end\n"
	var ffmpegArgs string
	ex := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "ffmpeg" {
			ffmpegArgs = strings.Join(args, " ")
			return []byte(srt), nil
		}
		return []byte("2,dvd_subtitle,jpn\n3,subrip,jpn\n"), nil
	}
	st := &memStore{scenes: []core.Scene{{ID: 1, Start: 0, End: sec(10)}, {ID: 2, Start: sec(10), End: sec(20)}}}
	rd := &bandReader{}
	var issues []ocr.Issue
	s := newStage(ex, st, rd)
	s.OnIssues = func(i []ocr.Issue) { issues = append(issues, i...) }
	for _, lang := range []string{"jpn", "auto"} {
		if err := s.Run(t.Context(), spec(map[string]string{"file": "f.mkv", "lang": lang}), noProgress); err != nil {
			t.Fatalf("lang %s: %v", lang, err)
		}
		if st.got[1] != "one\ntwo" || st.got[2] != "three" {
			t.Errorf("lang %s: stored %v", lang, st.got)
		}
	}
	if len(rd.calls) != 0 {
		t.Errorf("a text track must not go to the OCR model, got %d reads", len(rd.calls))
	}
	if !strings.Contains(ffmpegArgs, "-map 0:s:1") {
		t.Errorf("want the subrip track (s:1) read, ffmpeg got %s", ffmpegArgs)
	}
	if len(issues) == 0 || !strings.Contains(issues[0].Reason, "overlaps no scene") {
		t.Errorf("the cue after the last scene should be reported, got %+v", issues)
	}
}

func TestStageReadsASubtitleFileBesideTheVideo(t *testing.T) {
	dir := t.TempDir()
	video, vtt := filepath.Join(dir, "f.webm"), filepath.Join(dir, "f.en.vtt")
	if err := os.WriteFile(vtt, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var input string
	ex := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "ffmpeg" {
			input = args[slices.Index(args, "-i")+1]
			return []byte("1\n00:00:01,000 --> 00:00:02,000\nfrom the file\n"), nil
		}
		if slices.Contains(args, "s") {
			return []byte("0,subrip,eng\n"), nil // the same language inside the video
		}
		return []byte("eng\n"), nil
	}
	st := &memStore{scenes: []core.Scene{{ID: 1, Start: 0, End: sec(10)}}}
	if err := newStage(ex, st, &bandReader{}).Run(t.Context(), spec(map[string]string{"file": video, "lang": "auto"}), noProgress); err != nil {
		t.Fatal(err)
	}
	if input != vtt || st.got[1] != "from the file" {
		t.Errorf("read %q into %v, want the vtt", input, st.got)
	}
}

func TestStageAutoFollowsTheSpokenLanguage(t *testing.T) {
	for spoken, want := range map[string]string{"jpn": "-map 0:s:1", "": "-map 0:s:0"} {
		var ffmpeg string
		ex := func(_ context.Context, name string, args ...string) ([]byte, error) {
			switch {
			case name == "ffmpeg":
				ffmpeg = strings.Join(args, " ")
				return []byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"), nil
			case slices.Contains(args, "s"):
				return []byte("2,subrip,eng\n3,subrip,jpn\n4,subrip,fra\n"), nil
			}
			return []byte(spoken + "\n"), nil // the audio tag
		}
		st := &memStore{scenes: []core.Scene{{ID: 1, Start: 0, End: sec(10)}}}
		if err := newStage(ex, st, &bandReader{}).Run(t.Context(), spec(map[string]string{"file": "f.mkv", "lang": "auto"}), noProgress); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(ffmpeg, want) {
			t.Errorf("audio %q: ffmpeg %s, want %s", spoken, ffmpeg, want)
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
		"auto": "has no subtitle track to read",
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
