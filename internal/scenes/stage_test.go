package scenes_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/scenes"
	"github.com/tjagithub/rosebud/internal/store"
)

// fakeExec answers ffprobe and ffmpeg, writing the frame file for extractions.
type fakeExec struct {
	duration string
	cuts     string
	fail     map[string]error
	onFrame  func(n int)
	calls    [][]string
	frames   int
}

func (f *fakeExec) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	if err := f.fail[name+f.kind(args)]; err != nil {
		if f.kind(args) != "" {
			_ = os.WriteFile(args[len(args)-1], []byte("trunc"), 0o644)
		}
		return nil, err
	}
	switch {
	case name == "ffprobe":
		return []byte(f.duration), nil
	case slices.Contains(args, "-frames:v"):
		f.frames++
		if f.onFrame != nil {
			f.onFrame(f.frames)
		}
		return nil, os.WriteFile(args[len(args)-1], []byte("jpg"), 0o644)
	}
	return []byte(f.cuts), nil
}

func (f *fakeExec) kind(args []string) string {
	if slices.Contains(args, "-frames:v") {
		return ":frame"
	}
	return ""
}

func (f *fakeExec) frameCalls() (out [][]string) {
	for _, c := range f.calls {
		if slices.Contains(c, "-frames:v") {
			out = append(out, c)
		}
	}
	return out
}

type fakeSink struct {
	film   core.FilmID
	scenes []store.SceneText
	err    error
}

func (s *fakeSink) ReplaceScenes(_ context.Context, film core.FilmID, sc []store.SceneText) error {
	s.film, s.scenes = film, sc
	return s.err
}

type progressLog [][2]int64

func (p *progressLog) fn(done, total int64) { *p = append(*p, [2]int64{done, total}) }

func newStage(t *testing.T, fe *fakeExec, sink *fakeSink) (scenes.Stage, core.JobSpec, string) {
	t.Helper()
	bundle := t.TempDir()
	stage := scenes.Stage{
		Exec: fe.run, Sink: sink, MinLen: 2 * time.Second, Threshold: 0.3,
		Films: func(context.Context, core.FilmID) (core.Film, error) {
			return core.Film{ID: "f", Bundle: bundle}, nil
		},
	}
	return stage, core.JobSpec{Stage: "scenes", Film: "f", Params: map[string]string{"file": "/films/f.mkv"}}, bundle
}

const cutsOut = "frame:0 pts:1 pts_time:10\nlavfi.scene_score=0.5\nframe:1 pts:2 pts_time:40\n"

func TestStage_Run(t *testing.T) {
	fe, sink := &fakeExec{duration: "100\n", cuts: cutsOut}, &fakeSink{}
	stage, spec, bundle := newStage(t, fe, sink)
	var prog progressLog

	if err := stage.Run(context.Background(), spec, prog.fn); err != nil {
		t.Fatal(err)
	}

	if sink.film != "f" || len(sink.scenes) != 3 {
		t.Fatalf("sink got film %q, %d scenes; want f, 3", sink.film, len(sink.scenes))
	}
	second := sink.scenes[1]
	if second.Start != 10*time.Second || second.End != 40*time.Second || second.Frame != "frames/00025000.jpg" {
		t.Errorf("second scene = %+v", second.Scene)
	}
	if _, err := os.Stat(filepath.Join(bundle, second.Frame)); err != nil {
		t.Errorf("frame file missing: %v", err)
	}

	var seeks []string
	for _, c := range fe.frameCalls() {
		seeks = append(seeks, c[slices.Index(c, "-ss")+1])
	}
	if want := []string{"5.000", "25.000", "70.000"}; !slices.Equal(seeks, want) {
		t.Errorf("seeks = %v, want %v", seeks, want)
	}
	if last := prog[len(prog)-1]; last != [2]int64{3, 3} {
		t.Errorf("last progress = %v, want {3 3}", last)
	}
	for i := 1; i < len(prog); i++ {
		if prog[i][0] < prog[i-1][0] {
			t.Errorf("progress went backwards: %v", prog)
		}
	}
}

