package scheduler

import (
	"fmt"
	"sync"
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
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

	if line := scheduler.EvalCommandOutput(0, "run_gc(); return 1;"); line != "{1, 1}" {
		t.Fatalf("run_gc eval output = %q, want successful return", line)
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
		scheduler.taskManager.SuspendTask(holder, -1)
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
	if _, _, ok := scheduler.collectSiblingGCRefs(nil); ok {
		t.Fatal("per-task GC admitted active sibling VM")
	}
	if _, _, ok := scheduler.collectAllGCRefs(); ok {
		t.Fatal("deferred GC admitted active suspend handoff")
	}
	for _, property := range []string{"hold_handoff", "hold_orphan"} {
		if errCode := store.DeleteDefinedProperty(0, property); errCode != types.E_NONE {
			t.Fatalf("delete #0.%s: %v", property, errCode)
		}
	}

	if line := scheduler.EvalCommandOutput(0, "run_gc(); return 1;"); line != "{1, 1}" {
		t.Fatalf("run_gc eval output = %q, want successful return", line)
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

	if line := scheduler.EvalCommandOutput(0, "run_gc(); return 1;"); line != "{1, 1}" {
		t.Fatalf("post-handoff run_gc eval output = %q, want successful return", line)
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

func TestExecutionContextOwnershipSurvivesDuplicateAcquisitionUntilLastRelease(t *testing.T) {
	scheduler := NewScheduler(dbstore.NewStore())
	defer scheduler.Stop()
	ctx := kernel.NewTaskContext()

	scheduler.acquireExecutionContext(ctx, 92001)
	scheduler.acquireExecutionContext(ctx, 92001)
	if ownerID, ok := scheduler.executionContextOwner(ctx); !ok || ownerID != 92001 {
		t.Fatalf("duplicate context owner = (%d, %v), want (92001, true)", ownerID, ok)
	}
	scheduler.releaseExecutionContext(ctx, 92001)
	if ownerID, ok := scheduler.executionContextOwner(ctx); !ok || ownerID != 92001 {
		t.Fatalf("context owner after first release = (%d, %v), want retained owner", ownerID, ok)
	}
	scheduler.releaseExecutionContext(ctx, 92001)
	if ownerID, ok := scheduler.executionContextOwner(ctx); ok {
		t.Fatalf("context owner after last release = (%d, true), want absent", ownerID)
	}

	scheduler.acquireExecutionContext(ctx, 92001)
	scheduler.acquireExecutionContext(ctx, 92002)
	if ownerID, ok := scheduler.executionContextOwner(ctx); ok {
		t.Fatalf("mismatched context ownership resolved to %d, want fail-closed ambiguity", ownerID)
	}
	scheduler.releaseExecutionContext(ctx, 99999)
	if _, claimed, attributable := scheduler.executionContextClaim(ctx); !claimed || attributable {
		t.Fatalf("unknown-owner release changed ambiguous claim = (claimed %v, attributable %v)", claimed, attributable)
	}
	scheduler.releaseExecutionContext(ctx, 92001)
	if ownerID, ok := scheduler.executionContextOwner(ctx); !ok || ownerID != 92002 {
		t.Fatalf("unique owner after first mismatched release = (%d, %v), want (92002, true)", ownerID, ok)
	}
	scheduler.releaseExecutionContext(ctx, 92002)
}

func TestAmbiguousExecutionContextMakesExplicitGCNoOp(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	orphan, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create orphan: %v", errCode)
	}
	nestedVerb := dbstore.NewVerb(
		"ambiguous_nested_gc",
		[]string{"ambiguous_nested_gc"},
		0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{"ambiguous_nested_barrier();", "run_gc();", "return 1;"},
	)
	if _, errCode := store.AddVerb(0, nestedVerb); errCode != types.E_NONE {
		t.Fatalf("add ambiguous nested verb: %v", errCode)
	}

	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	nestedEntered := make(chan struct{})
	releaseNested := make(chan struct{})
	scheduler.registry.Register("ambiguous_nested_barrier", func(_ *kernel.TaskContext, _ []types.Value) types.Result {
		close(nestedEntered)
		<-releaseNested
		return types.Ok(types.NewInt(0))
	})
	caller := task.NewTask(93001, 0, 1000, 10)
	ctx := kernel.NewTaskContext()
	ctx.Player = 0
	ctx.Programmer = 0
	ctx.IsWizard = true
	ctx.Task = caller
	ctx.TaskID = caller.ID
	ctx.Store = store
	ctx.Registry = scheduler.registry
	ctx.StoreTxn = store.BeginReadOnly(0)
	scheduler.acquireTaskExecution(caller)
	defer scheduler.releaseTaskExecution(caller.ID)
	scheduler.acquireExecutionContext(ctx, caller.ID)
	defer scheduler.releaseExecutionContext(ctx, caller.ID)
	// The second provenance claimant deliberately has no physical lease.
	scheduler.acquireExecutionContext(ctx, 93002)
	nestedDone := make(chan types.Result, 1)
	go func() {
		nestedDone <- scheduler.CallVerbInContext(0, "ambiguous_nested_gc", nil, ctx)
	}()
	select {
	case <-nestedEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("nested VM did not install its ambiguity blocker and enter verb")
	}

	// Once the other outer claimant exits, ctx.Task names the sole real physical
	// lease. The nested VM's reserved claim/lease must keep provenance ambiguous
	// until it returns, so its run_gc remains a successful no-op.
	scheduler.releaseExecutionContext(ctx, 93002)
	if _, claimed, attributable := scheduler.executionContextClaim(ctx); !claimed || attributable {
		t.Fatalf("nested blocker claim = (claimed %v, attributable %v), want ambiguous", claimed, attributable)
	}
	close(releaseNested)
	select {
	case result := <-nestedDone:
		if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 1 {
			t.Fatalf("nested ambiguous run_gc result = %+v, want return 1", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nested ambiguous VM did not complete")
	}
	if !store.Valid(orphan) {
		t.Fatalf("orphan #%d was swept after ambiguity lost an outer claimant", orphan)
	}
	if ownerID, ok := scheduler.executionContextOwner(ctx); !ok || ownerID != caller.ID {
		t.Fatalf("owner after nested blocker cleanup = (%d, %v), want caller %d", ownerID, ok, caller.ID)
	}

	ctx.StoreTxn.Release()
	ctx.StoreTxn = nil
	runGC, ok := scheduler.registry.Get("run_gc")
	if !ok {
		t.Fatal("run_gc builtin not registered")
	}
	if result := runGC(ctx, nil); result.Flow != types.FlowNormal {
		t.Fatalf("unique-owner run_gc result = %+v, want success", result)
	}
	if store.Valid(orphan) {
		t.Fatalf("orphan #%d survived after provenance became uniquely attributable", orphan)
	}
}

func newInitializeRunGCStore(t *testing.T) (*dbstore.Store, types.ObjID) {
	t.Helper()
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	prototype, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("create initialized prototype: %v", errCode)
	}
	verb := dbstore.NewVerb(
		"initialize",
		[]string{"initialize"},
		0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{"run_gc();", "return 0;"},
	)
	if _, errCode := store.AddVerb(prototype, verb); errCode != types.E_NONE {
		t.Fatalf("add initialize verb: %v", errCode)
	}
	return store, prototype
}

func TestNestedEvalInitializeRunGCFailsClosedOverOuterVMRoots(t *testing.T) {
	store, prototype := newInitializeRunGCStore(t)
	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	code := fmt.Sprintf("held = create(#0, 1); created = create(#%d); return valid(held);", prototype)
	if line := scheduler.EvalCommandOutput(0, code); line != "{1, 1}" {
		t.Fatalf("nested eval run_gc output = %q, want outer anonymous root to remain valid", line)
	}
}

func TestNestedTaskInitializeRunGCFailsClosedOverOuterVMRoots(t *testing.T) {
	store, prototype := newInitializeRunGCStore(t)
	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	defer removeTasksForOwner(scheduler, 0)
	program := compileTestProgram(t, scheduler.registry, fmt.Sprintf(
		"held = create(#0, 1); created = create(#%d); return valid(held);",
		prototype,
	))
	taskID := scheduler.CreateBackgroundTask(0, program, 0)
	running := scheduler.GetTask(taskID)
	if err := scheduler.runTask(running); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if running.Result.Flow != types.FlowReturn || running.Result.Val.Type() != types.TYPE_INT || running.Result.Val.Int() != 1 {
		t.Fatalf("nested task run_gc result = %+v, want outer anonymous root to remain valid", running.Result)
	}
}

func TestRunGCSweepRecycleHookRetainsTaskTransaction(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	root.SetProperty("sweep_marker", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	class, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous class: %v", errCode)
	}
	recycleVerb := dbstore.NewVerb(
		"recycle",
		[]string{"recycle"},
		0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{"#0.sweep_marker = 42;", "return 0;"},
	)
	if _, errCode := store.AddVerb(class, recycleVerb); errCode != types.E_NONE {
		t.Fatalf("add recycle verb: %v", errCode)
	}

	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	defer removeTasksForOwner(scheduler, 0)
	program := compileTestProgram(t, scheduler.registry, fmt.Sprintf(
		"candidate = create(#%d, 1); candidate = 0; run_gc(); return #0.sweep_marker;",
		class,
	))
	taskID := scheduler.CreateBackgroundTask(0, program, 0)
	running := scheduler.GetTask(taskID)
	if err := scheduler.runTask(running); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if running.Result.Flow != types.FlowReturn || running.Result.Val.Type() != types.TYPE_INT || running.Result.Val.Int() != 42 {
		t.Fatalf("task result after sweep recycle hook = %+v, want shared-transaction value 42", running.Result)
	}
	marker, errCode := store.PropertyValue(0, "sweep_marker")
	if errCode != types.E_NONE || marker.Type() != types.TYPE_INT || marker.Int() != 42 {
		t.Fatalf("persisted sweep marker = %v (%v), want 42", marker, errCode)
	}
}
