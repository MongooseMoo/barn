package admission

import (
	"context"
	"sync/atomic"
	"time"
)

// Scope follows one synchronous invocation and is explicitly borrowed by its
// nested VMs. Only its owner starts/finishes segments. A real MOO suspension
// ends a segment; reacquisition can therefore let the awaited child run.
type Scope struct {
	controller  *Controller
	key         Key
	reservation *Reservation
	started     time.Time
	wait        atomic.Int64
}

func (c *Controller) Enter(ctx context.Context, key Key) (*Scope, error) {
	s := &Scope{controller: c, key: key}
	if err := s.Resume(ctx); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Scope) Resume(ctx context.Context) error {
	r, err := s.controller.Acquire(ctx, s.key)
	if err != nil {
		return err
	}
	s.reservation = r
	s.started = time.Time{}
	s.wait.Store(0)
	return nil
}

func (s *Scope) ResumeBackground(ctx context.Context, principal int64) error {
	s.key.Principal, s.key.Class = principal, Background
	return s.Resume(ctx)
}

// Start begins service after the physical VM lease, excluding GC barrier wait.
func (s *Scope) Start() { s.started = time.Now() }

// Yield lends the reservation to waiting work once this segment has used a
// quantum, and returns how long readmission took. The owner keeps its VM,
// transaction and physical lease: this is neither a MOO suspension nor a commit
// boundary. Only a root VM holding no gate or lock another admitted invocation
// could need may yield; readmission is therefore not cancellable.
func (s *Scope) Yield() time.Duration {
	if s == nil || s.reservation == nil || s.started.IsZero() || s.controller.waiting.Load() == 0 {
		return 0
	}
	elapsed := time.Since(s.started)
	if elapsed < Quantum {
		return 0
	}
	start := time.Now()
	q := s.reservation.preempt(elapsed, time.Duration(s.wait.Load()))
	s.reservation = <-q.ready
	s.wait.Store(0)
	s.started = time.Now()
	return s.started.Sub(start)
}
func (s *Scope) Waited(d time.Duration) { s.wait.Add(int64(d)) }
func (s *Scope) Finish() {
	if s == nil || s.reservation == nil {
		return
	}
	elapsed := time.Duration(0)
	if !s.started.IsZero() {
		// A coarse platform clock can report zero for a completed tiny slice.
		// Zero is reserved for an unstarted/refunded grant, so retain its floor.
		elapsed = max(time.Since(s.started), time.Nanosecond)
	}
	s.reservation.Finish(elapsed, time.Duration(s.wait.Load()))
	s.reservation = nil
}
