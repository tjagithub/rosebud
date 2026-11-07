// Package ui is the terminal interface: a stack of screens, with overlays
// above them. The first screen is the search; see screen.go for the rest.
package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Model is the Bubble Tea root. It owns navigation and the window size and
// leaves everything else to its screens.
type Model struct {
	stack, overlays []Screen
	size            tea.WindowSizeMsg
}

// New returns the interface, opening on the search screen that shows up to
// limit hits per search.
func New(src Searcher, limit int, opts ...Option) *Model {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	return &Model{stack: []Screen{newSearch(src, limit, cfg)}}
}

// Selected is the index of the highlighted hit on the search screen.
func (m *Model) Selected() int { return m.stack[0].(*searchScreen).sel }

func (m *Model) Init() tea.Cmd { return m.stack[0].Init() }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.size = msg
		return m, m.broadcast(msg)
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
	_, c := s.Update(m.size)
	return c
}

func (m *Model) View() tea.View {
	out := m.stack[len(m.stack)-1].View()
	if n := len(m.overlays); n > 0 {
		out = lipgloss.Place(m.size.Width, m.size.Height, lipgloss.Center, lipgloss.Center, m.overlays[n-1].View())
	}
	v := tea.NewView(out)
	v.AltScreen = true
	return v
}
