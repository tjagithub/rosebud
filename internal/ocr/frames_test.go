package ocr_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/ocr"
)

// subFrame is a transparent 1080p frame, with a white box if box is not empty.
func subFrame(box image.Rectangle) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	draw.Draw(img, box, image.NewUniform(color.White), image.Point{}, draw.Src)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func fake(out []byte, err error, got *[]string) ocr.Exec {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*got = append([]string{name}, args...)
		return out, err
	}
}

func TestFramesKeepsOnlyTextFramesCroppedToInk(t *testing.T) {
	var got []string
	out := cat(
		subFrame(image.Rectangle{}),
		subFrame(image.Rect(100, 900, 120, 910)),
		subFrame(image.Rectangle{}),
		subFrame(image.Rect(0, 0, 50, 50)),
	)
	frames, err := ocr.Frames(t.Context(), fake(out, nil, &got), "film.mkv", 1, testTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	// the box plus a 10 px margin, clamped to the frame
	if s := frames[0].Bounds().Size(); s != (image.Point{40, 30}) {
		t.Errorf("first frame is %v, want 40x30", s)
	}
	if s := frames[1].Bounds().Size(); s != (image.Point{60, 60}) {
		t.Errorf("second frame is %v, want 60x60", s)
	}
	if got[0] != "ffmpeg" || !slices.Contains(got, "film.mkv") || !slices.ContainsFunc(got, func(a string) bool { return strings.Contains(a, "0:s:1") }) {
		t.Errorf("command = %q", got)
	}
}

func TestFramesNoOutput(t *testing.T) {
	var got []string
	frames, err := ocr.Frames(t.Context(), fake(nil, nil, &got), "f.mkv", 0, testTimeout)
	if err != nil || len(frames) != 0 {
		t.Errorf("got %d frames, err %v", len(frames), err)
	}
}

func TestFramesErrors(t *testing.T) {
	var got []string
	boom := errors.New("boom")
	if _, err := ocr.Frames(t.Context(), fake(nil, boom, &got), "f.mkv", 0, testTimeout); !errors.Is(err, boom) {
		t.Errorf("exec error = %v", err)
	}
	trunc := subFrame(image.Rect(0, 0, 5, 5))
	if _, err := ocr.Frames(t.Context(), fake(trunc[:len(trunc)/2], nil, &got), "f.mkv", 0, testTimeout); err == nil {
		t.Error("truncated png: no error")
	}
}

func TestFramesRejectsBadInput(t *testing.T) {
	var got []string
	for name, c := range map[string]struct {
		file string
		sub  int
	}{"empty path": {"", 0}, "negative stream": {"f.mkv", -1}} {
		if _, err := ocr.Frames(t.Context(), fake(nil, nil, &got), c.file, c.sub, testTimeout); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if len(got) != 0 {
		t.Errorf("ran %q for bad input", got)
	}
}

// hang blocks like a stuck ffmpeg until its context ends.
func hang(called *bool) ocr.Exec {
	return func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		*called = true
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

func TestFramesTimesOutOnAHang(t *testing.T) {
	var called bool
	start := time.Now()
	_, err := ocr.Frames(t.Context(), hang(&called), "f.mkv", 0, 50*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), "ffmpeg took over 50ms to complete") || !strings.Contains(err.Error(), "--timeout 100ms") {
		t.Errorf("message = %q", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("took far longer than the timeout")
	}
}

func TestFramesStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var called bool
	if _, err := ocr.Frames(ctx, hang(&called), "f.mkv", 0, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want Canceled", err)
	}
}

func TestFramesNeedsATimeout(t *testing.T) {
	var called bool
	for _, d := range []time.Duration{0, -time.Second} {
		if _, err := ocr.Frames(t.Context(), hang(&called), "f.mkv", 0, d); err == nil {
			t.Errorf("timeout %v: no error", d)
		}
	}
	if called {
		t.Error("ran with no usable timeout")
	}
}

const testTimeout = time.Minute
