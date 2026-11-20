package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/task"
)

// Toast's do_resume finds a task woken from a suspension until its slice starts,
// and only replaces the value its suspend returns.
func TestResumeTaskReachesWokenTaskUntilItRuns(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()
	parked := registerRowTask(t, s, 7101, 5, "return suspend(0);")
	if err := s.runTask(parked); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if state := parked.GetState(); state != task.TaskQueued {
		t.Fatalf("suspend(0) state = %v, want queued", state)
	}

	if got := resumeVia(t, s, 7101, "42"); got != "0" {
		t.Fatalf("resume of a task in suspend(0) = %s, want 0", got)
	}
	if got := resumeVia(t, s, 7101, "43"); got != "0" {
		t.Fatalf("second resume before it runs = %s, want 0", got)
	}
	s.ProcessReadyTasks()
	if got := resultString(parked); got != "43" {
		t.Fatalf("suspend(0) returned %s, want the latest resume value 43", got)
	}
	if got := resumeVia(t, s, 7101, "44"); got != "E_INVARG" {
		t.Fatalf("resume of a task that has run = %s, want E_INVARG", got)
	}
}

func TestResumeTaskRejectsUnrunForkedTask(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()
	defer removeTasksForOwner(s, 5)
	parent := newRowTask(t, s, 7201, 5, "fork id (3600) endfork return `resume(id, 42) ! ANY';")
	if err := s.runTask(parent); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if got := resultString(parent); got != "E_INVARG" {
		t.Fatalf("resume of a forked task that has not run = %s, want E_INVARG", got)
	}
}
