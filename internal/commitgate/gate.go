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
	// writerLabel is the exclusive owner's label, for Ahead.
	writerLabel string
}

// Ahead is what stood in front of an exclusive request when it was queued:
// the exclusive owner's label (Holder, with HolderExclusive set) or the number
// of shared owners, and the requests queued before it, exclusive ones by label
// and shared ones counted. Depth is the number of queued requests ahead.
type Ahead struct {
	HolderExclusive bool
	Holder          string
	SharedOwners    int
	Depth           int
	Exclusive       []string
	Shared          int
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
	// blocked: the request was not granted when it was queued. A shared
	// request is only held back by an exclusive owner or an exclusive request
	// queued ahead of it, so for Shared this means it waited behind Exclusive.
	blocked bool
	// label and ahead are AcquireLabeled's: the request's label and, for an
	// exclusive request, what was in front of it.
	label string
	ahead Ahead
}

func (g *Gate) Acquire(ctx context.Context, mode Mode) (*Grant, error) {
	return g.AcquireLabeled(ctx, mode, "")
}

// AcquireLabeled is Acquire for a request named label. An exclusive request
// also records what was in front of it (Grant.Ahead), and while it owns the
// gate its label is what later exclusive requests see as the holder.
func (g *Gate) AcquireLabeled(ctx context.Context, mode Mode, label string) (*Grant, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	g.mu.Lock()
	g.sequence++
	r := &Grant{gate: g, mode: mode, ticket: g.sequence, ready: make(chan struct{}), label: label}
	if mode == Exclusive {
		r.ahead = g.aheadLocked()
	}
	g.queue = append(g.queue, r)
	g.dispatchLocked()
	r.blocked = !r.active
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
			g.writerLabel = r.label
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
		g.writerLabel = ""
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

// Blocked reports whether the request had to queue: it was not granted at
// once when it arrived.
func (r *Grant) Blocked() bool { return r.blocked }

// Ahead reports what was in front of an exclusive request when it was queued;
// it is empty for a shared one.
func (r *Grant) Ahead() Ahead { return r.ahead }

// aheadLocked describes the owners and the queue as a new request finds them.
func (g *Gate) aheadLocked() Ahead {
	a := Ahead{HolderExclusive: g.writer, SharedOwners: g.readers, Depth: len(g.queue)}
	if g.writer {
		a.Holder = g.writerLabel
	}
	for _, q := range g.queue {
		if q.mode == Exclusive {
			a.Exclusive = append(a.Exclusive, q.label)
		} else {
			a.Shared++
		}
	}
	return a
}
func (r *Grant) Ticket() uint64 { return r.ticket }
func (r *Grant) Owns(g *Gate, mode Mode) bool {
	r.gate.mu.Lock()
	defer r.gate.mu.Unlock()
	return r.gate == g && r.active && r.mode == mode
}
func (g *Gate) Queued() int { g.mu.Lock(); defer g.mu.Unlock(); return len(g.queue) }
