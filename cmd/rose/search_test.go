package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
)

func searchBundle(t *testing.T) string {
	t.Helper()
	bundle := filepath.Join(t.TempDir(), "a.film")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(bundle, "rose.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	st, err := store.New(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutFilm(t.Context(), core.Film{ID: "a", Title: "a", Bundle: bundle}); err != nil {
		t.Fatal(err)
	}
	scs := []store.SceneText{
		{Scene: core.Scene{Start: 65 * time.Second, End: 70 * time.Second, Frame: "f"}, Text: "hello there"},
		{Scene: core.Scene{Start: 3725 * time.Second, End: 3730 * time.Second, Frame: "f"}},
	}
	if err := st.ReplaceScenes(t.Context(), "a", scs); err != nil {
		t.Fatal(err)
	}
	var id core.SceneID
	for l, err := range st.Texts(t.Context(), "a") {
		if err != nil {
			t.Fatal(err)
		}
		id = l.ID
	}
	if err := st.SetTranslation(t.Context(), "a", map[core.SceneID]string{id: "I'll kill you!"}); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestRun_SearchPrintsTimeAndTranslation(t *testing.T) {
	bundle := searchBundle(t)
	var out, errOut bytes.Buffer
	if err := runSearch(t.Context(), []string{"--bundle", bundle, "kill"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"1:02:05", "I'll kill you!"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("want one line, got:\n%s", got)
	}
}

func TestRun_SearchFormatsTimeAndRejectsBadInput(t *testing.T) {
	bundle := searchBundle(t)
	for name, args := range map[string][]string{
		"no query":  {"--bundle", bundle},
		"no bundle": {"kill"},
		"bad limit": {"--bundle", bundle, "--limit", "0", "kill"},
		"no db":     {"--bundle", t.TempDir(), "kill"},
	} {
		var out, errOut bytes.Buffer
		if err := runSearch(t.Context(), args, &out, &errOut); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestClipCutsLongTextByRunes(t *testing.T) {
	if got := clip("あいうえお", 3); got != "あい…" {
		t.Errorf("clip = %q", got)
	}
	if got := clip("abc", 3); got != "abc" {
		t.Errorf("clip = %q", got)
	}
}

func TestRun_TUIRejectsBadInput(t *testing.T) {
	for name, args := range map[string][]string{
		"no bundle": {},
		"extra arg": {"--bundle", t.TempDir(), "x"},
		"no db":     {"--bundle", t.TempDir()},
		"bad limit": {"--bundle", t.TempDir(), "--limit", "0"},
	} {
		var out, errOut bytes.Buffer
		if err := runTUI(t.Context(), args, &out, &errOut); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
