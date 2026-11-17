package ui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
)

// The palette. Every colour the interface uses is named here, so the look
// changes in one place.
var (
	rose    = lipgloss.Color("#F2789F") // accent: focus, selection, the current crumb
	leaf    = lipgloss.Color("#8CC7A1") // times and timecodes
	ink     = lipgloss.Color("#ECE4E7") // body text
	muted   = lipgloss.Color("#9A8F95") // labels, secondary text
	faint   = lipgloss.Color("#5A4F55") // unfocused borders, rules
	surface = lipgloss.Color("#2B2328") // status bar
	shade   = lipgloss.Color("#3D2C34") // selected row
	night   = lipgloss.Color("#1C1619") // text on the accent
)

// Named styles, built from the palette.
var (
	dim        = lipgloss.NewStyle().Foreground(muted)
	timeText   = lipgloss.NewStyle().Foreground(leaf)
	label      = lipgloss.NewStyle().Foreground(muted).Italic(true)
	accentText = lipgloss.NewStyle().Foreground(rose)
	strong     = lipgloss.NewStyle().Foreground(ink).Bold(true)
	errorText  = lipgloss.NewStyle().Foreground(rose).Bold(true)
	selected   = lipgloss.NewStyle().Background(shade).Foreground(ink).Bold(true)
	marker     = lipgloss.NewStyle().Foreground(rose).Background(shade)
	header     = lipgloss.NewStyle().Foreground(muted).Bold(true)
)

// helpStyles colours the key help line: keys in the accent, words muted.
func helpStyles() help.Styles {
	s := help.DefaultDarkStyles()
	s.ShortKey = lipgloss.NewStyle().Foreground(rose)
	s.ShortDesc = dim
	s.ShortSeparator = lipgloss.NewStyle().Foreground(faint)
	s.Ellipsis = dim
	return s
}

// row draws one line of a list w cells wide; the chosen one gets the accent
// bar and the selection shade.
func row(line string, w int, on bool) string {
	line = truncate(line, max(w-2, 1))
	if !on {
		return "  " + line
	}
	return marker.Render("▌ ") + selected.Width(max(w-2, 1)).Render(line)
}
