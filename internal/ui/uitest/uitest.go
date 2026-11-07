// Package uitest drives a Bubble Tea model in tests and reads its screen.
package uitest

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TUI wraps a model; each call sends input and runs the commands it returns.
type TUI struct {
	t *testing.T
	m tea.Model
}

// New starts m at 80x24.
func New(t *testing.T, m tea.Model) *TUI {
	t.Helper()
	u := &TUI{t: t, m: m}
	u.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	return u
}

// Model is the current model, for asserting on its state.
func (u *TUI) Model() tea.Model { return u.m }

// Type sends each rune of s as a key press.
func (u *TUI) Type(s string) {
	u.t.Helper()
	for _, r := range s {
		u.Send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

var named = map[string]rune{
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab,
}

// Press sends a named key: up, down, left, right, enter, esc or tab.
func (u *TUI) Press(name string) {
	u.t.Helper()
	code, ok := named[name]
	if !ok {
		u.t.Fatalf("uitest: unknown key %q", name)
	}
	u.Send(tea.KeyPressMsg{Code: code})
}

// Click sends a left click at the cell (x, y).
func (u *TUI) Click(x, y int) {
	u.t.Helper()
	u.Send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
}

// Send delivers msg, then each command's message in turn. Batches are
// expanded, a quit ends the run, and a command that blocks is the model's bug.
func (u *TUI) Send(msg tea.Msg) {
	u.t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		var cmd tea.Cmd
		u.m, cmd = u.m.Update(queue[0])
		queue = queue[1:]
		if cmd == nil {
			continue
		}
		switch out := cmd().(type) {
		case nil, tea.QuitMsg:
		case tea.BatchMsg:
			for _, c := range out {
				if c != nil {
					if m := c(); m != nil {
						queue = append(queue, m)
					}
				}
			}
		default:
			queue = append(queue, out)
		}
	}
}

// Screen is the rendered text.
func (u *TUI) Screen() Screen { return Screen{t: u.t, text: u.m.View().Content} }

// Screen is a snapshot of what the user sees.
type Screen struct {
	t    *testing.T
	text string
}

// String is the screen text.
func (s Screen) String() string { return s.text }

// Contains fails the test unless the screen shows every part.
func (s Screen) Contains(parts ...string) {
	s.t.Helper()
	for _, p := range parts {
		if !strings.Contains(s.text, p) {
			s.t.Errorf("screen lacks %q:\n%s", p, s.text)
		}
	}
}

// Lacks fails the test if the screen shows any part.
func (s Screen) Lacks(parts ...string) {
	s.t.Helper()
	for _, p := range parts {
		if strings.Contains(s.text, p) {
			s.t.Errorf("screen shows %q:\n%s", p, s.text)
		}
	}
}
