package task

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// A completion goroutine started for one suspension must not be able to wake a
// later suspension of the same task (the retried or re-suspended task would
// otherwise receive a value that belongs to an abandoned attempt).
func TestResumeGenerationIgnoresCompletionFromEarlierSuspension(t *testing.T) {
	tk := NewTask(1, 2, 100, 5)
	tk.SetState(TaskRunning)

	tk.SuspendIndefinite()
	stale := tk.SuspendGeneration()

	// The task is re-run from the top and suspends again for its own operation.
	tk.SetState(TaskRunning)
	tk.SuspendIndefinite()
	current := tk.SuspendGeneration()
	if current == stale {
		t.Fatalf("second suspension reused generation %d", stale)
	}

	if tk.ResumeGeneration(stale, types.NewStr("stale")) {
		t.Fatal("stale completion resumed the task")
	}
	if got := tk.GetState(); got != TaskSuspended {
		t.Fatalf("state after stale completion = %v, want TaskSuspended", got)
	}
	if !tk.ResumeGeneration(current, types.NewStr("own")) {
		t.Fatal("current completion failed to resume the task")
	}
	if got := tk.GetState(); got != TaskQueued {
		t.Fatalf("state after own completion = %v, want TaskQueued", got)
	}
	if tk.WakeValue.Type() != types.TYPE_STR || tk.WakeValue.Str() != "own" {
		t.Fatalf("wake value = %v, want own completion's value", tk.WakeValue)
	}
}

// An indefinite (or zero-length) suspension must not inherit a wake deadline
// from an earlier timed suspend: the scheduler would treat it as due and resume
// it with 0 before the operation it is waiting on completes.
func TestIndefiniteSuspendClearsStaleWakeTime(t *testing.T) {
	tk := NewTask(1, 2, 100, 5)
	tk.SetState(TaskRunning)

	tk.Suspend(time.Millisecond)
	later := time.Now().Add(time.Second)
	if !tk.WakeDue(later) {
		t.Fatal("timed suspend not due after its deadline")
	}
	if !tk.Resume(types.NewInt(0)) {
		t.Fatal("timed suspend did not resume")
	}

	tk.SetState(TaskRunning)
	tk.SuspendIndefinite()
	if tk.WakeDue(later) {
		t.Fatal("indefinite suspend reported due via a stale WakeTime")
	}
	if !tk.WakeTime.IsZero() {
		t.Fatalf("indefinite suspend WakeTime = %v, want zero", tk.WakeTime)
	}

	tk.SetState(TaskRunning)
	tk.Suspend(time.Millisecond)
	tk.SetState(TaskRunning)
	tk.Suspend(0)
	if !tk.WakeTime.IsZero() {
		t.Fatalf("suspend(0) WakeTime = %v, want zero", tk.WakeTime)
	}
}
