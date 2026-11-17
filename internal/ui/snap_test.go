package ui_test

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tjagithub/rosebud/internal/shelf"
	"github.com/tjagithub/rosebud/internal/ui"
	"github.com/tjagithub/rosebud/internal/ui/uitest"
)

// TestSnapshot prints each screen of a real library as plain text, for
// eyeballing layout changes: ROSE_SNAP_DIR=~/Films go test ./internal/ui -run Snapshot -v
func TestSnapshot(t *testing.T) {
	dir := os.Getenv("ROSE_SNAP_DIR")
	if dir == "" {
		t.Skip("set ROSE_SNAP_DIR to a directory of .film bundles")
	}
	sh, err := shelf.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sh.Close() })
	u := uitest.New(t, ui.New(sh, 30, ui.WithLibrary(sh)))
	u.Send(tea.WindowSizeMsg{Width: 120, Height: 45})
	show := func(name string) { t.Logf("── %s ──\n%s", name, ansi.Strip(u.Screen().String())) }
	show("library")
	u.Press("down")
	u.Press("enter")
	show("scene")
	u.Type("s")
	show("picker")
	u.Press("esc")
	u.Type("t")
	show("timecode")
	u.Press("esc")
	u.Press("esc")
	u.Type("s")
	u.Type("boat")
	show("search")
}
