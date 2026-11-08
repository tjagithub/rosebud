package ui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tjagithub/rosebud/internal/core"
)

const (
	boxRows = 5 // lines of text each box shows

	dialogueBox = 0 // which box has focus
	detailsBox  = 1
)

var (
	focusBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	plainBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
)

// textBoxes are the two scrolling boxes under the image: the dialogue list
// and the scene's details. One has focus and takes the scroll keys.
type textBoxes struct {
	list, details viewport.Model
	focus         int
	w             int
}

func newTextBoxes() textBoxes {
	mk := func() viewport.Model {
		v := viewport.New()
		v.KeyMap.Left, v.KeyMap.Right = key.NewBinding(), key.NewBinding()
		return v
	}
	return textBoxes{list: mk(), details: mk()}
}

// resize fits the boxes to w cells, the list giving a row to the filter bar when barRows is 1.
func (t *textBoxes) resize(w, barRows int) {
	t.w = w
	left, right := t.widths()
	t.list.SetWidth(left - 2)
	t.list.SetHeight(boxRows - barRows)
	t.details.SetWidth(right - 2)
	t.details.SetHeight(boxRows)
}

// widths are the outer widths of the two boxes.
func (t *textBoxes) widths() (left, right int) { return t.w / 2, t.w - t.w/2 }

// show fills the boxes for scene cur among the hits idx lets through; a
// changed scene scrolls the list to it and the details to the top.
func (t *textBoxes) show(hits []core.Hit, idx []int, cur int, changed bool) {
	if len(hits) == 0 {
		t.list.SetContent(dim.Render("no scenes"))
		t.details.SetContent("")
		return
	}
	var rows []string
	at := 0
	for p, i := range idx {
		h := hits[i]
		line := lipgloss.NewStyle().MaxWidth(t.list.Width()).Render(fmt.Sprintf("%-7s %s", Clock(h.Scene.Start), firstLine(h)))
		if i == cur {
			line, at = selected.Render(line), p
		}
		rows = append(rows, line)
	}
	t.list.SetContentLines(rows)
	h, s := hits[cur], hits[cur].Scene
	info := fmt.Sprintf("%d/%d  %s – %s  %s", at+1, len(idx), Clock(s.Start), Clock(s.End), dim.Render(fmt.Sprintf("%.0fs", (s.End-s.Start).Seconds())))
	t.details.SetContent(lipgloss.NewStyle().Width(max(t.details.Width(), 1)).Render(info + "\n" + h.Translation))
	if changed {
		t.list.SetYOffset(max(at-boxRows/2, 0))
		t.details.GotoTop()
	}
}

// view draws both boxes, the focused one outlined, and bar under the list if set.
func (t *textBoxes) view(bar string) string {
	left, right := t.widths()
	l := t.list.View()
	if bar != "" {
		l += "\n" + bar
	}
	style := func(f int) lipgloss.Style {
		if t.focus == f {
			return focusBorder
		}
		return plainBorder
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		style(dialogueBox).Width(left).Render(l),
		style(detailsBox).Width(right).Render(t.details.View()))
}

// toggle moves focus to the other box.
func (t *textBoxes) toggle() { t.focus = 1 - t.focus }

// at is the box under column x, for clicks and the wheel.
func (t *textBoxes) at(x int) int {
	if left, _ := t.widths(); x < left {
		return dialogueBox
	}
	return detailsBox
}

// scroll gives msg to the box with focus, or to the one at wheelX for the wheel.
func (t *textBoxes) scroll(msg tea.Msg) tea.Cmd {
	v := &t.list
	if t.focus == detailsBox {
		v = &t.details
	}
	if w, ok := msg.(tea.MouseWheelMsg); ok && t.at(w.X) == detailsBox {
		v = &t.details
	} else if ok {
		v = &t.list
	}
	var cmd tea.Cmd
	*v, cmd = v.Update(msg)
	return cmd
}

// scrolls are the keys that move the box with focus.
func scrolls(k string) bool {
	switch k {
	case "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d", "home", "end":
		return true
	}
	return false
}
