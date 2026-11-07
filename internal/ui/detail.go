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
	f    filter
	th   strip
	w, h int
}

func newDetail(hits []core.Hit, i int, bundle string) *detailScreen {
	return &detailScreen{hits: hits, i: i, fr: pane{id: detailID, bundle: bundle}, f: newFilter(), th: newStrip(bundle)}
}

// hit is the scene on show.
func (d *detailScreen) hit() core.Hit { return d.hits[d.i] }

func (d *detailScreen) Init() tea.Cmd { return nil }

func (d *detailScreen) Title() string { return fmt.Sprintf("scene %d", d.hit().Scene.ID) }

var (
	backKey = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	stepKey = key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "previous/next"))
)

var filterKey = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "filter"))

func (d *detailScreen) ShortHelp() []key.Binding {
	return []key.Binding{stepKey, filterKey, backKey}
}
func (d *detailScreen) FullHelp() [][]key.Binding { return [][]key.Binding{d.ShortHelp()} }

func (d *detailScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.w, d.h = msg.Width, msg.Height
		d.fr.box = size{cols: max(d.w-4, 8), rows: max(d.h-detailChrome, 1)}
		d.th.resize(d.w)
		return d, d.reload()
	case FrameMsg:
		return d, tea.Batch(d.fr.onFrame(msg), d.th.onFrame(msg))
	case tea.KeyPressMsg:
		if d.f.editing {
			changed, cmd := d.f.update(msg)
			if changed {
				cmd = tea.Batch(cmd, d.settle())
			}
			return d, cmd
		}
		switch msg.String() {
		case "left":
			return d, d.step(-1)
		case "right":
			return d, d.step(1)
		case "s", "/":
			return d, d.f.open()
		case "S":
			d.f.clear()
			return d, nil
		case "esc":
			if d.f.active() {
				d.f.clear()
			}
		}
	}
	return d, nil
}

// Capturing is true while the filter bar takes keys, so esc closes it, not the screen.
func (d *detailScreen) Capturing() bool { return d.f.editing || d.f.active() }

// visible are the indexes of the hits the filter lets through.
func (d *detailScreen) visible() []int {
	var idx []int
	for i, h := range d.hits {
		if q := d.f.text(); q == "" || matches(h, q) {
			idx = append(idx, i)
		}
	}
	return idx
}

// settle moves to the first visible hit at or after the current one when the
// current no longer passes the filter.
func (d *detailScreen) settle() tea.Cmd {
	idx := d.visible()
	if len(idx) == 0 {
		return nil
	}
	cur := d.i
	for _, i := range idx {
		if i == cur {
			return nil
		}
	}
	d.i = idx[len(idx)-1]
	for p := len(idx) - 1; p >= 0 && idx[p] >= cur; p-- {
		d.i = idx[p]
	}
	return d.reload()
}

// detailChrome is the lines the strip, the two spare lines and the text take.
const detailChrome = stripRows + 2 + dialogueRows + 2

const dialogueRows = 4

// View stacks the scene strip, a spare line, the image, a spare line, and the
// dialogue beside the scene's details.
func (d *detailScreen) View() string {
	w := max(d.w, 20)
	imgRows := max(d.h-detailChrome, 1)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w)
	img := box.Render(lipgloss.Place(w-2, imgRows, lipgloss.Center, lipgloss.Center, d.image()))
	s := d.hit().Scene
	info := fmt.Sprintf("%d/%d  %s – %s  %s", d.i+1, len(d.hits), Clock(s.Start), Clock(s.End), dim.Render(fmt.Sprintf("%.0fs", (s.End-s.Start).Seconds())))
	rows, half := dialogueRows, max((w-5)/2, 8)
	if d.f.active() {
		rows--
	}
	list := lines(d.hits, d.visible(), d.i, rows, half)
	if d.f.active() {
		list += "\n" + d.f.bar()
	}
	text := box.Height(dialogueRows + 2).MaxHeight(dialogueRows + 2).Render(
		columns(w-2, list, info+"\n"+d.hit().Translation))
	return strings.Join([]string{d.th.view(w, d.pos(), len(d.visible())), img, text}, "\n")
}

// image is the frame's placeholder cells, or nothing while it loads.
func (d *detailScreen) image() string {
	if d.fr.shown == "" {
		return ""
	}
	return strings.TrimSuffix(d.fr.view(), "\n")
}

// pos is the place of the current scene among the visible ones.
func (d *detailScreen) pos() int {
	for p, i := range d.visible() {
		if i == d.i {
			return p
		}
	}
	return 0
}

// reload loads the big frame and the thumbnails around it.
func (d *detailScreen) reload() tea.Cmd {
	return tea.Batch(d.fr.load(d.hit().Scene.Frame), d.th.load(d.hits, d.visible(), d.pos()))
}

var _ help.KeyMap = (*detailScreen)(nil)

// step moves to the neighbouring visible hit, if there is one, and loads its frame.
func (d *detailScreen) step(by int) tea.Cmd {
	idx := d.visible()
	for p, i := range idx {
		if i == d.i {
			if p+by < 0 || p+by >= len(idx) {
				return nil
			}
			d.i = idx[p+by]
			return d.reload()
		}
	}
	return nil
}
