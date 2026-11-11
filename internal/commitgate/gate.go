// Package commitgate implements FIFO shared cohorts and exclusive ownership.
package commitgate

import (
	"context"
	"sync"
	"sync/atomic"
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
	// labelOwner is the latest tracked exclusive request granted the gate. Its
	// label names the owner to later tracked requests while it is still the
	// active owner; an untracked owner leaves it stale, and stale means
	// unlabeled.
	labelOwner *Grant
	// exclusiveGen moves once when an exclusive grant becomes active and once
	// when it is released, so it is odd exactly while the gate is held
	// exclusively. It is read without mu (ExclusiveGeneration).
	exclusiveGen atomic.Uint64
}

// ExclusiveGeneration returns the gate's exclusive generation: odd while an
// exclusive grant is active. Two equal readings mean no exclusive grant was
// taken or released between them.
func (g *Gate) ExclusiveGeneration() uint64 { return g.exclusiveGen.Load() }

// Ahead is what stood in front of a tracked request when it was queued (see
// AcquireTracked): the exclusive owner's label (Holder, with HolderExclusive
// set) or the number of shared owners, and the requests queued before it.
// Depth is the number of queued requests ahead. Of the first maxAheadScan of
// them, the exclusive ones are named by label (the first MaxAheadLabels in
// Exclusive, the rest counted in ExclusiveMore) and the shared ones counted in
// Shared; Unscanned counts those past maxAheadScan. Labels are whatever the
// requesters passed to AcquireTracked (nil for an untracked one); the gate
// only stores them, so naming them costs nothing until the owner renders them.
type Ahead struct {
	HolderExclusive bool
	Holder          any
	SharedOwners    int
	Depth           int
	Exclusive       []any
	ExclusiveMore   int
	Shared          int
	Unscanned       int
}

// maxAheadScan bounds the queued requests an Ahead looks at under the gate's
// mutex, and MaxAheadLabels the exclusive labels it keeps.
const (
	maxAheadScan   = 64
	MaxAheadLabels = 8
)

// Grant is an opaque, single-release capability. Cancellation can withdraw a
// queued request, but cannot revoke a capability already returned to its owner.
//
// Fields are ordered so mode and active share a word: every commit allocates a
// Grant, and this keeps it the size it was before tracked.
type Grant struct {
	gate   *Gate
	ticket uint64
	ready  chan struct{}
	wait   time.Duration
	// tracked is a tracked request's record; nil for Acquire.
	tracked *trackedRequest
	mode    Mode
	active  bool
}

// trackedRequest is AcquireTracked's record of a request: its label, whether
// it had to queue, and for an exclusive request what was in front of it.
type trackedRequest struct {
	label   any
	blocked bool
	ahead   Ahead
	labels  [MaxAheadLabels]any
}

func (g *Gate) Acquire(ctx context.Context, mode Mode) (*Grant, error) {
	return g.acquire(ctx, mode, nil)
}

// AcquireTracked is Acquire for a request named label that also records
// whether it had to queue (Grant.Blocked) and, when exclusive, what was in
// front of it (Grant.Ahead); while it owns the gate exclusively its label is
// what later tracked requests see as the holder. It costs one allocation and a
// bounded look at the queue that Acquire does not.
func (g *Gate) AcquireTracked(ctx context.Context, mode Mode, label any) (*Grant, error) {
	return g.acquire(ctx, mode, &trackedRequest{label: label})
}

func (g *Gate) acquire(ctx context.Context, mode Mode, tr *trackedRequest) (*Grant, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	g.mu.Lock()
	g.sequence++
	r := &Grant{gate: g, mode: mode, ticket: g.sequence, ready: make(chan struct{})}
	if tr != nil {
		g.trackLocked(r, tr)
	}
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

// trackLocked fills tr for r as the gate stands before r is queued. A shared
// request is held back only by an exclusive owner or by a queue, whose head is
// then exclusive; an exclusive one also by shared owners.
func (g *Gate) trackLocked(r *Grant, tr *trackedRequest) {
	r.tracked = tr
	tr.blocked = g.writer || len(g.queue) > 0 || (r.mode == Exclusive && g.readers > 0)
	if r.mode != Exclusive {
		return
	}
	a := &tr.ahead
	a.HolderExclusive, a.SharedOwners, a.Depth = g.writer, g.readers, len(g.queue)
	if o := g.labelOwner; g.writer && o != nil && o.active && o.mode == Exclusive {
		a.Holder = o.tracked.label
	}
	scan := g.queue
	if len(scan) > maxAheadScan {
		a.Unscanned = len(scan) - maxAheadScan
		scan = scan[:maxAheadScan]
	}
	n := 0
	for _, q := range scan {
		switch {
		case q.mode != Exclusive:
			a.Shared++
		case n == MaxAheadLabels:
			a.ExclusiveMore++
		default:
			var label any
			if q.tracked != nil {
				label = q.tracked.label
			}
			tr.labels[n] = label
			n++
		}
	}
	a.Exclusive = tr.labels[:n:n]
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
			g.exclusiveGen.Add(1)
			if r.tracked != nil {
				g.labelOwner = r
			}
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
		g.exclusiveGen.Add(1)
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

// Blocked reports whether a tracked request had to queue: it was not granted
// at once when it arrived. It is false for an untracked one.
func (r *Grant) Blocked() bool { return r.tracked != nil && r.tracked.blocked }

// Ahead reports what was in front of a tracked exclusive request when it was
// queued; it is empty for any other.
func (r *Grant) Ahead() Ahead {
	if r.tracked == nil {
		return Ahead{}
	}
	return r.tracked.ahead
}
func (r *Grant) Ticket() uint64 { return r.ticket }
func (r *Grant) Owns(g *Gate, mode Mode) bool {
	r.gate.mu.Lock()
	defer r.gate.mu.Unlock()
	return r.gate == g && r.active && r.mode == mode
}
func (g *Gate) Queued() int { g.mu.Lock(); defer g.mu.Unlock(); return len(g.queue) }
