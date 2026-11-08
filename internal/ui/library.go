package ui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"io"
	"iter"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
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
	N    int // -1 when the scenes could not be read
}

// filmItem is a film with its scene count, once known.
type filmItem struct {
	core.Film
	scenes int
	known  bool
}

func (f filmItem) FilterValue() string { return f.Title }

type libraryKeys struct{ open, search, filter, quit, focus key.Binding }

func (k libraryKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.open, k.focus, k.search, k.filter, k.quit}
}
func (k libraryKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// libraryScreen lists the films with how many scenes each has.
type libraryScreen struct {
	lib    Library
	search func() Screen
	open   func(core.Film) Screen
	list   list.Model
	keys   libraryKeys
	err    error
	menu   menu
	inMenu bool // the menu has focus, not the films
	w, h   int
}

func newLibrary(lib Library, search func() Screen, open func(core.Film) Screen) *libraryScreen {
	l := list.New(nil, filmRow{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	scr := &libraryScreen{lib: lib, search: search, open: open, list: l, keys: libraryKeys{
		focus:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "menu/films")),
		open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open film")),
		search: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "search all")),
		filter: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter films")),
		quit:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "quit")),
	}}
	scr.menu = menu{items: []menuItem{
		{"films", func() tea.Cmd { scr.inMenu = false; return nil }},
		{"search all", func() tea.Cmd { return Push(scr.search()) }},
		{"add media", func() tea.Cmd {
			return Open(notice{"add media", "Adding media from here is coming.\nFor now run: rose build FILE"})
		}},
		{"quit", func() tea.Cmd { return tea.Quit }},
	}}
	return scr
}

// menuWidth is the outer width of the menu box.
const menuWidth = 22

func (l *libraryScreen) ShortHelp() []key.Binding  { return l.keys.ShortHelp() }
func (l *libraryScreen) FullHelp() [][]key.Binding { return l.keys.FullHelp() }

// Capturing is true while the filter box is open or a filter is applied.
func (l *libraryScreen) Capturing() bool { return l.list.FilterState() != list.Unfiltered }

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
		l.list.SetSize(max(msg.Width-menuWidth-2, 10), max(msg.Height-2, 1))
		return l, nil
	case filmsMsg:
		l.err = msg.Err
		items := make([]list.Item, len(msg.Films))
		cmds := make([]tea.Cmd, 0, len(msg.Films)+1)
		for i, f := range msg.Films {
			items[i] = filmItem{Film: f}
			cmds = append(cmds, l.count(f.ID))
		}
		return l, tea.Batch(append(cmds, l.list.SetItems(items))...)
	case countMsg:
		for i, it := range l.list.Items() {
			if f := it.(filmItem); f.ID == msg.Film {
				f.scenes, f.known = msg.N, true
				return l, l.list.SetItem(i, f)
			}
		}
		return l, nil
	case tea.KeyPressMsg:
		if l.list.FilterState() == list.Unfiltered && msg.String() == "tab" {
			l.inMenu = !l.inMenu
			return l, nil
		}
		if l.inMenu {
			if key.Matches(msg, l.keys.search) {
				return l, Push(l.search())
			}
			return l, l.menu.update(msg)
		}
		if l.list.FilterState() == list.Unfiltered && key.Matches(msg, l.keys.search) {
			return l, Push(l.search())
		}
		if msg.String() == "enter" && l.list.FilterState() != list.Filtering {
			if f, ok := l.list.SelectedItem().(filmItem); ok {
				return l, Push(l.open(f.Film))
			}
		}
	}
	var cmd tea.Cmd
	l.list, cmd = l.list.Update(msg)
	return l, cmd
}

func (l *libraryScreen) View() string {
	films := ""
	switch {
	case l.err != nil:
		films = "error: " + l.err.Error()
	case len(l.list.Items()) == 0:
		films = dim.Render("no films yet: run `rose scenes FILE`")
	default:
		films = l.list.View()
	}
	h := max(l.h, 3)
	box := func(focus bool, w int) lipgloss.Style {
		s := plainBorder
		if focus {
			s = focusBorder
		}
		return s.Width(w).Height(h).MaxHeight(h)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		box(l.inMenu, menuWidth).Render(l.menu.view(l.inMenu)),
		box(!l.inMenu, max(l.w-menuWidth, 12)).Render(films))
}

// filmRow draws a film as one line.
type filmRow struct{}

func (filmRow) Height() int                         { return 1 }
func (filmRow) Spacing() int                        { return 0 }
func (filmRow) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (filmRow) Render(w io.Writer, m list.Model, i int, it list.Item) {
	f := it.(filmItem)
	title := max(m.Width()-9-2-10-2, 8)
	line := fmt.Sprintf("%-*s %9s  %s", title, truncate(f.Title, title), Clock(f.Duration), scenesLabel(f))
	line = lipgloss.NewStyle().MaxWidth(max(m.Width(), 20)).Render(line)
	if i == m.Index() {
		line = selected.Render(line)
	}
	_, _ = io.WriteString(w, line)
}

func scenesLabel(f filmItem) string {
	switch {
	case !f.known:
		return "…"
	case f.scenes < 0:
		return "?"
	case f.scenes == 1:
		return "1 scene"
	default:
		return fmt.Sprintf("%d scenes", f.scenes)
	}
}

var _ help.KeyMap = (*libraryScreen)(nil)

func (l *libraryScreen) Title() string { return "films" }

// truncate cuts s to at most n cells.
func truncate(s string, n int) string { return ansi.Truncate(s, n, "…") }
