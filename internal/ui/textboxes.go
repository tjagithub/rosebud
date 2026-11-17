package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tjagithub/rosebud/internal/core"
)

const (
	boxRows = 9 // lines of text each box shows

	dialogueBox = 0 // which box has focus
	detailsBox  = 1
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
		t.list.SetContent(dim.Render("  no scenes"))
		t.details.SetContent("")
		return
	}
	w := t.list.Width()
	var rows []string
	at := 0
	for p, i := range idx {
		h := hits[i]
		if i == cur {
			rows, at = append(rows, row(fmt.Sprintf("%-7s %s", Clock(h.Scene.Start), firstLine(h)), w, true)), p
			continue
		}
		rows = append(rows, "  "+timeText.Render(fmt.Sprintf("%-7s", Clock(h.Scene.Start)))+" "+truncate(firstLine(h), max(w-10, 1)))
	}
	t.list.SetContentLines(rows)
	t.details.SetContent(sceneText(hits[cur], max(t.details.Width()-2, 1), t.details.Height()))
	if changed {
		t.list.SetYOffset(max(at-boxRows/2, 0))
		t.details.GotoTop()
	}
}

// sceneText is a scene's original text beside its translation, w cells in all,
// split by a faint rule at least minRows tall.
func sceneText(h core.Hit, w, minRows int) string {
	if strings.TrimSpace(h.Subtitle+h.Text+h.Translation) == "" {
		return dim.Render(" no text in this scene")
	}
	half := max((w-1)/2, 4)
	orig := labelled(half, "subtitle", h.Subtitle, "speech", h.Text)
	if orig == "" {
		orig = dim.Render(" no original text")
	}
	tran := labelled(w-1-half, "english", h.Translation)
	if tran == "" {
		tran = dim.Render(" no translation")
	}
	rows := max(lipgloss.Height(orig), lipgloss.Height(tran), minRows)
	rule := lipgloss.NewStyle().Foreground(faint).Render(strings.TrimSuffix(strings.Repeat("│\n", rows), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(half).Render(orig), rule, tran)
}

// labelled is each non-empty text of name/text pairs under its label, wrapped to w cells.
func labelled(w int, pairs ...string) string {
	wrap := lipgloss.NewStyle().Width(w).PaddingLeft(1)
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		if t := strings.TrimSpace(pairs[i+1]); t != "" {
			parts = append(parts, label.Render(" "+pairs[i])+"\n"+wrap.Render(t))
		}
	}
	return strings.Join(parts, "\n")
}

// view draws both boxes, the focused one in the accent, and bar under the list if set.
func (t *textBoxes) view(bar, note string) string {
	left, right := t.widths()
	l := t.list.View()
	if bar != "" {
		l += "\n" + bar
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		panel{title: "dialogue", note: note, focus: t.focus == dialogueBox}.render(left, 0, l),
		panel{title: "text", note: overflow(t.details), focus: t.focus == detailsBox}.render(right, 0, t.details.View()))
}

// overflow marks which way a box has more text than it shows, or is empty if it all fits.
func overflow(v viewport.Model) string {
	up, down := !v.AtTop(), !v.AtBottom()
	switch {
	case up && down:
		return accentText.Render("↑↓ more")
	case up:
		return accentText.Render("↑ more")
	case down:
		return accentText.Render("↓ more")
	}
	return ""
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
