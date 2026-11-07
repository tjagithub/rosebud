package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/subs"
)

// makeTalkie is makeFilm with a 6s tone, so there is audio to transcribe.
func makeTalkie(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	file := filepath.Join(t.TempDir(), "Talkie.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:r=10:d=3",
		"-f", "lavfi", "-i", "color=c=blue:s=64x64:r=10:d=3",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=6",
		"-filter_complex", "[0][1]concat=n=2:v=1:a=0[v]", "-map", "[v]", "-map", "2:a",
		"-pix_fmt", "yuv420p", file)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	return file
}

// words says "hello" at 1s and "world" at 4s of whatever it is given.
type words struct{ model string }

func (w *words) Transcribe(context.Context, string) ([]subs.Token, error) {
	return []subs.Token{
		{Text: "hello", Start: time.Second, End: 2 * time.Second},
		{Text: "world", Start: 4 * time.Second, End: 5 * time.Second},
	}, nil
}

func fakeTr(got *string) func(string) subs.Transcriber {
	return func(model string) subs.Transcriber {
		*got = model
		return &words{model: model}
	}
}

func TestRun_Subs(t *testing.T) {
	file := makeTalkie(t)
	var out, errOut bytes.Buffer
	if err := run(t.Context(), []string{"scenes", file}, &out, &errOut); err != nil {
		t.Fatalf("scenes: %v", err)
	}
	var model string
	out.Reset()
	if err := runSubs(t.Context(), []string{"--lang", "ja", file}, &out, &errOut, fakeTr(&model)); err != nil {
		t.Fatalf("subs: %v\nstderr: %s", err, errOut.String())
	}
	if model != "mlx-community/parakeet-tdt_ctc-0.6b-ja" {
		t.Errorf("model = %q, want the Japanese one", model)
	}

	db, err := store.Open(filepath.Join(strings.TrimSuffix(file, ".mp4")+".film", "rose.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st, err := store.New(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := st.Search(t.Context(), core.SearchQuery{Text: "hello", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].Scene.Start != 0 {
		t.Fatalf("search hello = %v, %v; want one hit in the first scene", hits, err)
	}
	hits, err = st.Search(t.Context(), core.SearchQuery{Text: "world", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].Scene.Start != 3*time.Second {
		t.Fatalf("search world = %v, %v; want one hit in the second scene", hits, err)
	}
	if !strings.Contains(out.String(), "2 scenes") {
		t.Errorf("stdout = %q, want a summary naming 2 scenes", out.String())
	}
}

func TestRun_SubsModelOverride(t *testing.T) {
	file := makeTalkie(t)
	var out bytes.Buffer
	if err := run(t.Context(), []string{"scenes", file}, &out, &out); err != nil {
		t.Fatal(err)
	}
	var model string
	if err := runSubs(t.Context(), []string{"--model", "my/model", file}, &out, &out, fakeTr(&model)); err != nil {
		t.Fatal(err)
	}
	if model != "my/model" {
		t.Errorf("model = %q, want the override", model)
	}
}

func TestRun_SubsErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "Film.mkv")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no file", nil, "usage"},
		{"missing file", []string{"/nonexistent/film.mkv"}, "film.mkv"},
		{"unknown language", []string{"--lang", "xx", file}, "xx"},
		{"scenes not run", []string{file}, "scenes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			var model string
			err := runSubs(t.Context(), tt.args, &out, &out, fakeTr(&model))
			if err == nil || !strings.Contains(err.Error()+out.String(), tt.want) {
				t.Errorf("err = %v, out = %q; want it to mention %q", err, out.String(), tt.want)
			}
		})
	}
	if _, err := os.Stat(strings.TrimSuffix(file, ".mkv") + ".film"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a failed subs run created a bundle: %v", err)
	}
}
