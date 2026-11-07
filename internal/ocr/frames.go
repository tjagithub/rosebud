package ocr

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"time"

	"github.com/tjagithub/rosebud/internal/limit"
	"github.com/tjagithub/rosebud/internal/subs"
)

// Exec runs a program to completion and returns its stdout.
type Exec = subs.Exec

// Frames renders subtitle stream sub of file as video and keeps the frames with
// text. It gives up after timeout, which must be positive.
func Frames(ctx context.Context, ex Exec, file string, sub int, timeout time.Duration) ([]image.Image, error) {
	if file == "" || sub < 0 {
		return nil, fmt.Errorf("ocr: render subtitles: bad input (file %q, stream %d)", file, sub)
	}
	out, err := limit.Run(ctx, "ffmpeg", timeout, func(ctx context.Context) ([]byte, error) {
		return ex(ctx, "ffmpeg", "-nostdin", "-v", "error", "-i", file,
			"-filter_complex", "[0:s:"+strconv.Itoa(sub)+"]copy[s]", "-map", "[s]",
			"-fps_mode", "passthrough", "-f", "image2pipe", "-c:v", "png", "-")
	})
	if err != nil {
		return nil, fmt.Errorf("ocr: render subtitles of %s: %w", file, err)
	}
	var frames []image.Image
	r := bytes.NewReader(out)
	for r.Len() > 0 {
		img, err := png.Decode(r)
		if err != nil {
			return nil, fmt.Errorf("ocr: decode subtitle frame %d of %s: %w", len(frames), file, err)
		}
		if box := ink(img); !box.Empty() {
			frames = append(frames, crop(img, box.Inset(-margin).Intersect(img.Bounds())))
		}
	}
	return frames, nil
}

// crop copies box out of img as grey, so a full frame need not stay in memory.
func crop(img image.Image, box image.Rectangle) *image.Gray {
	g := image.NewGray(image.Rect(0, 0, box.Dx(), box.Dy()))
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			g.SetGray(x-box.Min.X, y-box.Min.Y, color.Gray{luma(img, x, y)})
		}
	}
	return g
}
