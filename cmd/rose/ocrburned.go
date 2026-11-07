package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/ocr"
)

// parseMasks reads "w,h,x,y;w,h,x,y" into masks; empty means none.
func parseMasks(s string) ([]ocr.Mask, error) {
	var out []ocr.Mask
	for _, p := range strings.Split(s, ";") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		m, err := ocr.ParseMask(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("--masks: %w", err)
		}
		out = append(out, m)
	}
	return out, nil
}

// parseRange reads "FROM:TO" durations; either side may be empty.
func parseRange(s string) (from, to time.Duration, err error) {
	if s == "" {
		return 0, 0, nil
	}
	a, b, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, fmt.Errorf("--range %q: want FROM:TO, such as 1m:4m", s)
	}
	for i, v := range []string{a, b} {
		if v == "" {
			continue
		}
		d, perr := time.ParseDuration(v)
		if perr != nil || d < 0 {
			return 0, 0, fmt.Errorf("--range %q: %q is not a duration like 90s or 1m30s", s, v)
		}
		if i == 0 {
			from = d
		} else {
			to = d
		}
	}
	if to != 0 && to <= from {
		return 0, 0, fmt.Errorf("--range %q: end must be after start", s)
	}
	return from, to, nil
}
