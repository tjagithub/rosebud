package subs

import (
	"context"
	"fmt"
	"time"
)

// Cutter writes the audio of a span to a file the Transcriber can read.
type Cutter func(ctx context.Context, s Span) (wav string, err error)

// Job is one film's transcription. Quiet spans are skipped. An empty loud
// chunk is retried up to Retries times, its start moved by Shift each time.
type Job struct {
	Chunks  []Span
	Quiet   []Span
	Cut     Cutter
	Tr      Transcriber
	Retries int
	Shift   time.Duration

	Progress func(done, total int) // called after each chunk, skipped or not
}

// Transcribe returns the tokens of every chunk in film time, each kept to
// the chunk it belongs to. Empty chunks give no tokens and no error.
func Transcribe(ctx context.Context, j Job) ([]Token, error) {
	var out []Token
	for i, c := range j.Chunks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !Covered(c, j.Quiet) {
			toks, err := j.chunk(ctx, c)
			if err != nil {
				return nil, err
			}
			out = append(out, toks...)
		}
		if j.Progress != nil {
			j.Progress(i+1, len(j.Chunks))
		}
	}
	return out, nil
}

func (j Job) chunk(ctx context.Context, c Span) ([]Token, error) {
	for k := 0; k <= j.Retries; k++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s := attempt(c, k, j.Shift)
		wav, err := j.Cut(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("subs: cut %v-%v: %w", s.Start, s.End, err)
		}
		toks, err := j.Tr.Transcribe(ctx, wav)
		if err != nil {
			return nil, fmt.Errorf("subs: transcribe %v-%v: %w", s.Start, s.End, err)
		}
		var kept []Token
		for _, t := range toks {
			t.Start, t.End = t.Start+s.Start, t.End+s.Start
			if t.Start >= c.Start && t.Start < c.End {
				kept = append(kept, t)
			}
		}
		if len(kept) > 0 {
			return kept, nil
		}
	}
	return nil, nil
}

// attempt is the span for the k-th try: the chunk padded earlier by k shifts,
// or trimmed later when that would start before the film.
func attempt(c Span, k int, shift time.Duration) Span {
	d := time.Duration(k) * shift
	if c.Start-d >= 0 {
		return Span{c.Start - d, c.End}
	}
	return Span{min(c.Start+d, c.End-1), c.End}
}

// Covered reports whether quiet spans together cover all of s.
func Covered(s Span, quiet []Span) bool {
	at := s.Start
	for changed := true; changed && at < s.End; {
		changed = false
		for _, q := range quiet {
			if q.Start <= at && q.End > at {
				at, changed = q.End, true
			}
		}
	}
	return at >= s.End
}
