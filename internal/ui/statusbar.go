package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Titler is a screen that has a name for the status bar. Screens without
// one are left out of the breadcrumb.
type Titler interface{ Title() string }

// Statuser is a screen with something to say at the right of the status bar,
// such as where in a film it is.
type Statuser interface{ Status() string }

var (
	badge     = lipgloss.NewStyle().Background(rose).Foreground(night).Bold(true)
	crumbText = lipgloss.NewStyle().Background(surface).Foreground(muted)
	nowText   = lipgloss.NewStyle().Background(surface).Foreground(ink).Bold(true)
	sepText   = lipgloss.NewStyle().Background(surface).Foreground(faint)
)

// statusBar draws the app badge and the breadcrumb on the left, the last
// crumb bright, and right at the end of a line w cells wide.
func statusBar(w int, crumbs []string, right string) string {
	var b strings.Builder
	b.WriteString(badge.Render(" rosebud "))
	for i, c := range crumbs {
		if i > 0 {
			b.WriteString(sepText.Render(" ›"))
		}
		style := crumbText
		if i == len(crumbs)-1 {
			style = nowText
		}
		b.WriteString(style.Render(" " + c))
	}
	left := b.String()
	tail := crumbText.Render(right + " ")
	gap := max(w-lipgloss.Width(left)-lipgloss.Width(tail), 1)
	return truncate(left+crumbText.Render(strings.Repeat(" ", gap))+tail, max(w, 1))
}
