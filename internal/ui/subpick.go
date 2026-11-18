package ui

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/tjagithub/rosebud/internal/core"
)

// Subtitles lists the subtitles a film could be read from and loads one into its scenes.
type Subtitles interface {
	Choices(ctx context.Context, film core.Film) ([]core.SubtitleChoice, error)
	Load(ctx context.Context, film core.Film, ref string, progress func(done, total int64)) error
}

// WithSubtitles lets the scene screen load subtitles with l.
func WithSubtitles(s Subtitles) Option { return func(c *config) { c.subs = s } }

type choicesMsg struct {
	to   *subPicker
	list []core.SubtitleChoice
	err  error
}

// subPicker is the overlay listing a film's subtitles, with a last row for
// typing the path of any other subtitle file.
type subPicker struct {
	owner   *detailScreen
	list    []core.SubtitleChoice
	err     error
	known   bool // the list has arrived
	cur     int
	confirm bool // the bitmap row under the cursor waits for a second enter
	typing  bool
	path    textinput.Model
}

func newSubPicker(d *detailScreen) *subPicker {
	ti := textinput.New()
	ti.Placeholder = "path to a .srt, .vtt or .ass file"
	ti.Prompt = "path: "
	ti.SetWidth(48)
	return &subPicker{owner: d, path: ti}
}

func (p *subPicker) Init() tea.Cmd {
	subs, film := p.owner.subs, p.owner.film
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		list, err := subs.Choices(ctx, film)
		return choicesMsg{to: p, list: list, err: err}
	}
}

func (p *subPicker) Title() string { return "subtitles" }

var subKeys = []key.Binding{
	key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "move")),
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "load")),
	backKey,
}

func (p *subPicker) ShortHelp() []key.Binding  { return subKeys }
func (p *subPicker) FullHelp() [][]key.Binding { return [][]key.Binding{subKeys} }

// Capturing is true while a path is typed, so esc returns to the list.
func (p *subPicker) Capturing() bool { return p.typing }

func (p *subPicker) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case choicesMsg:
		if msg.to == p {
			p.list, p.err, p.known = msg.list, msg.err, true
		}
	case tea.KeyPressMsg:
		if p.typing {
			return p, p.onPathKey(msg)
		}
		return p, p.onKey(msg)
	}
	return p, nil
}

func (p *subPicker) onKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k":
		p.cur, p.confirm = max(p.cur-1, 0), false
	case "down", "j":
		p.cur, p.confirm = min(p.cur+1, len(p.list)), false
	case "enter":
		if !p.known {
			return nil
		}
		if p.cur == len(p.list) {
			p.typing = true
			return p.path.Focus()
		}
		c := p.list[p.cur]
		if c.Bitmap && !p.confirm {
			p.confirm = true
			return nil
		}
		return tea.Batch(Close(), p.owner.loadSubs(c.Ref, langName(c.Lang)))
	}
	return nil
}

func (p *subPicker) onPathKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.typing = false
		p.path.Blur()
		return nil
	case "enter":
		path := expandHome(strings.TrimSpace(p.path.Value()))
		if path == "" {
			return nil
		}
		return tea.Batch(Close(), p.owner.loadSubs(path, filepath.Base(path)))
	}
	var cmd tea.Cmd
	p.path, cmd = p.path.Update(msg)
	return cmd
}

func (p *subPicker) View() string {
	var rows []string
	switch {
	case p.err != nil:
		rows = append(rows, errorText.Render(p.err.Error()), "")
	case !p.known:
		rows = append(rows, dim.Render("looking…"), "")
	case len(p.list) == 0:
		rows = append(rows, dim.Render("none in the video or beside it"), "")
	}
	for i, c := range p.list {
		rows = append(rows, row(choiceLine(c), 60, i == p.cur))
	}
	rows = append(rows, row("other file…", 60, p.cur == len(p.list)))
	foot := dim.Render("bitmap subtitles are read by OCR, which takes minutes")
	switch {
	case p.typing:
		foot = p.path.View()
	case p.confirm:
		foot = accentText.Render("OCR takes minutes: enter again to start")
	}
	body := lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(rows, "\n") + "\n\n" + foot)
	return panel{title: "subtitles", note: plural(len(p.list), "choice"), accent: true}.fit(body)
}

// choiceLine is one row: language, format, and where it is.
func choiceLine(c core.SubtitleChoice) string {
	kind, where := "text", "track "+c.Ref
	if c.Bitmap {
		kind = "bitmap · OCR"
	}
	if c.File != "" {
		where = c.File
	}
	if c.Title != "" {
		where += " · " + c.Title
	}
	codec := strings.TrimSuffix(strings.TrimPrefix(c.Codec, "hdmv_"), "_subtitle")
	return fmt.Sprintf("%-10s %-7s %-13s %s", truncate(langName(c.Lang), 10), truncate(codec, 7), kind, truncate(where, 26))
}

// langName is the English name of a language code, or the code itself.
func langName(code string) string {
	if code == "" || code == "und" {
		return "unknown"
	}
	tag, err := language.Parse(code)
	if err != nil {
		return code
	}
	return cmp.Or(display.English.Languages().Name(tag), code)
}

// expandHome turns a leading ~ into the home directory.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// subLoadMsg reports a subtitle load the scene screen started: progress, then the end.
type subLoadMsg struct {
	to          *detailScreen
	done, total int64
	finished    bool
	err         error
	ch          <-chan subLoadMsg
}

// loadSubs starts reading ref into the film's scenes and reports as it goes.
// Progress is dropped while a report waits, so the last send never blocks.
func (d *detailScreen) loadSubs(ref, name string) tea.Cmd {
	subs, film := d.subs, d.film
	ch := make(chan subLoadMsg, 2)
	d.subName, d.subNote = name, "loading "+name+"…"
	go func() {
		err := subs.Load(context.Background(), film, ref, func(done, total int64) {
			if len(ch) == 0 {
				ch <- subLoadMsg{to: d, done: done, total: total}
			}
		})
		ch <- subLoadMsg{to: d, finished: true, err: err}
		close(ch)
	}()
	return waitSub(ch)
}

func waitSub(ch <-chan subLoadMsg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return nil
		}
		m.ch = ch
		return m
	}
}

// onSubLoad shows a load's progress, and reads the scenes again when it ends.
func (d *detailScreen) onSubLoad(msg subLoadMsg) tea.Cmd {
	if msg.to != d {
		return nil
	}
	if !msg.finished {
		d.subNote = fmt.Sprintf("loading %s… %d/%d", d.subName, msg.done, msg.total)
		return waitSub(msg.ch)
	}
	if msg.err != nil {
		d.subNote = errorText.Render("subtitles: " + msg.err.Error())
		return nil
	}
	d.subNote = d.subName + " subtitles loaded"
	if d.load == nil {
		return nil
	}
	d.keep = d.hit().Scene.ID
	return d.load()
}
