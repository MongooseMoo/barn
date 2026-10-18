package task

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Toast's do_resume finds a task woken from a suspension until its slice starts,
// and only replaces the value its suspend returns.
func TestResumeTaskReachesWokenTaskUntilItRuns(t *testing.T) {
	manager := NewManager()
	parked := NewTask(1, 2, 100, 5)
	manager.RegisterTask(parked)
	parked.SetState(TaskRunning)
	manager.SuspendTask(parked, 0)
	if state := parked.GetState(); state != TaskQueued {
		t.Fatalf("suspend(0) state = %v, want queued", state)
	}

	if errCode := manager.ResumeTask(1, types.NewInt(42), 2, false, nil); errCode != types.E_NONE {
		t.Fatalf("resume of a task in suspend(0) = %v, want E_NONE", errCode)
	}
	if errCode := manager.ResumeTask(1, types.NewInt(43), 2, false, nil); errCode != types.E_NONE {
		t.Fatalf("second resume before it runs = %v, want E_NONE", errCode)
	}
	if !parked.WakeValue.Equal(types.NewInt(43)) {
		t.Fatalf("wake value = %v, want the latest resume value 43", parked.WakeValue)
	}

	if !parked.TryClaimQueued() {
		t.Fatal("woken task could not be claimed")
	}
	if errCode := manager.ResumeTask(1, types.NewInt(44), 2, false, nil); errCode != types.E_INVARG {
		t.Fatalf("resume of a running task = %v, want E_INVARG", errCode)
	}
}

func TestResumeTaskRejectsUnrunForkedTask(t *testing.T) {
	manager := NewManager()
	forked := NewTask(1, 2, 100, 5)
	manager.RegisterTask(forked)
	forked.SetState(TaskQueued)
	if errCode := manager.ResumeTask(1, types.NewInt(42), 2, false, nil); errCode != types.E_INVARG {
		t.Fatalf("resume of a forked task that has not run = %v, want E_INVARG", errCode)
	}
}
