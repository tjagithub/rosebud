package player_test

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/player"
)

// clock is a fake time: sleeping moves it.
type clock struct{ t time.Duration }

func (c *clock) now() time.Duration { return c.t }
func (c *clock) sleep(_ context.Context, d time.Duration) error {
	c.t += d
	return nil
}

// raw is n frames of w*h*3 bytes, frame i filled with byte i.
func raw(n, w, h int) *bytes.Reader {
	var b []byte
	for i := range n {
		b = append(b, bytes.Repeat([]byte{byte(i)}, w*h*3)...)
	}
	return bytes.NewReader(b)
}

func collect(ch <-chan player.Frame) (at []time.Duration, first []byte) {
	for f := range ch {
		at = append(at, f.At)
		first = append(first, f.Pix[0])
	}
	return
}

func TestFrames_PacedAtTheFrameRateFromTheStartTime(t *testing.T) {
	c := &clock{}
	ch := player.Frames(t.Context(), raw(4, 2, 2), 2, 2, 10, 90*time.Second, c.now, c.sleep)
	at, first := collect(ch)
	s := 90 * time.Second
	want := []time.Duration{s, s + 100*time.Millisecond, s + 200*time.Millisecond, s + 300*time.Millisecond}
	if !slices.Equal(at, want) || !slices.Equal(first, []byte{0, 1, 2, 3}) {
		t.Errorf("at %v frames %v", at, first)
	}
	if c.t != 300*time.Millisecond {
		t.Errorf("slept to %v, want 300ms", c.t)
	}
}

// lateClock runs 250 ms ahead after its second reading, as if the terminal stalled.
type lateClock struct {
	clock
	reads int
}

func (c *lateClock) now() time.Duration {
	c.reads++
	if c.reads > 2 {
		return c.t + 250*time.Millisecond
	}
	return c.t
}

func TestFrames_AFrameAWholeStepLateIsDropped(t *testing.T) {
	c := &lateClock{}
	at, first := collect(player.Frames(t.Context(), raw(6, 2, 2), 2, 2, 10, 0, c.now, c.sleep))
	if slices.Contains(first, 1) {
		t.Errorf("frames %v at %v: frame 1 was 150 ms late and must go", first, at)
	}
	if first[0] != 0 || first[len(first)-1] != 5 {
		t.Errorf("frames %v: the first and last must still arrive", first)
	}
}

func TestFrames_StopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	c := &clock{}
	ch := player.Frames(ctx, raw(1000, 2, 2), 2, 2, 10, 0, c.now, func(ctx context.Context, d time.Duration) error {
		cancel()
		return ctx.Err()
	})
	n := 0
	for range ch {
		n++
	}
	if n > 2 {
		t.Errorf("got %d frames after cancel", n)
	}
}

func TestFrames_PartialLastFrameIsDropped(t *testing.T) {
	r := bytes.NewReader(append(bytes.Repeat([]byte{7}, 12), 1, 2, 3))
	c := &clock{}
	_, first := collect(player.Frames(t.Context(), r, 2, 2, 10, 0, c.now, c.sleep))
	if !slices.Equal(first, []byte{7}) {
		t.Errorf("frames %v", first)
	}
}

func TestArgs(t *testing.T) {
	v := player.VideoArgs("f.mkv", 90*time.Second+500*time.Millisecond, 160, 90, 12)
	for _, seq := range [][]string{{"-ss", "90.5"}, {"-i", "f.mkv"}, {"-an"}, {"-f", "rawvideo"}, {"-pix_fmt", "rgb24"}, {"-vf", "fps=12,scale=160:90:force_original_aspect_ratio=decrease,pad=160:90:(ow-iw)/2:(oh-ih)/2"}} {
		if !contains(v, seq) {
			t.Errorf("video args %v lack %v", v, seq)
		}
	}
	if v[len(v)-1] != "-" || slices.Index(v, "-ss") > slices.Index(v, "-i") {
		t.Errorf("video args %v: want -ss before -i and output to stdout", v)
	}
	a := player.AudioArgs("f.mkv", 90*time.Second)
	for _, seq := range [][]string{{"-ss", "90"}, {"-nodisp"}, {"-vn"}, {"-autoexit"}} {
		if !contains(a, seq) {
			t.Errorf("audio args %v lack %v", a, seq)
		}
	}
	if a[len(a)-1] != "f.mkv" {
		t.Errorf("audio args %v: file must be last", a)
	}
}

func contains(args, seq []string) bool {
	for i := range args {
		if i+len(seq) <= len(args) && slices.Equal(args[i:i+len(seq)], seq) {
			return true
		}
	}
	return false
}
