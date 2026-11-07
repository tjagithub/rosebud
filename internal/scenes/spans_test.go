package scenes_test

import (
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/tjagithub/rosebud/internal/scenes"
)

func sec(n int) time.Duration { return time.Duration(n) * time.Second }

func TestBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		cuts   []time.Duration
		total  time.Duration
		minLen time.Duration
		want   []scenes.Span
	}{
		{"no cuts is one scene", nil, sec(100), sec(2), []scenes.Span{{0, sec(100)}}},
		{
			"cuts split the film",
			[]time.Duration{sec(10), sec(40)},
			sec(100), sec(2),
			[]scenes.Span{{0, sec(10)}, {sec(10), sec(40)}, {sec(40), sec(100)}},
		},
		{
			"unsorted cuts are sorted",
			[]time.Duration{sec(40), sec(10)},
			sec(100), sec(2),
			[]scenes.Span{{0, sec(10)}, {sec(10), sec(40)}, {sec(40), sec(100)}},
		},
		{
			"cuts outside the film are dropped",
			[]time.Duration{-sec(1), 0, sec(100), sec(200)},
			sec(100), sec(2),
			[]scenes.Span{{0, sec(100)}},
		},
		{
			"a cut too close to the previous is merged",
			[]time.Duration{sec(10), sec(11)},
			sec(100), sec(2),
			[]scenes.Span{{0, sec(10)}, {sec(10), sec(100)}},
		},
		{
			"a cut too close to the start is dropped",
			[]time.Duration{sec(1)},
			sec(100), sec(2),
			[]scenes.Span{{0, sec(100)}},
		},
		{
			"a cut too close to the end is dropped",
			[]time.Duration{sec(99)},
			sec(100), sec(2),
			[]scenes.Span{{0, sec(100)}},
		},
		{
			"a cut exactly minLen from each edge is kept",
			[]time.Duration{sec(2), sec(98)},
			sec(100), sec(2),
			[]scenes.Span{{0, sec(2)}, {sec(2), sec(98)}, {sec(98), sec(100)}},
		},
		{
			"duplicate cuts count once",
			[]time.Duration{sec(10), sec(10)},
			sec(100), sec(2),
			[]scenes.Span{{0, sec(10)}, {sec(10), sec(100)}},
		},
		{
			"film shorter than the minimum",
			[]time.Duration{sec(1)},
			sec(2), sec(5),
			[]scenes.Span{{0, sec(2)}},
		},
		{"no duration, no scenes", []time.Duration{sec(1)}, 0, sec(2), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scenes.Boundaries(tt.cuts, tt.total, tt.minLen)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpan_Mid(t *testing.T) {
	if got := (scenes.Span{Start: sec(10), End: sec(20)}).Mid(); got != sec(15) {
		t.Errorf("Mid = %v, want 15s", got)
	}
}

func TestBoundaries_Properties(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Whole seconds make exact-boundary cases common.
		total := sec(rapid.IntRange(1, 120).Draw(rt, "total"))
		minLen := sec(rapid.IntRange(0, 15).Draw(rt, "minLen"))
		cuts := rapid.SliceOfN(rapid.Custom(func(t *rapid.T) time.Duration {
			return sec(rapid.IntRange(-5, int(total/time.Second)+5).Draw(t, "cut"))
		}), 0, 60).Draw(rt, "cuts")

		spans := scenes.Boundaries(slices.Clone(cuts), total, minLen)

		if len(spans) == 0 || spans[0].Start != 0 || spans[len(spans)-1].End != total {
			rt.Fatalf("spans do not cover [0,%v]: %v", total, spans)
		}
		for i, s := range spans {
			if s.End <= s.Start {
				rt.Fatalf("empty or inverted span %v at %d", s, i)
			}
			if i > 0 && spans[i-1].End != s.Start {
				rt.Fatalf("gap or overlap before span %d: %v", i, spans)
			}
			if len(spans) > 1 && s.End-s.Start < minLen {
				rt.Fatalf("span %v shorter than minLen %v in %v", s, minLen, spans)
			}
		}
		again := scenes.Boundaries(cuts, total, minLen)
		if !slices.Equal(spans, again) {
			rt.Fatalf("not deterministic: %v vs %v", spans, again)
		}
	})
}

func TestSplitLong(t *testing.T) {
	s := time.Second
	got := scenes.SplitLong([]scenes.Span{{0, 10 * s}, {10 * s, 100 * s}, {100 * s, 101 * s}}, 40*s)
	want := []scenes.Span{{0, 10 * s}, {10 * s, 40 * s}, {40 * s, 70 * s}, {70 * s, 100 * s}, {100 * s, 101 * s}}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if same := scenes.SplitLong(want, 0); !slices.Equal(same, want) {
		t.Errorf("max 0 must leave spans alone, got %v", same)
	}
}

func TestSplitLong_NoPieceOverMaxAndNoGaps(t *testing.T) {
	s := time.Second
	in := []scenes.Span{{0, 307*s + 123}}
	out := scenes.SplitLong(in, 45*s)
	prev := time.Duration(0)
	for _, sp := range out {
		if sp.Start != prev || sp.End-sp.Start > 45*s || sp.End <= sp.Start {
			t.Fatalf("bad piece %v after %v in %v", sp, prev, out)
		}
		prev = sp.End
	}
	if prev != in[0].End {
		t.Errorf("ends at %v, want %v", prev, in[0].End)
	}
}
