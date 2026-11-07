package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi/kitty"
)

func TestOverKeepsKittyPlaceholdersAroundThePopup(t *testing.T) {
	cell := "\x1b[38;5;7m" + string(kitty.Placeholder) + string(kitty.Diacritic(0)) + string(kitty.Diacritic(0)) + "\x1b[39m"
	base := strings.Repeat(cell, 10) + "\n" + strings.Repeat(cell, 10) + "\n" + strings.Repeat(cell, 10)
	got := over(base, "POP")
	if n := strings.Count(got, string(kitty.Placeholder)); n < 20 {
		t.Errorf("only %d placeholders survive:\n%q", n, got)
	}
	if !strings.Contains(got, "POP") || !strings.Contains(got, string(kitty.Diacritic(0))) {
		t.Errorf("lost the popup or the diacritics: %q", got)
	}
}
