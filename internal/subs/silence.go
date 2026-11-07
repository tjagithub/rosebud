package subs

import (
	"bufio"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// ParseSilences reads ffmpeg's silencedetect metadata (lavfi.silence_start and
// _end lines) into spans clamped to [0, total]. A silence still open at the end runs to total.
func ParseSilences(r io.Reader, total time.Duration) ([]Span, error) {
	var out []Span
	var start time.Duration
	open := false
	add := func(end time.Duration) {
		start, end = max(start, 0), min(end, total)
		if end > start {
			out = append(out, Span{start, end})
		}
	}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		d, ok := seconds(val)
		if !ok {
			continue
		}
		switch key {
		case "lavfi.silence_start":
			start, open = d, true
		case "lavfi.silence_end":
			if open {
				add(d)
				open = false
			}
		}
	}
	if open {
		add(total)
	}
	return out, sc.Err()
}

func seconds(s string) (time.Duration, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return time.Duration(math.Round(f * float64(time.Second))), true
}
