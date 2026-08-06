package scheduler

import (
	"strings"
	"testing"

	dbstore "barn/db/store"
	"barn/task"
	"barn/types"
)

func TestRunTaskSettlesDeferredAnonymousGCAfterExecutionLeaseRelease(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}

	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	defer removeTasksForOwner(scheduler, 0)
	program := compileTestProgram(t, scheduler.registry, "create(#0, 1); return 1;")
	taskID := scheduler.CreateBackgroundTask(0, program, 0)
	orphanID := store.NextID()

	if err := scheduler.runTask(scheduler.GetTask(taskID)); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if store.Valid(orphanID) {
		t.Fatalf("anonymous orphan #%d remained valid after synchronous runTask returned", orphanID)
	}
	scheduler.pendingWaifMu.Lock()
	pending := len(scheduler.pendingAnonGC)
	scheduler.pendingWaifMu.Unlock()
	if pending != 0 {
		t.Fatalf("pending anonymous GC batches after synchronous runTask = %d, want 0", pending)
	}
}

func TestRunTaskPanicAfterGCDeferralStillSettlesAfterLeaseRelease(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}

	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	defer removeTasksForOwner(scheduler, 0)
	program := compileTestProgram(t, scheduler.registry, "create(#0, 1); return 1;")
	taskID := scheduler.CreateBackgroundTask(0, program, 0)
	orphanID := store.NextID()
	taskWithPanic := scheduler.GetTask(taskID)
	taskWithPanic.OnComplete = func(types.Result) { panic("completion barrier") }

	err := scheduler.runTask(taskWithPanic)
	if err == nil || !strings.Contains(err.Error(), "completion barrier") {
		t.Fatalf("runTask panic result = %v, want recovered completion panic", err)
	}
	if state := taskWithPanic.GetState(); state != task.TaskKilled {
		t.Fatalf("task state after recovered panic = %v, want killed", state)
	}
	if store.Valid(orphanID) {
		t.Fatalf("anonymous orphan #%d remained valid after recovered task panic", orphanID)
	}
	scheduler.pendingWaifMu.Lock()
	pending := len(scheduler.pendingAnonGC)
	scheduler.pendingWaifMu.Unlock()
	if pending != 0 {
		t.Fatalf("pending anonymous GC batches after recovered task panic = %d, want 0", pending)
	}
}
