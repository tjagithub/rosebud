// Package scenes detects cuts in a film with ffmpeg and extracts a frame per scene.
package scenes

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ParseCuts reads ffmpeg's metadata=print output and returns each cut's time.
// Lines that are not a valid pts_time are skipped.
func ParseCuts(r io.Reader) ([]time.Duration, error) {
	var cuts []time.Duration
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		_, rest, ok := strings.Cut(sc.Text(), "pts_time:")
		if !ok {
			continue
		}
		if d, err := parseSeconds(strings.Fields(rest)); err == nil {
			cuts = append(cuts, d)
		}
	}
	return cuts, sc.Err()
}

// ParseDuration reads ffprobe's duration output, in seconds. It must be positive.
func ParseDuration(out string) (time.Duration, error) {
	d, err := parseSeconds(strings.Fields(out))
	if err != nil {
		return 0, fmt.Errorf("scenes: duration %q: %w", strings.TrimSpace(out), err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("scenes: duration %q is not positive", strings.TrimSpace(out))
	}
	return d, nil
}

func parseSeconds(fields []string) (time.Duration, error) {
	if len(fields) == 0 {
		return 0, errors.New("no value")
	}
	f, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%q is not finite", fields[0])
	}
	return time.Duration(math.Round(f * float64(time.Second))), nil
}

// Span is one scene: [Start, End).
type Span struct{ Start, End time.Duration }

// Mid is the middle of the span, where its representative frame is taken.
func (s Span) Mid() time.Duration { return s.Start + (s.End-s.Start)/2 }

// Boundaries turns cut times into contiguous spans covering [0, total].
// Out-of-range and duplicate cuts, and cuts leaving a span under minLen, are dropped.
func Boundaries(cuts []time.Duration, total, minLen time.Duration) []Span {
	if total <= 0 {
		return nil
	}
	sorted := slices.Sorted(slices.Values(cuts))
	var spans []Span
	start := time.Duration(0)
	for _, c := range sorted {
		if c <= start || c >= total || c-start < minLen || total-c < minLen {
			continue
		}
		spans = append(spans, Span{start, c})
		start = c
	}
	return append(spans, Span{start, total})
}

// SplitLong cuts spans longer than max into equal pieces; max <= 0 keeps them.
func SplitLong(spans []Span, max time.Duration) []Span {
	if max <= 0 {
		return spans
	}
	var out []Span
	for _, sp := range spans {
		n := int((sp.End - sp.Start + max - 1) / max)
		if n <= 1 {
			out = append(out, sp)
			continue
		}
		at := sp.Start
		for i := 1; i <= n; i++ {
			end := sp.Start + (sp.End-sp.Start)*time.Duration(i)/time.Duration(n)
			out = append(out, Span{at, end})
			at = end
		}
	}
	return out
}
