package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/engine"
	"github.com/tjagithub/rosebud/internal/store"
)

var _ engine.Catalog = (*store.Store)(nil)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := store.New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func put(t *testing.T, s *store.Store, f core.Film, lines ...store.SceneText) {
	t.Helper()
	ctx := context.Background()
	if err := s.PutFilm(ctx, f); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceScenes(ctx, f.ID, lines); err != nil {
		t.Fatal(err)
	}
}

func line(startSec int, text string) store.SceneText {
	start := time.Duration(startSec) * time.Second
	return store.SceneText{
		Scene: core.Scene{Start: start, End: start + time.Second, Frame: "frames/" + text + ".jpg"},
		Text:  text,
	}
}

func collect[T any](t *testing.T, seq func(func(T, error) bool)) []T {
	t.Helper()
	var out []T
	for v, err := range seq {
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func TestNew(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for range 2 {
		if _, err := store.New(context.Background(), db); err != nil {
			t.Fatalf("New: %v", err)
		}
	}
	var v int
	if err := db.QueryRow(`pragma user_version`).Scan(&v); err != nil || v != 1 {
		t.Errorf("user_version = %d, %v; want 1", v, err)
	}
}

func TestStore_Films(t *testing.T) {
	s := newStore(t)
	put(t, s, core.Film{ID: "b", Title: "Zulu", Year: 1964, Bundle: "/films/zulu", Duration: 2 * time.Hour})
	put(t, s, core.Film{ID: "a", Title: "Citizen Kane", Year: 1941, Bundle: "/films/kane", Duration: time.Hour})

	got := collect(t, s.Films(context.Background()))
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("films = %+v, want kane then zulu", got)
	}
	if want := (core.Film{ID: "a", Title: "Citizen Kane", Year: 1941, Bundle: "/films/kane", Duration: time.Hour}); got[0] != want {
		t.Errorf("film = %+v, want %+v", got[0], want)
	}

	if err := s.PutFilm(context.Background(), core.Film{ID: "a", Title: "Kane", Year: 1941}); err != nil {
		t.Fatal(err)
	}
	if got := collect(t, s.Films(context.Background())); len(got) != 2 || got[0].Title != "Kane" {
		t.Errorf("after upsert = %+v", got)
	}
}

func TestStore_Scenes(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	put(t, s, core.Film{ID: "a"}, line(20, "later"), line(10, "earlier"))
	put(t, s, core.Film{ID: "b"}, line(1, "other"))

	got := collect(t, s.Scenes(ctx, "a"))
	if len(got) != 2 || got[0].Frame != "frames/earlier.jpg" || got[0].Film != "a" || got[0].ID == 0 {
		t.Fatalf("scenes = %+v, want earlier first with film and id set", got)
	}

	if err := s.ReplaceScenes(ctx, "a", []store.SceneText{line(5, "fresh")}); err != nil {
		t.Fatal(err)
	}
	if got := collect(t, s.Scenes(ctx, "a")); len(got) != 1 || got[0].Frame != "frames/fresh.jpg" {
		t.Errorf("after replace = %+v", got)
	}
	if got := collect(t, s.Scenes(ctx, "b")); len(got) != 1 {
		t.Errorf("other film changed: %+v", got)
	}

	if err := s.ReplaceScenes(ctx, "missing", []store.SceneText{line(1, "x")}); err == nil {
		t.Error("ReplaceScenes for an unknown film succeeded, want an error")
	}
}

func TestStore_Search(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	put(t, s, core.Film{ID: "a"},
		line(1, "rosebud"), line(2, "rosebud rosebud rosebud and a sled"), line(3, "no match here"))
	put(t, s, core.Film{ID: "b"}, line(1, "rosebud again"))

	tests := []struct {
		name    string
		q       core.SearchQuery
		want    int
		wantErr bool
	}{
		{"all films", core.SearchQuery{Text: "rosebud", Limit: 10}, 3, false},
		{"one film", core.SearchQuery{Film: "a", Text: "rosebud", Limit: 10}, 2, false},
		{"limit", core.SearchQuery{Text: "rosebud", Limit: 1}, 1, false},
		{"no match is empty", core.SearchQuery{Text: "xyzzy", Limit: 10}, 0, false},
		{"punctuation is not syntax", core.SearchQuery{Text: `rose"bud' OR (`, Limit: 10}, 0, false},
		{"blank text", core.SearchQuery{Text: "  ", Limit: 10}, 0, true},
		{"non-positive limit", core.SearchQuery{Text: "rosebud"}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Search(ctx, tt.q)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != tt.want {
				t.Errorf("got %d hits, want %d: %+v", len(got), tt.want, got)
			}
		})
	}

	got, _ := s.Search(ctx, core.SearchQuery{Film: "a", Text: "rosebud", Limit: 10})
	if len(got) != 2 || got[0].Text != "rosebud rosebud rosebud and a sled" && got[0].Text != "rosebud" {
		t.Fatalf("hits = %+v", got)
	}
	if got[0].Score < got[1].Score {
		t.Errorf("best first violated: %v then %v", got[0].Score, got[1].Score)
	}
	if h := got[0]; h.Scene.ID == 0 || h.Scene.Film != "a" || h.Scene.Frame == "" {
		t.Errorf("hit scene not populated: %+v", h)
	}
}

func TestStore_CancelledContext(t *testing.T) {
	s := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Search(ctx, core.SearchQuery{Text: "x", Limit: 1}); err == nil {
		t.Error("Search with a cancelled ctx succeeded")
	}
	if err := s.PutFilm(ctx, core.Film{ID: "a"}); err == nil {
		t.Error("PutFilm with a cancelled ctx succeeded")
	}
}

func TestStore_DeleteFilm(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	put(t, s, core.Film{ID: "a"}, line(1, "rosebud"))
	put(t, s, core.Film{ID: "b"}, line(1, "rosebud"))

	if err := s.DeleteFilm(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if got := collect(t, s.Scenes(ctx, "a")); len(got) != 0 {
		t.Errorf("scenes survived delete: %+v", got)
	}
	hits, err := s.Search(ctx, core.SearchQuery{Text: "rosebud", Limit: 10})
	if err != nil || len(hits) != 1 || hits[0].Scene.Film != "b" {
		t.Errorf("search after delete = %+v, %v; want only film b", hits, err)
	}
}

// SQLite reuses rowids, so a stale index entry would match the wrong scene.
func TestStore_ReplaceScenesDropsOldText(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	put(t, s, core.Film{ID: "a"}, line(1, "rosebud"))

	if err := s.ReplaceScenes(ctx, "a", []store.SceneText{line(1, "kane")}); err != nil {
		t.Fatal(err)
	}
	if hits, err := s.Search(ctx, core.SearchQuery{Text: "rosebud", Limit: 10}); err != nil || len(hits) != 0 {
		t.Errorf("old text still matches: %+v, %v", hits, err)
	}
	if hits, err := s.Search(ctx, core.SearchQuery{Text: "kane", Limit: 10}); err != nil || len(hits) != 1 {
		t.Errorf("new text lost: %+v, %v", hits, err)
	}
}
