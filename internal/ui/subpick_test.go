package ui_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/ui"
	"github.com/tjagithub/rosebud/internal/ui/uitest"
)

type fakeSubs struct {
	choices []core.SubtitleChoice
	loaded  []string
	err     error
}

func (f *fakeSubs) Choices(context.Context, core.Film) ([]core.SubtitleChoice, error) {
	return f.choices, nil
}

func (f *fakeSubs) Load(_ context.Context, _ core.Film, ref string, progress func(int64, int64)) error {
	f.loaded = append(f.loaded, ref)
	progress(1, 2)
	return f.err
}

func subsFilm(t *testing.T) (*uitest.TUI, *fakeSubs, *fakeSearch) {
	t.Helper()
	subs := &fakeSubs{choices: []core.SubtitleChoice{
		{Ref: "1", Lang: "eng", Codec: "subrip", Title: "SDH"},
		{Ref: "0", Lang: "jpn", Codec: "hdmv_pgs_subtitle", Bitmap: true},
		{Ref: "/films/Film.fr.vtt", Lang: "fra", Codec: "webvtt", File: "Film.fr.vtt"},
	}}
	f := &fakeSearch{hits: []core.Hit{hit(1, 0, "a line")}}
	lib := fakeLib{films: []core.Film{{ID: "a", Title: "Sonatine"}}, scenes: map[core.FilmID]int{"a": 1}}
	tui := uitest.New(t, ui.New(f, 10, ui.WithLibrary(lib), ui.WithSubtitles(subs)))
	tui.Press("enter")
	return tui, subs, f
}

func TestLKeyListsTheSubtitlesAndLoadsTheOneChosen(t *testing.T) {
	tui, subs, f := subsFilm(t)
	before := len(f.queries)
	tui.Type("l")
	tui.Screen().Contains("subtitles", "English", "SDH", "Japanese", "OCR", "French", "Film.fr.vtt", "other file…")
	tui.Press("enter")
	if len(subs.loaded) != 1 || subs.loaded[0] != "1" {
		t.Fatalf("loaded %v, want track 1", subs.loaded)
	}
	tui.Screen().Lacks("other file…")
	tui.Screen().Contains("English subtitles loaded")
	if len(f.queries) <= before {
		t.Error("the scenes must be read again after loading")
	}
}

func TestBitmapSubtitlesAskBeforeTheSlowOCR(t *testing.T) {
	tui, subs, _ := subsFilm(t)
	tui.Type("l")
	tui.Press("down")
	tui.Press("enter")
	if len(subs.loaded) != 0 {
		t.Fatalf("OCR started without asking: %v", subs.loaded)
	}
	tui.Screen().Contains("enter again")
	tui.Press("enter")
	if len(subs.loaded) != 1 || subs.loaded[0] != "0" {
		t.Errorf("loaded %v, want track 0", subs.loaded)
	}
}

func TestOtherFileTakesATypedPath(t *testing.T) {
	tui, subs, _ := subsFilm(t)
	tui.Type("l")
	for range 3 {
		tui.Press("down")
	}
	tui.Press("enter")
	tui.Screen().Contains("path")
	tui.Type("~/subs/x.srt")
	tui.Press("enter")
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, "subs", "x.srt"); len(subs.loaded) != 1 || subs.loaded[0] != want {
		t.Errorf("loaded %v, want %s", subs.loaded, want)
	}
}

func TestALoadErrorIsShown(t *testing.T) {
	tui, subs, _ := subsFilm(t)
	subs.err = errors.New("bad cue file")
	tui.Type("l")
	tui.Press("enter")
	tui.Screen().Contains("bad cue file")
}

func TestLWithoutSubtitlesDoesNothing(t *testing.T) {
	tui := filmWithMedia(t)
	tui.Type("l")
	tui.Screen().Lacks("other file…")
}
