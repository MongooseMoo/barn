package engine

import (
	"testing"
	"time"
)

// A world can hold hundreds of delayed forks. A scheduling scan runs under the
// scheduler's lock on every dispatch and every wake computation, so it must not
// take the lock of a task that is in the ready-time heap and not yet due.
// The test holds two such tasks' locks: a scan that visits them cannot finish.
func TestSchedulingScansDoNotLockDelayedForksThatAreNotDue(t *testing.T) {
	s := newScheduler(2, func(*Task) bool { return true }, func(*Task) error { return nil })
	t.Cleanup(s.Stop)
	now := time.Now()
	first := testTask(1, now.Add(time.Hour))
	second := testTask(2, now.Add(2*time.Hour))
	third := testTask(3, now.Add(3*time.Hour))
	catalog := []*Task{first, second, third}
	for _, queued := range catalog {
		s.Enqueue(queued)
	}

	second.Mu.Lock()
	third.Mu.Lock()
	unlocked := false
	unlock := func() {
		if !unlocked {
			unlocked = true
			second.Mu.Unlock()
			third.Mu.Unlock()
		}
	}
	defer unlock()

	type scan struct {
		started int
		wake    time.Time
	}
	done := make(chan scan, 1)
	go func() {
		started := s.Dispatch(now, catalog, func(*Task) bool { return true }, func(schedulerResult) {})
		done <- scan{started: started, wake: s.NextWake(now, catalog)}
	}()
	select {
	case got := <-done:
		if got.started != 0 {
			t.Fatalf("Dispatch started %d tasks, want none before any is due", got.started)
		}
		if !got.wake.Equal(first.StartTime) {
			t.Fatalf("NextWake = %v, want the earliest delayed fork at %v", got.wake, first.StartTime)
		}
	case <-time.After(10 * time.Second):
		unlock()
		<-done
		t.Fatal("a scheduling scan waited on the lock of a delayed fork that is not due")
	}

	// The skipped tasks are still scheduled: each runs once it is due.
	unlock()
	ran := make(chan int64, len(catalog))
	later := now.Add(4 * time.Hour)
	started := s.Dispatch(later, catalog, func(*Task) bool { return true }, func(result schedulerResult) { ran <- result.Task.ID })
	if started != 2 {
		t.Fatalf("Dispatch at the later time started %d tasks, want the 2 the workers allow", started)
	}
	for range started {
		select {
		case <-ran:
		case <-time.After(10 * time.Second):
			t.Fatal("a due delayed fork did not run")
		}
	}
}
