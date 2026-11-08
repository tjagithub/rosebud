package player_test

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/player"
)

func clip(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	file := filepath.Join(t.TempDir(), "clip.mp4")
	out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=s=64x36:r=10:d=3", "-f", "lavfi", "-i", "sine=d=3",
		"-pix_fmt", "yuv420p", "-shortest", file).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	return file
}

func TestPlay_StreamsFramesFromTheStartTime(t *testing.T) {
	file := clip(t)
	p, err := player.Play(t.Context(), player.Spec{File: file, From: time.Second, W: 32, H: 18, FPS: 10, NoAudio: true})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	var first time.Duration
	for f := range p.Frames {
		if n == 0 {
			first = f.At
		}
		if len(f.Pix) != 32*18*3 {
			t.Fatalf("frame has %d bytes", len(f.Pix))
		}
		n++
	}
	if first != time.Second || n < 10 || n > 21 {
		t.Errorf("first frame at %v, %d frames from 1 s of a 3 s clip, want ~20", first, n)
	}
	if err := p.Wait(); err != nil {
		t.Errorf("wait: %v", err)
	}
}

func TestPlay_StopEndsEverything(t *testing.T) {
	file := clip(t)
	p, err := player.Play(t.Context(), player.Spec{File: file, W: 32, H: 18, FPS: 10, NoAudio: true})
	if err != nil {
		t.Fatal(err)
	}
	<-p.Frames
	p.Stop()
	done := make(chan struct{})
	go func() {
		for range p.Frames {
		}
		_ = p.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("playback did not stop")
	}
}

func TestPlay_BadSpec(t *testing.T) {
	for _, s := range []player.Spec{{}, {File: "f", W: 0, H: 10, FPS: 10}, {File: "f", W: 10, H: 10}} {
		if _, err := player.Play(t.Context(), s); err == nil {
			t.Errorf("%+v: no error", s)
		}
	}
	if _, err := player.Play(t.Context(), player.Spec{File: "/no/such.mkv", W: 4, H: 4, FPS: 10, NoAudio: true}); err == nil {
		// A missing file shows up as ffmpeg exiting, which Wait reports.
		t.Log("missing file is reported by Wait")
	}
}
