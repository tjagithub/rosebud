package ocr_test

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/tjagithub/rosebud/internal/ocr"
)

// frame is a black frame with a white box, like a bitmap subtitle.
func frame(box image.Rectangle) image.Image {
	img := image.NewGray(image.Rect(0, 0, 1920, 400))
	draw.Draw(img, box, image.NewUniform(color.White), image.Point{}, draw.Src)
	return img
}

func dark(img *image.Gray, x, y int) bool { return img.GrayAt(x, y).Y < 128 }

func TestPageSize(t *testing.T) {
	p := ocr.Page(nil)
	if b := p.Bounds(); b.Dx() != ocr.PageW || b.Dy() != ocr.PageH {
		t.Errorf("size = %v, want %dx%d", b.Size(), ocr.PageW, ocr.PageH)
	}
	if dark(p, 10, 10) || dark(p, ocr.PageW-1, ocr.PageH-1) {
		t.Error("empty page is not white")
	}
}

func TestPageInvertsAndPlaces(t *testing.T) {
	p := ocr.Page([]image.Image{
		frame(image.Rect(100, 50, 120, 60)),
		frame(image.Rect(300, 80, 340, 100)),
	})
	// the box lands 10 px in from the paste point, which is (60, band*BandH+30)
	if !dark(p, 70, 40) || !dark(p, 89, 49) {
		t.Error("first box not drawn black at its slot")
	}
	if dark(p, 69, 40) || dark(p, 90, 49) || dark(p, 70, 39) || dark(p, 70, 50) {
		t.Error("first box drawn too large")
	}
	y := ocr.BandH + 30
	if !dark(p, 70, y+10) || !dark(p, 109, y+29) {
		t.Error("second box not drawn black in band 1")
	}
}

func TestPageBlankFrameLeavesBandWhite(t *testing.T) {
	p := ocr.Page([]image.Image{frame(image.Rectangle{}), frame(image.Rect(0, 0, 50, 50))})
	for y := 0; y < ocr.BandH; y += 7 {
		for x := 0; x < ocr.PageW; x += 7 {
			if dark(p, x, y) {
				t.Fatalf("band 0 has a dark pixel at %d,%d", x, y)
			}
		}
	}
	if !dark(p, 70, ocr.BandH+40) {
		t.Error("band 1 box missing")
	}
}

func TestPageClipsTallFrames(t *testing.T) {
	p := ocr.Page([]image.Image{frame(image.Rect(100, 0, 400, 400))})
	for x := 0; x < ocr.PageW; x += 5 {
		if dark(p, x, ocr.BandH) {
			t.Fatalf("band 0 spilled into band 1 at x=%d", x)
		}
	}
}

func TestPageIgnoresFramesPastTheLastBand(t *testing.T) {
	frames := make([]image.Image, ocr.PerPage+2)
	for i := range frames {
		frames[i] = frame(image.Rect(0, 0, 50, 50))
	}
	if b := ocr.Page(frames).Bounds(); b.Dy() != ocr.PageH {
		t.Errorf("height = %d", b.Dy())
	}
}
