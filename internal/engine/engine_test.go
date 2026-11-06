package engine_test

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"
	"testing/synctest"

	"go.uber.org/goleak"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/engine"
	"github.com/tjagithub/rosebud/internal/jobs"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

var _ core.Client = (*engine.Engine)(nil)

// fakeCatalog serves fixed data and records the last search it saw.
type fakeCatalog struct {
	films  []core.Film
	scenes []core.Scene
	hits   []core.Hit
	last   core.SearchQuery
}

func (c *fakeCatalog) Films(context.Context) iter.Seq2[core.Film, error] {
	return seq(c.films)
}

func (c *fakeCatalog) Scenes(_ context.Context, film core.FilmID) iter.Seq2[core.Scene, error] {
	var out []core.Scene
	for _, s := range c.scenes {
		if s.Film == film {
			out = append(out, s)
		}
	}
	return seq(out)
}

func (c *fakeCatalog) Search(_ context.Context, q core.SearchQuery) ([]core.Hit, error) {
	c.last = q
	return c.hits, nil
}

func seq[T any](items []T) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for _, it := range items {
			if !yield(it, nil) {
				return
			}
		}
	}
}

func okStage(context.Context, core.JobSpec, func(int64, int64)) error { return nil }

func collect[T any](t *testing.T, s iter.Seq2[T, error]) []T {
	t.Helper()
	var out []T
	for v, err := range s {
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func TestEngine_Catalog(t *testing.T) {
	cat := &fakeCatalog{
		films:  []core.Film{{ID: "a"}, {ID: "b"}},
		scenes: []core.Scene{{ID: 1, Film: "a"}, {ID: 2, Film: "b"}, {ID: 3, Film: "a"}},
	}
	e := engine.New(cat)
	defer func() { _ = e.Close() }()
	ctx := context.Background()

	if got := collect(t, e.Films(ctx)); len(got) != 2 || got[1].ID != "b" {
		t.Errorf("films = %+v", got)
	}
	if got := collect(t, e.Scenes(ctx, "a")); len(got) != 2 || got[1].ID != 3 {
		t.Errorf("scenes = %+v", got)
	}
}

func TestEngine_CatalogCancelled(t *testing.T) {
	e := engine.New(&fakeCatalog{films: []core.Film{{ID: "a"}}})
	defer func() { _ = e.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, err := range e.Films(ctx) {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		return
	}
	t.Error("cancelled Films yielded nothing, want an error")
}

func TestEngine_Search(t *testing.T) {
	hits := []core.Hit{{Text: "a"}, {Text: "b"}, {Text: "c"}}
	tests := []struct {
		name    string
		limit   int
		want    int
		wantErr bool
	}{
		{"truncates to the limit", 2, 2, false},
		{"limit above results", 10, 3, false},
		{"zero limit is rejected", 0, 0, true},
		{"negative limit is rejected", -1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := &fakeCatalog{hits: hits}
			e := engine.New(cat)
			defer func() { _ = e.Close() }()

			got, err := e.Search(context.Background(), core.SearchQuery{Text: "x", Limit: tt.limit})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != tt.want {
				t.Errorf("got %d hits, want %d", len(got), tt.want)
			}
		})
	}
}

func TestEngine_Jobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := engine.New(&fakeCatalog{}, engine.WithStage("scenes", okStage))
		defer func() { _ = e.Close() }()
		ctx := context.Background()

		events, err := e.Subscribe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		id, err := e.StartJob(ctx, core.JobSpec{Stage: "scenes", Film: "a"})
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()

		var states []core.JobState
	read:
		for {
			synctest.Wait()
			select {
			case ev := <-events:
				if c, ok := ev.(core.JobChanged); ok && c.Info.ID == id {
					states = append(states, c.Info.State)
				}
			default:
				break read
			}
		}
		want := []core.JobState{core.JobQueued, core.JobRunning, core.JobDone}
		if !slices.Equal(states, want) {
			t.Errorf("event states = %v, want %v", states, want)
		}

		list, err := e.Jobs(ctx)
		if err != nil || len(list) != 1 || list[0].State != core.JobDone {
			t.Errorf("Jobs = %+v, %v", list, err)
		}
	})
}

func TestEngine_JobErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := engine.New(&fakeCatalog{})
		defer func() { _ = e.Close() }()
		ctx := context.Background()

		if _, err := e.StartJob(ctx, core.JobSpec{Stage: "nope"}); !errors.Is(err, jobs.ErrUnknownStage) {
			t.Errorf("StartJob err = %v, want ErrUnknownStage", err)
		}
		if err := e.CancelJob(ctx, "nope"); !errors.Is(err, jobs.ErrNotFound) {
			t.Errorf("CancelJob err = %v, want ErrNotFound", err)
		}
	})
}

func TestEngine_Close(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := engine.New(&fakeCatalog{}, engine.WithStage("s", okStage))
		events, err := e.Subscribe(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		if err := e.Close(); err != nil {
			t.Fatal(err)
		}
		if err := e.Close(); err != nil {
			t.Errorf("second Close = %v, want nil", err)
		}
		synctest.Wait()
		if _, ok := <-events; ok {
			t.Error("subscriber channel still open after Close")
		}
		if _, err := e.StartJob(context.Background(), core.JobSpec{Stage: "s"}); !errors.Is(err, jobs.ErrClosed) {
			t.Errorf("StartJob after Close = %v, want ErrClosed", err)
		}
		if _, err := e.Subscribe(context.Background()); err == nil {
			t.Error("Subscribe after Close succeeded, want an error")
		}
	})
}
