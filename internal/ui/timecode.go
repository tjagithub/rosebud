package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// defaultFPS counts the frame part of a timecode until films record their own rate.
const defaultFPS = 24

// timecode is the overlay for going to a time as HH:MM:SS:FF. Tab moves
// between the four parts, up and down scroll one, and digits fill it in.
type timecode struct {
	owner *detailScreen
	fps   int
	part  [4]int // hours, minutes, seconds, frames
	at    int    // the part with focus
	typed int    // digits typed into it so far
}

func newTimecode(d *detailScreen, fps int) *timecode {
	t := &timecode{owner: d, fps: fps}
	s := d.hit().Scene.Start
	t.part = [4]int{int(s / time.Hour), int(s / time.Minute % 60), int(s / time.Second % 60), int(s % time.Second * time.Duration(fps) / time.Second)}
	return t
}

func (t *timecode) Init() tea.Cmd { return nil }

func (t *timecode) Title() string { return "go to time" }

var timeKeys = []key.Binding{
	key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next part")),
	key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll")),
	key.NewBinding(key.WithKeys("0"), key.WithHelp("0-9", "type")),
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "go")),
	backKey,
}

func (t *timecode) ShortHelp() []key.Binding  { return timeKeys }
func (t *timecode) FullHelp() [][]key.Binding { return [][]key.Binding{timeKeys} }

// limit is one more than the largest value of part i.
func (t *timecode) limit(i int) int {
	switch i {
	case 0:
		return 100
	case 3:
		return t.fps
	}
	return 60
}

// time is the timecode as a duration.
func (t *timecode) time() time.Duration {
	return time.Duration(t.part[0])*time.Hour + time.Duration(t.part[1])*time.Minute +
		time.Duration(t.part[2])*time.Second + time.Duration(t.part[3])*time.Second/time.Duration(t.fps)
}

func (t *timecode) Update(msg tea.Msg) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return t, nil
	}
	switch s := k.String(); {
	case s == "enter":
		return t, tea.Batch(Close(), func() tea.Msg { return jumpMsg{To: t.owner, I: t.owner.sceneAt(t.time())} })
	case s == "tab" || s == "right":
		t.move(1)
	case s == "shift+tab" || s == "left":
		t.move(-1)
	case s == "up":
		t.part[t.at] = (t.part[t.at] + 1) % t.limit(t.at)
		t.typed = 0
	case s == "down":
		t.part[t.at] = (t.part[t.at] + t.limit(t.at) - 1) % t.limit(t.at)
		t.typed = 0
	case len(s) == 1 && s[0] >= '0' && s[0] <= '9':
		t.digit(int(s[0] - '0'))
	}
	return t, nil
}

func (t *timecode) move(by int) {
	t.at = (t.at + by + len(t.part)) % len(t.part)
	t.typed = 0
}

// digit types d into the focused part; a second digit finishes it and moves on.
func (t *timecode) digit(d int) {
	if t.typed == 0 {
		t.part[t.at] = d
		t.typed = 1
		return
	}
	t.part[t.at] = min(t.part[t.at]*10+d, t.limit(t.at)-1)
	if t.at < len(t.part)-1 {
		t.move(1)
	} else {
		t.typed = 0
	}
}

func (t *timecode) View() string {
	parts := make([]string, len(t.part))
	for i, v := range t.part {
		parts[i] = fmt.Sprintf("%02d", v)
		if i == t.at {
			parts[i] = selected.Render(parts[i])
		}
	}
	line := strings.Join(parts, ":")
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2).Render(line + "\n" + dim.Render(fmt.Sprintf("HH:MM:SS:FF at %d fps", t.fps)))
}
