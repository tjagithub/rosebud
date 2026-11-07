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
}

// New returns a screen that shows up to limit hits per search.
func New(src Searcher, limit int) *Model {
	in := textinput.New()
	in.Prompt = "search: "
	in.Focus()
	return &Model{src: src, limit: limit, input: in}
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
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "up":
			m.sel = max(m.sel-1, 0)
			return m, nil
		case "down":
			m.sel = max(min(m.sel+1, len(m.hits)-1), 0)
			return m, nil
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
		for _, p := range []string{h.Translation, h.Subtitle, h.Text} {
			if p = strings.TrimSpace(p); p != "" {
				b.WriteString(lipgloss.NewStyle().Width(w).Render(p) + "\n")
			}
		}
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
