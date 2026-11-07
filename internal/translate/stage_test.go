package translate_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/store"
	"github.com/tjagithub/rosebud/internal/translate"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(t.TempDir() + "/rose.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return mustNew(t, db)
}

func mustNew(t *testing.T, db *sql.DB) *store.Store {
	st, err := store.New(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutFilm(t.Context(), core.Film{ID: "f", Title: "f"}); err != nil {
		t.Fatal(err)
	}
	return st
}

func scene(i int) core.Scene {
	return core.Scene{Start: time.Duration(i) * time.Second, End: time.Duration(i+1) * time.Second, Frame: "x"}
}

func TestStageTranslatesSubtitleElseText(t *testing.T) {
	st := newStore(t)
	ctx := t.Context()
	scs := []store.SceneText{{Scene: scene(0), Text: "spoken"}, {Scene: scene(1)}, {Scene: scene(2), Text: "only text"}}
	if err := st.ReplaceScenes(ctx, "f", scs); err != nil {
		t.Fatal(err)
	}
	var ids []core.SceneID
	for l, err := range st.Texts(ctx, "f") {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, l.ID)
	}
	if err := st.SetSubtitle(ctx, "f", map[core.SceneID]string{ids[0]: "subtitle"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeTr{}
	stage := translate.Stage{Store: st, Translator: f}
	if err := stage.Run(ctx, core.JobSpec{Film: "f"}, func(_, _ int64) {}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || !slices.Equal(f.calls[0], []string{"subtitle", "only text"}) {
		t.Errorf("sent %v", f.calls)
	}
	got := map[core.SceneID]string{}
	for l, err := range st.Texts(ctx, "f") {
		if err != nil {
			t.Fatal(err)
		}
		got[l.ID] = l.Translation
	}
	if got[ids[0]] != "EN:subtitle" || got[ids[1]] != "" || got[ids[2]] != "EN:only text" {
		t.Errorf("translations = %v", got)
	}
}

func TestStageErrors(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	if err := (translate.Stage{Store: st, Translator: &fakeTr{}}).Run(ctx, core.JobSpec{Film: "f"}, func(_, _ int64) {}); err == nil {
		t.Error("no text: want an error")
	}
	if err := (translate.Stage{Store: st}).Run(ctx, core.JobSpec{Film: "f"}, func(_, _ int64) {}); err == nil {
		t.Error("no translator: want an error")
	}
}
