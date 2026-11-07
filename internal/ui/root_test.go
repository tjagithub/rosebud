package ui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tjagithub/rosebud/internal/ui"
)

type ping struct{}

// note is a screen that shows its text and records what it was sent.
type note struct {
	text  string
	keys  []string
	pings int
	width int
}

func (n *note) Init() tea.Cmd { return nil }
func (n *note) View() string  { return n.text }
func (n *note) Update(msg tea.Msg) (ui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		n.keys = append(n.keys, msg.String())
	case tea.WindowSizeMsg:
		n.width = msg.Width
	case ping:
		n.pings++
	}
	return n, nil
}

func TestPushShowsScreenAndEscComesBack(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	n := &note{text: "NOTE SCREEN"}
	tui.Send(ui.PushMsg{Screen: n})
	tui.Screen().Contains("NOTE SCREEN")
	if n.width != 80 {
		t.Errorf("pushed screen got width %d, want 80", n.width)
	}
	tui.Press("esc")
	tui.Screen().Lacks("NOTE SCREEN")
	tui.Screen().Contains("search:")
}

func TestOverlayTakesKeysAndEscClosesItFirst(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	under := &note{text: "UNDER"}
	over := &note{text: "OVER"}
	tui.Send(ui.PushMsg{Screen: under})
	tui.Send(ui.OpenMsg{Overlay: over})
	tui.Screen().Contains("OVER")
	tui.Type("x")
	if len(over.keys) != 1 || len(under.keys) != 0 {
		t.Errorf("keys: overlay %v, screen %v", over.keys, under.keys)
	}
	tui.Press("esc")
	tui.Screen().Lacks("OVER")
	tui.Screen().Contains("UNDER")
}

func TestResultsReachScreensBelowTheTop(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	below := &note{text: "B"}
	tui.Send(ui.PushMsg{Screen: below})
	tui.Send(ui.PushMsg{Screen: &note{text: "TOP"}})
	tui.Send(ping{})
	if below.pings != 1 {
		t.Errorf("screen below got %d pings", below.pings)
	}
}

func TestEscOnTheFirstScreenQuitsAndCtrlCQuitsAnywhere(t *testing.T) {
	m := ui.New(&fakeSearch{}, 10)
	_, cmd := m.Update(ui.PushMsg{Screen: &note{}})
	_ = cmd
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd == nil {
		t.Fatal("ctrl+c gave no command")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c must quit from a pushed screen")
	}
}

func TestPopNeverRemovesTheFirstScreen(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	tui.Send(ui.PopMsg{})
	tui.Send(ui.CloseMsg{})
	tui.Screen().Contains("search:")
}
