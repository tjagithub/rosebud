package ui_test

import (
	"strings"
	"testing"
	"time"

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
	tui.Screen().Contains("scene 7", "1:05 – 1:11", "6s", "字幕", "I'll kill you!", "level 2")
	tui.Press("esc")
	tui.Screen().Contains("search:")
	tui.Screen().Lacks("scene 7")
}

func TestEnterWithNoHitsDoesNothing(t *testing.T) {
	tui, _ := start(t, &fakeSearch{})
	tui.Press("enter")
	tui.Screen().Contains("search:", "level 1")
}

func TestDetailUploadsItsFrameInItsOwnSlot(t *testing.T) {
	dir := bundleWithFrames(t, "a.jpg")
	f := &fakeSearch{hits: []core.Hit{withFrame(hit(1, time.Second, "one"), "frames/a.jpg")}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithBundle(dir)))
	tui.Type("x")
	before := len(tui.Raw)
	tui.Press("enter")
	if len(tui.Raw) != before+1 {
		t.Fatalf("detail must upload its own frame, raw %d -> %d", before, len(tui.Raw))
	}
	if !strings.Contains(tui.Raw[before], "i=8") {
		t.Errorf("detail frame must use slot 8: %q", tui.Raw[before])
	}
	tui.Screen().Contains(string(rune(0x10EEEE)))
}

func TestDetailStepsBetweenHitsWithinBounds(t *testing.T) {
	hits := []core.Hit{hit(1, time.Second, "first"), hit(2, 2*time.Second, "second"), hit(3, 3*time.Second, "third")}
	tui, _ := start(t, &fakeSearch{hits: hits})
	tui.Type("x")
	tui.Press("enter")
	tui.Press("left")
	tui.Screen().Contains("scene 1", "1/3", "first")
	tui.Press("right")
	tui.Press("right")
	tui.Press("right")
	tui.Screen().Contains("scene 3", "3/3", "third")
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
	if len(tui.Raw) != n+1 {
		t.Errorf("stepping must upload the next frame, raw %d -> %d", n, len(tui.Raw))
	}
}

func TestDetailFillsTheBodyAndSizesImageToIt(t *testing.T) {
	dir := bundleWithFrames(t, "a.jpg")
	f := &fakeSearch{hits: []core.Hit{withFrame(hit(1, time.Second, "one"), "frames/a.jpg")}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithBundle(dir)))
	tui.Type("x")
	tui.Press("enter")
	lines := strings.Split(tui.Screen().String(), "\n")
	if len(lines) != 24 {
		t.Fatalf("screen is %d lines, want 24", len(lines))
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.Contains(lines[1], "1") {
		t.Errorf("scene strip should open the screen: %q", lines[:3])
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
	tui.Type("s")
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
	tui.Type("s")
	tui.Press("esc")
	tui.Screen().Contains("scene 1")
}
