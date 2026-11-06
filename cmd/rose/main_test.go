package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/engine"
	"github.com/tjagithub/rosebud/internal/store"
)

func TestRun_NoArgs(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), nil, &out, &out); err != nil || !strings.Contains(out.String(), "rosebud") {
		t.Errorf("out = %q, err = %v", out.String(), err)
	}
}

func TestRun_Errors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown command", []string{"frobnicate"}, "unknown command"},
		{"scenes without a file", []string{"scenes"}, "usage"},
		{"scenes with a missing file", []string{"scenes", "/nonexistent/film.mkv"}, "film.mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(context.Background(), tt.args, &out, &out)
			if err == nil || !strings.Contains(err.Error()+out.String(), tt.want) {
				t.Errorf("err = %v, out = %q; want it to mention %q", err, out.String(), tt.want)
			}
		})
	}
}

// makeFilm renders 3s red then 3s blue: one hard cut at 3s.
func makeFilm(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	file := filepath.Join(t.TempDir(), "Two Colours.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:r=10:d=3",
		"-f", "lavfi", "-i", "color=c=blue:s=64x64:r=10:d=3",
		"-filter_complex", "[0][1]concat=n=2:v=1:a=0", "-pix_fmt", "yuv420p", file)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	return file
}

func TestRun_Scenes(t *testing.T) {
	file := makeFilm(t)
	var out, errOut bytes.Buffer

	if err := run(context.Background(), []string{"scenes", file}, &out, &errOut); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, errOut.String())
	}

	bundle := strings.TrimSuffix(file, ".mp4") + ".film"
	db, err := store.Open(filepath.Join(bundle, "rose.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var n int
	var frame string
	if err := db.QueryRow(`select count(*), min(frame) from scenes`).Scan(&n, &frame); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("%d scenes, want 2 (one cut at 3s)", n)
	}
	if fi, err := os.Stat(filepath.Join(bundle, frame)); err != nil || fi.Size() == 0 {
		t.Errorf("frame %q missing or empty: %v", frame, err)
	}
	if !strings.Contains(out.String(), "2 scenes") {
		t.Errorf("stdout = %q, want a summary naming 2 scenes", out.String())
	}

	// A second run reuses the bundle and replaces, not duplicates.
	if err := run(context.Background(), []string{"scenes", file}, &out, &errOut); err != nil {
		t.Fatalf("second run: %v", err)
	}
	assertScenes(t, db, 2)
}

func assertScenes(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var n int
	if err := db.QueryRow(`select count(*) from scenes`).Scan(&n); err != nil || n != want {
		t.Errorf("scenes = %d, %v; want %d", n, err, want)
	}
}

func TestRun_ScenesCancelled(t *testing.T) {
	file := makeFilm(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := run(ctx, []string{"scenes", file}, &out, &out); err == nil {
		t.Error("run with a cancelled ctx succeeded, want an error")
	}
}

func TestRunJob_CancelMidway(t *testing.T) {
	started := make(chan struct{})
	eng := engine.New(nil, engine.WithStage("slow", func(ctx context.Context, _ core.JobSpec, progress func(int64, int64)) error {
		progress(1, 10)
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}))
	t.Cleanup(func() { _ = eng.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	var progress bytes.Buffer
	info, err := runJob(ctx, eng, core.JobSpec{Stage: "slow", Film: "f"}, &progress)

	if err == nil || !strings.Contains(err.Error(), "cancelled") || info.State != core.JobCancelled {
		t.Errorf("info.State = %v, err = %v; want a cancelled job", info.State, err)
	}
}

func TestRunJob_Failure(t *testing.T) {
	eng := engine.New(nil, engine.WithStage("bad", func(context.Context, core.JobSpec, func(int64, int64)) error {
		return errors.New("kaboom")
	}))
	t.Cleanup(func() { _ = eng.Close() })

	_, err := runJob(context.Background(), eng, core.JobSpec{Stage: "bad", Film: "f"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("err = %v, want the stage's error", err)
	}
}
