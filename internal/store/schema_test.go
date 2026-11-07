package store

import (
	"context"
	"testing"

	"github.com/tjagithub/rosebud/internal/core"
)

// A v1 database has katakana text indexed as-is; upgrading must make it
// searchable the new way without losing the text.
func TestMigrate_V1ToV2(t *testing.T) {
	ctx := context.Background()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := apply(ctx, db, 0); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`insert into films(id, title, year, bundle, duration_ns) values ('a', '', 0, '', 0)`,
		`insert into scenes(film, start_ns, end_ns, frame, text) values ('a', 0, 1, 'f.jpg', 'サンドイッチ')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	s, err := New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search(ctx, core.SearchQuery{Text: "さんどいっち", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].Text != "サンドイッチ" {
		t.Fatalf("hits = %+v, %v; want the migrated scene", hits, err)
	}
	if err := s.SetText(ctx, "a", map[core.SceneID]string{hits[0].Scene.ID: "ラーメン"}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Search(ctx, core.SearchQuery{Text: "さんどいっち", Limit: 5}); err != nil || len(got) != 0 {
		t.Errorf("old text still matches after SetText: %+v, %v", got, err)
	}
}

// A v2 database has no subtitle column; upgrading keeps its text searchable
// and lets subtitles be added.
func TestMigrate_V2ToV3(t *testing.T) {
	ctx := context.Background()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for v := range 2 {
		if err := apply(ctx, db, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`insert into films(id, title, year, bundle, duration_ns) values ('a', '', 0, '', 0)`,
		`insert into scenes(film, start_ns, end_ns, frame, text, norm) values ('a', 0, 1, 'f.jpg', 'Rosebud', 'rosebud')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search(ctx, core.SearchQuery{Text: "rosebud", Limit: 5})
	if err != nil || len(hits) != 1 || hits[0].Subtitle != "" {
		t.Fatalf("hits = %+v, %v; want the old scene with no subtitle", hits, err)
	}
	if err := s.SetSubtitle(ctx, "a", map[core.SceneID]string{hits[0].Scene.ID: "sled"}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Search(ctx, core.SearchQuery{Text: "rosebud sled", Limit: 5}); err != nil || len(got) != 1 {
		t.Errorf("after SetSubtitle: %+v, %v", got, err)
	}
}
