package subs_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/tjagithub/rosebud/internal/subs"
)

// cutter names each wav after its span so the fake can read it back.
func cutter(spans *[]subs.Span) subs.Cutter {
	return func(_ context.Context, s subs.Span) (string, error) {
		*spans = append(*spans, s)
		return fmt.Sprintf("%d-%d", s.Start, s.End), nil
	}
}

func spanOf(wav string) subs.Span {
	var a, b int64
	_, _ = fmt.Sscanf(wav, "%d-%d", &a, &b)
	return subs.Span{Start: time.Duration(a), End: time.Duration(b)}
}

// speech hears one word per second of any wav, unless fail says the cut is bad.
type speech struct{ fail func(subs.Span) bool }

func (f speech) Transcribe(_ context.Context, wav string) ([]subs.Token, error) {
	s := spanOf(wav)
	if f.fail != nil && f.fail(s) {
		return nil, nil
	}
	var out []subs.Token
	for t := time.Duration(0); t < s.End-s.Start; t += time.Second {
		abs := s.Start + t
		out = append(out, subs.Token{Text: strconv.Itoa(int(abs / time.Second)), Start: t, End: t + time.Second})
	}
	return out, nil
}

func texts(toks []subs.Token) []string {
	var out []string
	for _, t := range toks {
		out = append(out, t.Text)
	}
	return out
}

func TestTranscribe(t *testing.T) {
	chunks := []subs.Span{{0, sec(3)}, {sec(3), sec(6)}}

	t.Run("tokens are moved to film time", func(t *testing.T) {
		var cuts []subs.Span
		got, err := subs.Transcribe(t.Context(), subs.Job{Chunks: chunks, Cut: cutter(&cuts), Tr: speech{}})
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"0", "1", "2", "3", "4", "5"}; !slices.Equal(texts(got), want) {
			t.Errorf("texts = %v, want %v", texts(got), want)
		}
		if got[4].Start != sec(4) {
			t.Errorf("token 4 starts at %v, want 4s", got[4].Start)
		}
	})

	t.Run("a silent chunk is not transcribed", func(t *testing.T) {
		var cuts []subs.Span
		_, err := subs.Transcribe(t.Context(), subs.Job{
			Chunks: chunks, Quiet: []subs.Span{{sec(3), sec(6)}}, Cut: cutter(&cuts), Tr: speech{},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(cuts, chunks[:1]) {
			t.Errorf("cut %v, want only %v", cuts, chunks[:1])
		}
	})

	t.Run("an empty loud chunk is retried with padding", func(t *testing.T) {
		var cuts []subs.Span
		tr := speech{fail: func(s subs.Span) bool { return s.Start == sec(3) }}
		got, err := subs.Transcribe(t.Context(), subs.Job{
			Chunks: chunks, Cut: cutter(&cuts), Tr: tr, Retries: 2, Shift: sec(1),
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"0", "1", "2", "3", "4", "5"}; !slices.Equal(texts(got), want) {
			t.Errorf("texts = %v, want %v", texts(got), want)
		}
		if want := (subs.Span{Start: sec(2), End: sec(6)}); cuts[len(cuts)-1] != want {
			t.Errorf("last cut = %v, want %v", cuts[len(cuts)-1], want)
		}
	})

	t.Run("padding at the film start trims instead", func(t *testing.T) {
		var cuts []subs.Span
		tr := speech{fail: func(s subs.Span) bool { return s.Start == 0 }}
		got, err := subs.Transcribe(t.Context(), subs.Job{
			Chunks: chunks[:1], Cut: cutter(&cuts), Tr: tr, Retries: 1, Shift: sec(1),
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"1", "2"}; !slices.Equal(texts(got), want) {
			t.Errorf("texts = %v, want %v", texts(got), want)
		}
	})

	t.Run("giving up is not an error", func(t *testing.T) {
		var cuts []subs.Span
		got, err := subs.Transcribe(t.Context(), subs.Job{
			Chunks: chunks, Cut: cutter(&cuts), Tr: speech{fail: func(subs.Span) bool { return true }},
			Retries: 2, Shift: sec(1),
		})
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, %v; want no tokens and no error", got, err)
		}
		if len(cuts) != 6 {
			t.Errorf("%d cuts, want 2 chunks x 3 attempts", len(cuts))
		}
	})

	t.Run("errors stop the run", func(t *testing.T) {
		boom := errors.New("boom")
		_, err := subs.Transcribe(t.Context(), subs.Job{
			Chunks: chunks,
			Cut:    func(context.Context, subs.Span) (string, error) { return "", boom },
			Tr:     speech{},
		})
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want %v", err, boom)
		}
	})

	t.Run("a cancelled context stops the run", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		var cuts []subs.Span
		_, err := subs.Transcribe(ctx, subs.Job{Chunks: chunks, Cut: cutter(&cuts), Tr: speech{}})
		if !errors.Is(err, context.Canceled) || len(cuts) != 0 {
			t.Errorf("err = %v after %d cuts, want Canceled and none", err, len(cuts))
		}
	})
}

