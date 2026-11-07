package subs_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/subs"
)

type fakeStore struct {
	scenes  []core.Scene
	text    map[core.SceneID]string
	setErr  error
	setCall int
}

func (f *fakeStore) Scenes(_ context.Context, _ core.FilmID) iter.Seq2[core.Scene, error] {
	return func(yield func(core.Scene, error) bool) {
		for _, s := range f.scenes {
			if !yield(s, nil) {
				return
			}
		}
	}
}

func (f *fakeStore) SetText(_ context.Context, _ core.FilmID, text map[core.SceneID]string) error {
	f.setCall++
	f.text = text
	return f.setErr
}

// msWords hears one word "wN" per second of a wav named <startms>-<endms>.wav.
type msWords struct {
	err       error
	emptyFrom int // ms; a wav starting here comes back empty
}

func (m msWords) Transcribe(_ context.Context, wav string) ([]subs.Token, error) {
	if m.err != nil {
		return nil, m.err
	}
	var a, b int
	name := wav[strings.LastIndex(wav, "/")+1:]
	if _, err := fmt.Sscanf(name, "%d-%d.wav", &a, &b); err != nil {
		return nil, err
	}
	if a == m.emptyFrom && a != 0 {
		return nil, nil
	}
	var out []subs.Token
	for off := 0; off < b-a; off += 1000 {
		out = append(out, subs.Token{
			Text:  fmt.Sprintf("w%d", (a+off)/1000),
			Start: time.Duration(off) * time.Millisecond,
			End:   time.Duration(off+1000) * time.Millisecond,
		})
	}
	return out, nil
}

// stageExec answers ffprobe, silencedetect and cuts (which create the wav).
func stageExec(duration, silence string) subs.Exec {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch {
		case name == "ffprobe":
			return []byte(duration), nil
		case slices.Contains(args, "-af"):
			return []byte(silence), nil
		}
		return nil, os.WriteFile(args[len(args)-1], []byte("wav"), 0o644)
	}
}

func scenesOf(bounds ...int) []core.Scene {
	var out []core.Scene
	for i := 0; i+1 < len(bounds); i++ {
		out = append(out, core.Scene{ID: core.SceneID(i + 1), Start: sec(bounds[i]), End: sec(bounds[i+1])})
	}
	return out
}

func newStage(st *fakeStore, ex subs.Exec, tr subs.Transcriber, tmp string) subs.Stage {
	return subs.Stage{Exec: ex, Store: st, Tr: tr, Max: sec(5), Retries: 1, Shift: sec(1), TempDir: tmp}
}

func spec(file string) core.JobSpec {
	return core.JobSpec{Stage: "subs", Film: "f", Params: map[string]string{"file": file}}
}

func TestStage_Run(t *testing.T) {
	st := &fakeStore{scenes: scenesOf(0, 4, 10)}
	tmp := t.TempDir()
	var progress [][2]int64
	err := newStage(st, stageExec("10\n", ""), msWords{}, tmp).Run(t.Context(), spec("f.mkv"),
		func(done, total int64) { progress = append(progress, [2]int64{done, total}) })
	if err != nil {
		t.Fatal(err)
	}
	want := map[core.SceneID]string{1: "w0w1w2w3", 2: "w4w5w6w7w8w9"}
	if fmt.Sprint(st.text) != fmt.Sprint(want) {
		t.Errorf("text = %v, want %v", st.text, want)
	}
	if last := progress[len(progress)-1]; last != [2]int64{2, 2} {
		t.Errorf("last progress = %v, want 2/2 (progress %v)", last, progress)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("%d temp entries left behind", len(left))
	}
}

func TestStage_SilentSceneIsCleared(t *testing.T) {
	st := &fakeStore{scenes: scenesOf(0, 5, 10)}
	silence := "lavfi.silence_start=5\nlavfi.silence_end=10\n"
	if err := newStage(st, stageExec("10\n", silence), msWords{}, t.TempDir()).Run(t.Context(), spec("f.mkv"), func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if st.text[2] != "" || st.text[1] != "w0w1w2w3w4" {
		t.Errorf("text = %v; want scene 2 empty and written", st.text)
	}
}

func TestStage_Failures(t *testing.T) {
	boom := errors.New("boom")
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name   string
		spec   core.JobSpec
		scenes []core.Scene
		tr     subs.Transcriber
		ctx    context.Context
		want   string
	}{
		{"missing file param", core.JobSpec{Film: "f"}, scenesOf(0, 10), msWords{}, t.Context(), "file"},
		{"no scenes", spec("f.mkv"), nil, msWords{}, t.Context(), "scenes"},
		{"transcriber error", spec("f.mkv"), scenesOf(0, 10), msWords{err: boom}, t.Context(), "boom"},
		{"cancelled", spec("f.mkv"), scenesOf(0, 10), msWords{}, cancelled, "canceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStore{scenes: tt.scenes}
			err := newStage(st, stageExec("10\n", ""), tt.tr, t.TempDir()).Run(tt.ctx, tt.spec, func(int64, int64) {})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want one mentioning %q", err, tt.want)
			}
			if st.setCall != 0 {
				t.Error("text was written despite the failure")
			}
		})
	}
}

func TestStage_StoreErrorIsReturned(t *testing.T) {
	boom := errors.New("boom")
	st := &fakeStore{scenes: scenesOf(0, 10), setErr: boom}
	err := newStage(st, stageExec("10\n", ""), msWords{}, t.TempDir()).Run(t.Context(), spec("f.mkv"), func(int64, int64) {})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestStage_RetriesAnEmptyChunk(t *testing.T) {
	st := &fakeStore{scenes: scenesOf(0, 5, 10)}
	var progress [][2]int64
	stage := newStage(st, stageExec("10\n", ""), msWords{emptyFrom: 5000}, t.TempDir())
	err := stage.Run(t.Context(), spec("f.mkv"), func(d, n int64) { progress = append(progress, [2]int64{d, n}) })
	if err != nil {
		t.Fatal(err)
	}
	if st.text[2] != "w5w6w7w8w9" {
		t.Errorf("scene 2 = %q, want it recovered by the retry", st.text[2])
	}
	if progress[0] != [2]int64{0, 2} {
		t.Errorf("first progress = %v, want 0/2", progress[0])
	}
}
