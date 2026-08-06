package scheduler

import (
	"sync"
	"testing"
	"time"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
)

func TestExplicitRunGCPreservesAnonymousCycleHeldBySuspendedSiblingVMs(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}

	anonA, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous A: %v", errCode)
	}
	anonB, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous B: %v", errCode)
	}
	if errCode := store.DefineProperty(anonA, "next", dbstore.NewProperty(types.NewAnon(anonB), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define A.next: %v", errCode)
	}
	if errCode := store.DefineProperty(anonB, "next", dbstore.NewProperty(types.NewAnon(anonA), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define B.next: %v", errCode)
	}
	for name, value := range map[string]types.Value{
		"hold_left":  types.NewAnon(anonA),
		"hold_right": types.NewAnon(anonB),
	} {
		if errCode := store.DefineProperty(0, name, dbstore.NewProperty(value, 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
			t.Fatalf("define #%d.%s: %v", 0, name, errCode)
		}
	}

	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	defer removeTasksForOwner(scheduler, 0)
	taskIDs := make([]int64, 0, 2)
	for _, property := range []string{"hold_left", "hold_right"} {
		program := compileTestProgram(t, scheduler.registry, "held = #0."+property+"; suspend(); return held;")
		taskIDs = append(taskIDs, scheduler.CreateBackgroundTask(0, program, 0))
	}
	if got := scheduler.ProcessReadyTasks(); got != 2 {
		t.Fatalf("processed tasks = %d, want two holders", got)
	}
	for _, taskID := range taskIDs {
		if state := scheduler.GetTask(taskID).GetState(); state != task.TaskSuspended {
			t.Fatalf("task %d state = %v, want suspended", taskID, state)
		}
	}
	for _, property := range []string{"hold_left", "hold_right"} {
		if errCode := store.DeleteDefinedProperty(0, property); errCode != types.E_NONE {
			t.Fatalf("delete #%d.%s: %v", 0, property, errCode)
		}
	}

	if lines := scheduler.EvalCommandOutput(0, "run_gc(); return 1;", "", ""); len(lines) != 1 || lines[0] != "{1, 1}" {
		t.Fatalf("run_gc eval output = %v, want successful return", lines)
	}
	for _, id := range []types.ObjID{anonA, anonB} {
		if !store.Valid(id) {
			t.Errorf("task-owned anonymous object #%d was recycled by explicit run_gc", id)
		}
	}
}

func TestExplicitRunGCSkipsSweepDuringSiblingSuspendHandoff(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}

	held, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}
	orphan, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create orphan candidate: %v", errCode)
	}
	for name, value := range map[string]types.Value{
		"hold_handoff": types.NewAnon(held),
		"hold_orphan":  types.NewAnon(orphan),
	} {
		if errCode := store.DefineProperty(0, name, dbstore.NewProperty(value, 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
			t.Fatalf("define #0.%s: %v", name, errCode)
		}
	}

	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	defer removeTasksForOwner(scheduler, 0)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandoff := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHandoff()
	scheduler.registry.Register("gc_suspend_barrier", func(ctx *kernel.TaskContext, _ []types.Value) types.Result {
		holder, ok := ctx.Task.(*task.Task)
		if !ok {
			return types.Err(types.E_INVARG)
		}
		task.GetManager().SuspendTask(holder, -1)
		close(entered)
		<-release
		return types.Suspend(-1)
	})

	program := compileTestProgram(t, scheduler.registry, "held = #0.hold_handoff; suspend(); gc_suspend_barrier(); return held;")
	taskID := scheduler.CreateBackgroundTask(0, program, 0)
	if got := scheduler.ProcessReadyTasks(); got != 1 {
		t.Fatalf("initial processed tasks = %d, want one holder", got)
	}
	holder := scheduler.GetTask(taskID)
	if state := holder.GetState(); state != task.TaskSuspended {
		t.Fatalf("task %d initial state = %v, want suspended", taskID, state)
	}
	savedBeforeResume := holder.BytecodeVMValue()
	if savedBeforeResume == nil {
		t.Fatalf("task %d has no saved VM before resume", taskID)
	}
	if !holder.Resume(types.NewInt(0)) {
		t.Fatalf("task %d did not resume", taskID)
	}

	processed := make(chan int, 1)
	go func() { processed <- scheduler.ProcessReadyTasks() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not reach suspend-transition barrier")
	}
	if state := holder.GetState(); state != task.TaskSuspended {
		t.Fatalf("task %d state = %v, want logical suspension at barrier", taskID, state)
	}
	if activeVM := holder.BytecodeVMValue(); activeVM == nil || activeVM != savedBeforeResume {
		t.Fatalf("task %d active saved VM = %p, want prior resumable VM %p", taskID, activeVM, savedBeforeResume)
	}
	if refs, _ := scheduler.collectSiblingGCRefs(nil); len(refs) != 0 {
		t.Fatalf("per-task GC scanned active sibling VM refs: %v", refs)
	}
	if _, _, ok := scheduler.collectAllGCRefs(); ok {
		t.Fatal("deferred GC admitted active suspend handoff")
	}
	for _, property := range []string{"hold_handoff", "hold_orphan"} {
		if errCode := store.DeleteDefinedProperty(0, property); errCode != types.E_NONE {
			t.Fatalf("delete #0.%s: %v", property, errCode)
		}
	}

	if lines := scheduler.EvalCommandOutput(0, "run_gc(); return 1;", "", ""); len(lines) != 1 || lines[0] != "{1, 1}" {
		t.Fatalf("run_gc eval output = %v, want successful return", lines)
	}
	for _, id := range []types.ObjID{held, orphan} {
		if !store.Valid(id) {
			t.Fatalf("anonymous object #%d was recycled during active suspend handoff", id)
		}
	}

	releaseHandoff()
	select {
	case got := <-processed:
		if got != 1 {
			t.Fatalf("processed tasks = %d, want one holder", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("task did not complete suspend handoff")
	}
	if state := holder.GetState(); state != task.TaskSuspended {
		t.Fatalf("task %d state after handoff = %v, want suspended", taskID, state)
	}
	if savedAfter := holder.BytecodeVMValue(); savedAfter == nil || savedAfter != savedBeforeResume {
		t.Fatalf("task %d saved VM after handoff = %p, want resumed VM %p", taskID, savedAfter, savedBeforeResume)
	}

	if lines := scheduler.EvalCommandOutput(0, "run_gc(); return 1;", "", ""); len(lines) != 1 || lines[0] != "{1, 1}" {
		t.Fatalf("post-handoff run_gc eval output = %v, want successful return", lines)
	}
	if !store.Valid(held) {
		t.Fatalf("saved suspended VM's anonymous object #%d was recycled", held)
	}
	if store.Valid(orphan) {
		t.Fatalf("separate orphan candidate #%d survived quiescent global GC", orphan)
	}
}

func TestExplicitGlobalGCExcludesOnlyOneCallerExecutionLease(t *testing.T) {
	scheduler := NewScheduler(dbstore.NewStore())
	defer scheduler.Stop()
	caller := task.NewTask(91001, 0, 1000, 10)

	scheduler.acquireTaskExecution(caller)
	scheduler.acquireTaskExecution(caller)
	t.Cleanup(func() {
		scheduler.releaseTaskExecution(caller.ID)
		scheduler.releaseTaskExecution(caller.ID)
	})
	if _, ok := scheduler.collectExplicitGlobalGCSiblingRefs(caller); ok {
		t.Fatal("global GC admitted duplicate active executions with the caller's task ID")
	}
	if _, _, ok := scheduler.collectAllGCRefs(); ok {
		t.Fatal("deferred GC admitted duplicate active executions")
	}

	scheduler.releaseTaskExecution(caller.ID)
	if _, ok := scheduler.collectExplicitGlobalGCSiblingRefs(caller); !ok {
		t.Fatal("global GC rejected its sole caller execution lease")
	}
	if _, _, ok := scheduler.collectAllGCRefs(); ok {
		t.Fatal("deferred GC admitted an active caller execution")
	}
	scheduler.releaseTaskExecution(caller.ID)
	if _, _, ok := scheduler.collectAllGCRefs(); !ok {
		t.Fatal("deferred GC remained blocked after all execution leases were released")
	}
}
