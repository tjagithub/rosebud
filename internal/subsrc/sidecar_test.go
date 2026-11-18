package subsrc_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tjagithub/rosebud/internal/subsrc"
)

func TestSidecarsFindsSubtitleFilesNamedAfterTheVideo(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Spirited Away [x].webm")
	for _, n := range []string{
		"Spirited Away [x].webm",
		"Spirited Away [x].srt",           // no language
		"Spirited Away [x].en.vtt",        // two-letter code
		"Spirited Away [x].ja.forced.ass", // code and a title
		"Spirited Away [x].sdh.fre.srt",   // title first, bibliographic code
		"Spirited Away [x].en.txt",        // not subtitles
		"Spirited Away [x] extras.en.srt", // another video's
		"Spirited Away.en.srt",            // another stem
	} {
		write(t, filepath.Join(dir, n), "")
	}
	if err := os.Mkdir(filepath.Join(dir, "Spirited Away [x].de.srt"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := subsrc.Sidecars(video)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]subsrc.Track{
		"Spirited Away [x].en.vtt":        {Codec: "webvtt", Lang: "eng"},
		"Spirited Away [x].ja.forced.ass": {Codec: "ass", Lang: "jpn", Title: "forced"},
		"Spirited Away [x].sdh.fre.srt":   {Codec: "subrip", Lang: "fra", Title: "sdh"},
		"Spirited Away [x].srt":           {Codec: "subrip"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sidecars, want %d: %+v", len(got), len(want), got)
	}
	for _, tr := range got {
		w, ok := want[filepath.Base(tr.Path)]
		w.Path = filepath.Join(dir, filepath.Base(tr.Path))
		if !ok || tr != w || tr.Kind() != subsrc.Text {
			t.Errorf("got %+v, want %+v", tr, w)
		}
	}
}

func TestFileIsAnySubtitleFile(t *testing.T) {
	dir := t.TempDir()
	srt := filepath.Join(dir, "Some Name.en.srt")
	write(t, srt, "")
	if tr, err := subsrc.File(srt); err != nil || tr != (subsrc.Track{Codec: "subrip", Path: srt}) {
		t.Errorf("got %+v, %v", tr, err)
	}
	txt := filepath.Join(dir, "notes.txt")
	write(t, txt, "")
	for _, bad := range []string{txt, filepath.Join(dir, "gone.srt"), dir + "/", ""} {
		if _, err := subsrc.File(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestSidecarsOfAMissingFolderIsAnError(t *testing.T) {
	if _, err := subsrc.Sidecars(filepath.Join(t.TempDir(), "gone", "f.mkv")); err == nil {
		t.Error("want an error for a folder that is not there")
	}
	if _, err := subsrc.Sidecars(""); err == nil {
		t.Error("want an error for an empty path")
	}
}
