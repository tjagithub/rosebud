// Package subs turns a film's speech into text per scene. It splits audio into
// independent chunks, transcribes each one, and assigns the words to scenes.
package subs

import (
	"context"
	"time"
)

// Span is a half-open time range [Start, End).
type Span struct{ Start, End time.Duration }

// Token is a piece of transcribed text with its timing. Text may hold a
// separator such as a space; join tokens without adding anything between them.
type Token struct {
	Text       string
	Start, End time.Duration
}

// Transcriber turns one audio file into tokens timed from the start of that
// file. It is the seam where a speech model plugs in.
type Transcriber interface {
	Transcribe(ctx context.Context, wav string) ([]Token, error)
}
