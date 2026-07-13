package scheduler

import (
	"testing"

	"barn/compiler"
	dbstore "barn/db/store"
	"barn/task"
	"barn/types"
)

func TestProcessReadyTasksReturnsAfterOneRunnableTask(t *testing.T) {
	resetServerVerbTaskManager(t)
	t.Cleanup(func() { resetServerVerbTaskManager(t) })

	s := NewScheduler(dbstore.NewStore())
	program, diagnostics := compiler.CompileMOO([]string{"return 1;"}, s.registry)
	if len(diagnostics) != 0 {
		t.Fatalf("compile task: %v", diagnostics)
	}

	firstID := s.CreateBackgroundTask(types.ObjNothing, program, 0)
	secondID := s.CreateBackgroundTask(types.ObjNothing, program, 0)

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("first scheduler pass ran %d tasks, want 1", got)
	}
	if got := s.GetTask(firstID).GetState(); got != task.TaskCompleted {
		t.Fatalf("first task state = %v, want completed", got)
	}
	if got := s.GetTask(secondID).GetState(); got != task.TaskQueued {
		t.Fatalf("second task state = %v, want queued for the next pass", got)
	}

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("second scheduler pass ran %d tasks, want 1", got)
	}
	if got := s.GetTask(secondID).GetState(); got != task.TaskCompleted {
		t.Fatalf("second task state = %v, want completed", got)
	}
}
