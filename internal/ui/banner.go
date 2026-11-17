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

// art is a rose over the name.
var art = []artLine{
	{"", bloom},
	{"  `,'^", bloom},
	{" / /  |", bloom},
	{"| (   )", bloom},
	{",`_\\__/,", bloom},
	{" =| |=", stem},
	{"R O S E B U D", name},
}

// logoWidth is the logo column, padding and rule included.
const logoWidth = 2 + 13 + 4

// bannerRows is the banner's full height: the taller of logo and menu, and the two borders.
func bannerRows(m menu) int { return max(len(art), len(m.items)+1) + 2 }

// banner is the top of the landing screen: the logo, a rule, then the menu.
// The logo is left out when the window is too narrow for both.
func banner(w int, m menu, focused bool) string {
	items := m.view(focused, menuWidth-2)
	body := items
	if w-2 >= logoWidth+menuWidth {
		body = lipgloss.JoinHorizontal(lipgloss.Top, logo(bannerRows(m)-2), items)
	}
	return panel{focus: focused}.render(w, bannerRows(m), body)
}

// logo is the rose in its colours, with a rule on its right, h rows tall.
func logo(h int) string {
	styles := map[int]lipgloss.Style{
		bloom: lipgloss.NewStyle().Foreground(rose).Bold(true),
		stem:  lipgloss.NewStyle().Foreground(leaf),
		name:  strong,
	}
	rule := lipgloss.NewStyle().Foreground(faint).Render("│")
	out := make([]string, h)
	for i := range out {
		cell := ""
		if i < len(art) {
			cell = styles[art[i].part].Render(art[i].text)
		}
		out[i] = "  " + cell + strings.Repeat(" ", max(logoWidth-4-lipgloss.Width(cell), 0)) + " " + rule
	}
	return strings.Join(out, "\n")
}
