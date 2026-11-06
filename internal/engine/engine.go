// Package engine is the in-process core.Client. It wires a catalog, the job
// runner and the event bus together.
package engine

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/events"
	"github.com/tjagithub/rosebud/internal/jobs"
)

// ErrClosed is returned by calls made after Close.
var ErrClosed = errors.New("engine: closed")

// Catalog is the read side: films, scenes and search. The store implements it.
type Catalog interface {
	Films(ctx context.Context) iter.Seq2[core.Film, error]
	Scenes(ctx context.Context, film core.FilmID) iter.Seq2[core.Scene, error]
	Search(ctx context.Context, q core.SearchQuery) ([]core.Hit, error)
}

// Option configures an Engine.
type Option func(*config)

type config struct {
	bus  []events.Option
	jobs []jobs.Option
}

// WithStage registers a job stage under name.
func WithStage(name string, s jobs.Stage) Option {
	return func(c *config) { c.jobs = append(c.jobs, jobs.WithStage(name, s)) }
}

// WithJobOptions passes options through to the job runner.
func WithJobOptions(opts ...jobs.Option) Option {
	return func(c *config) { c.jobs = append(c.jobs, opts...) }
}

// WithBusOptions passes options through to the event bus.
func WithBusOptions(opts ...events.Option) Option {
	return func(c *config) { c.bus = append(c.bus, opts...) }
}

// Engine implements core.Client in-process.
type Engine struct {
	cat    Catalog
	bus    *events.Bus
	runner *jobs.Runner

	mu     sync.RWMutex
	closed bool
}

var _ core.Client = (*Engine)(nil)

// New returns a running Engine reading from cat.
func New(cat Catalog, opts ...Option) *Engine {
	var c config
	for _, opt := range opts {
		opt(&c)
	}
	bus := events.New(c.bus...)
	return &Engine{cat: cat, bus: bus, runner: jobs.New(bus, c.jobs...)}
}

func (e *Engine) Films(ctx context.Context) iter.Seq2[core.Film, error] {
	return guard(ctx, func() iter.Seq2[core.Film, error] { return e.cat.Films(ctx) })
}

func (e *Engine) Scenes(ctx context.Context, film core.FilmID) iter.Seq2[core.Scene, error] {
	return guard(ctx, func() iter.Seq2[core.Scene, error] { return e.cat.Scenes(ctx, film) })
}

// guard yields ctx's error instead of data when ctx is already done.
func guard[T any](ctx context.Context, open func() iter.Seq2[T, error]) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		if err := ctx.Err(); err != nil {
			var zero T
			yield(zero, err)
			return
		}
		for v, err := range open() {
			if !yield(v, err) {
				return
			}
		}
	}
}

// Search returns at most q.Limit hits. A non-positive Limit is an error.
func (e *Engine) Search(ctx context.Context, q core.SearchQuery) ([]core.Hit, error) {
	if q.Limit <= 0 {
		return nil, fmt.Errorf("engine: search limit must be positive, got %d", q.Limit)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hits, err := e.cat.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(hits) > q.Limit {
		hits = hits[:q.Limit]
	}
	return hits, nil
}

func (e *Engine) StartJob(ctx context.Context, spec core.JobSpec) (core.JobID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return e.runner.Start(spec)
}

func (e *Engine) CancelJob(ctx context.Context, id core.JobID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return e.runner.Cancel(id)
}

func (e *Engine) Jobs(ctx context.Context) ([]core.JobInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return e.runner.List(), nil
}

// Subscribe streams events until ctx is done or the engine closes.
func (e *Engine) Subscribe(ctx context.Context) (<-chan core.Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return nil, ErrClosed
	}
	return e.bus.Subscribe(ctx), nil
}

// Close stops the runner, then closes the bus. It is safe to call again.
func (e *Engine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.mu.Unlock()

	e.runner.Close()
	e.bus.Close()
	return nil
}
