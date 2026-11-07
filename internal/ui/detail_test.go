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
