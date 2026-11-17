package ui

import (
	"context"
	"fmt"
	"io"
	"iter"
	"time"

	"github.com/charmbracelet/x/ansi"

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
	l.SetShowFilter(false) // the screen draws the filter box in the header row
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
		l.list.SetSize(l.w-2, max(l.filmsHeight()-3, 1))
		return l, nil
	case filmsMsg:
		return l, l.setFilms(msg)
	case countMsg:
		return l, l.setCount(msg)
	case tea.KeyPressMsg:
		if cmd, done := l.onKey(msg); done {
			return l, cmd
		}
	}
	var cmd tea.Cmd
	l.list, cmd = l.list.Update(msg)
	return l, cmd
}

// setFilms lists the films and starts counting their scenes.
func (l *libraryScreen) setFilms(msg filmsMsg) tea.Cmd {
	l.err = msg.Err
	items := make([]list.Item, len(msg.Films))
	cmds := make([]tea.Cmd, 0, len(msg.Films)+1)
	for i, f := range msg.Films {
		items[i] = filmItem{Film: f}
		cmds = append(cmds, l.count(f.ID))
	}
	return tea.Batch(append(cmds, l.list.SetItems(items))...)
}

// setCount fills in one film's scene count.
func (l *libraryScreen) setCount(msg countMsg) tea.Cmd {
	for i, it := range l.list.Items() {
		if f := it.(filmItem); f.ID == msg.Film {
			f.scenes, f.known = msg.N, true
			return l.list.SetItem(i, f)
		}
	}
	return nil
}

// onKey handles the keys the screen keeps for itself; done is false for keys
// the list should have.
func (l *libraryScreen) onKey(msg tea.KeyPressMsg) (cmd tea.Cmd, done bool) {
	free := l.list.FilterState() == list.Unfiltered
	switch {
	case free && msg.String() == "tab":
		l.inMenu = !l.inMenu
		return nil, true
	case (free || l.inMenu) && key.Matches(msg, l.keys.search):
		return Push(l.search()), true
	case l.inMenu:
		return l.menu.update(msg), true
	case msg.String() == "enter" && l.list.FilterState() != list.Filtering:
		if f, ok := l.list.SelectedItem().(filmItem); ok {
			return Push(l.open(f.Film)), true
		}
	}
	return nil, false
}

// filmsHeight is the outer height of the films panel, between the banner and the stats.
func (l *libraryScreen) filmsHeight() int { return max(l.h-bannerRows(l.menu, l.h)-statsRows, 3) }

func (l *libraryScreen) View() string {
	films := panel{title: "films", note: l.Status(), focus: !l.inMenu}.render(l.w, l.filmsHeight(), l.filmsBody())
	return lipgloss.JoinVertical(lipgloss.Left, banner(l.w, l.h, l.menu, l.inMenu), films, stats(l.w, l.list.Items()))
}

// filmsBody is the column header, or the filter box while filtering, over the list.
func (l *libraryScreen) filmsBody() string {
	switch {
	case l.err != nil:
		return errorText.Render("error: ") + l.err.Error()
	case len(l.list.Items()) == 0:
		return dim.Render("\n  No films yet.\n\n  Make one with ") + strong.Render("rose build FILE")
	}
	head := header.Render(filmColumns(l.list.Width(), "title", "length", "scenes"))
	if l.list.FilterState() != list.Unfiltered {
		head = "  " + l.list.FilterInput.View()
	}
	return head + "\n" + l.list.View()
}

// Status is how many films there are.
func (l *libraryScreen) Status() string { return plural(len(l.list.Items()), "film") }

// filmColumns lays out one film line w cells wide: a title, then right-aligned columns.
func filmColumns(w int, title, length, scenes string) string {
	tw := max(w-2-9-2-10, 8)
	return fmt.Sprintf("  %-*s %8s  %10s", tw, truncate(title, tw), length, scenes)
}

// filmRow draws a film as one line.
type filmRow struct{}

func (filmRow) Height() int                         { return 1 }
func (filmRow) Spacing() int                        { return 0 }
func (filmRow) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (filmRow) Render(w io.Writer, m list.Model, i int, it list.Item) {
	f := it.(filmItem)
	length := "–"
	if f.Duration > 0 {
		length = Clock(f.Duration)
	}
	line := filmColumns(m.Width(), f.Title, length, scenesLabel(f))[2:]
	_, _ = io.WriteString(w, row(line, m.Width(), i == m.Index()))
}

func scenesLabel(f filmItem) string {
	switch {
	case !f.known:
		return "…"
	case f.scenes < 0:
		return "?"
	default:
		return plural(f.scenes, "scene")
	}
}

// plural is n and the noun, with an s unless n is 1.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

var _ help.KeyMap = (*libraryScreen)(nil)

func (l *libraryScreen) Title() string { return "films" }

// truncate cuts s to at most n cells.
func truncate(s string, n int) string { return ansi.Truncate(s, n, "…") }
