package ui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
)

// statsRows is the stats panel's full height: one line and the two borders.
const statsRows = 3

// stats is a panel of library totals: films, scenes and running time.
func stats(w int, items []list.Item) string {
	var scenes, pending int
	var total time.Duration
	for _, it := range items {
		f := it.(filmItem)
		total += f.Duration
		switch {
		case !f.known:
			pending++
		case f.scenes > 0:
			scenes += f.scenes
		}
	}
	parts := []string{plural(len(items), "film"), plural(scenes, "scene"), Clock(total) + " in all"}
	if pending > 0 {
		parts = append(parts, "counting…")
	}
	line := "  " + strings.Join(parts, dim.Render("  ·  "))
	return panel{title: "stats"}.render(w, statsRows, line)
}
