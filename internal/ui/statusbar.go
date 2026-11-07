package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Titler is a screen that has a name for the status bar. Screens without
// one are left out of the breadcrumb.
type Titler interface{ Title() string }

var (
	barBG     = lipgloss.Color("236")
	currentBG = lipgloss.Color("62")
	crumbText = lipgloss.NewStyle().Background(barBG).Foreground(lipgloss.Color("250"))
	sepText   = lipgloss.NewStyle().Background(barBG).Foreground(lipgloss.Color("240"))
	nowText   = lipgloss.NewStyle().Background(currentBG).Foreground(lipgloss.Color("255")).Bold(true)
)

// statusBar draws the breadcrumb on the left, the last crumb highlighted,
// and right at the end of a line w cells wide.
func statusBar(w int, crumbs []string, right string) string {
	var b strings.Builder
	for i, c := range crumbs {
		if i == len(crumbs)-1 {
			b.WriteString(nowText.Render(" " + c + " "))
			continue
		}
		b.WriteString(crumbText.Render(" "+c+" ") + sepText.Render("›"))
	}
	left := b.String()
	tail := crumbText.Render(" " + right + " ")
	gap := max(w-lipgloss.Width(left)-lipgloss.Width(tail), 0)
	return left + crumbText.Render(strings.Repeat(" ", gap)) + tail
}

// depthLabel is how many screens and overlays deep the user is.
func depthLabel(n int) string { return fmt.Sprintf("level %d", n) }
