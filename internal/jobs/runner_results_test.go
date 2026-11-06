package jobs_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/jobs"
)

func TestRunner_Failure(t *testing.T) {
	tests := []struct {
		name  string
		stage jobs.Stage
		want  string
	}{
		{"error", func(context.Context, core.JobSpec, func(int64, int64)) error { return errors.New("boom") }, "boom"},
		{"panic", func(context.Context, core.JobSpec, func(int64, int64)) error { panic("kaboom") }, "kaboom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				rec := &recorder{}
				r := jobs.New(rec, jobs.WithWorkers(1), jobs.WithStage("bad", tt.stage), jobs.WithStage("ok", instant))
				defer r.Close()

				bad := start(t, r, "bad", 1)
				synctest.Wait()
				want := []core.JobState{core.JobQueued, core.JobRunning, core.JobFailed}
				if got := rec.states(bad); !slices.Equal(got, want) {
					t.Errorf("states = %v, want %v", got, want)
				}
				if j := info(t, r, bad); !strings.Contains(j.Err, tt.want) {
					t.Errorf("Err = %q, want it to contain %q", j.Err, tt.want)
				}

				ok := start(t, r, "ok", 2)
				synctest.Wait()
				if j := info(t, r, ok); j.State != core.JobDone {
					t.Errorf("worker did not survive: follow-up job is %v", j.State)
				}
			})
		})
	}
}

func TestRunner_History(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := jobs.New(&recorder{}, jobs.WithHistory(2), jobs.WithStage("ok", instant))
		defer r.Close()

		var ids []core.JobID
		for i := range 5 {
			ids = append(ids, start(t, r, "ok", i))
			synctest.Wait()
		}
		var got []core.JobID
		for _, j := range r.List() {
			got = append(got, j.ID)
		}
		if want := ids[3:]; !slices.Equal(got, want) {
			t.Errorf("listed %v, want the newest two %v", got, want)
		}
	})
}
