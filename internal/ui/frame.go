package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg" // frames are jpeg
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi/kitty"
)

const (
	frameID   = 7  // kitty image slot of the search screen
	frameCols = 36 // width of its frame pane in cells
	maxPixels = 640
)

// FrameMsg carries the kitty escape that uploads the frame at Path. Data is
// empty when the frame could not be read.
type FrameMsg struct {
	ID   int // the pane that asked
	Path string
	Data string
	Rows int
}

// Option configures the interface.
type Option func(*config)

type config struct {
	bundle string
	lib    Library
}

// WithBundle shows each scene's frame from this bundle directory, in
// terminals that support kitty's unicode placeholders.
func WithBundle(dir string) Option { return func(c *config) { c.bundle = dir } }

// pane shows one frame image in a fixed slot of the terminal.
type pane struct {
	id, cols    int
	bundle      string
	want, shown string // frame path being loaded, and the one on screen
	rows        int
}

// load starts loading the frame at rel, a bundle-relative path; "" clears the pane.
func (p *pane) load(rel string) tea.Cmd {
	p.shown = ""
	if p.bundle == "" || rel == "" {
		p.want = ""
		return nil
	}
	p.want = rel
	id, cols, dir := p.id, p.cols, p.bundle
	return func() tea.Msg {
		data, rows := loadFrame(context.Background(), filepath.Join(dir, filepath.FromSlash(rel)), id, cols)
		return FrameMsg{ID: id, Path: rel, Data: data, Rows: rows}
	}
}

// onFrame sends a loaded frame to the terminal if it is still the wanted one.
func (p *pane) onFrame(msg FrameMsg) tea.Cmd {
	if msg.ID != p.id || msg.Path != p.want || msg.Data == "" {
		return nil
	}
	p.shown, p.rows = msg.Path, msg.Rows
	return tea.Raw(msg.Data)
}

// frame loads the frame of the selected hit, if there is one.
func (m *searchScreen) frame() tea.Cmd {
	if m.sel >= len(m.hits) {
		return m.fr.load("")
	}
	return m.fr.load(m.hits[m.sel].Scene.Frame)
}

// loadFrame decodes a jpeg, shrinks it, and returns the kitty upload for a
// virtual placement plus the rows it covers. Any failure returns "".
func loadFrame(ctx context.Context, path string, id, cols int) (string, int) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	type result struct {
		data string
		rows int
	}
	done := make(chan result, 1)
	go func() {
		f, err := os.Open(path)
		if err != nil {
			done <- result{}
			return
		}
		defer func() { _ = f.Close() }()
		img, _, err := image.Decode(f)
		if err != nil {
			done <- result{}
			return
		}
		img = shrink(img, maxPixels)
		b := img.Bounds()
		// A cell is about twice as tall as wide.
		rows := max(1, cols*b.Dy()/b.Dx()/2)
		var buf bytes.Buffer
		err = kitty.EncodeGraphics(&buf, img, &kitty.Options{
			Action: kitty.TransmitAndPut, ID: id, Format: kitty.PNG, Transmission: kitty.Direct,
			VirtualPlacement: true, Columns: cols, Rows: rows, Quite: 2, Chunk: true,
		})
		if err != nil {
			done <- result{}
			return
		}
		done <- result{buf.String(), rows}
	}()
	select {
	case r := <-done:
		return r.data, r.rows
	case <-ctx.Done():
		return "", 0
	}
}

// shrink scales img down to at most max pixels wide, nearest neighbour.
func shrink(img image.Image, max int) image.Image {
	b := img.Bounds()
	if b.Dx() <= max {
		return img
	}
	h := b.Dy() * max / b.Dx()
	out := image.NewRGBA(image.Rect(0, 0, max, h))
	for y := range h {
		for x := range max {
			out.Set(x, y, img.At(b.Min.X+x*b.Dx()/max, b.Min.Y+y*b.Dy()/h))
		}
	}
	return out
}

// framePane is the placeholder cells that the terminal paints the frame into.
func (p *pane) view() string {
	var sb strings.Builder
	for r := range p.rows {
		fmt.Fprintf(&sb, "\x1b[38;5;%dm", p.id)
		for c := range p.cols {
			sb.WriteRune(kitty.Placeholder)
			sb.WriteRune(kitty.Diacritic(r))
			sb.WriteRune(kitty.Diacritic(c))
		}
		sb.WriteString("\x1b[39m\n")
	}
	return sb.String()
}
