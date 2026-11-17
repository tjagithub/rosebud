package ui_test

import (
	"strings"
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

func TestOverlayIsDrawnOverTheScreenNotInsteadOfIt(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	tui.Send(ui.PushMsg{Screen: &note{text: "LEFT-EDGE" + strings.Repeat(" ", 40) + "RIGHT-EDGE\n\n\n\n\n"}})
	tui.Send(ui.OpenMsg{Overlay: &note{text: "POPUP"}})
	tui.Screen().Contains("POPUP", "LEFT-EDGE", "RIGHT-EDGE")
}

type titled struct {
	note
	title string
}

func (t *titled) Title() string { return t.title }

func TestStatusBarShowsWhereYouAreAndHowDeep(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	tui.Screen().Contains("search")
	tui.Send(ui.PushMsg{Screen: &titled{title: "Sonatine"}})
	tui.Send(ui.OpenMsg{Overlay: &titled{title: "scene 12"}})
	s := tui.Screen().String()
	i, j, k := strings.Index(s, "search"), strings.Index(s, "Sonatine"), strings.Index(s, "scene 12")
	if i < 0 || j < i || k < j {
		t.Errorf("breadcrumb out of order or missing:\n%s", s)
	}
	tui.Screen().Contains("search › Sonatine › scene 12")
}

func TestBarsStayOnTheLastLinesWhateverTheScreenHolds(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	last := func() (string, int) {
		lines := strings.Split(tui.Screen().String(), "\n")
		return lines[len(lines)-1], len(lines)
	}
	bar, n := last()
	tui.Send(ui.PushMsg{Screen: &titled{note: note{text: strings.Repeat("row\n", 100)}, title: "tall"}})
	bar2, n2 := last()
	if n != 24 || n2 != 24 || !strings.Contains(bar, "rosebud  search") || !strings.Contains(bar2, "search › tall") {
		t.Errorf("height %d then %d; bars %q then %q", n, n2, bar, bar2)
	}
}
