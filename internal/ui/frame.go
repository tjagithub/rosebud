package ui

import (
	"bytes"
	"cmp"
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
	ID         int // the pane that asked
	Path       string
	Data       string
	Cols, Rows int
}

// Option configures the interface.
type Option func(*config)

type config struct {
	bundle string
	lib    Library
	subs   Subtitles
}

// WithBundle shows each scene's frame from this bundle directory, in
// terminals that support kitty's unicode placeholders.
func WithBundle(dir string) Option { return func(c *config) { c.bundle = dir } }

// size is a width and height in cells.
type size struct{ cols, rows int }

// pane shows one frame image in a fixed slot of the terminal.
type pane struct {
	id          int
	box         size // the most cells a frame may take
	cols        int  // size of the frame on show
	maxPx       int  // pixels wide to send; 0 for maxPixels
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
	id, box, dir, px := p.id, p.box, p.bundle, p.maxPx
	return func() tea.Msg {
		data, cols, rows := loadFrame(context.Background(), filepath.Join(dir, filepath.FromSlash(rel)), id, box, px)
		return FrameMsg{ID: id, Path: rel, Data: data, Cols: cols, Rows: rows}
	}
}

// onFrame sends a loaded frame to the terminal if it is still the wanted one.
func (p *pane) onFrame(msg FrameMsg) tea.Cmd {
	if msg.ID != p.id || msg.Path != p.want || msg.Data == "" {
		return nil
	}
	p.shown, p.cols, p.rows = msg.Path, msg.Cols, msg.Rows
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
func loadFrame(ctx context.Context, path string, id int, box size, px int) (string, int, int) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	type result struct {
		data       string
		cols, rows int
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
		data, cols, rows := encodeImage(img, id, box, px)
		done <- result{data, cols, rows}
	}()
	select {
	case r := <-done:
		return r.data, r.cols, r.rows
	case <-ctx.Done():
		return "", 0, 0
	}
}

// encodeImage shrinks img and returns its kitty upload and the cells it covers.
// Any failure returns "".
func encodeImage(img image.Image, id int, box size, px int) (string, int, int) {
	img = shrink(img, cmp.Or(px, maxPixels))
	b := img.Bounds()
	cols, rows := fit(b.Dx(), b.Dy(), box)
	var buf bytes.Buffer
	err := kitty.EncodeGraphics(&buf, img, &kitty.Options{
		Action: kitty.TransmitAndPut, ID: id, Format: kitty.PNG, Transmission: kitty.Direct,
		VirtualPlacement: true, Columns: cols, Rows: rows, Quiet: 2, Chunk: true,
	})
	if err != nil {
		return "", 0, 0
	}
	return buf.String(), cols, rows
}

// fit is the largest size within box that keeps the proportions of a w by h
// image. A cell is about twice as tall as wide.
func fit(w, h int, box size) (cols, rows int) {
	cols = box.cols
	rows = max(1, cols*h/w/2)
	if box.rows > 0 && rows > box.rows {
		rows = box.rows
		cols = max(1, rows*2*w/h)
	}
	return cols, rows
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