func TestStage_ResumesExistingFrames(t *testing.T) {
	fe, sink := &fakeExec{duration: "100", cuts: cutsOut}, &fakeSink{}
	stage, spec, bundle := newStage(t, fe, sink)
	if err := os.MkdirAll(filepath.Join(bundle, "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "frames", "00025000.jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := stage.Run(context.Background(), spec, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if n := len(fe.frameCalls()); n != 2 {
		t.Errorf("extracted %d frames, want 2 (one already on disk)", n)
	}
	if len(sink.scenes) != 3 {
		t.Errorf("sink got %d scenes, want all 3", len(sink.scenes))
	}
}

func TestStage_Errors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		tweak     func(*fakeExec, *fakeSink, *scenes.Stage, *core.JobSpec)
		want      string
		wantCalls bool
	}{
		{"missing file param", func(_ *fakeExec, _ *fakeSink, _ *scenes.Stage, s *core.JobSpec) {
			delete(s.Params, "file")
		}, "file", false},
		{"unknown film", func(_ *fakeExec, _ *fakeSink, st *scenes.Stage, _ *core.JobSpec) {
			st.Films = func(context.Context, core.FilmID) (core.Film, error) { return core.Film{}, boom }
		}, "boom", false},
		{"probe fails", func(fe *fakeExec, _ *fakeSink, _ *scenes.Stage, _ *core.JobSpec) {
			fe.fail = map[string]error{"ffprobe": boom}
		}, "boom", true},
		{"bad duration", func(fe *fakeExec, _ *fakeSink, _ *scenes.Stage, _ *core.JobSpec) {
			fe.duration = "N/A"
		}, "duration", true},
		{"frame extraction fails", func(fe *fakeExec, _ *fakeSink, _ *scenes.Stage, _ *core.JobSpec) {
			fe.fail = map[string]error{"ffmpeg:frame": boom}
		}, "boom", true},
		{"sink fails", func(_ *fakeExec, sk *fakeSink, _ *scenes.Stage, _ *core.JobSpec) {
			sk.err = boom
		}, "boom", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fe, sink := &fakeExec{duration: "100", cuts: cutsOut}, &fakeSink{}
			stage, spec, bundle := newStage(t, fe, sink)
			tt.tweak(fe, sink, &stage, &spec)

			err := stage.Run(context.Background(), spec, func(int64, int64) {})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
			if got := len(fe.calls) > 0; got != tt.wantCalls {
				t.Errorf("ran programs = %v, want %v", got, tt.wantCalls)
			}
			left, _ := filepath.Glob(filepath.Join(bundle, "frames", "*.jpg"))
			for _, f := range left {
				if b, _ := os.ReadFile(f); string(b) == "trunc" {
					t.Errorf("truncated frame left behind: %s", f)
				}
			}
		})
	}
}

func TestStage_CancelWritesNothing(t *testing.T) {
	for _, at := range []int{1, 3} {
		t.Run(fmt.Sprintf("cancelled during frame %d of 3", at), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			fe, sink := &fakeExec{duration: "100", cuts: cutsOut}, &fakeSink{}
			fe.onFrame = func(n int) {
				if n == at {
					cancel()
				}
			}
			stage, spec, _ := newStage(t, fe, sink)

			err := stage.Run(ctx, spec, func(int64, int64) {})
			if !errors.Is(err, context.Canceled) {
				t.Errorf("err = %v, want context.Canceled", err)
			}
			if sink.scenes != nil {
				t.Errorf("sink was written after cancel: %+v", sink.scenes)
			}
			if n := len(fe.frameCalls()); n != at {
				t.Errorf("extracted %d frames, want %d", n, at)
			}
		})
	}
}

func TestStage_RerunWithDifferentCuts(t *testing.T) {
	fe, sink := &fakeExec{duration: "100", cuts: cutsOut}, &fakeSink{}
	stage, spec, bundle := newStage(t, fe, sink)
	run := func() {
		t.Helper()
		if err := stage.Run(context.Background(), spec, func(int64, int64) {}); err != nil {
			t.Fatal(err)
		}
	}
	run()

	fe.cuts, fe.calls = "frame:0 pts:1 pts_time:50\n", nil
	run()

	if n := len(fe.frameCalls()); n != 1 {
		t.Errorf("second run extracted %d frames, want 1 (25s frame is reusable)", n)
	}
	var got []string
	for _, sc := range sink.scenes {
		got = append(got, sc.Frame)
	}
	if want := []string{"frames/00025000.jpg", "frames/00075000.jpg"}; !slices.Equal(got, want) {
		t.Errorf("scene frames = %v, want %v", got, want)
	}
	files, _ := filepath.Glob(filepath.Join(bundle, "frames", "*"))
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	if want := []string{"00025000.jpg", "00075000.jpg"}; !slices.Equal(names, want) {
		t.Errorf("frames on disk = %v, want only the referenced %v", names, want)
	}
}
