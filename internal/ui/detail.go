package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/tjagithub/rosebud/internal/core"
)

const detailID = 8 // kitty image slot of the detail screen

// detailScreen shows one scene: a large frame, its time, and the original
// text beside its translation.
type detailScreen struct {
	hit  core.Hit
	fr   pane
	w, h int
}

func newDetail(h core.Hit, bundle string) *detailScreen {
	return &detailScreen{hit: h, fr: pane{id: detailID, cols: 60, bundle: bundle}}
}

func (d *detailScreen) Init() tea.Cmd { return nil }

func (d *detailScreen) Title() string { return fmt.Sprintf("scene %d", d.hit.Scene.ID) }

var backKey = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))

func (d *detailScreen) ShortHelp() []key.Binding  { return []key.Binding{backKey} }
func (d *detailScreen) FullHelp() [][]key.Binding { return [][]key.Binding{{backKey}} }

func (d *detailScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.w, d.h = msg.Width, msg.Height
		d.fr.cols = min(max(msg.Width-4, 20), 72)
		return d, d.fr.load(d.hit.Scene.Frame)
	case FrameMsg:
		return d, d.fr.onFrame(msg)
	}
	return d, nil
}

func (d *detailScreen) View() string {
	s := d.hit.Scene
	var b strings.Builder
	fmt.Fprintf(&b, "%s – %s  %s\n\n", Clock(s.Start), Clock(s.End), dim.Render(fmt.Sprintf("%.0fs", (s.End-s.Start).Seconds())))
	if d.fr.shown != "" {
		b.WriteString(d.fr.view() + "\n")
	}
	b.WriteString(columns(max(d.w, 20), original(d.hit), d.hit.Translation))
	return b.String()
}

var _ help.KeyMap = (*detailScreen)(nil)
