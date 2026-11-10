package commitgate

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// The cost of an exclusive request on an idle gate and behind a queue of
// waiters. Both use only Acquire and Release, so the same file measures
// master. Run with -benchmem.

// cancelAfterEnqueue is a context whose first Err is nil and whose Done is
// already closed: Acquire queues the request, finds it cancelled, and
// withdraws it, all on one goroutine.
type cancelAfterEnqueue struct {
	context.Context
	checked atomic.Bool
}

var closedDone = func() chan struct{} { c := make(chan struct{}); close(c); return c }()

func (c *cancelAfterEnqueue) Done() <-chan struct{} { return closedDone }
func (c *cancelAfterEnqueue) Err() error {
	if c.checked.Swap(true) {
		return context.Canceled
	}
	return nil
}
func (c *cancelAfterEnqueue) Deadline() (time.Time, bool) { return time.Time{}, false }

func BenchmarkExclusiveIdle(b *testing.B) {
	var g Gate
	ctx := context.Background()
	b.ReportAllocs()
	for range b.N {
		r, err := g.Acquire(ctx, Exclusive)
		if err != nil {
			b.Fatal(err)
		}
		r.Release()
	}
}

// gateBehindWaiters returns a gate with a shared owner and 32 queued requests,
// exclusive and shared alternating from an exclusive head, so nothing queued
// is granted.
func gateBehindWaiters() (*Gate, *Grant) {
	g := new(Gate)
	owner, _ := g.Acquire(context.Background(), Shared)
	g.mu.Lock()
	for i := range 32 {
		g.queue = append(g.queue, &Grant{gate: g, mode: Mode((i + 1) % 2), ready: make(chan struct{})})
	}
	g.mu.Unlock()
	return g, owner
}

// BenchmarkExclusiveBehindWaiters queues an exclusive request behind
// gateBehindWaiters' owner and queue, then withdraws it.
func BenchmarkExclusiveBehindWaiters(b *testing.B) {
	g, owner := gateBehindWaiters()
	defer owner.Release()
	b.ReportAllocs()
	for range b.N {
		ctx := &cancelAfterEnqueue{Context: context.Background()}
		if _, err := g.Acquire(ctx, Exclusive); err == nil {
			b.Fatal("granted behind a shared owner")
		}
	}
}
