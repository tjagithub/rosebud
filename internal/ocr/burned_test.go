package ocr_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/ocr"
)

// textImg is a dark band with a lit block at r, or nothing if r is empty.
func textImg(r image.Rectangle, lum uint8) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, 200, 40))
	draw.Draw(g, r, image.NewUniform(color.Gray{lum}), image.Point{}, draw.Src)
	return g
}

func TestWhitenKeepsOnlyBrightPixels(t *testing.T) {
	g := image.NewGray(image.Rect(0, 0, 2, 1))
	g.SetGray(0, 0, color.Gray{250})
	g.SetGray(1, 0, color.Gray{100})
	w := ocr.Whiten(g, 200)
	if w.GrayAt(0, 0).Y != 250 || w.GrayAt(1, 0).Y != 0 {
		t.Errorf("got %v %v", w.GrayAt(0, 0), w.GrayAt(1, 0))
	}
}

func TestBurnedEventsGroupsSameTextAndSplitsOnChange(t *testing.T) {
	a, b := image.Rect(10, 10, 100, 20), image.Rect(10, 25, 100, 35)
	step := 500 * time.Millisecond
	s := []ocr.Sample{
		{At: 0, Img: textImg(image.Rectangle{}, 0)},
		{At: step, Img: textImg(a, 255)},
		{At: 2 * step, Img: textImg(a, 160)}, // fading, still the same line
		{At: 3 * step, Img: textImg(b, 255)}, // a different line
		{At: 4 * step, Img: textImg(image.Rectangle{}, 0)},
		{At: 5 * step, Img: textImg(a, 255)}, // the first line again, later
	}
	ev, imgs := ocr.BurnedEvents(s, step, 50)
	want := []ocr.Event{{Start: step, End: 3 * step}, {Start: 3 * step, End: 4 * step}, {Start: 5 * step, End: 6 * step}}
	if len(ev) != 3 || len(imgs) != 3 {
		t.Fatalf("events = %v, images = %d", ev, len(imgs))
	}
	for i := range want {
		if ev[i] != want[i] {
			t.Errorf("event %d = %v, want %v", i, ev[i], want[i])
		}
	}
	if imgs[0].(*image.Gray).GrayAt(20, 15).Y != 255 {
		t.Error("the sharpest sample of a group must represent it")
	}
}

func TestBurnedEventsIgnoresSpecks(t *testing.T) {
	step := time.Second
	ev, _ := ocr.BurnedEvents([]ocr.Sample{{At: 0, Img: textImg(image.Rect(0, 0, 3, 3), 255)}}, step, 50)
	if len(ev) != 0 {
		t.Errorf("a speck is not a subtitle: %v", ev)
	}
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestBurnedStageReadsTextIntoScenes(t *testing.T) {
	line := textImg(image.Rect(10, 10, 100, 20), 255)
	blank := textImg(image.Rectangle{}, 0)
	frames := [][]byte{pngBytes(t, blank), pngBytes(t, line), pngBytes(t, line), pngBytes(t, blank)}
	ex := func(context.Context, string, ...string) ([]byte, error) {
		return bytes.Join(frames, nil), nil
	}
	st := &memStore{scenes: []core.Scene{{ID: 1, Start: 0, End: sec(1)}, {ID: 2, Start: sec(1), End: sec(3)}}}
	rd := &bandReader{texts: [][]string{{"hello there"}}}
	s := ocr.BurnedStage{Exec: ex, Store: st, Reader: rd, FPS: 1, Timeout: time.Minute, MinInk: 20}
	if err := s.Run(t.Context(), spec(map[string]string{"file": "f.mkv"}), noProgress); err != nil {
		t.Fatal(err)
	}
	if st.got[2] != "hello there" || st.got[1] != "" {
		t.Errorf("stored %v", st.got)
	}
}

func TestBurnedStageSkipsWhenThereIsNoText(t *testing.T) {
	ex := func(context.Context, string, ...string) ([]byte, error) {
		return pngBytes(t, textImg(image.Rectangle{}, 0)), nil
	}
	st := &memStore{scenes: []core.Scene{{ID: 1, End: sec(5)}}}
	s := ocr.BurnedStage{Exec: ex, Store: st, Reader: &bandReader{}, FPS: 1, Timeout: time.Minute, MinInk: 20}
	err := s.Run(t.Context(), spec(map[string]string{"file": "f.mkv"}), noProgress)
	if err == nil || err.Error()[:len(core.SkipPrefix)] != core.SkipPrefix || st.set {
		t.Errorf("err = %v, wrote = %v", err, st.set)
	}
}
