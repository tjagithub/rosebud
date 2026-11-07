package store_test

import (
	"context"
	"testing"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
)

func hitsFor(t *testing.T, s *store.Store, text string) []core.Hit {
	t.Helper()
	hits, err := s.Search(context.Background(), core.SearchQuery{Text: text, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func TestStore_SetSubtitle(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "ちょっと"), line(2, "other"))
	ids := sceneIDs(t, s, "a")

	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{ids[0]: "Wait a moment", ids[1]: "Sled"}); err != nil {
		t.Fatal(err)
	}
	hits := hitsFor(t, s, "moment")
	if len(hits) != 1 || hits[0].Scene.ID != ids[0] || hits[0].Text != "ちょっと" || hits[0].Subtitle != "Wait a moment" {
		t.Fatalf("hits = %+v, want the scene with its text and subtitle", hits)
	}
	if hitCount(t, s, "ちょっと") != 1 {
		t.Error("spoken text stopped matching")
	}
	if got := sceneIDs(t, s, "a"); got[0] != ids[0] || got[1] != ids[1] {
		t.Errorf("scene IDs changed: %v, want %v", got, ids)
	}

	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{ids[0]: ""}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "moment") != 0 || hitCount(t, s, "ちょっと") != 1 || hitCount(t, s, "sled") != 1 {
		t.Error("an empty string should clear only that scene's subtitle")
	}
}

func TestStore_TextAndSubtitleStaySearchableTogether(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "first"))
	id := sceneIDs(t, s, "a")[0]

	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{id: "rosebud"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetText(ctx, "a", map[core.SceneID]string{id: "sled"}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "rosebud") != 1 || hitCount(t, s, "sled") != 1 || hitCount(t, s, "first") != 0 {
		t.Error("SetText lost the subtitle, or kept the old text")
	}
	if hitCount(t, s, "rosebud sled") != 1 {
		t.Error("a query with one word from each column should match")
	}
	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{id: "snow"}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "sled") != 1 || hitCount(t, s, "snow") != 1 || hitCount(t, s, "rosebud") != 0 {
		t.Error("SetSubtitle lost the text, or kept the old subtitle")
	}
}

func TestStore_SubtitleShortTerms(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "x"))
	id := sceneIDs(t, s, "a")[0]
	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{id: "Hey, sink it"}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "it") != 1 {
		t.Error("a two-letter term should match inside the subtitle")
	}
}

func TestStore_SetSubtitleIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "one"))
	put(t, s, core.Film{ID: "b"}, line(1, "other"))
	a, b := sceneIDs(t, s, "a"), sceneIDs(t, s, "b")
	for name, sub := range map[string]map[core.SceneID]string{
		"unknown scene":        {a[0]: "changed", 9999: "x"},
		"another film's scene": {a[0]: "changed", b[0]: "x"},
	} {
		if err := s.SetSubtitle(ctx, "a", sub); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if hitCount(t, s, "changed") != 0 {
			t.Errorf("%s: a failed SetSubtitle changed something", name)
		}
	}
}

func TestStore_ReplaceScenesDropsOldSubtitles(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	put(t, s, core.Film{ID: "a"}, line(1, "one"))
	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{sceneIDs(t, s, "a")[0]: "caption"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceScenes(ctx, "a", []store.SceneText{line(1, "two")}); err != nil {
		t.Fatal(err)
	}
	if hitCount(t, s, "caption") != 0 {
		t.Error("a subtitle outlived its scene")
	}
}
