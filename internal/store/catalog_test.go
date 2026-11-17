package store_test

import (
	"context"
	"reflect"
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
	if err := db.QueryRow(`pragma user_version`).Scan(&v); err != nil || v != 5 {
		t.Errorf("user_version = %d, %v; want 5", v, err)
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
	if want := (core.Film{ID: "a", Title: "Citizen Kane", Year: 1941, Bundle: "/films/kane", Duration: time.Hour}); !reflect.DeepEqual(got[0], want) {
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
		{"blank text in one film lists it", core.SearchQuery{Film: "a", Limit: 10}, 3, false},
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

func sceneIDs(t *testing.T, s *store.Store, film core.FilmID) []core.SceneID {
	t.Helper()
	var ids []core.SceneID
	for _, sc := range collect(t, s.Scenes(context.Background(), film)) {
		ids = append(ids, sc.ID)
	}
	return ids
}

func hitCount(t *testing.T, s *store.Store, text string) int {
	t.Helper()
	hits, err := s.Search(context.Background(), core.SearchQuery{Text: text, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	return len(hits)
}

func TestStore_SetText(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "one"), line(2, "two"))
	ids := sceneIDs(t, s, "a")

	if err := s.SetText(ctx, "a", map[core.SceneID]string{ids[0]: "rosebud", ids[1]: "kane"}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "rosebud") != 1 || hitCount(t, s, "kane") != 1 {
		t.Error("new text is not searchable")
	}
	if hitCount(t, s, "one") != 0 {
		t.Error("old text still matches")
	}
	if got := sceneIDs(t, s, "a"); len(got) != 2 || got[0] != ids[0] || got[1] != ids[1] {
		t.Errorf("scene IDs changed: %v, want %v", got, ids)
	}

	if err := s.SetText(ctx, "a", map[core.SceneID]string{ids[0]: ""}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "rosebud") != 0 || hitCount(t, s, "kane") != 1 {
		t.Error("an empty string should clear only that scene")
	}
}

func TestStore_SetTextIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "one"))
	put(t, s, core.Film{ID: "b"}, line(1, "other"))
	a, b := sceneIDs(t, s, "a"), sceneIDs(t, s, "b")

	tests := []struct {
		name string
		text map[core.SceneID]string
	}{
		{"unknown scene", map[core.SceneID]string{a[0]: "changed", 9999: "x"}},
		{"another film's scene", map[core.SceneID]string{a[0]: "changed", b[0]: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := s.SetText(ctx, "a", tt.text); err == nil {
				t.Fatal("expected an error")
			}
			if hitCount(t, s, "changed") != 0 || hitCount(t, s, "one") != 1 || hitCount(t, s, "other") != 1 {
				t.Error("a failed SetText changed something")
			}
		})
	}
}

