package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tjagithub/rosebud/internal/core"
)

// Searcher is the part of core.Client the screen needs.
type Searcher interface {
	Search(ctx context.Context, q core.SearchQuery) ([]core.Hit, error)
}

// ResultMsg carries the hits of the search numbered Seq.
type ResultMsg struct {
	Seq  int
	Hits []core.Hit
	Err  error
}

// searchScreen is the search box, its hits and the chosen one. Searches run as
// commands, so Update never blocks.
type searchScreen struct {
	src           Searcher
	limit         int
	input         textinput.Model
	width, height int
	seq           int // the newest search; older results are dropped
	hits          []core.Hit
	sel           int
	err           error
	fr            pane
}

func newSearch(src Searcher, limit int, cfg config) *searchScreen {
	in := textinput.New()
	in.Prompt = "search: "
	in.Focus()
	return &searchScreen{src: src, limit: limit, input: in, fr: pane{id: frameID, box: size{cols: frameCols}, bundle: cfg.bundle}}
}

func (m *searchScreen) Init() tea.Cmd { return nil }

func (m *searchScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(msg.Width-10, 10))
	case ResultMsg:
		if msg.Seq != m.seq {
			return m, nil
		}
		m.hits, m.err, m.sel = msg.Hits, msg.Err, 0
		return m, m.frame()
	case FrameMsg:
		return m, m.fr.onFrame(msg)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if m.sel < len(m.hits) {
				return m, Push(newDetail(m.hits, m.sel, m.fr.bundle))
			}
			return m, nil
		case "up":
			m.sel = max(m.sel-1, 0)
			return m, m.frame()
		case "down":
			m.sel = max(min(m.sel+1, len(m.hits)-1), 0)
			return m, m.frame()
		}
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			return m, m.search()
		}
		return m, cmd
	}
	return m, nil
}

// search starts a search for the current text.
func (m *searchScreen) search() tea.Cmd {
	m.seq++
	seq, text := m.seq, strings.TrimSpace(m.input.Value())
	if text == "" {
		m.hits, m.err, m.sel = nil, nil, 0
		m.fr.shown, m.fr.want = "", ""
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hits, err := m.src.Search(ctx, core.SearchQuery{Text: text, Limit: m.limit})
		return ResultMsg{Seq: seq, Hits: hits, Err: err}
	}
}

func (m *searchScreen) View() string {
	w := max(m.width, 20)
	out := []string{" " + m.input.View(), panel{title: "results", note: m.Status(), focus: true}.render(w, 0, m.results(w-2))}
	if m.sel < len(m.hits) {
		h := m.hits[m.sel]
		body := columns(w-4, original(h), h.Translation)
		if m.fr.shown != "" {
			body = strings.TrimSuffix(m.fr.view(), "\n") + "\n" + body
		}
		out = append(out, panel{title: "scene", note: Clock(h.Scene.Start)}.render(w, 0, " "+body))
	}
	return strings.Join(out, "\n")
}

// results are the hit lines w cells wide, or why there are none.
func (m *searchScreen) results(w int) string {
	q := strings.TrimSpace(m.input.Value())
	switch {
	case m.err != nil:
		return errorText.Render(" error: ") + m.err.Error()
	case q == "":
		return dim.Render(" Type to search the dialogue, subtitles and translations of every film.")
	case len(m.hits) == 0:
		return dim.Render(" No scenes match ") + strong.Render("“"+q+"”")
	}
	lines := make([]string, len(m.hits))
	for i, h := range m.hits {
		if i == m.sel {
			lines[i] = row(fmt.Sprintf("%7s  %s", Clock(h.Scene.Start), oneLine(h)), w, true)
			continue
		}
		lines[i] = "  " + timeText.Render(fmt.Sprintf("%7s", Clock(h.Scene.Start))) + "  " + truncate(oneLine(h), max(w-11, 1))
	}
	return strings.Join(lines, "\n")
}

// Status is how many scenes the search found.
func (m *searchScreen) Status() string {
	if strings.TrimSpace(m.input.Value()) == "" {
		return ""
	}
	return plural(len(m.hits), "scene")
}

// oneLine is the first of a hit's translation, subtitle and text that has any.
func oneLine(h core.Hit) string {
	for _, p := range []string{h.Translation, h.Subtitle, h.Text} {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			return p
		}
	}
	return ""
}

// Clock formats d as M:SS, or H:MM:SS from an hour on.
func Clock(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// original is the on-screen subtitle and the spoken text of a hit, if any.
func original(h core.Hit) string {
	var parts []string
	for _, p := range []string{h.Subtitle, h.Text} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n")
}

// columns lays left and right side by side in a width of w cells.
func columns(w int, left, right string) string {
	half := max((w-3)/2, 8)
	l := lipgloss.NewStyle().Width(half).Render(strings.TrimSpace(left))
	r := lipgloss.NewStyle().Width(half).Render(strings.TrimSpace(right))
	return lipgloss.JoinHorizontal(lipgloss.Top, l, lipgloss.NewStyle().Foreground(faint).Render(" │ "), r)
}

func (m *searchScreen) Title() string { return "search" }

var (
	openKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open scene"))
	moveKey = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "move"))
	quitKey = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
)

func (m *searchScreen) ShortHelp() []key.Binding  { return []key.Binding{moveKey, openKey, quitKey} }
func (m *searchScreen) FullHelp() [][]key.Binding { return [][]key.Binding{m.ShortHelp()} }
