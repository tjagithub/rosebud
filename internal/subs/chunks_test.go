package subs_test

import (
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/tjagithub/rosebud/internal/subs"
)

func sec(n int) time.Duration { return time.Duration(n) * time.Second }

// quietAt is a one-second silence centred on whole second n.
func quietAt(n int) subs.Span {
	return subs.Span{Start: sec(n) - 500*time.Millisecond, End: sec(n) + 500*time.Millisecond}
}

func TestChunks(t *testing.T) {
	tests := []struct {
		name  string
		total time.Duration
		quiet []subs.Span
		max   time.Duration
		want  []subs.Span
	}{
		{"empty film", 0, nil, sec(30), nil},
		{"short film is one chunk", sec(20), nil, sec(30), []subs.Span{{0, sec(20)}}},
		{"exactly max is one chunk", sec(30), nil, sec(30), []subs.Span{{0, sec(30)}}},
		{
			"no silence cuts at max", sec(100), nil, sec(30),
			[]subs.Span{{0, sec(30)}, {sec(30), sec(60)}, {sec(60), sec(90)}, {sec(90), sec(100)}},
		},
		{
			"cuts in a silence", sec(50),
			[]subs.Span{quietAt(25)},
			sec(30),
			[]subs.Span{{0, sec(25)}, {sec(25), sec(50)}},
		},
		{
			"latest silence wins", sec(50),
			[]subs.Span{quietAt(20), quietAt(28)},
			sec(30),
			[]subs.Span{{0, sec(28)}, {sec(28), sec(50)}},
		},
		{
			"silence in the first half is ignored", sec(50),
			[]subs.Span{quietAt(10)},
			sec(30),
			[]subs.Span{{0, sec(30)}, {sec(30), sec(50)}},
		},
		{
			"silence exactly half way is used", sec(50),
			[]subs.Span{quietAt(15)},
			sec(30),
			[]subs.Span{{0, sec(15)}, {sec(15), sec(45)}, {sec(45), sec(50)}},
		},
		{
			"silence exactly at max is used", sec(50),
			[]subs.Span{quietAt(30)},
			sec(30),
			[]subs.Span{{0, sec(30)}, {sec(30), sec(50)}},
		},
		{
			"silence at max beats an earlier one", sec(50),
			[]subs.Span{quietAt(28), quietAt(30)},
			sec(30),
			[]subs.Span{{0, sec(30)}, {sec(30), sec(50)}},
		},
		{
			"silence past the film is ignored", sec(40),
			[]subs.Span{quietAt(60)},
			sec(30),
			[]subs.Span{{0, sec(30)}, {sec(30), sec(40)}},
		},
		{
			"unsorted silences", sec(50),
			[]subs.Span{quietAt(28), quietAt(20)},
			sec(30),
			[]subs.Span{{0, sec(28)}, {sec(28), sec(50)}},
		},
		{"no limit is one chunk", sec(100), nil, 0, []subs.Span{{0, sec(100)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subs.Chunks(tt.total, tt.quiet, tt.max)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Chunks(%v, max %v) = %v, want %v", tt.total, tt.max, got, tt.want)
			}
		})
	}
}

func TestChunks_Properties(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Whole seconds make exact-boundary cases common.
		total := sec(rapid.IntRange(1, 200).Draw(rt, "total"))
		max := sec(rapid.IntRange(2, 40).Draw(rt, "max"))
		quiet := rapid.SliceOfN(rapid.Custom(func(t *rapid.T) subs.Span {
			s := rapid.IntRange(0, int(total/time.Second)+5).Draw(t, "start")
			return subs.Span{Start: sec(s), End: sec(s + rapid.IntRange(0, 4).Draw(t, "len"))}
		}), 0, 30).Draw(rt, "quiet")

		got := subs.Chunks(total, slices.Clone(quiet), max)

		if len(got) == 0 || got[0].Start != 0 || got[len(got)-1].End != total {
			rt.Fatalf("chunks do not cover [0,%v]: %v", total, got)
		}
		mids := make(map[time.Duration]bool)
		for _, q := range quiet {
			mids[q.Start+(q.End-q.Start)/2] = true
		}
		for i, c := range got {
			if c.End <= c.Start || c.End-c.Start > max {
				rt.Fatalf("chunk %v at %d is empty, inverted or longer than %v", c, i, max)
			}
			if i > 0 && got[i-1].End != c.Start {
				rt.Fatalf("gap or overlap before chunk %d: %v", i, got)
			}
			if i == len(got)-1 {
				continue
			}
			// A cut is either in a silence or forced by the limit, and a forced
			// cut means no silence was available in the second half.
			if mids[c.End] {
				continue
			}
			if c.End != c.Start+max {
				rt.Fatalf("cut %v is neither a silence nor the limit: %v", c.End, got)
			}
			for m := range mids {
				if m >= c.Start+max/2 && m < c.End && m < total {
					rt.Fatalf("forced cut at %v ignored silence at %v: %v", c.End, m, got)
				}
			}
		}
		if !slices.Equal(got, subs.Chunks(total, quiet, max)) {
			rt.Fatalf("not deterministic")
		}
	})
}