func TestStore_SearchFolds(t *testing.T) {
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "サンドイッチを食べる"), line(2, "Rosebud"), line(3, "other"))
	for _, tt := range []struct {
		name, text string
		want       int
	}{
		{"hiragana finds katakana", "さんどいっち", 1},
		{"katakana finds katakana", "サンドイッチ", 1},
		{"fullwidth finds ASCII", "ＲＯＳＥＢＵＤ", 1},
		{"case is ignored", "ROSEBUD", 1},
		{"substring inside a word", "ndoi", 0},
		{"unrelated", "ぱすた", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := hitCount(t, s, tt.text); got != tt.want {
				t.Errorf("hits for %q = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

// Trigram search cannot see terms under three characters, which Japanese
// words often are, so those must still match as substrings.
func TestStore_SearchShortTerms(t *testing.T) {
	s := newStore(t)
	put(t, s, core.Film{ID: "a"},
		line(1, "村川さんやめてください"), line(2, "100% sure"), line(3, "ab"), line(4, "a_b"), line(5, "沈めろ"))
	for _, tt := range []struct {
		name, text string
		want       int
	}{
		{"two characters", "さん", 1},
		{"two kanji", "沈め", 1},
		{"one character", "沈", 1},
		{"short with long", "さん ください", 1},
		{"short misses", "さん 沈め", 0},
		{"percent is literal", "%", 1},
		{"underscore is literal", "_", 1},
		{"backslash is literal", `\`, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := hitCount(t, s, tt.text); got != tt.want {
				t.Errorf("hits for %q = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestStore_SearchKeepsOriginalText(t *testing.T) {
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "サンドイッチ"))
	hits, err := s.Search(context.Background(), core.SearchQuery{Text: "さんどいっち", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].Text != "サンドイッチ" {
		t.Fatalf("hits = %+v, %v; want the original katakana back", hits, err)
	}
}

// A rerun of the same detection must not wipe the text read since.
func TestStore_ReplaceScenesKeepsTextOfUnchangedScenes(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "x"), line(2, "y"))
	ids := sceneIDs(t, s, "a")
	if err := s.SetText(ctx, "a", map[core.SceneID]string{ids[0]: "rosebud", ids[1]: "kane"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{ids[0]: "ばら", ids[1]: "ケーン"}); err != nil {
		t.Fatal(err)
	}

	again := line(1, "x")
	again.Text = ""
	moved := line(2, "y")
	moved.Text = ""
	moved.End += time.Second // boundary changed: the old text no longer fits
	if err := s.ReplaceScenes(ctx, "a", []store.SceneText{again, moved}); err != nil {
		t.Fatal(err)
	}

	hits, err := s.Search(ctx, core.SearchQuery{Text: "rosebud", Limit: 10})
	if err != nil || len(hits) != 1 || hits[0].Subtitle != "ばら" {
		t.Errorf("unchanged scene lost its text: %+v, %v", hits, err)
	}
	if hitCount(t, s, "ばら") != 1 {
		t.Error("kept subtitle is not searchable")
	}
	if hitCount(t, s, "kane") != 0 || hitCount(t, s, "ケーン") != 0 {
		t.Error("a scene with a new boundary kept stale text")
	}
}

func TestStore_ReplaceScenesPrefersNewText(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "old"))
	if err := s.ReplaceScenes(ctx, "a", []store.SceneText{line(1, "new")}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "old") != 0 || hitCount(t, s, "new") != 1 {
		t.Error("supplied text should replace the old text")
	}
}

func TestStore_SetTranslationIsSearchableAndKeepsOtherText(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "沈めろ"))
	id := sceneIDs(t, s, "a")[0]
	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{id: "おい"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTranslation(ctx, "a", map[core.SceneID]string{id: "Hey, sink it"}); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search(ctx, core.SearchQuery{Text: "sink", Limit: 10})
	if err != nil || len(hits) != 1 {
		t.Fatalf("english query: %+v, %v", hits, err)
	}
	if h := hits[0]; h.Translation != "Hey, sink it" || h.Text != "沈めろ" || h.Subtitle != "おい" {
		t.Errorf("hit = %+v", h)
	}
	if hitCount(t, s, "沈めろ") != 1 || hitCount(t, s, "おい") != 1 {
		t.Error("setting a translation broke the original search")
	}
	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{id: "ねえ"}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "sink") != 1 {
		t.Error("setting a subtitle dropped the translation from search")
	}
}

func TestStore_SetTranslationIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "x"))
	id := sceneIDs(t, s, "a")[0]
	if err := s.SetTranslation(ctx, "a", map[core.SceneID]string{id: "changed", id + 99: "y"}); err == nil {
		t.Error("expected an error for a scene that is not the film's")
	}
	if hitCount(t, s, "changed") != 0 {
		t.Error("a failed SetTranslation changed something")
	}
}

func TestStore_TextsListsWhatToTranslate(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "one"), line(2, "two"))
	ids := sceneIDs(t, s, "a")
	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{ids[1]: "sub"}); err != nil {
		t.Fatal(err)
	}
	got := collect(t, s.Texts(ctx, "a"))
	if len(got) != 2 || got[0].Text != "one" || got[1].Subtitle != "sub" || got[1].ID != ids[1] {
		t.Errorf("texts = %+v", got)
	}
}

func TestStore_ReplaceScenesKeepsTranslation(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "x"))
	if err := s.SetTranslation(ctx, "a", map[core.SceneID]string{sceneIDs(t, s, "a")[0]: "hello"}); err != nil {
		t.Fatal(err)
	}
	again := line(1, "x")
	again.Text = ""
	if err := s.ReplaceScenes(ctx, "a", []store.SceneText{again}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "hello") != 1 {
		t.Error("rescan dropped the translation of an unchanged scene")
	}
}

func TestStore_Media(t *testing.T) {
	s := newStore(t)
	put(t, s, core.Film{ID: "a", Title: "A", Bundle: "/a"})
	ch := []core.Chapter{{Start: 0, Title: "One"}, {Start: 90 * time.Second, Title: "Two"}}
	if err := s.SetMedia(context.Background(), "a", core.Media{Duration: 2 * time.Hour, FPS: 23.976, Chapters: ch}); err != nil {
		t.Fatal(err)
	}
	for f, err := range s.Films(context.Background()) {
		if err != nil || f.Duration != 2*time.Hour || f.FPS != 23.976 || len(f.Chapters) != 2 || f.Chapters[1] != ch[1] {
			t.Errorf("film = %+v, %v", f, err)
		}
	}
}
