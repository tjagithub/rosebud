package ui

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

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
	film   core.Film // what is known of the film: frame rate and chapters
	hits   []core.Hit
	i      int
	fr     pane
	f      filter
	th     strip
	tb     textBoxes
	load   func() tea.Cmd // fetches the hits, for a screen opened before they are known
	err    error
	loadID int
	start  startFunc // starts playback; replaced in tests
	play   *playState
	seq    int
	w, h   int
}

// fps is the frame rate for timecodes: the film's, rounded, else defaultFPS.
func (d *detailScreen) fps() int {
	if d.film.FPS >= 1 {
		return int(d.film.FPS + 0.5)
	}
	return defaultFPS
}

// chapter is the title of the chapter playing at t, or "–".
func (d *detailScreen) chapter(t time.Duration) string {
	name := "–"
	for _, c := range d.film.Chapters {
		if c.Start > t {
			break
		}
		name = cmp.Or(c.Title, "–")
	}
	return name
}

func newDetail(hits []core.Hit, i int, bundle string) *detailScreen {
	return &detailScreen{hits: hits, i: i, fr: pane{id: detailID, bundle: bundle}, f: newFilter(), th: newStrip(bundle), tb: newTextBoxes(), start: startPlayer}
}

// hit is the scene on show, or the zero Hit while there are none.
func (d *detailScreen) hit() core.Hit {
	if d.i < len(d.hits) {
		return d.hits[d.i]
	}
	return core.Hit{}
}

func (d *detailScreen) Init() tea.Cmd {
	if d.load == nil {
		return nil
	}
	return d.load()
}

func (d *detailScreen) Title() string {
	if len(d.hits) == 0 {
		return "scenes"
	}
	return fmt.Sprintf("scene %d", d.hit().Scene.ID)
}

var (
	backKey = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	stepKey = key.NewBinding(key.WithKeys("left", "right", ",", "."), key.WithHelp("←/→ ,/.", "previous/next"))
	leapKey = key.NewBinding(key.WithKeys("<", ">"), key.WithHelp("</>", "5 scenes"))
)

var (
	filterKey = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	pickKey   = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "scenes"))
	playKey   = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "play"))
	timeKey   = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "time"))
	focusKey  = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus box"))
	scrollKey = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll"))
)

func (d *detailScreen) ShortHelp() []key.Binding {
	return []key.Binding{stepKey, leapKey, focusKey, scrollKey, pickKey, playKey, timeKey, filterKey, backKey}
}
func (d *detailScreen) FullHelp() [][]key.Binding { return [][]key.Binding{d.ShortHelp()} }

func (d *detailScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return d, d.resize(msg)
	case FrameMsg:
		return d, tea.Batch(d.fr.onFrame(msg), d.th.onFrame(msg))
	case playFrameMsg:
		return d, d.onPlayFrame(msg)
	case jumpMsg:
		return d, d.jump(msg)
	case scenesMsg:
		return d, d.setScenes(msg)
	case tea.MouseMsg:
		return d, d.onMouse(msg)
	case tea.KeyPressMsg:
		return d, d.onKey(msg)
	}
	return d, nil
}

func (d *detailScreen) resize(msg tea.WindowSizeMsg) tea.Cmd {
	d.w, d.h = msg.Width, msg.Height
	d.fr.box = size{cols: max(d.w-4, 8), rows: max(d.h-detailChrome, 1)}
	d.th.resize(d.w)
	d.layout()
	return d.reload()
}

// jump goes to the scene a picker or timecode overlay chose for this screen.
func (d *detailScreen) jump(msg jumpMsg) tea.Cmd {
	if msg.To != d || msg.I >= len(d.hits) {
		return nil
	}
	d.i = msg.I
	return d.reload()
}

// setScenes takes the scenes this screen asked for.
func (d *detailScreen) setScenes(msg scenesMsg) tea.Cmd {
	if msg.ID != d.loadID {
		return nil
	}
	d.hits, d.err, d.i = msg.Hits, msg.Err, 0
	return d.reload()
}

// onMouse focuses the text box clicked and scrolls the one under the wheel.
func (d *detailScreen) onMouse(msg tea.MouseMsg) tea.Cmd {
	m := msg.Mouse()
	if m.Y < d.h-boxRows-2 {
		return nil
	}
	if _, click := msg.(tea.MouseClickMsg); click {
		d.tb.focus = d.tb.at(m.X)
		return nil
	}
	if _, wheel := msg.(tea.MouseWheelMsg); wheel {
		return d.tb.scroll(msg)
	}
	return nil
}

// onKey sends a key to the filter bar while it is open, to playback while
// it runs, and otherwise to the scene.
func (d *detailScreen) onKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case d.f.editing:
		return d.onFilterKey(msg)
	case d.play != nil:
		return d.onPlayKey(msg)
	case scrolls(msg.String()):
		return d.tb.scroll(msg)
	}
	return d.onSceneKey(msg)
}

func (d *detailScreen) onFilterKey(msg tea.KeyPressMsg) tea.Cmd {
	wasActive := d.f.active()
	changed, cmd := d.f.update(msg)
	if changed {
		cmd = tea.Batch(cmd, d.settle())
	}
	d.relayout(wasActive)
	return cmd
}

