package jobs_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"go.uber.org/goleak"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/jobs"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type recorder struct {
	mu     sync.Mutex
	events []core.Event
}

func (r *recorder) Publish(e core.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) states(id core.JobID) []core.JobState {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []core.JobState
	for _, e := range r.events {
		if c, ok := e.(core.JobChanged); ok && c.Info.ID == id {
			out = append(out, c.Info.State)
		}
	}
	return out
}

func (r *recorder) progress(id core.JobID) []core.JobProgress {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []core.JobProgress
	for _, e := range r.events {
		if p, ok := e.(core.JobProgress); ok && p.Job == id {
			out = append(out, p)
		}
	}
	return out
}

func start(t *testing.T, r *jobs.Runner, stage string, n int) core.JobID {
	t.Helper()
	id, err := r.Start(core.JobSpec{Stage: stage, Film: core.FilmID(fmt.Sprint(n))})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func info(t *testing.T, r *jobs.Runner, id core.JobID) core.JobInfo {
	t.Helper()
	for _, j := range r.List() {
		if j.ID == id {
			return j
		}
	}
	t.Fatalf("job %q not listed", id)
	return core.JobInfo{}
}

func countState(r *jobs.Runner, s core.JobState) (n int) {
	for _, j := range r.List() {
		if j.State == s {
			n++
		}
	}
	return n
}

func blockOn(release <-chan struct{}) jobs.Stage {
	return func(ctx context.Context, _ core.JobSpec, _ func(int64, int64)) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func instant(context.Context, core.JobSpec, func(int64, int64)) error { return nil }

func TestRunner_Start(t *testing.T) {
	t.Run("runs a job through queued, running, done", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			rec := &recorder{}
			r := jobs.New(rec, jobs.WithStage("scenes", func(_ context.Context, _ core.JobSpec, progress func(int64, int64)) error {
				progress(1, 2)
				progress(2, 2)
				return nil
			}))
			defer r.Close()

			id := start(t, r, "scenes", 1)
			synctest.Wait()

			want := []core.JobState{core.JobQueued, core.JobRunning, core.JobDone}
			if got := rec.states(id); !slices.Equal(got, want) {
				t.Errorf("states = %v, want %v", got, want)
			}
			if j := info(t, r, id); j.State != core.JobDone || j.Done != 2 || j.Total != 2 {
				t.Errorf("info = %+v", j)
			}
			if p := rec.progress(id); len(p) != 2 || p[1].Done != 2 {
				t.Errorf("progress events = %+v", p)
			}
		})
	})

	t.Run("reports progress while running", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			rec := &recorder{}
			release := make(chan struct{})
			r := jobs.New(rec, jobs.WithStage("s", func(ctx context.Context, spec core.JobSpec, progress func(int64, int64)) error {
				progress(5, 10)
				return blockOn(release)(ctx, spec, progress)
			}))
			defer r.Close()

			id := start(t, r, "s", 1)
			synctest.Wait()

			if j := info(t, r, id); j.State != core.JobRunning || j.Done != 5 || j.Total != 10 {
				t.Errorf("info = %+v", j)
			}
			want := []core.JobProgress{{Job: id, Done: 5, Total: 10}}
			if got := rec.progress(id); !slices.Equal(got, want) {
				t.Errorf("progress = %+v, want %+v", got, want)
			}
		})
	})

	t.Run("rejects an unknown stage", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := jobs.New(&recorder{})
			defer r.Close()
			if _, err := r.Start(core.JobSpec{Stage: "nope"}); !errors.Is(err, jobs.ErrUnknownStage) {
				t.Errorf("err = %v, want ErrUnknownStage", err)
			}
			if n := len(r.List()); n != 0 {
				t.Errorf("%d jobs listed, want 0", n)
			}
		})
	})

	t.Run("rejects work when the queue is full", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := jobs.New(&recorder{}, jobs.WithWorkers(1), jobs.WithQueue(1),
				jobs.WithStage("s", blockOn(make(chan struct{}))))
			defer r.Close()

			start(t, r, "s", 1)
			synctest.Wait()
			start(t, r, "s", 2)
			if _, err := r.Start(core.JobSpec{Stage: "s", Film: "3"}); !errors.Is(err, jobs.ErrQueueFull) {
				t.Errorf("err = %v, want ErrQueueFull", err)
			}
			if n := len(r.List()); n != 2 {
				t.Errorf("%d jobs listed, want 2", n)
			}
		})
	})

	t.Run("returns the active job for an identical spec", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			release := make(chan struct{})
			r := jobs.New(&recorder{}, jobs.WithStage("s", blockOn(release)))
			defer r.Close()

			spec := core.JobSpec{Stage: "s", Film: "f", Params: map[string]string{"lang": "en"}}
			a, _ := r.Start(spec)
			b, _ := r.Start(spec)
			if a != b || len(r.List()) != 1 {
				t.Fatalf("a=%q b=%q jobs=%d, want one shared job", a, b, len(r.List()))
			}
			other := core.JobSpec{Stage: "s", Film: "f", Params: map[string]string{"lang": "fr"}}
			if c, _ := r.Start(other); c == a {
				t.Error("different params shared a job")
			}

			close(release)
			synctest.Wait()
			if d, _ := r.Start(spec); d == a {
				t.Error("finished job was reused")
			}
		})
	})

	t.Run("copies the caller's params", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := jobs.New(&recorder{}, jobs.WithStage("s", blockOn(make(chan struct{}))))
			defer r.Close()

			params := map[string]string{"lang": "en"}
			id, _ := r.Start(core.JobSpec{Stage: "s", Params: params})
			params["lang"] = "de"
			if got := info(t, r, id).Spec.Params["lang"]; got != "en" {
				t.Errorf("lang = %q, want en", got)
			}
		})
	})
}

func TestRunner_Concurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var running, peak atomic.Int32
		release := make(chan struct{})
		r := jobs.New(&recorder{}, jobs.WithWorkers(2),
			jobs.WithStage("s", func(ctx context.Context, spec core.JobSpec, progress func(int64, int64)) error {
				n := running.Add(1)
				defer running.Add(-1)
				for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
				}
				return blockOn(release)(ctx, spec, progress)
			}))
		defer r.Close()

		for i := range 5 {
			start(t, r, "s", i)
		}
		synctest.Wait()
		if run, queued := countState(r, core.JobRunning), countState(r, core.JobQueued); run != 2 || queued != 3 {
			t.Fatalf("running=%d queued=%d, want 2 and 3", run, queued)
		}

		close(release)
		synctest.Wait()
		if done := countState(r, core.JobDone); done != 5 {
			t.Errorf("done = %d, want 5", done)
		}
		if p := peak.Load(); p != 2 {
			t.Errorf("peak concurrency = %d, want 2", p)
		}
	})
}
