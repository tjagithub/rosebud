package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Titler is a screen that has a name for the status bar. Screens without
// one are left out of the breadcrumb.
type Titler interface{ Title() string }

const arrow = "\ue0b0" // powerline right arrow; needs a patched font

var (
	crumbBG   = lipgloss.Color("238")
	currentBG = lipgloss.Color("62")
	barBG     = lipgloss.Color("236")
)

// statusBar draws the breadcrumb on the left, the last crumb highlighted,
// and right at the end of a line w cells wide.
func statusBar(w int, crumbs []string, right string) string {
	var b strings.Builder
	for i, c := range crumbs {
		bg := crumbBG
		if i == len(crumbs)-1 {
			bg = currentBG
		}
		b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("255")).Render(" " + c + " "))
		b.WriteString(arrowTo(bg, next(crumbs, i)))
	}
	left := b.String()
	tail := lipgloss.NewStyle().Background(barBG).Foreground(lipgloss.Color("250")).Render(" " + right + " ")
	gap := max(w-lipgloss.Width(left)-lipgloss.Width(tail), 0)
	fill := lipgloss.NewStyle().Background(barBG).Render(strings.Repeat(" ", gap))
	return left + fill + tail
}

// next is the colour that follows crumb i: the next crumb's, or the bar's.
func next(crumbs []string, i int) color.Color {
	if i == len(crumbs)-1 {
		return barBG
	}
	if i+1 == len(crumbs)-1 {
		return currentBG
	}
	return crumbBG
}

func arrowTo(from, to color.Color) string {
	return lipgloss.NewStyle().Foreground(from).Background(to).Render(arrow)
}

// depthLabel is how many screens and overlays deep the user is.
func depthLabel(n int) string { return fmt.Sprintf("level %d", n) }
