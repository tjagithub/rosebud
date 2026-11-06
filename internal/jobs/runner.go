// Package jobs runs stages on a bounded worker pool and reports their state.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"sync"

	"github.com/tjagithub/rosebud/internal/core"
)

var (
	ErrClosed       = errors.New("jobs: runner closed")
	ErrQueueFull    = errors.New("jobs: queue full")
	ErrNotFound     = errors.New("jobs: no such job")
	ErrUnknownStage = errors.New("jobs: unknown stage")
)

// Publisher receives events. Publish must not block or call back into Runner.
type Publisher interface{ Publish(core.Event) }

// Stage does the work for one job. It must return promptly once ctx is
// cancelled and be safe to run again.
type Stage func(ctx context.Context, spec core.JobSpec, progress func(done, total int64)) error

// Option configures a Runner.
type Option func(*Runner)

// WithStage registers a stage under name.
func WithStage(name string, s Stage) Option {
	return func(r *Runner) { r.stages[name] = s }
}

// WithWorkers sets how many jobs run at once. Values below 1 are ignored.
func WithWorkers(n int) Option {
	return func(r *Runner) {
		if n >= 1 {
			r.workers = n
		}
	}
}

// WithQueue sets how many jobs may wait. Values below 1 are ignored.
func WithQueue(n int) Option {
	return func(r *Runner) {
		if n >= 1 {
			r.queueCap = n
		}
	}
}

// WithHistory sets how many finished jobs List keeps. Values below 1 are ignored.
func WithHistory(n int) Option {
	return func(r *Runner) {
		if n >= 1 {
			r.history = n
		}
	}
}

type job struct {
	info   core.JobInfo
	ctx    context.Context
	cancel context.CancelFunc
}

// Runner runs jobs on a fixed pool. State changes and their events happen
// under one lock, so subscribers see each job's transitions in order.
type Runner struct {
	pub      Publisher
	stages   map[string]Stage
	workers  int
	queueCap int
	history  int

	ctx  context.Context
	stop context.CancelFunc
	wg   sync.WaitGroup

	mu     sync.Mutex
	closed bool
	seq    int
	jobs   []*job
	queue  chan *job
}

// New returns a running Runner.
func New(pub Publisher, opts ...Option) *Runner {
	r := &Runner{pub: pub, stages: make(map[string]Stage), workers: 2, queueCap: 1024, history: 256}
	for _, opt := range opts {
		opt(r)
	}
	r.ctx, r.stop = context.WithCancel(context.Background())
	r.queue = make(chan *job, r.queueCap)
	for range r.workers {
		r.wg.Add(1)
		go r.worker()
	}
	return r
}

// Start queues a job. An identical spec that is still active returns that
// job's ID instead of starting a second one.
func (r *Runner) Start(spec core.JobSpec) (core.JobID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return "", ErrClosed
	}
	if _, ok := r.stages[spec.Stage]; !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownStage, spec.Stage)
	}
	for _, j := range r.jobs {
		if active(j.info.State) && sameSpec(j.info.Spec, spec) {
			return j.info.ID, nil
		}
	}
	if len(r.queue) == cap(r.queue) {
		return "", ErrQueueFull
	}

	r.seq++
	spec.Params = maps.Clone(spec.Params)
	j := &job{info: core.JobInfo{
		ID:    core.JobID("job-" + strconv.Itoa(r.seq)),
		Spec:  spec,
		State: core.JobQueued,
	}}
	j.ctx, j.cancel = context.WithCancel(r.ctx)
	r.jobs = append(r.jobs, j)
	r.publishChanged(j)
	r.queue <- j
	return j.info.ID, nil
}

// Cancel stops a queued or running job. Cancelling a finished job does nothing.
func (r *Runner) Cancel(id core.JobID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.jobs {
		if j.info.ID != id {
			continue
		}
		switch j.info.State {
		case core.JobQueued:
			r.finish(j, core.JobCancelled, "")
		case core.JobRunning:
			j.cancel()
		}
		return nil
	}
	return ErrNotFound
}

// List returns a snapshot of active jobs and recent finished ones, oldest first.
func (r *Runner) List() []core.JobInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]core.JobInfo, len(r.jobs))
	for i, j := range r.jobs {
		out[i] = snapshot(j)
	}
	return out
}

// Close cancels every job and waits for the workers. It is safe to call again.
func (r *Runner) Close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
	}
	r.mu.Unlock()
	r.stop()
	r.wg.Wait()
}

func (r *Runner) worker() {
	defer r.wg.Done()
	for j := range r.queue {
		r.execute(j)
	}
}

func (r *Runner) execute(j *job) {
	r.mu.Lock()
	if j.info.State != core.JobQueued {
		r.mu.Unlock()
		return
	}
	if j.ctx.Err() != nil {
		r.finish(j, core.JobCancelled, "")
		r.mu.Unlock()
		return
	}
	j.info.State = core.JobRunning
	r.publishChanged(j)
	stage, spec := r.stages[j.info.Spec.Stage], snapshot(j).Spec
	r.mu.Unlock()

	err := r.call(stage, j, spec)

	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case err == nil:
		r.finish(j, core.JobDone, "")
	case j.ctx.Err() != nil:
		r.finish(j, core.JobCancelled, "")
	default:
		r.finish(j, core.JobFailed, err.Error())
	}
}

func (r *Runner) call(stage Stage, j *job, spec core.JobSpec) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return stage(j.ctx, spec, func(done, total int64) { r.progress(j, done, total) })
}

func (r *Runner) progress(j *job, done, total int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j.info.State != core.JobRunning {
		return
	}
	j.info.Done, j.info.Total = done, total
	r.pub.Publish(core.JobProgress{Job: j.info.ID, Done: done, Total: total})
}

// finish moves j to a terminal state. The caller holds r.mu.
func (r *Runner) finish(j *job, state core.JobState, msg string) {
	j.info.State, j.info.Err = state, msg
	j.cancel()
	r.publishChanged(j)
	r.trim()
}

// trim drops the oldest finished jobs beyond the history limit.
func (r *Runner) trim() {
	finished := 0
	for _, j := range r.jobs {
		if !active(j.info.State) {
			finished++
		}
	}
	drop := finished - r.history
	if drop <= 0 {
		return
	}
	kept := r.jobs[:0]
	for _, j := range r.jobs {
		if drop > 0 && !active(j.info.State) {
			drop--
			continue
		}
		kept = append(kept, j)
	}
	clear(r.jobs[len(kept):])
	r.jobs = kept
}

func (r *Runner) publishChanged(j *job) {
	r.pub.Publish(core.JobChanged{Info: snapshot(j)})
}

func snapshot(j *job) core.JobInfo {
	info := j.info
	info.Spec.Params = maps.Clone(info.Spec.Params)
	return info
}

func active(s core.JobState) bool { return s == core.JobQueued || s == core.JobRunning }

func sameSpec(a, b core.JobSpec) bool {
	return a.Stage == b.Stage && a.Film == b.Film && maps.Equal(a.Params, b.Params)
}
