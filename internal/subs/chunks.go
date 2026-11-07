package subs

import (
	"slices"
	"time"
)

// Chunks splits [0, total) into contiguous spans of at most max, cutting at the
// latest silence midpoint in each span's second half, else at max. max <= 0 means no limit.
func Chunks(total time.Duration, quiet []Span, max time.Duration) []Span {
	if total <= 0 {
		return nil
	}
	if max <= 0 || total <= max {
		return []Span{{0, total}}
	}
	mids := make([]time.Duration, len(quiet))
	for i, q := range quiet {
		mids[i] = q.Start + (q.End-q.Start)/2
	}
	slices.Sort(mids)

	var out []Span
	start := time.Duration(0)
	for total-start > max {
		limit := start + max
		cut := limit
		// i is the first silence after the limit, so i-1 is the latest at or before it.
		if i, _ := slices.BinarySearch(mids, limit+1); i > 0 && mids[i-1] >= start+max/2 {
			cut = mids[i-1]
		}
		out = append(out, Span{start, cut})
		start = cut
	}
	return append(out, Span{start, total})
}
