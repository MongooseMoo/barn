package engine

import (
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"testing"
)

func TestProcessReadyBatchRetainsTasksBetweenSelections(t *testing.T) {
	s := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{}, 1)
	defer s.Stop()
	program := compileTestProgram(t, s.registry, "return 1;")
	firstID := s.CreateBackgroundTask(types.ObjNothing, program, 0)
	secondID := s.CreateBackgroundTask(types.ObjNothing, program, 0)
	if got := s.ProcessReadyBatch(); got != 1 {
		t.Fatalf("first selection = %d, want 1", got)
	}
	if s.GetTask(firstID).GetState() != task.TaskCompleted || s.GetTask(secondID).GetState() != task.TaskQueued {
		t.Fatal("bounded selection did not leave its sibling queued")
	}
	// The all-ready API must also see pending work removed from the heap.
	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("remaining selection = %d, want 1", got)
	}
	if s.GetTask(secondID).GetState() != task.TaskCompleted {
		t.Fatal("pending task was lost")
	}
	if got := s.ProcessReadyBatch(); got != 0 {
		t.Fatalf("completed tasks selected again: %d", got)
	}
}

// On the concurrent (MVCC) scheduler, a pass runs all ready optimistic tasks in
// one batch rather than one task per pass — so two ready background tasks both
// complete in a single ProcessReadyTasks call.
func TestProcessReadyTasksRunsAllReadyTasksInOnePass(t *testing.T) {
	s := NewRuntime(dbstore.NewStore())
	program, diagnostics := s.registry.Compiler().CompileMOO([]string{"return 1;"})
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
