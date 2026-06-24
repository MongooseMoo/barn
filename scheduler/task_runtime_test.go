package scheduler

import (
	"testing"
	"time"

	dbstore "barn/db/store"
	"barn/parser"
	"barn/task"
	"barn/types"
)

// TestRunTaskStaleStartTimeDoesNotExpireDeadline covers a checkpoint-restored
// (or otherwise long-delayed) background task whose StartTime is already far
// in the past by the time it actually runs — e.g. the server was offline for
// a while between a checkpoint and the next restart. The budget deadline
// must be anchored to when the task actually starts executing, not to that
// stale StartTime, or it computes an already-expired context.Deadline and
// the task is killed instantly with context.DeadlineExceeded even though its
// code never got a chance to run.
func TestRunTaskStaleStartTimeDoesNotExpireDeadline(t *testing.T) {
	store := dbstore.NewStore()
	s := NewScheduler(store)

	p := parser.NewParser("return 1;")
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	taskID := s.CreateBackgroundTask(types.ObjNothing, stmts, 0)

	s.mu.Lock()
	bgTask := s.tasks[taskID]
	bgTask.StartTime = time.Now().Add(-10 * time.Minute)
	s.mu.Unlock()

	s.ProcessReadyTasks()

	if got := bgTask.GetState(); got != task.TaskCompleted {
		t.Fatalf("task state = %v, want TaskCompleted (stale StartTime should not expire the budget deadline)", got)
	}
}