// The transcript must not depend on where the chunks were cut.
func TestTranscribe_BoundaryIndependent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		total := sec(rapid.IntRange(1, 60).Draw(rt, "total"))
		max := sec(rapid.IntRange(2, 20).Draw(rt, "max"))
		badStart := sec(rapid.IntRange(0, 60).Draw(rt, "bad"))
		tr := speech{fail: func(s subs.Span) bool { return s.Start == badStart }}

		var cuts []subs.Span
		got, err := subs.Transcribe(context.Background(), subs.Job{
			Chunks: subs.Chunks(total, nil, max), Cut: cutter(&cuts), Tr: tr, Retries: 2, Shift: sec(1),
		})
		if err != nil {
			rt.Fatal(err)
		}
		// A chunk that fails on a padded start is recovered by the padding.
		for i, tok := range got {
			if tok.Text != strconv.Itoa(int(tok.Start/time.Second)) {
				rt.Fatalf("token %d %q is at %v", i, tok.Text, tok.Start)
			}
			if tok.Start < 0 || tok.Start >= total || (i > 0 && tok.Start <= got[i-1].Start) {
				rt.Fatalf("token %d at %v is out of order or outside [0,%v)", i, tok.Start, total)
			}
		}
	})
}

func TestCovered(t *testing.T) {
	tests := []struct {
		name  string
		s     subs.Span
		quiet []subs.Span
		want  bool
	}{
		{"no silence", subs.Span{0, sec(5)}, nil, false},
		{"one span covers", subs.Span{sec(1), sec(4)}, []subs.Span{{0, sec(5)}}, true},
		{"exact cover", subs.Span{sec(1), sec(4)}, []subs.Span{{sec(1), sec(4)}}, true},
		{"touching spans cover", subs.Span{0, sec(4)}, []subs.Span{{sec(2), sec(4)}, {0, sec(2)}}, true},
		{"a gap breaks it", subs.Span{0, sec(4)}, []subs.Span{{0, sec(1)}, {sec(2), sec(4)}}, false},
		{"one end short", subs.Span{0, sec(4)}, []subs.Span{{0, sec(3)}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subs.Covered(tt.s, tt.quiet); got != tt.want {
				t.Errorf("Covered(%v, %v) = %v, want %v", tt.s, tt.quiet, got, tt.want)
			}
		})
	}
}

func TestTranscribe_Progress(t *testing.T) {
	var cuts []subs.Span
	var got [][2]int
	_, err := subs.Transcribe(t.Context(), subs.Job{
		Chunks: []subs.Span{{0, sec(3)}, {sec(3), sec(6)}, {sec(6), sec(9)}},
		Quiet:  []subs.Span{{sec(3), sec(6)}},
		Cut:    cutter(&cuts), Tr: speech{},
		Progress: func(done, total int) { got = append(got, [2]int{done, total}) },
	})
	if err != nil {
		t.Fatal(err)
	}
	// A skipped silent chunk still counts as done.
	if want := [][2]int{{1, 3}, {2, 3}, {3, 3}}; !slices.Equal(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
	}
}
