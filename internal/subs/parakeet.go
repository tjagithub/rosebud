package subs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	modelV3 = "mlx-community/parakeet-tdt-0.6b-v3"
	modelJA = "mlx-community/parakeet-tdt_ctc-0.6b-ja"
)

// v3 covers 25 European languages; Japanese needs its own model.
var v3 = strings.Fields("bg hr cs da nl en et fi fr de el hu it lv lt mt pl pt ro sk sl es sv ru uk")

// ModelFor names the Parakeet model that handles a language code.
func ModelFor(lang string) (string, error) {
	switch {
	case lang == "ja":
		return modelJA, nil
	case slices.Contains(v3, lang):
		return modelV3, nil
	}
	return "", fmt.Errorf("subs: no Parakeet model for language %q", lang)
}

// parakeetPackage is pinned so a new release cannot change transcripts unseen.
const parakeetPackage = "parakeet-mlx==0.7.0"

// Parakeet transcribes with parakeet-mlx, run through uvx.
type Parakeet struct {
	Exec  Exec
	Model string
}

// Transcribe returns the tokens of a WAV, timed from its start.
func (p Parakeet) Transcribe(ctx context.Context, wav string) ([]Token, error) {
	dir, err := os.MkdirTemp("", "rose-parakeet-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if _, err := p.Exec(ctx, "uvx", "--from", parakeetPackage, "parakeet-mlx", wav,
		"--model", p.Model, "--output-format", "json", "--output-dir", dir); err != nil {
		return nil, fmt.Errorf("subs: parakeet %s: %w", wav, err)
	}
	base := strings.TrimSuffix(filepath.Base(wav), filepath.Ext(wav))
	f, err := os.Open(filepath.Join(dir, base+".json"))
	if err != nil {
		return nil, fmt.Errorf("subs: parakeet %s wrote no result: %w", wav, err)
	}
	defer func() { _ = f.Close() }()
	return ParseParakeet(f)
}

// ParseParakeet reads parakeet-mlx's JSON into one token list. Tokens with
// negative or inverted times are dropped.
func ParseParakeet(r io.Reader) ([]Token, error) {
	var doc struct {
		Sentences []struct {
			Tokens []struct {
				Text       string
				Start, End float64
			}
		}
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("subs: parakeet json: %w", err)
	}
	var out []Token
	for _, s := range doc.Sentences {
		for _, t := range s.Tokens {
			if t.Start < 0 || t.End < t.Start {
				continue
			}
			out = append(out, Token{
				Text:  t.Text,
				Start: time.Duration(t.Start * float64(time.Second)).Round(time.Millisecond),
				End:   time.Duration(t.End * float64(time.Second)).Round(time.Millisecond),
			})
		}
	}
	return out, nil
}
