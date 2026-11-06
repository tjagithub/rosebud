// Package events fans core events out to subscribers without blocking publishers.
package events

import (
	"container/list"
	"context"
	"sync"

	"github.com/tjagithub/rosebud/internal/core"
)

const defaultMaxPending = 1024

// Option configures a Bus.
type Option func(*Bus)

// WithMaxPending sets how many events may queue per subscriber before it is
// dropped. Values below 1 are ignored.
func WithMaxPending(n int) Option {
	return func(b *Bus) {
		if n >= 1 {
			b.maxPending = n
		}
	}
}

// Bus delivers each event to every subscriber in order, replacing queued events
// that share a CoalesceKey. A subscriber that overflows is closed and must resync.
type Bus struct {
	mu         sync.RWMutex
	subs       map[*sub]struct{}
	closed     bool
	maxPending int
	wg         sync.WaitGroup
}

// New returns a running Bus.
func New(opts ...Option) *Bus {
	b := &Bus{subs: make(map[*sub]struct{}), maxPending: defaultMaxPending}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Subscribe returns a channel of events, closed when ctx is done, the
// subscriber falls too far behind, or the Bus closes.
func (b *Bus) Subscribe(ctx context.Context) <-chan core.Event {
	out := make(chan core.Event)
	s := newSub(b.maxPending)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(out)
		return out
	}
	b.subs[s] = struct{}{}
	b.wg.Add(1)
	b.mu.Unlock()

	go b.run(ctx, s, out)
	return out
}

// Publish queues e for every subscriber and never blocks. Nil is ignored.
func (b *Bus) Publish(e core.Event) {
	if e == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for s := range b.subs {
		s.push(e)
	}
}

// Close closes every subscriber channel, discarding queued events. It is safe
// to call more than once.
func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	subs := b.subs
	b.subs = make(map[*sub]struct{})
	b.mu.Unlock()

	for s := range subs {
		s.kill()
	}
	b.wg.Wait()
}

func (b *Bus) run(ctx context.Context, s *sub, out chan<- core.Event) {
	defer b.wg.Done()
	defer close(out)
	defer b.remove(s)
	for {
		e, ok := s.pop()
		if !ok {
			select {
			case <-s.wake:
				continue
			case <-s.done:
				return
			case <-ctx.Done():
				return
			}
		}
		select {
		case out <- e:
		case <-s.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (b *Bus) remove(s *sub) {
	s.kill()
	b.mu.Lock()
	delete(b.subs, s)
	b.mu.Unlock()
}

type sub struct {
	mu    sync.Mutex
	q     *list.List
	byKey map[string]*list.Element
	max   int
	wake  chan struct{}
	done  chan struct{}
	once  sync.Once
}

func newSub(maxPending int) *sub {
	return &sub{
		q:     list.New(),
		byKey: make(map[string]*list.Element),
		max:   maxPending,
		wake:  make(chan struct{}, 1),
		done:  make(chan struct{}),
	}
}

func (s *sub) kill() { s.once.Do(func() { close(s.done) }) }

func (s *sub) push(e core.Event) {
	key := e.CoalesceKey()
	s.mu.Lock()
	if el, ok := s.byKey[key]; ok && key != "" {
		// Re-append so the newer event cannot overtake earlier ones.
		s.q.Remove(el)
	}
	el := s.q.PushBack(e)
	if key != "" {
		s.byKey[key] = el
	}
	over := s.q.Len() > s.max
	s.mu.Unlock()

	if over {
		s.kill()
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *sub) pop() (core.Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el := s.q.Front()
	if el == nil {
		return nil, false
	}
	s.q.Remove(el)
	e := el.Value.(core.Event)
	if key := e.CoalesceKey(); key != "" {
		delete(s.byKey, key)
	}
	return e, true
}
