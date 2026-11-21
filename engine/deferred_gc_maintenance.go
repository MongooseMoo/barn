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

func (s *Runtime) noteDeferredGCQueuedLocked() uint64 {
	if s.lifecycle.PendingSince.IsZero() {
		s.lifecycle.PendingSince = time.Now()
	}
	s.lifecycle.QueuedSeq++
	s.wakeDeferredGCMaintenanceLocked()
	return s.lifecycle.QueuedSeq
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
	// A suspended task is held until its batch is swept, and its own slice
	// boundary could not sweep it: sweep now.
	if s.lifecycle.WaiterUnserved && s.finalizationAwaitedLocked() {
		deadline = time.Now()
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
			// with a later one, since the timer was armed. Never park VMs for
			// an empty or no-longer-due batch.
			if deadline := s.deferredGCDeadline(); !deadline.IsZero() && !deadline.After(time.Now()) {
				s.rendezvousDeferredGC()
			}
		}
	}
}

func (s *Runtime) rendezvousDeferredGC() {
	// This goroutine never owns an admission reservation or physical VM lease.
	// Admission keeps flowing: executing VMs park at their tick checkpoint for
	// the walk and sweep only, and new VM starts wait for the sweep only.
	s.flushDeferredGCBarrier(maintenanceBarrierPatience, true)
	s.lifecycle.Mu.Lock()
	// A checkpoint or another sweep may own the barriers, or a lease may not
	// have parked. Give up instead of spinning or blocking cancellation; the
	// next deadline retries that pending batch.
	if len(s.lifecycle.PendingWaifs) != 0 || len(s.lifecycle.PendingAnonGC) != 0 {
		retry := gcSweepInterval
		if s.lifecycle.WaiterUnserved && s.finalizationAwaitedLocked() {
			retry = maintenanceBarrierPatience
		}
		s.lifecycle.RetryAfter = time.Now().Add(retry)
	}
	s.lifecycle.Mu.Unlock()
}
