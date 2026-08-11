package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestRuntimesOwnIsolatedTaskManagers(t *testing.T) {
	first := NewRuntime(dbstore.NewStore())
	second := NewRuntime(dbstore.NewStore())
	t.Cleanup(first.Stop)
	t.Cleanup(second.Stop)

	if first.taskManager == second.taskManager {
		t.Fatal("two runtimes share one task manager")
	}

	firstTask := task.NewTask(101, 7, 1000, 10)
	secondTask := task.NewTask(202, 8, 1000, 10)
	first.QueueTask(firstTask)
	second.QueueTask(secondTask)

	if got := first.taskManager.GetTask(firstTask.ID); got != firstTask {
		t.Fatalf("first manager task = %p, want %p", got, firstTask)
	}
	if got := first.taskManager.GetTask(secondTask.ID); got != nil {
		t.Fatalf("first manager contains second task %d", got.ID)
	}
	if got := second.taskManager.GetTask(secondTask.ID); got != secondTask {
		t.Fatalf("second manager task = %p, want %p", got, secondTask)
	}
	if got := second.taskManager.GetTask(firstTask.ID); got != nil {
		t.Fatalf("second manager contains first task %d", got.ID)
	}

	assertQueuedTask := func(t *testing.T, rt *Runtime, wantID int64) {
		t.Helper()
		ctx := kernel.NewTaskContext()
		ctx.Programmer = types.ObjID(0)
		ctx.IsWizard = true

		result, ok := rt.registry.CallByNameWithExecution("queued_tasks", rt.registry.NewExecution(ctx, nil), nil)
		if !ok {
			t.Fatal("queued_tasks is not registered")
		}
		if result.Error != types.E_NONE {
			t.Fatalf("queued_tasks error = %s", result.Error)
		}
		entries := result.Val.Elements()
		if len(entries) != 1 || entries[0].Get(1).Int() != wantID {
			t.Fatalf("queued_tasks = %s, want only task %d", result.Val, wantID)
		}
	}
	assertQueuedTask(t, first, firstTask.ID)
	assertQueuedTask(t, second, secondTask.ID)

	first.taskManager.SuspendTask(firstTask, -1)
	if got := firstTask.GetState(); got != task.TaskSuspended {
		t.Fatalf("first task state after suspend = %v, want suspended", got)
	}
	if got := secondTask.GetState(); got != task.TaskQueued {
		t.Fatalf("second task state changed with first suspend: %v", got)
	}

	if errCode := first.taskManager.ResumeTask(firstTask.ID, types.NewInt(42), firstTask.Owner, false); errCode != types.E_NONE {
		t.Fatalf("resume first task: %s", errCode)
	}
	if got := firstTask.GetState(); got != task.TaskQueued {
		t.Fatalf("first task state after resume = %v, want queued", got)
	}
	if got := secondTask.GetState(); got != task.TaskQueued {
		t.Fatalf("second task state changed with first resume: %v", got)
	}

	first.taskManager.RemoveTask(firstTask.ID)
	if got := first.taskManager.GetTask(firstTask.ID); got != nil {
		t.Fatalf("first task remains after removal: %d", got.ID)
	}
	if got := second.taskManager.GetTask(secondTask.ID); got != secondTask {
		t.Fatalf("removing first task removed second task: got %p, want %p", got, secondTask)
	}
}
