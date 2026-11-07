package ocr

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/tjagithub/rosebud/internal/limit"
)

//go:embed nemotron.py
var nemotronScript []byte

// nemotronDeps pins what the model's remote code needs, audited in the spike.
// mlx-vlm comes from the unmerged Nemotron-Parse port, at a fixed commit.
var nemotronDeps = []string{
	"torch", "torchvision", "timm==1.0.22", "open_clip_torch==3.2.0", "einops", "beautifulsoup4", "setuptools",
	"mlx-vlm @ git+https://github.com/axiom-of-choice/mlx-vlm@e44ee362ceb2298e6b154d8b387a1d27ff2fb121",
}

// Nemotron reads pages with Nemotron-Parse through one uv-run Python process.
// Read gives up after Load + PerPage for each page.
type Nemotron struct {
	Exec    Exec
	Model   string // Hugging Face repo, e.g. mlx-community/Nemotron-Parse-2.0-8bit
	Load    time.Duration
	PerPage time.Duration
}

// Read runs the model over pages and returns the blocks found on each.
func (n Nemotron) Read(ctx context.Context, pages []image.Image) ([][]Block, error) {
	if n.Exec == nil || n.Model == "" || n.Load <= 0 || n.PerPage <= 0 {
		return nil, fmt.Errorf("ocr: nemotron: bad config (exec set %t, model %q, load %v, per page %v)",
			n.Exec != nil, n.Model, n.Load, n.PerPage)
	}
	if len(pages) == 0 {
		return nil, nil
	}
	dir, err := os.MkdirTemp("", "rose-ocr-")
	if err != nil {
		return nil, fmt.Errorf("ocr: nemotron: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	pagesDir := filepath.Join(dir, "pages")
	if err := writePages(pagesDir, pages); err != nil {
		return nil, err
	}
	script := filepath.Join(dir, "nemotron.py")
	if err := os.WriteFile(script, nemotronScript, 0o600); err != nil {
		return nil, fmt.Errorf("ocr: nemotron: %w", err)
	}
	args := []string{"run", "--python", "3.12"}
	for _, dep := range nemotronDeps {
		args = append(args, "--with", dep)
	}
	args = append(args, "python", script, pagesDir, n.Model)
	budget := n.Load + time.Duration(len(pages))*n.PerPage
	out, err := limit.Run(ctx, path.Base(n.Model), budget, func(ctx context.Context) ([]byte, error) {
		return n.Exec(ctx, "uv", args...)
	})
	if err != nil {
		return nil, fmt.Errorf("ocr: nemotron: %w", err)
	}
	return parseNemotron(out, len(pages))
}

func pageName(i int) string { return fmt.Sprintf("p%04d.png", i) }

func writePages(dir string, pages []image.Image) error {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return fmt.Errorf("ocr: nemotron: %w", err)
	}
	for i, p := range pages {
		f, err := os.Create(filepath.Join(dir, pageName(i)))
		if err != nil {
			return fmt.Errorf("ocr: nemotron: %w", err)
		}
		err = png.Encode(f, p)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("ocr: nemotron: write page %d: %w", i, err)
		}
	}
	return nil
}

// parseNemotron reads the script's JSON lines, matching them to pages by
// name. Lines that are not JSON objects are progress noise and are skipped.
func parseNemotron(out []byte, n int) ([][]Block, error) {
	index := make(map[string]int, n)
	for i := range n {
		index[pageName(i)] = i
	}
	texts := make(map[string]string, n)
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(nil, 16<<20)
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(ln, "{") {
			continue
		}
		var m struct{ Page, Text, Error string }
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			return nil, fmt.Errorf("ocr: nemotron: bad output line %.60q: %w", ln, err)
		}
		i, known := index[m.Page]
		switch {
		case !known:
			return nil, fmt.Errorf("ocr: nemotron: output for unknown page %s", m.Page)
		case m.Error != "":
			return nil, fmt.Errorf("ocr: nemotron: page %d: %s", i, m.Error)
		}
		texts[m.Page] = m.Text
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("ocr: nemotron: read output: %w", err)
	}
	blocks := make([][]Block, n)
	for i := range blocks {
		text, ok := texts[pageName(i)]
		if !ok {
			return nil, fmt.Errorf("ocr: nemotron: no output for page %d", i)
		}
		blocks[i] = ParseBlocks(text)
	}
	return blocks, nil
}

// CacheKey names the model and the code that drives it, so a change to
// either misses every cached page.
func (n Nemotron) CacheKey() string {
	h := sha256.New()
	_, _ = h.Write([]byte(n.Model + "\x00" + strings.Join(nemotronDeps, "\x00") + "\x00"))
	_, _ = h.Write(nemotronScript)
	return "nemotron:" + hex.EncodeToString(h.Sum(nil))[:16]
}
