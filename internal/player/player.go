// Package player plays a film from a point: video as paced raw frames and
// audio from a separate ffplay. Both start together; the wall clock rules.
package player

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"
)

// Frame is one decoded picture: w*h RGB bytes, and the film time it shows at.
type Frame struct {
	At  time.Duration
	Pix []byte
}

// VideoArgs are ffmpeg's arguments to write raw RGB frames at fps, scaled to
// w by h, from the given film time to stdout.
func VideoArgs(file string, from time.Duration, w, h int, fps float64) []string {
	return []string{"-nostdin", "-v", "error", "-ss", secs(from), "-i", file, "-an",
		"-vf", fmt.Sprintf("fps=%s,scale=%d:%d", strconv.FormatFloat(fps, 'f', -1, 64), w, h),
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-"}
}

// AudioArgs are ffplay's arguments to play the sound from the given film time.
func AudioArgs(file string, from time.Duration) []string {
	return []string{"-nostdin", "-nodisp", "-vn", "-autoexit", "-loglevel", "error", "-ss", secs(from), file}
}

func secs(d time.Duration) string { return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) }

// Frames sends the w by h RGB frames of r when due, the i-th at i/fps. A frame
// a whole frame late is dropped. The channel closes at the end of r or of ctx.
func Frames(ctx context.Context, r io.Reader, w, h int, fps float64, from time.Duration,
	now func() time.Duration, sleep func(context.Context, time.Duration) error) <-chan Frame {
	out := make(chan Frame)
	go func() {
		defer close(out)
		start := now()
		step := time.Duration(float64(time.Second) / fps)
		for i := 0; ; i++ {
			buf := make([]byte, w*h*3)
			if _, err := io.ReadFull(r, buf); err != nil {
				return
			}
			due := start + time.Duration(i)*step
			if wait := due - now(); wait > 0 {
				if sleep(ctx, wait) != nil {
					return
				}
			} else if -wait >= step {
				continue
			}
			select {
			case out <- Frame{At: from + time.Duration(i)*step, Pix: buf}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
