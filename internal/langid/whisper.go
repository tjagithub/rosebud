package langid

import (
	"bytes"
	"context"
	_ "embed" // whisper.py
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/tjagithub/rosebud/internal/limit"
)

// Exec runs a program and returns its stdout.
type Exec func(ctx context.Context, name string, args ...string) ([]byte, error)

//go:embed whisper.py
var whisperScript []byte

// whisperDeps are what whisper.py imports.
var whisperDeps = []string{"torch", "transformers", "numpy"}

// Whisper reads the language from a Whisper model's first decoder step,
// through one uv-run Python process. It gives up after Load + PerWindow each.
type Whisper struct {
	Exec      Exec
	Model     string // Hugging Face repo, e.g. openai/whisper-tiny
	Load      time.Duration
	PerWindow time.Duration
}

// Windows scores the WAVs.
func (w Whisper) Windows(ctx context.Context, wavs []string) ([]Probs, error) {
	if w.Exec == nil || w.Model == "" || w.Load <= 0 || w.PerWindow <= 0 {
		return nil, fmt.Errorf("whisper: bad config (exec set %t, model %q, load %v, per window %v)",
			w.Exec != nil, w.Model, w.Load, w.PerWindow)
	}
	dir, err := os.MkdirTemp("", "rose-langid-")
	if err != nil {
		return nil, fmt.Errorf("whisper: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	script := filepath.Join(dir, "whisper.py")
	if err := os.WriteFile(script, whisperScript, 0o600); err != nil {
		return nil, fmt.Errorf("whisper: %w", err)
	}
	args := []string{"run", "--python", "3.12"}
	for _, dep := range whisperDeps {
		args = append(args, "--with", dep)
	}
	args = append(append(args, "python", script, w.Model), wavs...)
	budget := w.Load + time.Duration(len(wavs))*w.PerWindow
	out, err := limit.Run(ctx, path.Base(w.Model), budget, func(ctx context.Context) ([]byte, error) {
		return w.Exec(ctx, "uv", args...)
	})
	if err != nil {
		return nil, fmt.Errorf("whisper: %w", err)
	}
	// The result is the last line; libraries may print before it.
	out = bytes.TrimSpace(out)
	if i := bytes.LastIndexByte(out, '\n'); i >= 0 {
		out = out[i+1:]
	}
	var got []Probs
	if err := json.Unmarshal(out, &got); err != nil {
		return nil, fmt.Errorf("whisper: unreadable result: %w", err)
	}
	if len(got) != len(wavs) {
		return nil, fmt.Errorf("whisper: %d results for %d windows", len(got), len(wavs))
	}
	return got, nil
}