// onPlayKey stops playback; left and right also step to the next scene.
func (d *detailScreen) onPlayKey(msg tea.KeyPressMsg) tea.Cmd {
	by := stepBy(msg.String())
	switch msg.String() {
	case "esc", "p", "space", "left", "right", ",", ".", "<", ">":
		cmd := d.stopPlay()
		if by != 0 {
			cmd = tea.Batch(cmd, d.step(by))
		}
		return cmd
	}
	return nil
}

// stepBy is how many scenes a key steps, and which way: one for the arrows
// and , . (for a hand already on the letter keys), five for < >.
func stepBy(k string) int {
	return map[string]int{"left": -1, ",": -1, "right": 1, ".": 1, "<": -leap, ">": leap}[k]
}

const leap = 5

func (d *detailScreen) onSceneKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "tab":
		d.tb.toggle()
	case "left", "right", ",", ".", "<", ">":
		return d.step(stepBy(msg.String()))
	case "p":
		return d.startPlay()
	case "s":
		return Open(newPicker(d))
	case "t":
		return Open(newTimecode(d, d.fps()))
	case "/":
		cmd := d.f.open()
		d.relayout(false)
		return cmd
	case "S", "esc":
		if d.f.active() {
			d.f.clear()
			d.relayout(true)
		}
	}
	return nil
}

// Capturing is true while the filter bar takes keys, so esc closes it, not the screen.
func (d *detailScreen) Capturing() bool { return d.f.editing || d.f.active() || d.play != nil }

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

const dialogueRows = boxRows

// View stacks the scene strip, a spare line, the image, a spare line, and the
// dialogue beside the scene's details.
func (d *detailScreen) View() string {
	w := max(d.w, 20)
	imgRows := max(d.h-detailChrome, 1)
	frame := panel{title: d.sceneTitle(), note: d.sceneNote(), focus: d.play != nil}
	img := frame.render(w, imgRows+2, lipgloss.Place(w-2, imgRows, lipgloss.Center, lipgloss.Center, d.image()))
	bar, note := "", ""
	if d.f.active() {
		bar, note = d.f.bar(), plural(len(d.visible()), "match")
	}
	text := d.tb.view(bar, note)
	return strings.Join([]string{d.th.view(w, d.pos(), len(d.visible())), img, text}, "\n")
}

// sceneTitle names the scene on show and its chapter, if it has one.
func (d *detailScreen) sceneTitle() string {
	if len(d.hits) == 0 {
		return "scene"
	}
	s := d.hit().Scene
	if c := d.chapter(s.Start); c != "–" {
		return fmt.Sprintf("scene %d · %s", s.ID, c)
	}
	return fmt.Sprintf("scene %d", s.ID)
}

// sceneNote is the scene's span and length, or that it is playing.
func (d *detailScreen) sceneNote() string {
	if d.play != nil {
		return accentText.Render("▶ playing")
	}
	if len(d.hits) == 0 {
		return ""
	}
	s := d.hit().Scene
	return fmt.Sprintf("%s – %s · %.0fs", Clock(s.Start), Clock(s.End), (s.End - s.Start).Seconds())
}

// Status is the film's title.
func (d *detailScreen) Status() string { return d.film.Title }

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

// layout sizes the text boxes and fills them for the current scene.
func (d *detailScreen) layout() {
	d.relayout(true)
}

// relayout refits the boxes, since the filter bar takes a row of the list,
// and refills them; follow scrolls the list to the current scene.
func (d *detailScreen) relayout(follow bool) {
	bar := 0
	if d.f.active() {
		bar = 1
	}
	d.tb.resize(max(d.w, 20), bar)
	if d.err != nil {
		d.tb.list.SetContent("error: " + d.err.Error())
		return
	}
	d.tb.show(d.hits, d.visible(), d.i, follow)
}

// reload loads the big frame and the thumbnails around it.
func (d *detailScreen) reload() tea.Cmd {
	d.relayout(true)
	return tea.Batch(d.fr.load(d.hit().Scene.Frame), d.th.load(d.hits, d.visible(), d.pos()))
}

var _ help.KeyMap = (*detailScreen)(nil)

// step moves by visible hits, stopping at the first and last, and loads the frame.
func (d *detailScreen) step(by int) tea.Cmd {
	idx := d.visible()
	for p, i := range idx {
		if i == d.i {
			to := min(max(p+by, 0), len(idx)-1)
			if to == p {
				return nil
			}
			d.i = idx[to]
			return d.reload()
		}
	}
	return nil
}

// scenesMsg carries the hits a film screen asked for.
type scenesMsg struct {
	ID   int
	Hits []core.Hit
	Err  error
}

var nextLoad atomic.Int64

// newFilmScreen is the scene screen on every scene of a film, read from src.
func newFilmScreen(src Searcher, film core.Film, bundle string) *detailScreen {
	if film.Bundle != "" {
		bundle = film.Bundle
	}
	d := newDetail(nil, 0, bundle)
	d.film = film
	id := int(nextLoad.Add(1))
	d.loadID = id
	d.load = func() tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			hits, err := src.Search(ctx, core.SearchQuery{Film: film.ID, Limit: 100000})
			return scenesMsg{ID: id, Hits: hits, Err: err}
		}
	}
	return d
}

// sceneAt is the hit of the scene playing at time t among the visible ones:
// the last that starts at or before t, else the first.
func (d *detailScreen) sceneAt(t time.Duration) int {
	idx := d.visible()
	if len(idx) == 0 {
		return d.i
	}
	at := idx[0]
	for _, i := range idx {
		if d.hits[i].Scene.Start > t {
			break
		}
		at = i
	}
	return at
}
