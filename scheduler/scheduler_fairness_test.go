package scheduler

import (
	"testing"

	"barn/compiler"
	dbstore "barn/db/store"
	"barn/task"
	"barn/types"
)

// On the concurrent (MVCC) scheduler, a pass runs all ready optimistic tasks in
// one batch rather than one task per pass — so two ready background tasks both
// complete in a single ProcessReadyTasks call.
func TestProcessReadyTasksRunsAllReadyTasksInOnePass(t *testing.T) {
	resetServerVerbTaskManager(t)
	t.Cleanup(func() { resetServerVerbTaskManager(t) })

	s := NewScheduler(dbstore.NewStore())
	program, diagnostics := compiler.CompileMOO([]string{"return 1;"}, s.registry)
	if len(diagnostics) != 0 {
		t.Fatalf("compile task: %v", diagnostics)
	}

	firstID := s.CreateBackgroundTask(types.ObjNothing, program, 0)
	secondID := s.CreateBackgroundTask(types.ObjNothing, program, 0)

	if got := s.ProcessReadyTasks(); got != 2 {
		t.Fatalf("scheduler pass ran %d tasks, want 2", got)
	}
	if got := s.GetTask(firstID).GetState(); got != task.TaskCompleted {
		t.Fatalf("first task state = %v, want completed", got)
	}
	if got := s.GetTask(secondID).GetState(); got != task.TaskCompleted {
		t.Fatalf("second task state = %v, want completed", got)
	}
}
