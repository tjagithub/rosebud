// Package ui is the terminal interface: a search box, the matching scenes and
// the text of the chosen one.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// Model is the Bubble Tea model. Searches run as commands, so Update never blocks.
type Model struct {
	src           Searcher
	limit         int
	input         textinput.Model
	width, height int
	seq           int // the newest search; older results are dropped
	hits          []core.Hit
	sel           int
	err           error
	bundle        string
	want, shown   string // frame path being loaded, and the one on screen
	rows          int
}

// New returns a screen that shows up to limit hits per search.
func New(src Searcher, limit int, opts ...Option) *Model {
	in := textinput.New()
	in.Prompt = "search: "
	in.Focus()
	m := &Model{src: src, limit: limit, input: in}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Selected is the index of the highlighted hit.
func (m *Model) Selected() int { return m.sel }

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		return m, m.onFrame(msg)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
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
func (m *Model) search() tea.Cmd {
	m.seq++
	seq, text := m.seq, strings.TrimSpace(m.input.Value())
	if text == "" {
		m.hits, m.err, m.sel = nil, nil, 0
		m.shown, m.want = "", ""
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hits, err := m.src.Search(ctx, core.SearchQuery{Text: text, Limit: m.limit})
		return ResultMsg{Seq: seq, Hits: hits, Err: err}
	}
}

var (
	dim      = lipgloss.NewStyle().Faint(true)
	selected = lipgloss.NewStyle().Bold(true).Reverse(true)
)

func (m *Model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.input.View() + "\n\n")
	if m.err != nil {
		b.WriteString("error: " + m.err.Error() + "\n")
	}
	w := max(m.width, 20)
	for i, h := range m.hits {
		line := fmt.Sprintf("%7s  %s", Clock(h.Scene.Start), oneLine(h))
		line = lipgloss.NewStyle().MaxWidth(w).Render(line)
		if i == m.sel {
			line = selected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if m.sel < len(m.hits) {
		h := m.hits[m.sel]
		b.WriteString("\n" + dim.Render(strings.Repeat("─", w)) + "\n")
		if m.shown != "" {
			b.WriteString(m.framePane())
		}
		b.WriteString(columns(w, original(h), h.Translation) + "\n")
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
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
	return lipgloss.JoinHorizontal(lipgloss.Top, l, dim.Render(" │ "), r)
}
