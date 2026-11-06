package events_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/events"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func newBus(t *testing.T, opts ...events.Option) *events.Bus {
	t.Helper()
	b := events.New(opts...)
	t.Cleanup(b.Close)
	return b
}

func recv(t *testing.T, ch <-chan core.Event) (core.Event, bool) {
	t.Helper()
	select {
	case e, ok := <-ch:
		return e, ok
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
		return nil, false
	}
}

func changed(id string) core.Event {
	return core.JobChanged{Info: core.JobInfo{ID: core.JobID(id), State: core.JobRunning}}
}

func progress(job string, done int64) core.Event {
	return core.JobProgress{Job: core.JobID(job), Done: done, Total: 1000}
}

func changedID(t *testing.T, e core.Event) core.JobID {
	t.Helper()
	c, ok := e.(core.JobChanged)
	if !ok {
		t.Fatalf("got %T, want core.JobChanged", e)
	}
	return c.Info.ID
}

func TestBus_Subscribe(t *testing.T) {
	t.Run("closes on cancel", func(t *testing.T) {
		b := newBus(t)
		ctx, cancel := context.WithCancel(t.Context())
		ch := b.Subscribe(ctx)
		cancel()
		if _, ok := recv(t, ch); ok {
			t.Fatal("channel still open after cancel")
		}
	})

	t.Run("closed bus gives a closed channel", func(t *testing.T) {
		b := events.New()
		b.Close()
		if _, ok := recv(t, b.Subscribe(t.Context())); ok {
			t.Fatal("channel open on a closed bus")
		}
	})
}

func TestBus_Publish(t *testing.T) {
	t.Run("fans out to every subscriber in order", func(t *testing.T) {
		b := newBus(t)
		chans := []<-chan core.Event{b.Subscribe(t.Context()), b.Subscribe(t.Context())}
		for _, id := range []string{"a", "b", "c"} {
			b.Publish(changed(id))
		}
		for i, ch := range chans {
			for _, want := range []core.JobID{"a", "b", "c"} {
				e, ok := recv(t, ch)
				if !ok {
					t.Fatalf("subscriber %d closed early", i)
				}
				if got := changedID(t, e); got != want {
					t.Errorf("subscriber %d: got %q, want %q", i, got, want)
				}
			}
		}
	})

	t.Run("never blocks on a subscriber that does not read", func(t *testing.T) {
		b := newBus(t)
		_ = b.Subscribe(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := range 10000 {
				b.Publish(changed("x"))
				b.Publish(progress("j", int64(i)))
			}
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Publish blocked")
		}
	})

	t.Run("coalesces progress but keeps every state change", func(t *testing.T) {
		b := newBus(t)
		ch := b.Subscribe(t.Context())
		b.Publish(changed("a"))
		for i := int64(1); i <= 1000; i++ {
			b.Publish(progress("j", i))
		}
		b.Publish(changed("b"))

		var progressed []core.JobProgress
		var ids []core.JobID
		for len(ids) < 2 {
			e, ok := recv(t, ch)
			if !ok {
				t.Fatal("channel closed early")
			}
			if p, isProgress := e.(core.JobProgress); isProgress {
				progressed = append(progressed, p)
				continue
			}
			ids = append(ids, changedID(t, e))
		}
		if ids[0] != "a" || ids[1] != "b" {
			t.Errorf("state changes = %v, want [a b]", ids)
		}
		if n := len(progressed); n < 1 || n > 2 {
			t.Fatalf("got %d progress events, want 1 or 2", n)
		}
		if last := progressed[len(progressed)-1].Done; last != 1000 {
			t.Errorf("last progress = %d, want 1000", last)
		}
	})

	t.Run("drops a subscriber that falls too far behind", func(t *testing.T) {
		b := newBus(t, events.WithMaxPending(2))
		fast := b.Subscribe(t.Context())
		slow := b.Subscribe(t.Context())
		for i := range 10 {
			id := fmt.Sprint(i)
			b.Publish(changed(id))
			e, ok := recv(t, fast)
			if !ok || changedID(t, e) != core.JobID(id) {
				t.Fatalf("fast subscriber lost event %d", i)
			}
		}
		for {
			if _, ok := recv(t, slow); !ok {
				return
			}
		}
	})
}

func TestBus_Close(t *testing.T) {
	b := events.New()
	ch := b.Subscribe(t.Context())
	b.Close()
	b.Close()
	if _, ok := recv(t, ch); ok {
		t.Fatal("subscriber still open after Close")
	}
	b.Publish(changed("late"))
}

func TestBus_ConcurrentUse(t *testing.T) {
	b := newBus(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range 1000 {
				b.Publish(progress("j", int64(i)))
				b.Publish(changed("c"))
			}
		})
		wg.Go(func() {
			for range 50 {
				ctx, cancel := context.WithCancel(t.Context())
				ch := b.Subscribe(ctx)
				select {
				case <-ch:
				default:
				}
				cancel()
			}
		})
	}
	wg.Wait()
}

func BenchmarkBus_Publish(b *testing.B) {
	for _, n := range []int{1, 8, 64} {
		b.Run(fmt.Sprintf("subs=%d", n), func(b *testing.B) {
			bus := events.New()
			defer bus.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			for range n {
				ch := bus.Subscribe(ctx)
				go func() {
					for range ch {
					}
				}()
			}
			e := progress("j", 1)
			for b.Loop() {
				bus.Publish(e)
			}
		})
	}
}
