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

//go:embed rikai.py
var rikaiScript []byte

// Backend is a way of running a translation model: a script and what it needs.
type Backend struct {
	Name     string
	Script   []byte
	Deps     []string // pinned, as `uv run --with` takes them
	Model    string   // default Hugging Face repo
	Revision string   // audited commit of Model
}

// Backends are the supported ways to translate. Plamo runs the model's remote code.
var Backends = map[string]Backend{
	"rikai": {
		"rikai", rikaiScript,
		[]string{"mlx-lm==0.32.2"},
		"rikai-translate/rikai-jp-eng-Bench-265-0.8B", "ec42fcdcd4d3b1f3465b612d5c07ce040ece6d90",
	},
	"plamo": {
		"plamo", plamoScript,
		[]string{"mlx-lm==0.32.2", "transformers==5.22.2", "numba==0.68.1"},
		"pfnet/plamo-3-translate-0.8b-MLX-4bit", "e5fe5b7f26ebaeae48db0ce644b932d1d9b91aaa",
	},
}

// Local translates with a Backend through one uv-run Python process. Translate
// gives up after Load + PerLine for each line.
type Local struct {
	Exec     Exec
	Backend  Backend
	Model    string // Hugging Face repo
	Revision string // the commit that may be downloaded and run
	Source   string // language code of the lines, such as "ja"
	Load     time.Duration
	PerLine  time.Duration
}

// Translate runs the model over lines. Empty lines are not sent.
func (p Local) Translate(ctx context.Context, lines []string) ([]string, error) {
	if p.Exec == nil || len(p.Backend.Script) == 0 || p.Model == "" || p.Revision == "" || p.Source == "" || p.Load <= 0 || p.PerLine <= 0 {
		return nil, fmt.Errorf("translate: : bad config (exec set %t, backend %q, model %q, revision %q, source %q, load %v, per line %v)",
			p.Exec != nil, p.Backend.Name, p.Model, p.Revision, p.Source, p.Load, p.PerLine)
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
		return nil, fmt.Errorf("translate: : %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	in, script := filepath.Join(dir, "in.jsonl"), filepath.Join(dir, p.Backend.Name+".py")
	if err := writeInput(in, lines, send); err != nil {
		return nil, err
	}
	if err := os.WriteFile(script, p.Backend.Script, 0o600); err != nil {
		return nil, fmt.Errorf("translate: : %w", err)
	}
	args := []string{"run", "--python", "3.12"}
	for _, dep := range p.Backend.Deps {
		args = append(args, "--with", dep)
	}
	args = append(args, "python", script, in, p.Model, p.Revision, p.Source)
	budget := p.Load + time.Duration(len(send))*p.PerLine
	raw, err := limit.Run(ctx, path.Base(p.Model), budget, func(ctx context.Context) ([]byte, error) {
		return p.Exec(ctx, "uv", args...)
	})
	if err != nil {
		return nil, fmt.Errorf("translate: : %w", err)
	}
	return parseOutput(raw, lines, send)
}

func writeInput(file string, lines []string, send []int) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, i := range send {
		if err := enc.Encode(map[string]any{"i": i, "text": lines[i]}); err != nil {
			return fmt.Errorf("translate: : %w", err)
		}
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("translate: : %w", err)
	}
	return nil
}

// parseOutput reads the script's JSON lines. Lines that are not JSON objects
// are progress noise and are skipped; every sent line must have a result.
func parseOutput(raw []byte, lines []string, send []int) ([]string, error) {
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
			return nil, fmt.Errorf("translate: : bad output line %.60q: %w", ln, err)
		}
		switch {
		case !want[m.I]:
			return nil, fmt.Errorf("translate: : output for unknown line %d", m.I)
		case m.Error != "":
			return nil, fmt.Errorf("translate: : line %d: %s", m.I, m.Error)
		}
		out[m.I] = m.Text
		delete(want, m.I)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("translate: : read output: %w", err)
	}
	for _, i := range send {
		if want[i] {
			return nil, fmt.Errorf("translate: : no output for line %d", i)
		}
	}
	return out, nil
}

// CacheKey names the model, its pinned code and the script that drives it, so
// a change to any of them misses every cached line.
func (p Local) CacheKey() string {
	h := sha256.New()
	_, _ = h.Write([]byte(p.Model + "\x00" + p.Revision + "\x00" + strings.Join(p.Backend.Deps, "\x00") + "\x00" + p.Source + "\x00"))
	_, _ = h.Write(p.Backend.Script)
	return p.Backend.Name + ":" + hex.EncodeToString(h.Sum(nil))[:16]
}
