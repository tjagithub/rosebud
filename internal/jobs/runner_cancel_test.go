package jobs_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/jobs"
)

func TestRunner_Cancel(t *testing.T) {
	t.Run("running job ends cancelled, not failed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			rec := &recorder{}
			r := jobs.New(rec, jobs.WithStage("s", blockOn(make(chan struct{}))))
			defer r.Close()

			id := start(t, r, "s", 1)
			synctest.Wait()
			if err := r.Cancel(id); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()

			want := []core.JobState{core.JobQueued, core.JobRunning, core.JobCancelled}
			if got := rec.states(id); !slices.Equal(got, want) {
				t.Errorf("states = %v, want %v", got, want)
			}
		})
	})

	t.Run("any error after cancel counts as cancelled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := jobs.New(&recorder{}, jobs.WithStage("s", func(ctx context.Context, _ core.JobSpec, _ func(int64, int64)) error {
				<-ctx.Done()
				return errors.New("signal: killed")
			}))
			defer r.Close()

			id := start(t, r, "s", 1)
			synctest.Wait()
			_ = r.Cancel(id)
			synctest.Wait()
			if j := info(t, r, id); j.State != core.JobCancelled {
				t.Errorf("state = %v, want cancelled", j.State)
			}
		})
	})

	t.Run("queued job is cancelled at once and never runs", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var ran atomic.Int32
			release := make(chan struct{})
			rec := &recorder{}
			r := jobs.New(rec, jobs.WithWorkers(1),
				jobs.WithStage("s", func(ctx context.Context, spec core.JobSpec, progress func(int64, int64)) error {
					ran.Add(1)
					return blockOn(release)(ctx, spec, progress)
				}))
			defer r.Close()

			start(t, r, "s", 1)
			synctest.Wait()
			queued := start(t, r, "s", 2)
			if err := r.Cancel(queued); err != nil {
				t.Fatal(err)
			}
			if j := info(t, r, queued); j.State != core.JobCancelled {
				t.Fatalf("state = %v, want cancelled immediately", j.State)
			}

			close(release)
			synctest.Wait()
			want := []core.JobState{core.JobQueued, core.JobCancelled}
			if got := rec.states(queued); !slices.Equal(got, want) {
				t.Errorf("states = %v, want %v", got, want)
			}
			if n := ran.Load(); n != 1 {
				t.Errorf("stage ran %d times, want 1", n)
			}
		})
	})

	t.Run("unknown job", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := jobs.New(&recorder{})
			defer r.Close()
			if err := r.Cancel("nope"); !errors.Is(err, jobs.ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
		})
	})

	t.Run("finished job is left alone", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := jobs.New(&recorder{}, jobs.WithStage("ok", instant))
			defer r.Close()

			id := start(t, r, "ok", 1)
			synctest.Wait()
			if err := r.Cancel(id); err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if j := info(t, r, id); j.State != core.JobDone {
				t.Errorf("state = %v, want done", j.State)
			}
		})
	})
}

func TestRunner_Close(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := jobs.New(&recorder{}, jobs.WithWorkers(1), jobs.WithStage("s", blockOn(make(chan struct{}))))

		running := start(t, r, "s", 1)
		synctest.Wait()
		queued := start(t, r, "s", 2)

		r.Close()
		r.Close()

		for _, id := range []core.JobID{running, queued} {
			if j := info(t, r, id); j.State != core.JobCancelled {
				t.Errorf("job %q state = %v, want cancelled", id, j.State)
			}
		}
		if _, err := r.Start(core.JobSpec{Stage: "s", Film: "3"}); !errors.Is(err, jobs.ErrClosed) {
			t.Errorf("err = %v, want ErrClosed", err)
		}
	})
}
