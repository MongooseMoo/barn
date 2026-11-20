package task

import (
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// Choice pairs are concrete actions and generation selectors. Truncation
// removes a suffix; every remaining action has a defined precondition. The
// model contains only suspension identity, terminal state and wake readiness.
func FuzzSuspensionCompletionHistory(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0, 1, 0, 1, 1, 1, 1, 2, 0, 3, 0, 1, 1})
	f.Add([]byte{0, 0, 4, 0, 0, 0, 1, 0, 2, 0})
	f.Fuzz(func(t *testing.T, actions []byte) {
		checkSuspensionHistory(t, actions)
	})
}

func checkSuspensionHistory(t *testing.T, actions []byte) {
	t.Helper()
	tk := NewState(1, 2, 100, 5)
	state := TaskCreated
	var generations []uint64
	var generation uint64
	var ready bool
	wake := types.NewInt(0)
	if tk.GetState() != state || tk.SuspendGeneration() != 0 || !tk.ExecReadyTime().IsZero() || !tk.WakeValue.Identical(wake) {
		t.Fatal("new task violates the initial lifecycle model")
	}
	if len(actions) > 256 {
		actions = actions[:256]
	}
	for i := 0; i+1 < len(actions); i += 2 {
		op, selector := actions[i]%5, actions[i+1]
		t.Logf("step %d: action=%d selector=%d", i/2, op, selector)
		switch op {
		case 0:
			if state == TaskKilled {
				break // cancellation is terminal; do not manufacture resurrection
			}
			tk.SetState(TaskRunning)
			tk.SuspendIndefinite()
			generation++
			generations = append(generations, generation)
			state = TaskSuspended
		case 1:
			if len(generations) == 0 {
				break
			}
			selected := generations[int(selector)%len(generations)]
			want := state == TaskSuspended && selected == generation
			value := types.NewInt(int64(i + 1))
			if got := tk.ResumeGeneration(selected, value); got != want {
				t.Fatalf("resume generation %d = %v, want %v", selected, got, want)
			}
			if want {
				state, ready, wake = TaskQueued, true, value
			}
		case 2:
			if observed := tk.TakeExecReadyTime(); observed.IsZero() == ready {
				t.Fatalf("readiness=%v, want present=%v", observed, ready)
			}
			ready = false
		case 3:
			tk.Kill()
			state = TaskKilled
		case 4:
			// Starting a new task is a new obligation and generation namespace.
			tk = NewState(int64(i+2), 2, 100, 5)
			state, generation, ready, wake = TaskCreated, 0, false, types.NewInt(0)
			generations = nil
		}
		if got := tk.GetState(); got != state {
			t.Fatalf("state=%v, want %v", got, state)
		}
		if got := tk.SuspendGeneration(); got != generation {
			t.Fatalf("generation=%d, want %d", got, generation)
		}
		if tk.ExecReadyTime().IsZero() == ready {
			t.Fatal("readiness differs from accepted/consumed completions")
		}
		if !tk.WakeValue.Identical(wake) {
			t.Fatalf("wake=%v, want %v", tk.WakeValue, wake)
		}
	}
}

func TestCompletionHistoryOverlapsCancellation(t *testing.T) {
	for round := 0; round < 32; round++ {
		tk := NewState(1, 2, 100, 5)
		tk.SetState(TaskRunning)
		tk.SuspendIndefinite()
		generation := tk.SuspendGeneration()
		start := make(chan struct{})
		var workers sync.WaitGroup
		for _, cancel := range []bool{false, true} {
			workers.Add(1)
			go func() {
				defer workers.Done()
				<-start
				if cancel {
					tk.Kill()
				} else {
					tk.ResumeGeneration(generation, types.NewInt(73))
				}
			}()
		}
		close(start)
		workers.Wait()
		if tk.GetState() != TaskKilled || !tk.CancellationRequested() {
			t.Fatal("completed cancellation was lost")
		}
		if tk.ResumeGeneration(generation, types.NewInt(99)) {
			t.Fatal("completion resurrected cancelled task")
		}
	}
}

// A completion goroutine started for one suspension must not be able to wake a
// later suspension of the same task (the retried or re-suspended task would
// otherwise receive a value that belongs to an abandoned attempt).
func TestResumeGenerationIgnoresCompletionFromEarlierSuspension(t *testing.T) {
	tk := NewState(1, 2, 100, 5)
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
	if !tk.ExecReadyTime().IsZero() {
		t.Fatal("stale completion published a readiness timestamp")
	}
	if got := tk.GetState(); got != TaskSuspended {
		t.Fatalf("state after stale completion = %v, want TaskSuspended", got)
	}
	if !tk.ResumeGeneration(current, types.NewStr("own")) {
		t.Fatal("current completion failed to resume the task")
	}
	ready := tk.ExecReadyTime()
	if ready.IsZero() {
		t.Fatal("threaded completion did not publish its readiness timestamp")
	}
	if tk.ResumeGeneration(current, types.NewStr("duplicate")) || !tk.ExecReadyTime().Equal(ready) {
		t.Fatal("duplicate completion changed readiness")
	}
	if !tk.TakeExecReadyTime().Equal(ready) || !tk.TakeExecReadyTime().IsZero() {
		t.Fatal("readiness timestamp was not consumed exactly once")
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
	tk := NewState(1, 2, 100, 5)
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
