package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// artLine is one line of the logo and the part of the rose it draws.
type artLine struct {
	text string
	part int
}

const (
	bloom = iota
	stem
	name
)

// bigRose is the logo for tall windows.
var bigRose = []artLine{
	{"    █░ ▄░", bloom},
	{"   █▒▒░█▒░", bloom},
	{"  █▒▒░█▒▒▒░", bloom},
	{" █▒▒░█▒▒▒▒▒░", bloom},
	{" █▒▒░█▒▒▒▒▒░", bloom},
	{" ██▒░█▒▒▒▒▒░", bloom},
	{"  ██▒░█▒▒▒░", bloom},
	{"   ██▒░█▒░", bloom},
	{"   ▄▄█▓▓", stem},
	{"  ▄▀  ▓ ▀▄", stem},
	{"R O S E B U D", name},
}

// smallRose is the logo when a big one would squeeze the films.
var smallRose = []artLine{
	{"   .--.--.", bloom},
	{"  ( ( @ ) )", bloom},
	{"   \\ `-' /", bloom},
	{"    `-.-'", bloom},
	{"   __ |", stem},
	{"  (__\\|", stem},
	{"R O S E B U D", name},
}

// tallScreen is the least screen height that gets the big rose.
const tallScreen = 32

// art is the logo for a screen h rows tall.
func art(h int) []artLine {
	if h >= tallScreen {
		return bigRose
	}
	return smallRose
}

// logoWidth is the logo column, padding and rule included.
const logoWidth = 2 + 13 + 4

// bannerRows is the banner's full height on a screen h rows tall: the taller
// of logo and menu, and the two borders.
func bannerRows(m menu, h int) int { return max(len(art(h)), len(m.items)+1) + 2 }

// banner is the top of a w by h landing screen: the logo, a rule, then the
// menu. The logo is left out when the window is too narrow for both.
func banner(w, h int, m menu, focused bool) string {
	items := m.view(focused, menuWidth-2)
	body := items
	rows := bannerRows(m, h)
	if w-2 >= logoWidth+menuWidth {
		body = lipgloss.JoinHorizontal(lipgloss.Top, logo(art(h), rows-2), items)
	}
	return panel{focus: focused}.render(w, rows, body)
}

// logo draws lines in the rose's colours, with a rule on the right, h rows tall.
func logo(lines []artLine, h int) string {
	styles := map[int]lipgloss.Style{
		bloom: lipgloss.NewStyle().Foreground(rose).Bold(true),
		stem:  lipgloss.NewStyle().Foreground(leaf),
		name:  strong,
	}
	rule := lipgloss.NewStyle().Foreground(faint).Render("│")
	out := make([]string, h)
	for i := range out {
		cell := ""
		if i < len(lines) {
			cell = styles[lines[i].part].Render(lines[i].text)
		}
		out[i] = "  " + cell + strings.Repeat(" ", max(logoWidth-4-lipgloss.Width(cell), 0)) + " " + rule
	}
	return strings.Join(out, "\n")
}
