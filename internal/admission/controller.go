// Package admission bounds execution occupancy and shares service between
// principals. It does not own VM state, worker goroutines, or commit gates.
package admission

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Quantum is how long a root invocation keeps its reservation while another
// request waits. MOO slices are cooperative and can run for seconds; without a
// quantum a single-slot controller serializes every player behind them.
const Quantum = 10 * time.Millisecond

type Class uint8

const (
	Input Class = iota
	Background
)

type Group uint8

const (
	User Group = iota
	Anonymous
	System
)

type Key struct {
	Principal int64
	Group     Group
	Class     Class
}
type identity struct {
	principal int64
	group     Group
}

func (k Key) identity() identity { return identity{k.Principal, k.Group} }

type Options struct {
	Limit, PrincipalLimit                                        int
	InputWeight, BackgroundWeight, AnonymousWeight, SystemWeight int
}
type ledger struct {
	settled, reserved float64
	last              uint64
}
type principal struct {
	ledger
	classes                  [2]ledger
	estimate                 [2]time.Duration
	active, queued           int
	classActive, classQueued [2]int
	classWatermark           float64
}
type Controller struct {
	mu                             sync.Mutex
	options                        Options
	principals                     map[identity]*principal
	queue                          []*Request
	active                         int
	preempted                      int // yielded segments awaiting readmission
	paused                         int
	waiting                        atomic.Int32
	preemptions                    uint64
	idle                           chan struct{}
	sequence                       uint64
	watermark                      float64
	service, gateWait, maintenance time.Duration
}
type Request struct {
	c      *Controller
	key    Key
	ready  chan *Reservation
	queued bool
	resume bool // readmits a preempted segment; granted even while paused
}
type Reservation struct {
	c                   *Controller
	key                 Key
	estimate            time.Duration
	weight, classWeight float64
	finished            bool
}
type Stats struct {
	Active, Queued, Preempted      int
	Preemptions                    uint64
	Service, GateWait, Maintenance time.Duration
}

func New(o Options) *Controller {
	if o.Limit <= 0 {
		o.Limit = 1
	}
	if o.PrincipalLimit <= 0 {
		o.PrincipalLimit = o.Limit
	}
	if o.InputWeight <= 0 {
		o.InputWeight = 3
	}
	if o.BackgroundWeight <= 0 {
		o.BackgroundWeight = 1
	}
	if o.AnonymousWeight <= 0 {
		o.AnonymousWeight = 1
	}
	if o.SystemWeight <= 0 {
		o.SystemWeight = 1
	}
	idle := make(chan struct{})
	close(idle)
	return &Controller{options: o, principals: make(map[identity]*principal), idle: idle}
}

// Pause excludes new owners and drains existing reservations, including
// preempted segments, which it readmits so they can reach a real boundary.
// Callers must not own a reservation or run admission-taking hooks until the
// returned resume.
// Several maintenance callers can pause concurrently; only the last resumes.
func (c *Controller) Pause(ctx context.Context) (func(), error) {
	c.mu.Lock()
	c.paused++
	idle := c.idle
	c.mu.Unlock()
	var once sync.Once
	resume := func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.paused--
			c.dispatchLocked()
		})
	}
	select {
	case <-idle:
		if err := ctx.Err(); err != nil {
			resume()
			return nil, err
		}
		return resume, nil
	case <-ctx.Done():
		resume()
		return nil, ctx.Err()
	}
}

func (c *Controller) Enqueue(key Key) *Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	q := c.enqueueLocked(key, false)
	c.dispatchLocked()
	return q
}
func (c *Controller) enqueueLocked(key Key, resume bool) *Request {
	p := c.principals[key.identity()]
	if p == nil {
		p = &principal{estimate: [2]time.Duration{time.Millisecond, time.Millisecond}}
		c.principals[key.identity()] = p
	}
	if p.active+p.queued == 0 {
		p.settled = max(p.settled, c.watermark)
	}
	cl := key.Class
	if p.classActive[cl]+p.classQueued[cl] == 0 {
		p.classes[cl].settled = max(p.classes[cl].settled, p.classWatermark)
	}
	p.queued++
	p.classQueued[cl]++
	q := &Request{c: c, key: key, ready: make(chan *Reservation, 1), queued: true, resume: resume}
	c.queue = append(c.queue, q)
	c.waiting.Store(int32(len(c.queue)))
	return q
}
func (q *Request) Ready() <-chan *Reservation { return q.ready }

