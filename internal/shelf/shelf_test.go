package shelf_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/shelf"
	"github.com/tjagithub/rosebud/internal/store"
)

// bundle makes NAME.film under dir holding one film with the given lines.
func bundle(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	b := filepath.Join(dir, name+".film")
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(b, "rose.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	st, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	id := core.FilmID(name)
	if err := st.PutFilm(context.Background(), core.Film{ID: id, Title: name, Bundle: b}); err != nil {
		t.Fatal(err)
	}
	var sc []store.SceneText
	for i, l := range lines {
		sc = append(sc, store.SceneText{Scene: core.Scene{Start: time.Duration(i) * time.Second, End: time.Duration(i+1) * time.Second, Frame: "frames/x.jpg"}, Text: l})
	}
	if err := st.ReplaceScenes(context.Background(), id, sc); err != nil {
		t.Fatal(err)
	}
	return b
}

func films(t *testing.T, s *shelf.Shelf) []string {
	t.Helper()
	var out []string
	for f, err := range s.Films(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, f.Title)
	}
	return out
}

func TestShelfListsEveryBundleUnderADirectory(t *testing.T) {
	dir := t.TempDir()
	bundle(t, dir, "beta", "hello")
	bundle(t, dir, "alpha", "hello")
	if err := os.MkdirAll(filepath.Join(dir, "not-a-bundle"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := shelf.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if got := films(t, s); len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("films = %v, want [alpha beta]", got)
	}
}

func TestShelfTakesASingleBundleToo(t *testing.T) {
	b := bundle(t, t.TempDir(), "solo", "hello")
	s, err := shelf.Open(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if got := films(t, s); len(got) != 1 || got[0] != "solo" {
		t.Fatalf("films = %v", got)
	}
}

func TestShelfSearchesOneFilmOrAll(t *testing.T) {
	dir := t.TempDir()
	bundle(t, dir, "alpha", "rosebud one", "other")
	bundle(t, dir, "beta", "rosebud two")
	s, err := shelf.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	all, err := s.Search(ctx, core.SearchQuery{Text: "rosebud", Limit: 10})
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %v, %v", all, err)
	}
	one, err := s.Search(ctx, core.SearchQuery{Film: "beta", Text: "rosebud", Limit: 10})
	if err != nil || len(one) != 1 || one[0].Scene.Film != "beta" {
		t.Fatalf("one = %v, %v", one, err)
	}
	whole, err := s.Search(ctx, core.SearchQuery{Film: "alpha", Limit: 10})
	if err != nil || len(whole) != 2 {
		t.Fatalf("whole film = %v, %v", whole, err)
	}
	if _, err := s.Search(ctx, core.SearchQuery{Film: "missing", Text: "x", Limit: 10}); err == nil {
		t.Error("search in an unknown film succeeded, want an error")
	}
	n := 0
	for _, err := range s.Scenes(ctx, "alpha") {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 2 {
		t.Errorf("scenes of alpha = %d, want 2", n)
	}
}

func TestShelfWithNothingToShowIsAnError(t *testing.T) {
	if _, err := shelf.Open(context.Background(), t.TempDir()); err == nil {
		t.Fatal("an empty directory opened, want an error")
	}
}
