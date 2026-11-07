package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/translate"
)

type upper struct{}

func (upper) Translate(_ context.Context, l []string) ([]string, error) {
	out := make([]string, len(l))
	for i, s := range l {
		out[i] = "EN " + s
	}
	return out, nil
}

func TestRun_TranslateRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.mkv")
	for name, args := range map[string][]string{
		"no file":        {},
		"no scenes yet":  {file},
		"zero timeout":   {"--timeout", "0s", file},
		"empty language": {"--lang", "", file},
	} {
		var out, errOut bytes.Buffer
		if err := runTranslate(t.Context(), args, &out, &errOut, nil); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestRun_TranslateWritesTranslations(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	file := filepath.Join(dir, "a.mkv")
	bundle := filepath.Join(dir, "a.film")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(bundle, "rose.db"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.New(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutFilm(t.Context(), core.Film{ID: "a", Title: "a", Bundle: bundle}); err != nil {
		t.Fatal(err)
	}
	sc := store.SceneText{Scene: core.Scene{End: time.Second, Frame: "f"}, Text: "はい"}
	if err := st.ReplaceScenes(t.Context(), "a", []store.SceneText{sc}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	var out, errOut bytes.Buffer
	newTr := func(string, string, time.Duration) translate.Translator { return upper{} }
	if err := runTranslate(t.Context(), []string{file}, &out, &errOut, newTr); err != nil {
		t.Fatalf("%v\n%s", err, errOut.String())
	}
	db, err = store.Open(filepath.Join(bundle, "rose.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	st, _ = store.New(t.Context(), db)
	hits, err := st.Search(t.Context(), core.SearchQuery{Text: "EN はい", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].Translation != "EN はい" {
		t.Errorf("hits %v, err %v", hits, err)
	}
}