// Cancel only removes an ungranted request. A grant belongs to its recipient;
// cancellation of its originating context cannot revoke a running invocation.
func (q *Request) Cancel() bool {
	c := q.c
	c.mu.Lock()
	defer c.mu.Unlock()
	if !q.queued {
		return false
	}
	for i, r := range c.queue {
		if r == q {
			c.queue = append(c.queue[:i], c.queue[i+1:]...)
			break
		}
	}
	c.waiting.Store(int32(len(c.queue)))
	q.queued = false
	p := c.principals[q.key.identity()]
	p.queued--
	p.classQueued[q.key.Class]--
	c.dispatchLocked()
	return true
}
func (c *Controller) Acquire(ctx context.Context, key Key) (*Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	q := c.Enqueue(key)
	select {
	case r := <-q.ready:
		if err := ctx.Err(); err != nil {
			r.Finish(0, 0)
			return nil, err
		}
		return r, nil
	case <-ctx.Done():
		if !q.Cancel() {
			(<-q.ready).Finish(0, 0)
		}
		return nil, ctx.Err()
	}
}
func less(a, b ledger) bool {
	av, bv := a.settled+a.reserved, b.settled+b.reserved
	return av < bv || av == bv && a.last < b.last
}
func (c *Controller) dispatchLocked() {
	for c.active < c.options.Limit {
		best := -1
		for i, q := range c.queue {
			if c.paused != 0 && !q.resume {
				continue
			}
			p := c.principals[q.key.identity()]
			if p.active >= c.options.PrincipalLimit {
				continue
			}
			if best < 0 {
				best = i
				continue
			}
			b := c.queue[best]
			bp := c.principals[b.key.identity()]
			if p != bp && less(p.ledger, bp.ledger) || p == bp && less(p.classes[q.key.Class], p.classes[b.key.Class]) {
				best = i
			}
		}
		if best < 0 {
			return
		}
		q := c.queue[best]
		c.queue = append(c.queue[:best], c.queue[best+1:]...)
		c.waiting.Store(int32(len(c.queue)))
		q.queued = false
		p := c.principals[q.key.identity()]
		cl := q.key.Class
		w := 1.0
		if q.key.Group == Anonymous {
			w = float64(c.options.AnonymousWeight)
		}
		if q.key.Group == System {
			w = float64(c.options.SystemWeight)
		}
		cw := float64(c.options.InputWeight)
		if cl == Background {
			cw = float64(c.options.BackgroundWeight)
		}
		r := &Reservation{c: c, key: q.key, estimate: p.estimate[cl], weight: w, classWeight: cw}
		p.reserved += float64(r.estimate) / w
		p.classes[cl].reserved += float64(r.estimate) / cw
		c.sequence++
		p.last = c.sequence
		p.classes[cl].last = c.sequence
		p.queued--
		p.classQueued[cl]--
		p.active++
		p.classActive[cl]++
		if c.active+c.preempted == 0 {
			c.idle = make(chan struct{})
		}
		if q.resume {
			c.preempted--
		}
		c.active++
		q.ready <- r
	}
}

// Finish replaces the dispatch estimate with measured execution occupancy.
// Gate wait is retained separately. A zero elapsed duration cancels an unstarted
// grant without service charge. Only the invocation owner calls Finish.
func (r *Reservation) Finish(elapsed, gateWait time.Duration) {
	c := r.c
	c.mu.Lock()
	defer c.mu.Unlock()
	r.settleLocked(elapsed, gateWait)
	c.dispatchLocked()
}

// preempt settles the owner's segment and queues its readmission atomically,
// so a concurrent Pause can never observe the in-flight invocation as idle.
func (r *Reservation) preempt(elapsed, gateWait time.Duration) *Request {
	c := r.c
	c.mu.Lock()
	defer c.mu.Unlock()
	c.preempted++
	c.preemptions++
	q := c.enqueueLocked(r.key, true)
	r.settleLocked(elapsed, gateWait)
	c.dispatchLocked()
	return q
}

func (r *Reservation) settleLocked(elapsed, gateWait time.Duration) {
	c := r.c
	if r.finished {
		return
	}
	r.finished = true
	p := c.principals[r.key.identity()]
	cl := r.key.Class
	p.reserved -= float64(r.estimate) / r.weight
	p.classes[cl].reserved -= float64(r.estimate) / r.classWeight
	p.active--
	p.classActive[cl]--
	c.active--
	if c.active+c.preempted == 0 {
		close(c.idle)
	}
	gateWait = min(max(gateWait, 0), elapsed)
	service := max(elapsed-gateWait, 0)
	if elapsed > 0 {
		service = max(service, time.Microsecond)
	}
	c.service += service
	c.gateWait += gateWait
	p.settled += float64(service) / r.weight
	p.classes[cl].settled += float64(service) / r.classWeight
	if service > 0 {
		p.estimate[cl] = min(max((7*p.estimate[cl]+service)/8, time.Microsecond), 100*time.Millisecond)
	}
	minimum := p.settled
	for _, other := range c.principals {
		if other.active+other.queued > 0 {
			minimum = min(minimum, other.settled)
		}
	}
	c.watermark = max(c.watermark, minimum)
	classMin := p.classes[cl].settled
	for i := range p.classes {
		if p.classActive[i]+p.classQueued[i] > 0 {
			classMin = min(classMin, p.classes[i].settled)
		}
	}
	p.classWatermark = max(p.classWatermark, classMin)
}
func (r *Reservation) Active() bool { r.c.mu.Lock(); defer r.c.mu.Unlock(); return !r.finished }
func (c *Controller) Snapshot() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Stats{Active: c.active, Queued: len(c.queue), Preempted: c.preempted, Preemptions: c.preemptions,
		Service: c.service, GateWait: c.gateWait, Maintenance: c.maintenance}
}

// Order ranks candidates against one consistent ledger snapshot. It does not
// grant execution. Dormant principals cannot regain old idle credit.
func (c *Controller) Order(keys []Key) []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	value := func(k Key) ledger {
		if p := c.principals[k.identity()]; p != nil {
			v := p.ledger
			if p.active+p.queued == 0 {
				v.settled = max(v.settled, c.watermark)
			}
			return v
		}
		return ledger{settled: c.watermark}
	}
	order := make([]int, len(keys))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return less(value(keys[order[i]]), value(keys[order[j]])) })
	return order
}

// Maintenance is exempt from admission because it owns the GC start barrier.
// Keep its occupancy visible separately; it is not principal execution service.
func (c *Controller) NoteMaintenance(elapsed time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maintenance += max(elapsed, 0)
}
