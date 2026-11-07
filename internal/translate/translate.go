// Package translate turns scene text into English so an English query can find
// a line said or shown in another language.
package translate

import (
	"context"

	"github.com/tjagithub/rosebud/internal/subs"
)

// Exec runs an external command and returns its standard output.
type Exec = subs.Exec

// Translator translates each line, keeping the order. An empty line stays empty.
type Translator interface {
	Translate(ctx context.Context, lines []string) ([]string, error)
}
