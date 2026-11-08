// Package langid tells which language a film speaks. A Detector scores
// windows of audio; Detect averages them, which absorbs fades and strays.
package langid

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Probs maps a language code (ISO 639-1 where one exists) to its probability.
type Probs map[string]float64

// Detector scores 16 kHz mono WAVs in one go, since loading the model costs
// more than scoring. The result has one entry per WAV, nil if it was unreadable.
type Detector interface {
	Windows(ctx context.Context, wavs []string) ([]Probs, error)
}

// Cutter writes the window of audio starting at the given time as a WAV.
type Cutter func(ctx context.Context, at time.Duration) (string, error)

// Result is the language and how sure we are, e.g. {"ja", 0.80}.
type Result struct {
	Language string
	Score    float64 // mean probability over the windows that scored
	Windows  int
}

// ErrNoSpeech is returned when no window could be cut and scored.
var ErrNoSpeech = errors.New("langid: no window could be scored")

// Detect cuts the windows at the given times, scores them together and
// averages. A window that cannot be cut or scored is skipped.
func Detect(ctx context.Context, d Detector, cut Cutter, at []time.Duration) (Result, error) {
	var wavs []string
	for _, t := range at {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if w, err := cut(ctx, t); err == nil {
			wavs = append(wavs, w)
		}
	}
	if len(wavs) == 0 {
		return Result{}, ErrNoSpeech
	}
	scored, err := d.Windows(ctx, wavs)
	if err != nil {
		return Result{}, fmt.Errorf("langid: %w", err)
	}
	sum := Probs{}
	n := 0
	for _, p := range scored {
		if p == nil {
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
