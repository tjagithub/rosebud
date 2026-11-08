// Package langid tells which language a film speaks. A Detector scores one
// window of audio; Detect averages over windows, which absorbs fades and strays.
package langid

import (
	"context"
	"errors"
	"slices"
	"time"
)

// Probs maps a language code (ISO 639-1 where one exists) to its probability.
type Probs map[string]float64

// Detector scores the window of audio that starts at the given time.
type Detector interface {
	Window(ctx context.Context, at time.Duration) (Probs, error)
}

// Result is the language and how sure we are, e.g. {"ja", 0.80}.
type Result struct {
	Language string
	Score    float64 // mean probability over the windows that scored
	Windows  int
}

// ErrNoSpeech is returned when no window could be scored.
var ErrNoSpeech = errors.New("langid: no window could be scored")

// Detect averages the windows at the given times. A failed window is skipped;
// Detect fails only if all do, or ctx ends.
func Detect(ctx context.Context, d Detector, at []time.Duration) (Result, error) {
	sum := Probs{}
	n := 0
	for _, t := range at {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		p, err := d.Window(ctx, t)
		if err != nil {
			continue
		}
		for l, v := range p {
			sum[l] += v
		}
		n++
	}
	if n == 0 {
		return Result{}, ErrNoSpeech
	}
	best := ""
	for l, v := range sum {
		if best == "" || v > sum[best] || (v == sum[best] && l < best) {
			best = l
		}
	}
	return Result{Language: best, Score: sum[best] / float64(n), Windows: n}, nil
}

// LoudWindows are the window starts whose rms is at least min, in time order.
// A silent window scores confident nonsense, so it must not vote.
func LoudWindows(rms map[time.Duration]float64, min float64) []time.Duration {
	var out []time.Duration
	for t, v := range rms {
		if v >= min {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return out
}
