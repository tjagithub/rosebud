package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// wordmark is "rosebud" in box-drawing letters.
var wordmark = []string{
	"┏━┓┏━┓┏━┓┏━╸┏┓ ╻ ╻╺┳┓",
	"┣┳┛┃ ┃┗━┓┣╸ ┣┻┓┃ ┃ ┃┃",
	"╹┗╸┗━┛┗━┛┗━╸┗━┛┗━┛╺┻┛",
}

// bannerRows is the banner's full height: the menu's rows and the two borders.
func bannerRows(m menu) int { return len(m.items) + 1 + 2 }

// logoWidth is the logo column, padding and rule included.
const logoWidth = 2 + 21 + 3

// banner is the top of the landing screen: the logo, a rule, then the menu.
// The logo is left out when the window is too narrow for both.
func banner(w int, m menu, focused bool) string {
	items := m.view(focused, menuWidth-2)
	body := items
	if w-2 >= logoWidth+menuWidth {
		body = lipgloss.JoinHorizontal(lipgloss.Top, logo(len(m.items)+1), items)
	}
	return panel{focus: focused}.render(w, bannerRows(m), body)
}

// logo is the wordmark over a tagline, with a rule on its right, h rows tall.
func logo(h int) string {
	mark := lipgloss.NewStyle().Foreground(rose).Bold(true)
	rows := []string{""}
	for _, l := range wordmark {
		rows = append(rows, mark.Render(l))
	}
	rows = append(rows, dim.Render("find the frame"))
	rule := lipgloss.NewStyle().Foreground(faint).Render("│")
	out := make([]string, h)
	for i := range out {
		cell := ""
		if i < len(rows) {
			cell = rows[i]
		}
		out[i] = "  " + cell + strings.Repeat(" ", max(logoWidth-3-lipgloss.Width(cell), 0)) + " " + rule
	}
	return strings.Join(out, "\n")
}
