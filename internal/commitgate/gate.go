// Package commitgate implements FIFO shared cohorts and exclusive ownership.
package commitgate

import (
	"context"
	"sync"
	"time"
)

type Mode uint8

const (
	Shared Mode = iota
	Exclusive
)

// Gate's zero value is ready for use. It never invokes owner code under mu.
type Gate struct {
	mu       sync.Mutex
	queue    []*Grant
	readers  int
	writer   bool
	sequence uint64
}

// Grant is an opaque, single-release capability. Cancellation can withdraw a
// queued request, but cannot revoke a capability already returned to its owner.
type Grant struct {
	gate   *Gate
	mode   Mode
	ticket uint64
	ready  chan struct{}
	active bool
	wait   time.Duration
}

func (g *Gate) Acquire(ctx context.Context, mode Mode) (*Grant, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	g.mu.Lock()
	g.sequence++
	r := &Grant{gate: g, mode: mode, ticket: g.sequence, ready: make(chan struct{})}
	g.queue = append(g.queue, r)
	g.dispatchLocked()
	g.mu.Unlock()
	select {
	case <-r.ready:
		if err := ctx.Err(); err != nil {
			r.Release()
			return nil, err
		}
		r.wait = time.Since(start)
		return r, nil
	case <-ctx.Done():
		g.mu.Lock()
		if r.active {
			g.releaseLocked(r)
		} else {
			for i, q := range g.queue {
				if q == r {
					g.queue = append(g.queue[:i], g.queue[i+1:]...)
					break
				}
			}
			g.dispatchLocked()
		}
		g.mu.Unlock()
		return nil, ctx.Err()
	}
}
func (g *Gate) dispatchLocked() {
	if g.writer {
		return
	}
	for len(g.queue) > 0 {
		r := g.queue[0]
		if r.mode == Exclusive && g.readers != 0 {
			return
		}
		g.queue[0] = nil
		g.queue = g.queue[1:]
		r.active = true
		if r.mode == Exclusive {
			g.writer = true
		} else {
			g.readers++
		}
		close(r.ready)
		if g.writer {
			return
		}
	}
}
func (g *Gate) releaseLocked(r *Grant) bool {
	if !r.active {
		return false
	}
	r.active = false
	if r.mode == Exclusive {
		g.writer = false
	} else {
		g.readers--
	}
	g.dispatchLocked()
	return true
}
func (r *Grant) Release() bool {
	r.gate.mu.Lock()
	defer r.gate.mu.Unlock()
	return r.gate.releaseLocked(r)
}
func (r *Grant) Wait() time.Duration { return r.wait }
func (r *Grant) Ticket() uint64      { return r.ticket }
func (r *Grant) Owns(g *Gate, mode Mode) bool {
	r.gate.mu.Lock()
	defer r.gate.mu.Unlock()
	return r.gate == g && r.active && r.mode == mode
}
func (g *Gate) Queued() int { g.mu.Lock(); defer g.mu.Unlock(); return len(g.queue) }
