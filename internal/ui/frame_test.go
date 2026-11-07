package ui_test

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/ui"
	"github.com/tjagithub/rosebud/internal/ui/uitest"
)

// bundleWithFrames writes a tiny jpeg per name under frames/.
func bundleWithFrames(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		img := image.NewRGBA(image.Rect(0, 0, 64, 36))
		for i := range img.Pix {
			img.Pix[i] = 200
		}
		img.Set(1, 1, color.Black)
		f, err := os.Create(filepath.Join(dir, "frames", n))
		if err != nil {
			t.Fatal(err)
		}
		if err := jpeg.Encode(f, img, nil); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	return dir
}

func withFrame(h core.Hit, frame string) core.Hit {
	h.Scene.Frame = frame
	return h
}

func TestSelectedFrameIsSentAndShown(t *testing.T) {
	dir := bundleWithFrames(t, "a.jpg", "b.jpg")
	f := &fakeSearch{hits: []core.Hit{
		withFrame(hit(1, time.Second, "one"), "frames/a.jpg"),
		withFrame(hit(2, 2*time.Second, "two"), "frames/b.jpg"),
	}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithBundle(dir)))
	tui.Type("x")
	if len(tui.Raw) != 1 || !strings.Contains(tui.Raw[0], "\x1b_G") {
		t.Fatalf("raw = %q", tui.Raw)
	}
	tui.Screen().Contains(string(rune(0x10EEEE)))
	tui.Press("down")
	if len(tui.Raw) != 2 {
		t.Errorf("moving to another hit must send its frame, raw = %d", len(tui.Raw))
	}
}

func TestMissingFrameIsSkipped(t *testing.T) {
	f := &fakeSearch{hits: []core.Hit{withFrame(hit(1, time.Second, "one"), "frames/none.jpg")}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithBundle(t.TempDir())))
	tui.Type("x")
	if len(tui.Raw) != 0 {
		t.Errorf("raw = %q", tui.Raw)
	}
	tui.Screen().Lacks(string(rune(0x10EEEE)))
	tui.Screen().Contains("one")
}

func TestStaleFrameIsDropped(t *testing.T) {
	dir := bundleWithFrames(t, "a.jpg")
	f := &fakeSearch{hits: []core.Hit{withFrame(hit(1, time.Second, "one"), "frames/a.jpg")}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithBundle(dir)))
	tui.Type("x")
	n := len(tui.Raw)
	tui.Send(ui.FrameMsg{Path: "frames/other.jpg", Data: "late"})
	if len(tui.Raw) != n {
		t.Error("a frame for a hit no longer selected must not be sent")
	}
}
