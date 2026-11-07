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
	frameID   = 7  // the one kitty image slot; each frame replaces the last
	frameCols = 36 // width of the frame pane in cells
	maxPixels = 640
)

// FrameMsg carries the kitty escape that uploads the frame at Path. Data is
// empty when the frame could not be read.
type FrameMsg struct {
	Path string
	Data string
	Rows int
}

// Option configures the interface.
type Option func(*config)

type config struct{ bundle string }

// WithBundle shows each scene's frame from this bundle directory, in
// terminals that support kitty's unicode placeholders.
func WithBundle(dir string) Option { return func(c *config) { c.bundle = dir } }

// frame starts loading the selected hit's frame, if there is one to show.
func (m *searchScreen) frame() tea.Cmd {
	m.shown = ""
	if m.bundle == "" || m.sel >= len(m.hits) || m.hits[m.sel].Scene.Frame == "" {
		return nil
	}
	rel, dir := m.hits[m.sel].Scene.Frame, m.bundle
	m.want = rel
	return func() tea.Msg {
		data, rows := loadFrame(context.Background(), filepath.Join(dir, filepath.FromSlash(rel)))
		return FrameMsg{Path: rel, Data: data, Rows: rows}
	}
}

// onFrame sends a loaded frame to the terminal if it is still the selected one.
func (m *searchScreen) onFrame(msg FrameMsg) tea.Cmd {
	if msg.Path != m.want || msg.Data == "" {
		return nil
	}
	m.shown, m.rows = msg.Path, msg.Rows
	return tea.Raw(msg.Data)
}

// loadFrame decodes a jpeg, shrinks it, and returns the kitty upload for a
// virtual placement plus the rows it covers. Any failure returns "".
func loadFrame(ctx context.Context, path string) (string, int) {
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
		rows := max(1, frameCols*b.Dy()/b.Dx()/2)
		var buf bytes.Buffer
		err = kitty.EncodeGraphics(&buf, img, &kitty.Options{
			Action: kitty.TransmitAndPut, ID: frameID, Format: kitty.PNG, Transmission: kitty.Direct,
			VirtualPlacement: true, Columns: frameCols, Rows: rows, Quite: 2, Chunk: true,
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
func (m *searchScreen) framePane() string {
	var sb strings.Builder
	for r := range m.rows {
		fmt.Fprintf(&sb, "\x1b[38;5;%dm", frameID)
		for c := range frameCols {
			sb.WriteRune(kitty.Placeholder)
			sb.WriteRune(kitty.Diacritic(r))
			sb.WriteRune(kitty.Diacritic(c))
		}
		sb.WriteString("\x1b[39m\n")
	}
	return sb.String()
}
