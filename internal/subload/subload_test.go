package subload_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/subload"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSourceIsTheRecordedOneElseTheVideoBesideTheBundle(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "Film.film")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Film.en.srt", "Film.webm", "Film.nfo", "Other.mkv"} {
		write(t, filepath.Join(dir, n), "")
	}
	if got, err := subload.Source(core.Film{Bundle: bundle, Source: "/x/rec.mkv"}); err != nil || got != "/x/rec.mkv" {
		t.Errorf("recorded: %q, %v", got, err)
	}
	if got, err := subload.Source(core.Film{Bundle: bundle}); err != nil || got != filepath.Join(dir, "Film.webm") {
		t.Errorf("guessed: %q, %v", got, err)
	}
	if _, err := subload.Source(core.Film{Bundle: filepath.Join(dir, "Gone.film")}); err == nil || !strings.Contains(err.Error(), "rose scenes") {
		t.Errorf("want an error saying to run rose scenes, got %v", err)
	}
}

// fakeExec lists one bitmap and one text track and renders any text track as srt.
func fakeExec(srt string) func(context.Context, string, ...string) ([]byte, error) {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		switch {
		case name == "ffmpeg":
			return []byte(srt), nil
		case slices.Contains(args, "s"):
			return []byte("2,hdmv_pgs_subtitle,jpn,\n3,subrip,eng,SDH\n"), nil
		}
		return []byte("jpn\n"), nil
	}
}

func TestChoicesListTracksThenFiles(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Film.mkv")
	write(t, video, "")
	write(t, filepath.Join(dir, "Film.fr.vtt"), "")
	l := subload.Loader{Exec: fakeExec(""), Timeout: time.Minute}
	got, err := l.Choices(t.Context(), core.Film{Source: video})
	if err != nil {
		t.Fatal(err)
	}
	want := []core.SubtitleChoice{
		{Ref: "0", Lang: "jpn", Codec: "hdmv_pgs_subtitle", Bitmap: true},
		{Ref: "1", Lang: "eng", Codec: "subrip", Title: "SDH"},
		{Ref: filepath.Join(dir, "Film.fr.vtt"), Lang: "fra", Codec: "webvtt", File: "Film.fr.vtt"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestLoadReadsTheChoiceIntoTheFilmsScenes(t *testing.T) {
	dir := t.TempDir()
	bundle := filepath.Join(dir, "Film.film")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(dir, "Film.mkv")
	write(t, video, "")
	db, err := store.Open(filepath.Join(bundle, "rose.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st, err := store.New(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	film := core.Film{ID: "Film", Title: "Film", Bundle: bundle, Source: video}
	if err := st.PutFilm(t.Context(), film); err != nil {
		t.Fatal(err)
	}
	scenes := []store.SceneText{{Scene: core.Scene{Start: 0, End: 10 * time.Second, Frame: "frames/a.jpg"}}}
	if err := st.ReplaceScenes(t.Context(), film.ID, scenes); err != nil {
		t.Fatal(err)
	}
	l := subload.Loader{
		Exec: fakeExec("1\n00:00:01,000 --> 00:00:02,000\nhello there\n"), Timeout: time.Minute,
		Store: func(context.Context, core.FilmID) (*store.Store, error) { return st, nil },
	}
	var calls int
	if err := l.Load(t.Context(), film, "1", func(int64, int64) { calls++ }); err != nil {
		t.Fatal(err)
	}
	hits, err := st.Search(t.Context(), core.SearchQuery{Film: film.ID, Text: "hello", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].Subtitle != "hello there" {
		t.Errorf("hits %+v, %v", hits, err)
	}
	if err := l.Load(t.Context(), film, "0", nil); err == nil || !strings.Contains(err.Error(), "OCR") {
		t.Errorf("a bitmap track with no OCR reader must say so, got %v", err)
	}
}
