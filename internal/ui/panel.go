package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// panel is a rounded box with its title set into the top edge and a note at
// the right end of that edge. A focused panel is drawn in the accent.
type panel struct {
	title, note string
	focus       bool
	accent      bool // drawn in the accent even without focus, as overlays are
}

// render draws body in the panel, w cells wide; h is the full height, or 0 to fit the body.
func (p panel) render(w, h int, body string) string {
	edge, name := faint, dim
	if p.focus || p.accent {
		edge, name = rose, lipgloss.NewStyle().Foreground(rose).Bold(true)
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderTop(false).BorderForeground(edge).Width(w)
	if h > 0 {
		box = box.Height(h - 1).MaxHeight(h - 1)
	}
	return p.top(w, edge, name) + "\n" + box.Render(body)
}

// fit draws body in a panel just wide enough for it and for the title and note.
func (p panel) fit(body string) string {
	w := max(lipgloss.Width(body)+2, lipgloss.Width(p.title)+lipgloss.Width(p.note)+8)
	return p.render(w, 0, body)
}

// top is the top edge: corner, title, rule, note, corner.
func (p panel) top(w int, edge color.Color, name lipgloss.Style) string {
	line := lipgloss.NewStyle().Foreground(edge)
	note := ""
	if p.note != "" {
		note = " " + p.note + " "
	}
	room := max(w-4-lipgloss.Width(note)-2, 0)
	title := ""
	if p.title != "" && room > 0 {
		title = " " + truncate(p.title, room) + " "
	}
	fill := max(w-4-lipgloss.Width(title)-lipgloss.Width(note), 0)
	return line.Render("╭─") + name.Render(title) + line.Render(strings.Repeat("─", fill)) + dim.Render(note) + line.Render("─╮")
}
