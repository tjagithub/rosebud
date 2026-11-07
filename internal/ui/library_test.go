package ui_test

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/ui"
	"github.com/tjagithub/rosebud/internal/ui/uitest"
)

type fakeLib struct {
	films  []core.Film
	scenes map[core.FilmID]int
	err    error
}

func (f fakeLib) Films(context.Context) iter.Seq2[core.Film, error] {
	return func(yield func(core.Film, error) bool) {
		if f.err != nil {
			yield(core.Film{}, f.err)
			return
		}
		for _, x := range f.films {
			if !yield(x, nil) {
				return
			}
		}
	}
}

func (f fakeLib) Scenes(_ context.Context, id core.FilmID) iter.Seq2[core.Scene, error] {
	return func(yield func(core.Scene, error) bool) {
		for range f.scenes[id] {
			if !yield(core.Scene{Film: id}, nil) {
				return
			}
		}
	}
}

func library(t *testing.T, lib fakeLib) (*uitest.TUI, *fakeSearch) {
	f := &fakeSearch{hits: []core.Hit{hit(1, time.Second, "found it")}}
	return uitest.New(t, ui.New(f, 10, ui.WithLibrary(lib))), f
}

func TestLibraryListsFilmsWithSceneCounts(t *testing.T) {
	tui, _ := library(t, fakeLib{
		films:  []core.Film{{ID: "a", Title: "Sonatine", Duration: 94 * time.Minute}, {ID: "b", Title: "Short"}},
		scenes: map[core.FilmID]int{"a": 294, "b": 1},
	})
	tui.Screen().Contains("Sonatine", "1:34:00", "294 scenes", "Short", "1 scene")
}

func TestLibrarySlashOpensSearchAndEscReturns(t *testing.T) {
	tui, f := library(t, fakeLib{films: []core.Film{{ID: "a", Title: "Sonatine"}}})
	tui.Type("s")
	tui.Screen().Contains("search:")
	tui.Type("x")
	if len(f.queries) == 0 {
		t.Error("typing on the search screen must search")
	}
	tui.Press("esc")
	tui.Screen().Contains("Sonatine")
	tui.Type("s")
	tui.Screen().Contains("found it")
}

func TestLibraryShowsErrorAndEmptyHint(t *testing.T) {
	tui, _ := library(t, fakeLib{err: errors.New("db gone")})
	tui.Screen().Contains("db gone")
	tui, _ = library(t, fakeLib{})
	tui.Screen().Contains("rose scenes")
}

func TestLibraryShowsKeyHelp(t *testing.T) {
	tui, _ := library(t, fakeLib{films: []core.Film{{ID: "a", Title: "Sonatine"}}})
	tui.Screen().Contains("s search", "/ filter films", "esc quit")
}

func TestLibraryFilterNarrowsFilmsAndEscClearsItBeforeQuitting(t *testing.T) {
	tui, _ := library(t, fakeLib{films: []core.Film{{ID: "a", Title: "Sonatine"}, {ID: "b", Title: "Hana-bi"}}})
	tui.Type("/")
	tui.Type("hana")
	tui.Screen().Contains("Hana-bi")
	tui.Screen().Lacks("Sonatine")
	tui.Press("esc")
	tui.Press("esc")
	tui.Screen().Contains("Sonatine", "Hana-bi")
	if _, cmd := tui.Model().Update(escKey()); cmd == nil {
		t.Fatal("esc on an unfiltered library must quit")
	}
}

func TestLibrarySKeyTypesIntoTheFilterInsteadOfOpeningSearch(t *testing.T) {
	tui, _ := library(t, fakeLib{films: []core.Film{{ID: "a", Title: "Sonatine"}}})
	tui.Type("/")
	tui.Type("s")
	tui.Screen().Lacks("search:")
}

func escKey() tea.Msg { return tea.KeyPressMsg{Code: tea.KeyEscape} }
