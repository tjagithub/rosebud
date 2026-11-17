package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/tjagithub/rosebud/internal/core"
)

// filter narrows the dialogue list to the lines containing a piece of text.
// It is typed into a slim bar; enter keeps it, and S or esc clears it.
type filter struct {
	in      textinput.Model
	editing bool
	kept    string
}

func newFilter() filter {
	in := textinput.New()
	in.Prompt = "/ "
	return filter{in: in}
}

// text is the filter in force: what is being typed, or else what was kept.
func (f *filter) text() string {
	if f.editing {
		return strings.TrimSpace(f.in.Value())
	}
	return f.kept
}

func (f *filter) active() bool { return f.editing || f.kept != "" }

func (f *filter) open() tea.Cmd {
	f.editing = true
	f.in.SetValue(f.kept)
	return f.in.Focus()
}

// update handles a key while the bar is open; changed says the filter text moved.
func (f *filter) update(msg tea.KeyPressMsg) (changed bool, cmd tea.Cmd) {
	before := f.text()
	switch msg.String() {
	case "enter":
		f.kept, f.editing = strings.TrimSpace(f.in.Value()), false
		f.in.Blur()
	case "esc":
		f.editing = false
		f.in.Blur()
	default:
		f.in, cmd = f.in.Update(msg)
	}
	return f.text() != before, cmd
}

func (f *filter) clear() { f.kept, f.editing = "", false; f.in.Blur() }

// bar is the line shown under the list while a filter is open or kept.
func (f *filter) bar() string {
	if f.editing {
		return f.in.View()
	}
	return dim.Render("filter: " + f.kept + "  (S clears)")
}

// matches reports whether any text of h contains q, ignoring case.
func matches(h core.Hit, q string) bool {
	q = strings.ToLower(q)
	for _, p := range []string{h.Subtitle, h.Text, h.Translation} {
		if strings.Contains(strings.ToLower(p), q) {
			return true
		}
	}
	return false
}

// firstLine is a hit's own words on one line, else its translation.
func firstLine(h core.Hit) string {
	for _, p := range []string{h.Subtitle, h.Text, h.Translation} {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			return p
		}
	}
	return ""
}
