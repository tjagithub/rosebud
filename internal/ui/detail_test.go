package ui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/ui"
	"github.com/tjagithub/rosebud/internal/ui/uitest"
)

func TestEnterOpensSceneDetailAndEscReturns(t *testing.T) {
	h := hit(7, 65*time.Second, "I'll kill you!")
	h.Scene.End = 71 * time.Second
	tui, _ := start(t, &fakeSearch{hits: []core.Hit{h}})
	tui.Type("x")
	tui.Press("enter")
	tui.Screen().Contains("scene 7", "1:05 – 1:11", "6s", "字幕", "I'll kill you!", "search › scene 7")
	tui.Press("esc")
	tui.Screen().Contains("search:")
	tui.Screen().Lacks("scene 7")
}

func TestSceneTextSetsOriginalBesideTranslation(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: []core.Hit{hit(7, 65*time.Second, "I'll kill you!")}})
	tui.Type("x")
	tui.Press("enter")
	for _, line := range strings.Split(tui.Screen().String(), "\n") {
		if i, j := strings.Index(line, "subtitle"), strings.Index(line, "english"); i >= 0 && j > i {
			return
		}
	}
	t.Errorf("want the subtitle and english labels side by side:\n%s", tui.Screen())
}

func TestEnterWithNoHitsDoesNothing(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	tui.Press("enter")
	tui.Screen().Contains("search:")
	tui.Screen().Lacks("›")
}

func TestDetailUploadsItsFrameInItsOwnSlot(t *testing.T) {
	dir := bundleWithFrames(t, "a.jpg")
	f := &fakeSearch{hits: []core.Hit{withFrame(hit(1, time.Second, "one"), "frames/a.jpg")}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithBundle(dir)))
	tui.Type("x")
	before := len(tui.Raw)
	tui.Press("enter")
	var slots []string
	for _, r := range tui.Raw[before:] {
		slots = append(slots, r[:min(len(r), 40)])
	}
	all := strings.Join(tui.Raw[before:], "")
	if !strings.Contains(all, "i=8") || !strings.Contains(all, "i=22") {
		t.Errorf("detail must upload its frame in slot 8 and its thumbnail in slot 22: %q", slots)
	}
	tui.Screen().Contains(string(rune(0x10EEEE)))
}

func TestCommaAndPeriodStepOneAngleBracketsStepFive(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: manyHits(12)})
	tui.Type("x")
	tui.Press("enter")
	tui.Type(".")
	tui.Screen().Contains("scene 2", "2 / 12")
	tui.Type(">")
	tui.Screen().Contains("scene 7", "7 / 12")
	tui.Type(",")
	tui.Screen().Contains("scene 6", "6 / 12")
	tui.Type("<")
	tui.Screen().Contains("scene 1", "1 / 12")
	tui.Screen().Contains(",/.", "</>")
}

func TestBigStepsStopAtTheEnds(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: manyHits(7)})
	tui.Type("x")
	tui.Press("enter")
	tui.Type("<")
	tui.Screen().Contains("scene 1", "1 / 7")
	tui.Type(">>")
	tui.Screen().Contains("scene 7", "7 / 7")
}

func TestDetailStepsBetweenHitsWithinBounds(t *testing.T) {
	hits := []core.Hit{hit(1, time.Second, "first"), hit(2, 2*time.Second, "second"), hit(3, 3*time.Second, "third")}
	tui, _ := start(t, &fakeSearch{hits: hits})
	tui.Type("x")
	tui.Press("enter")
	tui.Press("left")
	tui.Screen().Contains("scene 1", "1 / 3", "first")
	tui.Press("right")
	tui.Press("right")
	tui.Press("right")
	tui.Screen().Contains("scene 3", "3 / 3", "third")
	tui.Screen().Lacks("first")
}

func TestSteppingLoadsTheNextFrame(t *testing.T) {
	dir := bundleWithFrames(t, "a.jpg", "b.jpg")
	f := &fakeSearch{hits: []core.Hit{
		withFrame(hit(1, time.Second, "one"), "frames/a.jpg"),
		withFrame(hit(2, 2*time.Second, "two"), "frames/b.jpg"),
	}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithBundle(dir)))
	tui.Type("x")
	tui.Press("enter")
	n := len(tui.Raw)
	tui.Press("right")
	if !strings.Contains(strings.Join(tui.Raw[n:], ""), "i=8") {
		t.Errorf("stepping must upload the next frame, raw %d -> %d", n, len(tui.Raw))
	}
}

func TestDetailFillsTheBodyAndSizesImageToIt(t *testing.T) {
	dir := bundleWithFrames(t, "a.jpg")
	f := &fakeSearch{hits: []core.Hit{withFrame(hit(1, time.Second, "one"), "frames/a.jpg")}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithBundle(dir)))
	tui.Send(tea.WindowSizeMsg{Width: 100, Height: 40})
	tui.Type("x")
	tui.Press("enter")
	lines := strings.Split(tui.Screen().String(), "\n")
	if len(lines) != 40 {
		t.Fatalf("screen is %d lines, want 40", len(lines))
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasPrefix(lines[8], "╰") {
		t.Errorf("a boxed thumbnail strip, 7 rows inside, should open the screen: %q", lines[:9])
	}
	n := strings.Count(tui.Screen().String(), string(rune(0x10EEEE)))
	if n < 40 {
		t.Errorf("image uses %d cells, want it to fill the area", n)
	}
}

