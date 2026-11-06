package events_test

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"

	"pgregory.net/rapid"

	"github.com/tjagithub/rosebud/internal/core"
	"github.com/tjagithub/rosebud/internal/events"
)

type opKind int

const (
	opChanged opKind = iota
	opProgress
	opRead
)

// op is one step: publish a state change or progress, or read up to n events.
type op struct {
	kind opKind
	job  core.JobID
	n    int
}

func genOps(maxLen int) *rapid.Generator[[]op] {
	one := rapid.Custom(func(t *rapid.T) op {
		return op{
			kind: rapid.SampledFrom([]opKind{opChanged, opProgress, opProgress, opRead}).Draw(t, "kind"),
			job:  rapid.SampledFrom([]core.JobID{"a", "b"}).Draw(t, "job"),
			n:    rapid.IntRange(1, 4).Draw(t, "n"),
		}
	})
	return rapid.SliceOfN(one, 0, maxLen)
}

// published records what was sent, keyed by publish index so a received event
// can be traced back to when it was sent.
type published struct {
	event core.Event
	kind  opKind
	job   core.JobID
}

// publish sends the op's event, tagged with its index, and records it.
func publish(b *events.Bus, pubs *[]published, o op) {
	i := len(*pubs)
	var e core.Event
	switch o.kind {
	case opChanged:
		e = core.JobChanged{Info: core.JobInfo{ID: core.JobID(strconv.Itoa(i))}}
	case opProgress:
		e = core.JobProgress{Job: o.job, Done: int64(i)}
	}
	*pubs = append(*pubs, published{event: e, kind: o.kind, job: o.job})
	b.Publish(e)
}

// index recovers the publish index that publish() tagged an event with.
func index(e core.Event) int {
	switch v := e.(type) {
	case core.JobChanged:
		n, _ := strconv.Atoi(string(v.Info.ID))
		return n
	case core.JobProgress:
		return int(v.Done)
	}
	return -1
}

// drain reads up to max events (all when max < 0), letting the bubble settle
// between reads. closed reports whether the channel was seen closed.
func drain(ch <-chan core.Event, max int) (got []core.Event, closed bool) {
	for max != 0 {
		synctest.Wait()
		select {
		case e, ok := <-ch:
			if !ok {
				return got, true
			}
			got = append(got, e)
			max--
		default:
			return got, false
		}
	}
	return got, false
}

func indexes(es []core.Event) []int {
	out := make([]int, len(es))
	for i, e := range es {
		out[i] = index(e)
	}
	return out
}

func TestBus_Properties_oneSubscriber(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		ops := genOps(120).Draw(rt, "ops")

		var pubs []published
		var got []core.Event
		synctest.Test(t, func(t *testing.T) {
			b := events.New(events.WithMaxPending(1000))
			defer b.Close()
			ch := b.Subscribe(context.Background())
			for _, o := range ops {
				if o.kind == opRead {
					part, _ := drain(ch, o.n)
					got = append(got, part...)
					continue
				}
				publish(b, &pubs, o)
			}
			rest, _ := drain(ch, -1)
			got = append(got, rest...)
		})

		idx := indexes(got)
		if !slices.IsSorted(idx) || len(slices.Compact(slices.Clone(idx))) != len(idx) {
			rt.Fatalf("events not delivered once each in publish order: %v", idx)
		}

		seen := make(map[int]bool, len(idx))
		for _, i := range idx {
			seen[i] = true
		}
		lastProgress := map[core.JobID]int{}
		for i, p := range pubs {
			switch p.kind {
			case opChanged:
				if !seen[i] {
					rt.Fatalf("state change %d was lost", i)
				}
			case opProgress:
				lastProgress[p.job] = i
			}
		}
		for job, i := range lastProgress {
			if !seen[i] {
				rt.Fatalf("latest progress for %q (%d) was lost or went stale", job, i)
			}
		}
	})
}

func TestBus_Properties_slowSubscriber(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		ops := genOps(120).Draw(rt, "ops")
		limit := rapid.IntRange(1, 8).Draw(rt, "limit")

		var pubs []published
		var fast, slow []core.Event
		var slowClosed bool
		synctest.Test(t, func(t *testing.T) {
			b := events.New(events.WithMaxPending(limit))
			defer b.Close()
			fastCh := b.Subscribe(context.Background())
			slowCh := b.Subscribe(context.Background())
			for _, o := range ops {
				if o.kind == opRead {
					continue
				}
				publish(b, &pubs, o)
				part, _ := drain(fastCh, -1)
				fast = append(fast, part...)
			}
			slow, slowClosed = drain(slowCh, -1)
		})

		if want := len(pubs); len(fast) != want {
			rt.Fatalf("fast subscriber got %d of %d events", len(fast), want)
		}
		for i, e := range fast {
			if index(e) != i {
				rt.Fatalf("fast subscriber out of order at %d: %v", i, indexes(fast))
			}
		}

		idx := indexes(slow)
		if !slices.IsSorted(idx) || len(slices.Compact(slices.Clone(idx))) != len(idx) {
			rt.Fatalf("slow subscriber saw duplicates or disorder: %v", idx)
		}
		if slowClosed {
			return
		}
		seen := make(map[int]bool, len(idx))
		for _, i := range idx {
			seen[i] = true
		}
		for i, p := range pubs {
			if p.kind == opChanged && !seen[i] {
				rt.Fatalf("slow subscriber stayed open but lost state change %d (limit %d)", i, limit)
			}
		}
	})
}
