package ui_test

import (
	"context"
	"errors"
	"iter"
	"slices"
	"strings"
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
	tui.Screen().Contains("Sonatine", "1:34:00", "294 scenes", "Short", "1 scene", "2 films")
	tui.Screen().Contains("–") // Short has no known length
}

func TestLibraryHasABannerOverTheFilmsAndStatsUnder(t *testing.T) {
	tui, _ := library(t, fakeLib{
		films:  []core.Film{{ID: "a", Title: "Sonatine", Duration: 94 * time.Minute}, {ID: "b", Title: "Short", Duration: time.Minute}},
		scenes: map[core.FilmID]int{"a": 294, "b": 1},
	})
	s := tui.Screen()
	s.Contains("R O S E B U D", "search all", "stats", "295 scenes", "1:35:00")
	lines := strings.Split(s.String(), "\n")
	at := func(part string) int {
		return slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, part) })
	}
	logo, menu, films, stats := at("`,'^"), at("search all"), at("Sonatine"), at("─ stats ")
	if logo < 0 || menu < 0 || menu > logo+3 || films < logo || stats < films {
		t.Errorf("want logo beside the menu on top, then films, then stats:\n%s", s)
	}
}

func TestSceneScreenNamesTheFilmInTheStatusBar(t *testing.T) {
	tui := filmWithMedia(t)
	tui.Screen().Contains("films › scene 1", "Sonatine")
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
	tui.Screen().Contains("No films yet", "rose build FILE")
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

func TestLibraryEnterOpensTheFilmsScenes(t *testing.T) {
	tui, f := library(t, fakeLib{
		films:  []core.Film{{ID: "a", Title: "Sonatine"}},
		scenes: map[core.FilmID]int{"a": 1},
	})
	tui.Press("enter")
	tui.Screen().Contains("films › scene 1", "found it")
	if len(f.films) != 1 || f.films[0] != "a" || f.queries[0] != "" {
		t.Errorf("must list film a without text, got films %v queries %q", f.films, f.queries)
	}
	tui.Press("esc")
	tui.Screen().Contains("Sonatine")
}

func TestLibraryMenuTabsFocusAndRunsItems(t *testing.T) {
	tui, _ := library(t, fakeLib{films: []core.Film{{ID: "a", Title: "Sonatine"}}, scenes: map[core.FilmID]int{"a": 1}})
	tui.Screen().Contains("films", "search all", "add media", "quit", "Sonatine")
	tui.Press("tab")
	tui.Press("down")
	tui.Press("down")
	tui.Press("enter")
	tui.Screen().Contains("rose build FILE")
	tui.Press("esc")
	tui.Screen().Lacks("rose build FILE")
	tui.Press("up")
	tui.Press("enter")
	tui.Screen().Contains("search:")
}

func TestLibraryMenuFilmsReturnsFocusToTheList(t *testing.T) {
	tui, _ := library(t, fakeLib{films: []core.Film{{ID: "a", Title: "Sonatine"}}, scenes: map[core.FilmID]int{"a": 1}})
	tui.Press("tab")
	tui.Press("enter")
	tui.Press("enter")
	tui.Screen().Contains("scene 1")
}
