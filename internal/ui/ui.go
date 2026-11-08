// Package ui is the terminal interface: a stack of screens, with overlays
// above them. The first screen is the search; see screen.go for the rest.
package ui

import (
	"cmp"
	"strings"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tjagithub/rosebud/internal/core"
)

// Model is the Bubble Tea root. It owns navigation and the window size and
// leaves everything else to its screens.
type Model struct {
	stack, overlays []Screen
	size            tea.WindowSizeMsg
	help            help.Model
}

// New returns the interface, opening on the search screen that shows up to
// limit hits per search.
func New(src Searcher, limit int, opts ...Option) *Model {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.lib == nil {
		return &Model{help: help.New(), stack: []Screen{newSearch(src, limit, cfg)}}
	}
	return &Model{help: help.New(), stack: []Screen{newLibrary(cfg.lib, searchOnce(newSearch(src, limit, cfg)), func(f core.Film) Screen { return newFilmScreen(src, f, cmp.Or(f.Bundle, cfg.bundle)) })}}
}

// Selected is the index of the highlighted hit on the search screen.
func (m *Model) Selected() int {
	for _, s := range m.stack {
		if s, ok := s.(*searchScreen); ok {
			return s.sel
		}
	}
	return 0
}

func (m *Model) Init() tea.Cmd { return m.stack[0].Init() }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.size = msg
		m.help.SetWidth(msg.Width)
		return m, m.broadcast(m.body())
	case PushMsg:
		m.stack = append(m.stack, msg.Screen)
		return m, tea.Batch(msg.Screen.Init(), m.sendSize(msg.Screen))
	case OpenMsg:
		m.overlays = append(m.overlays, msg.Overlay)
		return m, tea.Batch(msg.Overlay.Init(), m.sendSize(msg.Overlay))
	case PopMsg:
		m.pop()
		return m, nil
	case CloseMsg:
		m.closeOverlay()
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if c, ok := m.top().(Capturer); ok && c.Capturing() {
				return m, m.updateTop(msg)
			}
			switch {
			case len(m.overlays) > 0:
				m.closeOverlay()
			case len(m.stack) > 1:
				m.pop()
			default:
				return m, tea.Quit
			}
			return m, nil
		}
		return m, m.updateTop(msg)
	}
	return m, m.broadcast(msg)
}

func (m *Model) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m *Model) closeOverlay() {
	if n := len(m.overlays); n > 0 {
		m.overlays = m.overlays[:n-1]
	}
}

// updateTop gives msg to the overlay on top, or else to the top screen.
func (m *Model) updateTop(msg tea.Msg) tea.Cmd {
	list := m.stack
	if len(m.overlays) > 0 {
		list = m.overlays
	}
	var cmd tea.Cmd
	list[len(list)-1], cmd = list[len(list)-1].Update(msg)
	return cmd
}

// broadcast gives msg to every screen and overlay. Results of work a screen
// started may arrive while another is on top; each screen ignores what is not its own.
func (m *Model) broadcast(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for _, list := range [][]Screen{m.stack, m.overlays} {
		for i, s := range list {
			var c tea.Cmd
			list[i], c = s.Update(msg)
			cmds = append(cmds, c)
		}
	}
	return tea.Batch(cmds...)
}

// sendSize tells a newly opened screen how big the window is.
func (m *Model) sendSize(s Screen) tea.Cmd {
	if m.size.Width == 0 {
		return nil
	}
	_, c := s.Update(m.body())
	return c
}

func (m *Model) View() tea.View {
	out := fitHeight(m.stack[len(m.stack)-1].View(), m.body().Height)
	out += "\n"
	if km, ok := m.top().(help.KeyMap); ok {
		out += m.help.View(km)
	}
	out += "\n" + statusBar(m.size.Width, m.crumbs(), depthLabel(len(m.stack)+len(m.overlays)))
	for _, o := range m.overlays {
		out = over(out, o.View())
	}
	v := tea.NewView(out)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// over draws top, centred, above base; base shows around it.
func over(base, top string) string {
	x := max((lipgloss.Width(base)-lipgloss.Width(top))/2, 0)
	y := max((lipgloss.Height(base)-lipgloss.Height(top))/2, 0)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(top).X(x).Y(y).Z(1),
	).Render()
}

// searchOnce keeps one search screen, so coming back to it finds the query as left.
func searchOnce(s *searchScreen) func() Screen { return func() Screen { return s } }

// top is the overlay on top, or else the top screen.
func (m *Model) top() Screen {
	if n := len(m.overlays); n > 0 {
		return m.overlays[n-1]
	}
	return m.stack[len(m.stack)-1]
}

// body is the window size left for a screen once the help line and status bar are taken.
func (m *Model) body() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: m.size.Width, Height: max(m.size.Height-2, 1)}
}

// crumbs are the titles of the screens and overlays, shallowest first.
func (m *Model) crumbs() []string {
	var out []string
	for _, list := range [][]Screen{m.stack, m.overlays} {
		for _, s := range list {
			if t, ok := s.(Titler); ok {
				out = append(out, t.Title())
			}
		}
	}
	return out
}

// fitHeight cuts or pads s to exactly h lines, so the bars below stay put.
func fitHeight(s string, h int) string {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
