package engine

import "time"

// All deadline state is owned by lifecycle.Mu. Queue notifications are hints:
// the worker always rereads the pending batches and holds before arming a timer.
func (s *Runtime) wakeDeferredGCMaintenanceLocked() {
	select {
	case s.lifecycle.MaintenanceWake <- struct{}{}:
	default:
	}
}

func (s *Runtime) noteDeferredGCQueuedLocked() {
	if s.lifecycle.PendingSince.IsZero() {
		s.lifecycle.PendingSince = time.Now()
	}
	s.wakeDeferredGCMaintenanceLocked()
}

func (s *Runtime) deferredGCDeadline() time.Time {
	s.lifecycle.Mu.Lock()
	defer s.lifecycle.Mu.Unlock()
	if s.lifecycle.ShutdownRequested || s.lifecycle.FinalizationHeld || s.lifecycle.GCRunning ||
		(len(s.lifecycle.PendingWaifs) == 0 && len(s.lifecycle.PendingAnonGC) == 0) {
		return time.Time{}
	}
	// Cheap collections get the ordinary task boundaries first, but repeated
	// overlap must not postpone them forever. Expensive sweeps retain their
	// existing cadence, even if the last task has already left the runtime idle.
	deadline := s.lifecycle.PendingSince.Add(gcSweepInterval)
	if s.lifecycle.LastGCCost >= cheapGCSweep {
		deadline = s.lifecycle.LastGCSweep.Add(gcSweepInterval)
	}
	if deadline.Before(s.lifecycle.RetryAfter) {
		deadline = s.lifecycle.RetryAfter
	}
	return deadline
}

func (s *Runtime) runDeferredGCMaintenance() {
	defer close(s.lifecycle.MaintenanceDone)
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	for {
		var due <-chan time.Time
		if deadline := s.deferredGCDeadline(); !deadline.IsZero() {
			timer.Reset(time.Until(deadline))
			due = timer.C
		}
		select {
		case <-s.ctx.Done():
			return
		case <-s.lifecycle.MaintenanceWake:
			timer.Stop()
		case <-due:
			// An opportunistic sweep may have drained this batch, or replaced it
			// with a later one, since the timer was armed. Never pause input for
			// an empty or no-longer-due batch.
			if deadline := s.deferredGCDeadline(); !deadline.IsZero() && !deadline.After(time.Now()) {
				s.rendezvousDeferredGC()
			}
		}
	}
}

func (s *Runtime) rendezvousDeferredGC() {
	// This goroutine never owns an admission reservation or physical VM lease.
	// Pause stops fresh grants and readmits preempted owners until they reach a
	// true handoff. Taking VMStartMu before this drain would deadlock an owner
	// that has admission but has not acquired its execution lease yet.
	resume, err := s.admission.Pause(s.ctx)
	if err != nil {
		return
	}
	defer resume()
	s.flushDeferredGC()
	s.lifecycle.Mu.Lock()
	// A checkpoint or another sweep may own the barriers, or an explicit lease
	// outside admission may still be live. Release the pause instead of spinning
	// or blocking cancellation; the next deadline retries that pending batch.
	if len(s.lifecycle.PendingWaifs) != 0 || len(s.lifecycle.PendingAnonGC) != 0 {
		s.lifecycle.RetryAfter = time.Now().Add(gcSweepInterval)
	}
	s.lifecycle.Mu.Unlock()
}
