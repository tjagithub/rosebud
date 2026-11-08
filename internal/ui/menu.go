package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// menuItem is one line of a menu and what choosing it does.
type menuItem struct {
	label string
	do    func() tea.Cmd
}

// menu is a short vertical list of actions.
type menu struct {
	items []menuItem
	cur   int
}

// update moves the cursor on up and down and runs the item on enter.
func (m *menu) update(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "up", "k":
		m.cur = max(m.cur-1, 0)
	case "down", "j":
		m.cur = min(m.cur+1, len(m.items)-1)
	case "enter":
		if m.cur < len(m.items) && m.items[m.cur].do != nil {
			return m.items[m.cur].do()
		}
	}
	return nil
}

// view draws the items, the current one highlighted when focused.
func (m *menu) view(focused bool) string {
	var rows []string
	for i, it := range m.items {
		line := " " + it.label + " "
		if i == m.cur && focused {
			line = selected.Render(line)
		} else if i == m.cur {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

// notice is an overlay with a title and a few lines, closed by esc.
type notice struct{ title, text string }

func (n notice) Init() tea.Cmd                    { return nil }
func (n notice) Update(tea.Msg) (Screen, tea.Cmd) { return n, nil }
func (n notice) Title() string                    { return n.title }
func (n notice) ShortHelp() []key.Binding         { return []key.Binding{backKey} }
func (n notice) FullHelp() [][]key.Binding        { return [][]key.Binding{n.ShortHelp()} }
func (n notice) View() string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2).Render(n.text)
}
