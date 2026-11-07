package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tjagithub/rosebud/internal/core"
)

const detailID = 8 // kitty image slot of the detail screen

// detailScreen shows one scene: a large frame, its time, and the original
// text beside its translation.
type detailScreen struct {
	hits []core.Hit
	i    int
	fr   pane
	w, h int
}

func newDetail(hits []core.Hit, i int, bundle string) *detailScreen {
	return &detailScreen{hits: hits, i: i, fr: pane{id: detailID, bundle: bundle}}
}

// hit is the scene on show.
func (d *detailScreen) hit() core.Hit { return d.hits[d.i] }

func (d *detailScreen) Init() tea.Cmd { return nil }

func (d *detailScreen) Title() string { return fmt.Sprintf("scene %d", d.hit().Scene.ID) }

var (
	backKey = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	stepKey = key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "previous/next"))
)

func (d *detailScreen) ShortHelp() []key.Binding  { return []key.Binding{stepKey, backKey} }
func (d *detailScreen) FullHelp() [][]key.Binding { return [][]key.Binding{d.ShortHelp()} }

func (d *detailScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.w, d.h = msg.Width, msg.Height
		d.fr.box = size{cols: max(d.w-2, 8), rows: max(d.h-detailChrome, 1)}
		return d, d.fr.load(d.hit().Scene.Frame)
	case FrameMsg:
		return d, d.fr.onFrame(msg)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "left":
			return d, d.step(-1)
		case "right":
			return d, d.step(1)
		}
	}
	return d, nil
}

// detailChrome is the lines the strip, the two spare lines and the text take.
const detailChrome = 3 + 1 + 1 + dialogueRows

const dialogueRows = 4

// View stacks the scene strip, a spare line, the image, a spare line, and the
// dialogue beside the scene's details.
func (d *detailScreen) View() string {
	w := max(d.w, 20)
	imgRows := max(d.h-detailChrome, 1)
	img := lipgloss.Place(w, imgRows, lipgloss.Center, lipgloss.Center, d.image())
	s := d.hit().Scene
	info := fmt.Sprintf("%d/%d  %s – %s  %s", d.i+1, len(d.hits), Clock(s.Start), Clock(s.End), dim.Render(fmt.Sprintf("%.0fs", (s.End-s.Start).Seconds())))
	text := lipgloss.NewStyle().Height(dialogueRows).MaxHeight(dialogueRows).Render(
		columns(w, original(d.hit()), info+"\n"+d.hit().Translation))
	return strings.Join([]string{d.strip(w), "", img, "", text}, "\n")
}

// image is the frame's placeholder cells, or nothing while it loads.
func (d *detailScreen) image() string {
	if d.fr.shown == "" {
		return ""
	}
	return strings.TrimSuffix(d.fr.view(), "\n")
}

// strip is a boxed row of scene numbers around the current one, with arrows
// where there are more to the sides.
func (d *detailScreen) strip(w int) string {
	var parts []string
	for i := max(d.i-2, 0); i <= min(d.i+2, len(d.hits)-1); i++ {
		label := fmt.Sprintf("%d", d.hits[i].Scene.ID)
		if i == d.i {
			label = selected.Render(" " + label + " ")
		}
		parts = append(parts, label)
	}
	left, right := " ", " "
	if d.i > 0 {
		left = "<"
	}
	if d.i < len(d.hits)-1 {
		right = ">"
	}
	inner := lipgloss.PlaceHorizontal(max(w-4, 1), lipgloss.Center, strings.Join(parts, "  "))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w).Render(left + inner + right)
}

var _ help.KeyMap = (*detailScreen)(nil)

// step moves to the neighbouring hit, if there is one, and loads its frame.
func (d *detailScreen) step(by int) tea.Cmd {
	n := d.i + by
	if n < 0 || n >= len(d.hits) {
		return nil
	}
	d.i = n
	return d.fr.load(d.hit().Scene.Frame)
}
