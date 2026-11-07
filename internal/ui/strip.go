package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tjagithub/rosebud/internal/core"
)

const (
	thumbID    = 20 // kitty image slots of the strip, thumbID .. thumbID+thumbN-1
	thumbN     = 5
	thumbRows  = 3
	stripRows  = thumbRows + 2 // with its border
	thumbPixel = 160           // thumbnails are sent this many pixels wide
)

// strip is a row of thumbnails around the current scene, with arrows where
// there are more scenes to either side.
type strip struct {
	panes [thumbN]pane
	cols  int // width of one thumbnail
}

func newStrip(bundle string) strip {
	var s strip
	for k := range s.panes {
		s.panes[k] = pane{id: thumbID + k, bundle: bundle, maxPx: thumbPixel}
	}
	return s
}

// resize sizes the thumbnails to fit a window w cells wide.
func (s *strip) resize(w int) {
	s.cols = min(max((w-6)/thumbN-2, 4), 12)
	for k := range s.panes {
		s.panes[k].box = size{cols: s.cols, rows: thumbRows}
	}
}

// load starts loading the frames of the scenes around position pos in idx.
func (s *strip) load(hits []core.Hit, idx []int, pos int) tea.Cmd {
	var cmds []tea.Cmd
	for k := range s.panes {
		rel := ""
		if p := pos - thumbN/2 + k; p >= 0 && p < len(idx) {
			rel = hits[idx[p]].Scene.Frame
		}
		cmds = append(cmds, s.panes[k].load(rel))
	}
	return tea.Batch(cmds...)
}

func (s *strip) onFrame(msg FrameMsg) tea.Cmd {
	var cmds []tea.Cmd
	for k := range s.panes {
		cmds = append(cmds, s.panes[k].onFrame(msg))
	}
	return tea.Batch(cmds...)
}

// view draws the boxed strip w cells wide; pos is the current scene's place
// among n, and the current thumbnail sits between highlighted gutters.
func (s *strip) view(w, pos, n int) string {
	var cells []string
	for k := range s.panes {
		p := pos - thumbN/2 + k
		cells = append(cells, s.cell(k, k == thumbN/2, p >= 0 && p < n))
	}
	left, right := arrow(pos > 0, "<"), arrow(pos < n-1, ">")
	row := lipgloss.JoinHorizontal(lipgloss.Top, append(append([]string{left}, cells...), right)...)
	row = lipgloss.PlaceHorizontal(w-2, lipgloss.Center, row)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w).Render(row)
}

// arrow is a one-cell column, vertically centred.
func arrow(on bool, glyph string) string {
	if !on {
		glyph = " "
	}
	return lipgloss.Place(1, thumbRows, lipgloss.Left, lipgloss.Center, glyph)
}

// cell is thumbnail k with a gutter either side, highlighted for the current one.
func (s *strip) cell(k int, current, real bool) string {
	img := ""
	if p := s.panes[k]; real && p.shown != "" {
		img = strings.TrimSuffix(p.view(), "\n")
	}
	gutter := strings.TrimSuffix(strings.Repeat(" \n", thumbRows), "\n")
	if current {
		gutter = strings.TrimSuffix(strings.Repeat(selected.Render(" ")+"\n", thumbRows), "\n")
	}
	body := lipgloss.Place(s.cols, thumbRows, lipgloss.Center, lipgloss.Center, img)
	return lipgloss.JoinHorizontal(lipgloss.Top, gutter, body, gutter)
}
