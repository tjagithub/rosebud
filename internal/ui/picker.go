package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// jumpMsg asks one scene screen to show hit I of its hits.
type jumpMsg struct {
	To *detailScreen
	I  int
}

// picker is the overlay table for choosing a scene: its number, chapter,
// time and length. Typing digits moves to that scene number.
type picker struct {
	owner *detailScreen
	idx   []int // the hits listed, as indexes into owner.hits
	tbl   table.Model
	typed string
}

func newPicker(d *detailScreen) *picker {
	p := &picker{owner: d, idx: d.visible()}
	var rows []table.Row
	for _, i := range p.idx {
		s := d.hits[i].Scene
		rows = append(rows, table.Row{strconv.Itoa(int(s.ID)), truncate(d.chapter(s.Start), 15), Clock(s.Start), fmt.Sprintf("%.0fs", (s.End - s.Start).Seconds())})
	}
	st := table.DefaultStyles()
	st.Selected = selected
	st.Cell = lipgloss.NewStyle().Padding(0, 1)
	st.Header = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	p.tbl = table.New(
		table.WithColumns([]table.Column{{Title: "scene", Width: 7}, {Title: "chapter", Width: 15}, {Title: "time", Width: 9}, {Title: "length", Width: 7}}),
		table.WithRows(rows), table.WithStyles(st), table.WithFocused(true), table.WithWidth(50), table.WithHeight(12),
	)
	p.tbl.SetCursor(d.pos())
	return p
}

func (p *picker) Init() tea.Cmd { return nil }

func (p *picker) Title() string { return "pick scene" }

var pickKeys = []key.Binding{
	key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "move")),
	key.NewBinding(key.WithKeys("0"), key.WithHelp("0-9", "go to scene")),
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "go")),
	backKey,
}

func (p *picker) ShortHelp() []key.Binding  { return pickKeys }
func (p *picker) FullHelp() [][]key.Binding { return [][]key.Binding{pickKeys} }

func (p *picker) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.tbl.SetHeight(max(min(msg.Height-8, 14), 3))
	case tea.KeyPressMsg:
		k := msg.String()
		switch {
		case k == "enter":
			if len(p.idx) == 0 {
				return p, Close()
			}
			return p, tea.Batch(Close(), func() tea.Msg { return jumpMsg{To: p.owner, I: p.idx[p.tbl.Cursor()]} })
		case len(k) == 1 && k[0] >= '0' && k[0] <= '9':
			p.typed += k
			p.goTo()
			return p, nil
		case k == "backspace":
			if p.typed != "" {
				p.typed = p.typed[:len(p.typed)-1]
				p.goTo()
			}
			return p, nil
		}
		p.typed = ""
	}
	var cmd tea.Cmd
	p.tbl, cmd = p.tbl.Update(msg)
	return p, cmd
}

// goTo moves to the first scene whose number starts with what was typed.
func (p *picker) goTo() {
	for r, row := range p.tbl.Rows() {
		if strings.HasPrefix(row[0], p.typed) {
			p.tbl.SetCursor(r)
			return
		}
	}
}

func (p *picker) View() string {
	foot := dim.Render("scene: " + p.typed + "▏")
	if p.typed == "" {
		foot = dim.Render("type a scene number")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(p.tbl.View() + "\n" + foot)
}
