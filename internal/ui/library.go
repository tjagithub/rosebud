package ui

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tjagithub/rosebud/internal/core"
)

// Library is the part of core.Client the film list needs.
type Library interface {
	Films(ctx context.Context) iter.Seq2[core.Film, error]
	Scenes(ctx context.Context, film core.FilmID) iter.Seq2[core.Scene, error]
}

// WithLibrary opens the interface on the list of films instead of the search.
func WithLibrary(lib Library) Option { return func(c *config) { c.lib = lib } }

type filmsMsg struct {
	Films []core.Film
	Err   error
}

type countMsg struct {
	Film core.FilmID
	N    int
}

// libraryScreen lists the films with how many scenes each has.
type libraryScreen struct {
	lib    Library
	search func() Screen
	films  []core.Film
	counts map[core.FilmID]int
	sel    int
	err    error
	w, h   int
}

func newLibrary(lib Library, search func() Screen) *libraryScreen {
	return &libraryScreen{lib: lib, search: search, counts: map[core.FilmID]int{}}
}

func (l *libraryScreen) Init() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var out filmsMsg
		for f, err := range l.lib.Films(ctx) {
			if err != nil {
				return filmsMsg{Err: err}
			}
			out.Films = append(out.Films, f)
		}
		return out
	}
}

func (l *libraryScreen) count(id core.FilmID) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		n := 0
		for _, err := range l.lib.Scenes(ctx, id) {
			if err != nil {
				return countMsg{Film: id, N: -1}
			}
			n++
		}
		return countMsg{Film: id, N: n}
	}
}

func (l *libraryScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.w, l.h = msg.Width, msg.Height
	case filmsMsg:
		l.films, l.err = msg.Films, msg.Err
		var cmds []tea.Cmd
		for _, f := range msg.Films {
			cmds = append(cmds, l.count(f.ID))
		}
		return l, tea.Batch(cmds...)
	case countMsg:
		l.counts[msg.Film] = msg.N
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			l.sel = max(l.sel-1, 0)
		case "down", "j":
			l.sel = max(min(l.sel+1, len(l.films)-1), 0)
		case "/":
			return l, Push(l.search())
		}
	}
	return l, nil
}

func (l *libraryScreen) View() string {
	var b strings.Builder
	b.WriteString(dim.Render("films    / search   esc quit") + "\n\n")
	if l.err != nil {
		b.WriteString("error: " + l.err.Error() + "\n")
	}
	if len(l.films) == 0 && l.err == nil {
		b.WriteString(dim.Render("no films yet: run `rose scenes FILE`") + "\n")
	}
	w := max(l.w, 20)
	for i, f := range l.films {
		line := fmt.Sprintf("%-40s %9s  %s", f.Title, Clock(f.Duration), scenesLabel(l.counts, f.ID))
		line = lipgloss.NewStyle().MaxWidth(w).Render(line)
		if i == l.sel {
			line = selected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func scenesLabel(counts map[core.FilmID]int, id core.FilmID) string {
	switch n, ok := counts[id]; {
	case !ok:
		return "…"
	case n < 0:
		return "?"
	case n == 1:
		return "1 scene"
	default:
		return fmt.Sprintf("%d scenes", n)
	}
}
