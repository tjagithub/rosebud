package translate

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/limit"
)

//go:embed plamo.py
var plamoScript []byte

// plamoDeps pins what the model's remote code ran with in the spike.
var plamoDeps = []string{"mlx-lm==0.32.2", "transformers==5.22.2", "numba==0.68.1"}

// PlamoRevision is the audited revision of the model's remote code.
const PlamoRevision = "e5fe5b7f26ebaeae48db0ce644b932d1d9b91aaa"

// Plamo translates with PLaMo-3 through one uv-run Python process. Translate
// gives up after Load + PerLine for each line.
type Plamo struct {
	Exec     Exec
	Model    string // Hugging Face repo, e.g. pfnet/plamo-3-translate-0.8b-MLX-4bit
	Revision string // the commit whose code may run
	Source   string // language code of the lines, such as "ja"
	Load     time.Duration
	PerLine  time.Duration
}

// Translate runs the model over lines. Empty lines are not sent.
func (p Plamo) Translate(ctx context.Context, lines []string) ([]string, error) {
	if p.Exec == nil || p.Model == "" || p.Revision == "" || p.Source == "" || p.Load <= 0 || p.PerLine <= 0 {
		return nil, fmt.Errorf("translate: plamo: bad config (exec set %t, model %q, revision %q, source %q, load %v, per line %v)",
			p.Exec != nil, p.Model, p.Revision, p.Source, p.Load, p.PerLine)
	}
	out := make([]string, len(lines))
	var send []int
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			send = append(send, i)
		}
	}
	if len(send) == 0 {
		return out, nil
	}
	dir, err := os.MkdirTemp("", "rose-translate-")
	if err != nil {
		return nil, fmt.Errorf("translate: plamo: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	in, script := filepath.Join(dir, "in.jsonl"), filepath.Join(dir, "plamo.py")
	if err := writeInput(in, lines, send); err != nil {
		return nil, err
	}
	if err := os.WriteFile(script, plamoScript, 0o600); err != nil {
		return nil, fmt.Errorf("translate: plamo: %w", err)
	}
	args := []string{"run", "--python", "3.12"}
	for _, dep := range plamoDeps {
		args = append(args, "--with", dep)
	}
	args = append(args, "python", script, in, p.Model, p.Revision, p.Source)
	budget := p.Load + time.Duration(len(send))*p.PerLine
	raw, err := limit.Run(ctx, path.Base(p.Model), budget, func(ctx context.Context) ([]byte, error) {
		return p.Exec(ctx, "uv", args...)
	})
	if err != nil {
		return nil, fmt.Errorf("translate: plamo: %w", err)
	}
	return parsePlamo(raw, lines, send)
}

func writeInput(file string, lines []string, send []int) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, i := range send {
		if err := enc.Encode(map[string]any{"i": i, "text": lines[i]}); err != nil {
			return fmt.Errorf("translate: plamo: %w", err)
		}
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("translate: plamo: %w", err)
	}
	return nil
}

// parsePlamo reads the script's JSON lines. Lines that are not JSON objects
// are progress noise and are skipped; every sent line must have a result.
func parsePlamo(raw []byte, lines []string, send []int) ([]string, error) {
	want := make(map[int]bool, len(send))
	for _, i := range send {
		want[i] = true
	}
	out := make([]string, len(lines))
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(nil, 16<<20)
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(ln, "{") {
			continue
		}
		var m struct {
			I     int
			Text  string
			Error string
		}
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			return nil, fmt.Errorf("translate: plamo: bad output line %.60q: %w", ln, err)
		}
		switch {
		case !want[m.I]:
			return nil, fmt.Errorf("translate: plamo: output for unknown line %d", m.I)
		case m.Error != "":
			return nil, fmt.Errorf("translate: plamo: line %d: %s", m.I, m.Error)
		}
		out[m.I] = m.Text
		delete(want, m.I)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("translate: plamo: read output: %w", err)
	}
	for _, i := range send {
		if want[i] {
			return nil, fmt.Errorf("translate: plamo: no output for line %d", i)
		}
	}
	return out, nil
}

// CacheKey names the model, its pinned code and the script that drives it, so
// a change to any of them misses every cached line.
func (p Plamo) CacheKey() string {
	h := sha256.New()
	_, _ = h.Write([]byte(p.Model + "\x00" + p.Revision + "\x00" + strings.Join(plamoDeps, "\x00") + "\x00" + p.Source + "\x00"))
	_, _ = h.Write(plamoScript)
	return "plamo:" + hex.EncodeToString(h.Sum(nil))[:16]
}