func TestFilterNarrowsDialogueAndKeepsOnEnter(t *testing.T) {
	hits := []core.Hit{hit(1, time.Second, "alpha"), hit(2, 2*time.Second, "beta"), hit(3, 3*time.Second, "alpine")}
	tui, _ := start(t, &fakeSearch{hits: hits})
	tui.Type("x")
	tui.Press("enter")
	tui.Type("/")
	tui.Type("alp")
	tui.Press("enter")
	tui.Screen().Contains("filter: alp", "scene 1")
	tui.Press("right")
	tui.Screen().Contains("scene 3")
	tui.Type("S")
	tui.Screen().Lacks("filter: alp")
	tui.Press("esc")
	tui.Screen().Contains("search:")
}

func TestEscWhileTypingFilterStaysOnScene(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: []core.Hit{hit(1, time.Second, "one")}})
	tui.Type("x")
	tui.Press("enter")
	tui.Type("/")
	tui.Press("esc")
	tui.Screen().Contains("scene 1")
}

func manyHits(n int) []core.Hit {
	var hs []core.Hit
	for i := 1; i <= n; i++ {
		hs = append(hs, hit(i, time.Duration(i)*time.Second, fmt.Sprintf("line%02d", i)))
	}
	return hs
}

func TestArrowsScrollTheFocusedBox(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: manyHits(20)})
	tui.Type("x")
	tui.Press("enter")
	tui.Screen().Lacks("0:20")
	for range 30 {
		tui.Press("down")
	}
	tui.Screen().Contains("0:20", "scene 1")
}

func TestTextBoxShowsWhenItHasMoreAboveOrBelow(t *testing.T) {
	h := hit(1, time.Second, "short")
	h.Translation = strings.Repeat("a long line of translation ", 40)
	tui, _ := start(t, &fakeSearch{hits: []core.Hit{h}})
	tui.Type("x")
	tui.Press("enter")
	tui.Screen().Contains("↓ more")
	tui.Press("tab")
	tui.Press("down")
	tui.Screen().Contains("↑↓ more")
	for range 200 {
		tui.Press("down")
	}
	tui.Screen().Contains("↑ more")
	tui.Screen().Lacks("↓ more", "↑↓")
}

func TestTextBoxFittingItsTextHasNoMoreMark(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: []core.Hit{hit(1, time.Second, "short")}})
	tui.Type("x")
	tui.Press("enter")
	tui.Screen().Lacks("more")
}

func TestWheelScrollsTheBoxUnderThePointer(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: manyHits(20)})
	tui.Type("x")
	tui.Press("enter")
	for range 10 {
		tui.Send(tea.MouseWheelMsg{X: 5, Y: 20, Button: tea.MouseWheelDown})
	}
	tui.Screen().Contains("0:20")
}

func TestClickFocusesBoxAndTabCycles(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: manyHits(3)})
	tui.Type("x")
	tui.Press("enter")
	tui.Click(60, 20)
	tui.Press("tab")
	tui.Screen().Contains("scene 1")
}

func TestSOpensScenePickerAndEnterJumps(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: manyHits(20)})
	tui.Type("x")
	tui.Press("enter")
	tui.Type("s")
	tui.Screen().Contains("length", "pick scene")
	tui.Screen().Lacks("chapter") // the film has no chapters
	tui.Type("14")
	tui.Press("enter")
	tui.Screen().Contains("scene 14")
	tui.Screen().Lacks("length")
}

func TestPickerEscCancels(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: manyHits(5)})
	tui.Type("x")
	tui.Press("enter")
	tui.Type("s")
	tui.Press("esc")
	tui.Screen().Contains("scene 1")
	tui.Screen().Lacks("length")
}

func TestTGoesToATimecode(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: manyHits(20)})
	tui.Type("x")
	tui.Press("enter")
	tui.Type("t")
	tui.Screen().Contains("go to time", "00:00:01:00")
	tui.Press("tab")
	tui.Press("tab")
	for range 6 {
		tui.Press("up")
	}
	tui.Screen().Contains("00:00:07:00")
	tui.Press("enter")
	tui.Screen().Contains("scene 7")
}

func TestTimecodeDigitsFillPartsInOrder(t *testing.T) {
	tui, _ := start(t, &fakeSearch{hits: manyHits(20)})
	tui.Type("x")
	tui.Press("enter")
	tui.Type("t")
	tui.Type("00" + "00" + "12")
	tui.Screen().Contains("00:00:12:00")
	tui.Press("enter")
	tui.Screen().Contains("scene 12")
}

func filmWithMedia(t *testing.T) *uitest.TUI {
	f := &fakeSearch{hits: manyHits(20)}
	lib := fakeLib{films: []core.Film{{
		ID: "a", Title: "Sonatine", FPS: 25,
		Chapters: []core.Chapter{{Start: 0, Title: "Opening"}, {Start: 10 * time.Second, Title: "Beach"}},
	}}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithLibrary(lib)))
	tui.Press("enter")
	return tui
}

func TestTimecodeUsesTheFilmsFrameRate(t *testing.T) {
	tui := filmWithMedia(t)
	tui.Type("t")
	tui.Screen().Contains("25 fps")
}

func TestSearchSaysWhenNothingMatches(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	tui.Screen().Contains("Type to search")
	tui.Type("zzz")
	tui.Screen().Contains("No scenes match", "“zzz”", "0 scenes")
}

func TestPickerShowsChapterNames(t *testing.T) {
	tui := filmWithMedia(t)
	tui.Type("s")
	tui.Screen().Contains("Opening", "Beach")
}
