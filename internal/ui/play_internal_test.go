package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/player"
)

type fakePlayback struct {
	ch      chan player.Frame
	stopped bool
}

func (f *fakePlayback) Out() <-chan player.Frame { return f.ch }
func (f *fakePlayback) Stop() {
	if !f.stopped {
		f.stopped = true
		close(f.ch)
	}
}
func (f *fakePlayback) Wait() error { return nil }

func TestSourceFileSitsBesideTheBundle(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"Hana-Bi.film", "Hana-Bi.mp4", "Other.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := sourceFile(filepath.Join(dir, "Hana-Bi.film")); got != filepath.Join(dir, "Hana-Bi.mp4") {
		t.Errorf("got %q", got)
	}
	if got := sourceFile(filepath.Join(dir, "Missing.film")); got != "" {
		t.Errorf("got %q for a film with no file", got)
	}
}

func TestFramePixelsAreEvenAndKeepTheBoxShape(t *testing.T) {
	w, h := framePixels(size{cols: 100, rows: 20})
	if w != 640 || h != 256 || w%2 != 0 || h%2 != 0 {
		t.Errorf("got %dx%d", w, h)
	}
}

func playScreen(t *testing.T, fp *fakePlayback, from *time.Duration) *detailScreen {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"f.film", "f.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hits := []core.Hit{
		{Scene: core.Scene{ID: 1, Start: 0}},
		{Scene: core.Scene{ID: 2, Start: 10 * time.Second}},
		{Scene: core.Scene{ID: 3, Start: 20 * time.Second}},
	}
	d := newDetail(hits, 1, filepath.Join(dir, "f.film"))
	d.fr.box = size{cols: 40, rows: 10}
	d.start = func(file string, at time.Duration, w, h int) (playback, error) {
		*from = at
		return fp, nil
	}
	return d
}

func press(d *detailScreen, k string) tea.Cmd {
	_, cmd := d.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	return cmd
}

func TestPlayStartsAtTheSceneAndStopLandsOnTheSceneReached(t *testing.T) {
	fp := &fakePlayback{ch: make(chan player.Frame, 1)}
	var from time.Duration
	d := playScreen(t, fp, &from)
	if cmd := press(d, "p"); cmd == nil || d.play == nil {
		t.Fatal("p must start playback")
	}
	if from != 10*time.Second || !d.Capturing() {
		t.Errorf("started at %v, capturing %v", from, d.Capturing())
	}
	d.Update(playFrameMsg{Seq: d.play.seq, At: 25 * time.Second, Data: "x", Cols: 40, Rows: 10})
	if d.fr.shown == "" {
		t.Error("a frame on show must fill the picture box")
	}
	d.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if d.play != nil || !fp.stopped || d.i != 2 {
		t.Errorf("playing %v, stopped %v, scene %d: want stopped on scene 2", d.play != nil, fp.stopped, d.i)
	}
}

func TestPlayEndsWhenTheFilmDoes(t *testing.T) {
	fp := &fakePlayback{ch: make(chan player.Frame)}
	var from time.Duration
	d := playScreen(t, fp, &from)
	press(d, "p")
	d.Update(playFrameMsg{Seq: d.play.seq})
	if d.play != nil || !fp.stopped {
		t.Error("closed playback must end the play state")
	}
}

func TestStalePlayFramesAreIgnored(t *testing.T) {
	fp := &fakePlayback{ch: make(chan player.Frame, 1)}
	var from time.Duration
	d := playScreen(t, fp, &from)
	press(d, "p")
	if _, cmd := d.Update(playFrameMsg{Seq: d.play.seq + 5, At: time.Second, Data: "x", Cols: 1, Rows: 1}); cmd != nil {
		t.Error("a frame of another playback must do nothing")
	}
}

func TestPlayNeedsTheFilmFile(t *testing.T) {
	fp := &fakePlayback{ch: make(chan player.Frame)}
	var from time.Duration
	d := playScreen(t, fp, &from)
	d.fr.bundle = filepath.Join(t.TempDir(), "none.film")
	if press(d, "p"); d.play != nil {
		t.Error("no file, no playback")
	}
}
