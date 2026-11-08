package ui_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/ui"
	"github.com/tjagithub/rosebud/internal/ui/uitest"
)

type fakeSearch struct {
	queries []string
	films   []core.FilmID
	hits    []core.Hit
	err     error
}

func (f *fakeSearch) Search(_ context.Context, q core.SearchQuery) ([]core.Hit, error) {
	f.queries = append(f.queries, q.Text)
	f.films = append(f.films, q.Film)
	return f.hits, f.err
}

func hit(id int, at time.Duration, tr string) core.Hit {
	return core.Hit{Scene: core.Scene{ID: core.SceneID(id), Start: at}, Translation: tr, Subtitle: "字幕"}
}

func start(t *testing.T, f *fakeSearch) (*uitest.TUI, *ui.Model) {
	m := ui.New(f, 10)
	return uitest.New(t, m), m
}

func TestTypingSearchesAndShowsHits(t *testing.T) {
	f := &fakeSearch{hits: []core.Hit{hit(1, 65*time.Second, "I'll kill you!"), hit(2, 3725*time.Second, "Dangerous.")}}
	tui, _ := start(t, f)
	tui.Type("ki")
	if len(f.queries) == 0 || f.queries[len(f.queries)-1] != "ki" {
		t.Errorf("queries = %v", f.queries)
	}
	tui.Screen().Contains("1:05", "I'll kill you!", "1:02:05", "Dangerous.")
}

func TestArrowKeysMoveSelectionWithinBounds(t *testing.T) {
	f := &fakeSearch{hits: []core.Hit{hit(1, time.Second, "a"), hit(2, 2*time.Second, "b")}}
	tui, m := start(t, f)
	tui.Type("x")
	tui.Press("up")
	if m.Selected() != 0 {
		t.Errorf("up at top = %d", m.Selected())
	}
	tui.Press("down")
	tui.Press("down")
	tui.Press("down")
	if m.Selected() != 1 {
		t.Errorf("down past end = %d", m.Selected())
	}
}

func TestStaleResultsAreDropped(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	tui.Type("a")
	tui.Send(ui.ResultMsg{Seq: 0, Hits: []core.Hit{hit(9, 0, "stale")}})
	tui.Screen().Lacks("stale")
}

func TestSearchErrorIsShown(t *testing.T) {
	tui, _ := start(t, &fakeSearch{err: errors.New("db gone")})
	tui.Type("a")
	tui.Screen().Contains("db gone")
}

func TestEscQuits(t *testing.T) {
	_, cmd := ui.New(&fakeSearch{}, 10).Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("esc must quit")
	}
}

func TestDetailShowsOriginalBesideTranslation(t *testing.T) {
	f := &fakeSearch{hits: []core.Hit{hit(1, time.Second, "I'll kill you!")}}
	tui, _ := start(t, f)
	tui.Type("x")
	for _, line := range strings.Split(tui.Screen().String(), "\n") {
		if strings.Contains(line, "字幕") && strings.Contains(line, "I'll kill you!") {
			if strings.Index(line, "字幕") > strings.Index(line, "I'll kill you!") {
				t.Errorf("original must be on the left: %q", line)
			}
			return
		}
	}
	t.Errorf("no line holds both columns:\n%s", tui.Screen())
}
