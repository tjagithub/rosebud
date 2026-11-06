package jobs_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"

	"pgregory.net/rapid"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/jobs"
)

type stepKind int

const (
	stepStart stepKind = iota
	stepCancel
	stepFinish
)

// step is one action: start a film's job, cancel the nth job, or let a
// running job for a film end (failing when fail is set).
type step struct {
	kind stepKind
	film int
	pick int
	fail bool
}

func genSteps() *rapid.Generator[[]step] {
	one := rapid.Custom(func(t *rapid.T) step {
		return step{
			kind: rapid.SampledFrom([]stepKind{stepStart, stepStart, stepCancel, stepFinish}).Draw(t, "kind"),
			film: rapid.IntRange(0, 3).Draw(t, "film"),
			pick: rapid.IntRange(0, 50).Draw(t, "pick"),
			fail: rapid.Bool().Draw(t, "fail"),
		}
	})
	return rapid.SliceOfN(one, 0, 80)
}

// gatedStage blocks until the film's gate yields a verdict (true = fail).
func gatedStage(gates map[core.FilmID]chan bool) jobs.Stage {
	return func(ctx context.Context, spec core.JobSpec, _ func(int64, int64)) error {
		select {
		case fail := <-gates[spec.Film]:
			if fail {
				return errors.New("stage failed")
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// validNext reports whether a job may move from state a to state b.
func validNext(a, b core.JobState) bool {
	switch a {
	case core.JobUnknown:
		return b == core.JobQueued
	case core.JobQueued:
		return b == core.JobRunning || b == core.JobCancelled
	case core.JobRunning:
		return b == core.JobDone || b == core.JobFailed || b == core.JobCancelled
	}
	return false
}

// snapshotCheck returns a complaint when a List result breaks a limit.
func snapshotCheck(list []core.JobInfo, workers, history int) string {
	var running, finished int
	seen := map[core.JobID]bool{}
	active := map[core.FilmID]int{}
	for _, j := range list {
		if seen[j.ID] {
			return fmt.Sprintf("job %s listed twice", j.ID)
		}
		seen[j.ID] = true
		switch j.State {
		case core.JobRunning:
			running++
			active[j.Spec.Film]++
		case core.JobQueued:
			active[j.Spec.Film]++
		default:
			finished++
		}
	}
	for film, n := range active {
		if n > 1 {
			return fmt.Sprintf("%d active jobs share film %s", n, film)
		}
	}
	if running > workers {
		return fmt.Sprintf("%d running with %d workers", running, workers)
	}
	if finished > history {
		return fmt.Sprintf("%d finished jobs kept, history is %d", finished, history)
	}
	return ""
}

func TestRunner_Properties(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		steps := genSteps().Draw(rt, "steps")
		workers := rapid.IntRange(1, 3).Draw(rt, "workers")
		history := rapid.IntRange(1, 4).Draw(rt, "history")

		var complaints []string
		var rec *recorder
		var ids []core.JobID
		var final []core.JobInfo
		synctest.Test(t, func(t *testing.T) {
			gates := map[core.FilmID]chan bool{}
			for i := range 4 {
				gates[core.FilmID(fmt.Sprint(i))] = make(chan bool, 64)
			}
			rec = &recorder{}
			r := jobs.New(rec, jobs.WithWorkers(workers), jobs.WithHistory(history),
				jobs.WithStage("s", gatedStage(gates)))
			defer r.Close()

			for _, s := range steps {
				film := core.FilmID(fmt.Sprint(s.film))
				switch s.kind {
				case stepStart:
					if id, err := r.Start(core.JobSpec{Stage: "s", Film: film}); err == nil {
						ids = append(ids, id)
					}
				case stepCancel:
					if len(ids) > 0 {
						_ = r.Cancel(ids[s.pick%len(ids)])
					}
				case stepFinish:
					select {
					case gates[film] <- s.fail:
					default:
					}
				}
				synctest.Wait()
				if c := snapshotCheck(r.List(), workers, history); c != "" {
					complaints = append(complaints, c)
				}
			}
			r.Close()
			final = r.List()
		})

		for _, c := range complaints {
			rt.Fatalf("%s", c)
		}
		for _, j := range final {
			if j.State == core.JobQueued || j.State == core.JobRunning {
				rt.Fatalf("job %s still %v after Close", j.ID, j.State)
			}
		}
		checkTransitions(rt, rec, ids)
	})
}

func checkTransitions(rt *rapid.T, rec *recorder, ids []core.JobID) {
	for _, id := range ids {
		prev := core.JobUnknown
		for _, s := range rec.states(id) {
			if !validNext(prev, s) {
				rt.Fatalf("job %s moved %v -> %v (states %v)", id, prev, s, rec.states(id))
			}
			prev = s
		}
		if prev == core.JobQueued || prev == core.JobRunning {
			rt.Fatalf("job %s never reached a final state (states %v)", id, rec.states(id))
		}
	}
}
