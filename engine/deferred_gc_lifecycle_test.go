package engine

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
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
	rt := NewRuntime(store)
	defer rt.Stop()
	defer removeTasksForOwner(rt, 0)
	program := compileTestProgram(t, rt.registry, "create(#0, 1); return 1;")
	taskID := rt.CreateBackgroundTask(0, program, 0)
	orphanID := store.NextID()

	if err := rt.runTask(rt.GetTask(taskID)); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if store.DirectTxn().Valid(orphanID) {
		t.Fatalf("anonymous orphan #%d remained valid after synchronous runTask returned", orphanID)
	}
	rt.lifecycle.Mu.Lock()
	pending := len(rt.lifecycle.PendingAnonGC)
	rt.lifecycle.Mu.Unlock()
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

	rt := NewRuntime(store)
	defer rt.Stop()
	defer removeTasksForOwner(rt, 0)
	program := compileTestProgram(t, rt.registry, "create(#0, 1); return 1;")
	taskID := rt.CreateBackgroundTask(0, program, 0)
	orphanID := store.NextID()
	taskWithPanic := rt.GetTask(taskID)
	taskWithPanic.OnComplete = func(types.Result) { panic("completion barrier") }

	err := rt.runTask(taskWithPanic)
	if err == nil || !strings.Contains(err.Error(), "completion barrier") {
		t.Fatalf("runTask panic result = %v, want recovered completion panic", err)
	}
	if state := taskWithPanic.GetState(); state != task.TaskKilled {
		t.Fatalf("task state after recovered panic = %v, want killed", state)
	}
	if store.DirectTxn().Valid(orphanID) {
		t.Fatalf("anonymous orphan #%d remained valid after recovered task panic", orphanID)
	}
	rt.lifecycle.Mu.Lock()
	pending := len(rt.lifecycle.PendingAnonGC)
	rt.lifecycle.Mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending anonymous GC batches after recovered task panic = %d, want 0", pending)
	}
}

func TestDeferredAnonymousGCRecycleUsesStandaloneCallerWithCompletedTask(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	root.SetProperty("recycle_caller", dbstore.NewProperty(types.NewObj(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	class, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous class: %v", errCode)
	}
	if _, errCode := store.AddVerb(class, dbstore.NewVerb(
		"recycle",
		[]string{"recycle"},
		0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{"#0.recycle_caller = caller;"},
	)); errCode != types.E_NONE {
		t.Fatalf("add recycle verb: %v", errCode)
	}
	candidate, errCode := store.DirectTxn().CreateObject([]types.ObjID{class}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous candidate: %v", errCode)
	}

	rt := NewRuntime(store)
	defer rt.Stop()
	ctx := kernel.NewTaskContext()
	ctx.Player = 0
	ctx.Programmer = 0
	ctx.IsWizard = true
	ctx.ThisObj = 0
	ctx.Store = store
	completed := task.NewTask(98001, 0, 1000, 10)
	completed.SetState(task.TaskCompleted)
	machine := vm.NewVM(store, rt.session)
	machine.Context = ctx
	machine.Task = completed

	rt.deferAnonGC(ctx, candidate, machine)
	rt.flushDeferredGC()

	caller, errCode := store.DirectTxn().PropertyValue(0, "recycle_caller")
	if errCode != types.E_NONE || caller.Type() != types.TYPE_OBJ || caller.Obj() != types.ObjNothing {
		t.Fatalf("deferred :recycle caller = %v (%v), want #-1", caller, errCode)
	}
}

func TestDeferredGCSweepBlocksNewVMStartUntilSweepCompletes(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	probeVerb := dbstore.NewVerb(
		"sweep_probe",
		[]string{"sweep_probe"},
		0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{"return 42;"},
	)
	if _, errCode := store.AddVerb(0, probeVerb); errCode != types.E_NONE {
		t.Fatalf("add sweep probe verb: %v", errCode)
	}

	rt := NewRuntime(store)
	defer rt.Stop()
	defer removeTasksForOwner(rt, 0)

	sweepEntered := make(chan struct{})
	reentrantRunGC := make(chan types.Result, 1)
	nestedSweepVerb := make(chan types.Result, 1)
	releaseSweep := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSweep) }) }
	defer release()
	var sweepOnce sync.Once
	rt.registry.Register("recycle", func(ctx *builtins.Execution, args []types.Value) types.Result {
		sweepOnce.Do(func() {
			nestedSweepVerb <- rt.session.CallVerb(0, "sweep_probe", nil, ctx)
			runGC, ok := rt.registry.Get("run_gc")
			if !ok {
				reentrantRunGC <- types.Err(types.E_VERBNF)
			} else {
				reentrantRunGC <- runGC(ctx, nil)
			}
			close(sweepEntered)
			<-releaseSweep
		})
		if len(args) != 1 || args[0].Type() != types.TYPE_ANON {
			return types.Err(types.E_INVARG)
		}
		if err := store.Recycle(args[0].Obj()); err != nil {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.NewInt(0))
	})

	taskEntered := make(chan types.ObjID, 1)
	rt.registry.Register("gc_task_started", func(_ *builtins.Execution, args []types.Value) types.Result {
		if len(args) != 1 || args[0].Type() != types.TYPE_ANON {
			return types.Err(types.E_INVARG)
		}
		taskEntered <- args[0].Obj()
		return types.Ok(types.NewInt(0))
	})

	firstCandidate, errCode := store.DirectTxn().CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create first sweep candidate: %v", errCode)
	}
	gcCtx := kernel.NewTaskContext()
	gcCtx.Player = 0
	gcCtx.Programmer = 0
	gcCtx.IsWizard = true
	gcCtx.Store = store
	// Two requests are intentional. The first request computes its candidates and
	// then blocks in recycle(); without a start barrier, a newly-running VM can
	// create a second anonymous object before the second request computes its
	// candidates from the already-captured root snapshot.
	rt.deferAnonGC(gcCtx, firstCandidate, nil)
	rt.deferAnonGC(gcCtx, firstCandidate, nil)

	sweepDone := make(chan struct{})
	go func() {
		rt.flushDeferredGC()
		close(sweepDone)
	}()
	select {
	case <-sweepEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("deferred sweep did not enter recycle after capturing roots")
	}
	select {
	case result := <-nestedSweepVerb:
		if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 42 {
			t.Fatalf("nested sweep-owned verb = %+v, want return 42", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nested sweep-owned verb deadlocked acquiring VM start ownership")
	}
	select {
	case result := <-reentrantRunGC:
		if result.Flow != types.FlowNormal {
			t.Fatalf("reentrant run_gc during recycle = %+v, want success", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reentrant run_gc deadlocked inside recycle hook")
	}

	program := compileTestProgram(t, rt.registry, "held = create(#0, 1); gc_task_started(held); suspend(); return held;")
	taskID := rt.CreateBackgroundTask(0, program, 0)
	startAttempted := make(chan struct{})
	var attemptOnce sync.Once
	rt.lifecycle.ExecutionStartObserver = func() { attemptOnce.Do(func() { close(startAttempted) }) }
	processed := make(chan int, 1)
	go func() {
		processed <- rt.ProcessReadyTasks()
	}()
	select {
	case <-startAttempted:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not attempt to acquire VM-start ownership")
	}
	select {
	case id := <-taskEntered:
		t.Fatalf("task VM began and created anonymous object #%d while sweep owned its root snapshot", id)
	default:
	}

	release()
	select {
	case <-sweepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("deferred sweep did not finish after release")
	}

	var heldID types.ObjID
	select {
	case heldID = <-taskEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("task VM did not begin after sweep released")
	}
	select {
	case got := <-processed:
		if got != 1 {
			t.Fatalf("processed tasks = %d, want one", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("task did not finish suspend handoff")
	}
	if state := rt.GetTask(taskID).GetState(); state != task.TaskSuspended {
		t.Fatalf("task state = %v, want suspended", state)
	}
	if !store.DirectTxn().Valid(heldID) {
		t.Fatalf("suspended VM's anonymous root #%d was recycled", heldID)
	}
}

func TestDeferredGCSweepBlocksEvalVMStart(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}

	rt := NewRuntime(store)
	defer rt.Stop()
	candidate, errCode := store.DirectTxn().CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create sweep candidate: %v", errCode)
	}

	sweepEntered := make(chan struct{})
	releaseSweep := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSweep) }) }
	defer release()
	rt.registry.Register("recycle", func(_ *builtins.Execution, args []types.Value) types.Result {
		close(sweepEntered)
		<-releaseSweep
		if err := store.Recycle(args[0].Obj()); err != nil {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.NewInt(0))
	})

	gcCtx := kernel.NewTaskContext()
	gcCtx.Player = 0
	gcCtx.Programmer = 0
	gcCtx.IsWizard = true
	gcCtx.Store = store
	rt.deferAnonGC(gcCtx, candidate, nil)
	sweepDone := make(chan struct{})
	go func() {
		rt.flushDeferredGC()
		close(sweepDone)
	}()
	select {
	case <-sweepEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("deferred sweep did not enter recycle")
	}

	evalEntered := make(chan struct{})
	rt.registry.Register("gc_eval_started", func(_ *builtins.Execution, _ []types.Value) types.Result {
		close(evalEntered)
		return types.Ok(types.NewInt(0))
	})
	startAttempted := make(chan struct{})
	var attemptOnce sync.Once
	rt.lifecycle.ExecutionStartObserver = func() { attemptOnce.Do(func() { close(startAttempted) }) }
	evalDone := make(chan string, 1)
	go func() {
		evalDone <- rt.EvalCommandOutput(0, "gc_eval_started(); return 1;")
	}()
	select {
	case <-startAttempted:
	case <-time.After(5 * time.Second):
		t.Fatal("eval did not attempt to acquire VM-start ownership")
	}
	select {
	case <-evalEntered:
		t.Fatal("eval VM began while sweep owned its root snapshot")
	default:
	}

	release()
	select {
	case <-sweepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("deferred sweep did not finish after release")
	}
	select {
	case <-evalEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("eval VM did not begin after sweep released")
	}
	select {
	case line := <-evalDone:
		if line != "{1, 1}" {
			t.Fatalf("eval output = %q, want successful return", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("eval did not complete after sweep released")
	}
}

func TestDeferredGCSweepBlocksServerHookVMStart(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}
	verb := dbstore.NewVerb(
		"server_probe",
		[]string{"server_probe"},
		0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
		[]string{"gc_server_hook_started();", "return 1;"},
	)
	if _, errCode := store.AddVerb(0, verb); errCode != types.E_NONE {
		t.Fatalf("add server probe verb: %v", errCode)
	}

	rt := NewRuntime(store)
	defer rt.Stop()
	candidate, errCode := store.DirectTxn().CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create sweep candidate: %v", errCode)
	}
	sweepEntered := make(chan struct{})
	releaseSweep := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSweep) }) }
	defer release()
	rt.registry.Register("recycle", func(_ *builtins.Execution, args []types.Value) types.Result {
		close(sweepEntered)
		<-releaseSweep
		if err := store.Recycle(args[0].Obj()); err != nil {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.NewInt(0))
	})
	hookEntered := make(chan struct{})
	rt.registry.Register("gc_server_hook_started", func(_ *builtins.Execution, _ []types.Value) types.Result {
		close(hookEntered)
		return types.Ok(types.NewInt(0))
	})
	startAttempted := make(chan struct{})
	var attemptOnce sync.Once
	rt.lifecycle.ExecutionStartObserver = func() { attemptOnce.Do(func() { close(startAttempted) }) }

	gcCtx := kernel.NewTaskContext()
	gcCtx.Player = 0
	gcCtx.Programmer = 0
	gcCtx.IsWizard = true
	gcCtx.Store = store
	rt.deferAnonGC(gcCtx, candidate, nil)
	sweepDone := make(chan struct{})
	go func() {
		rt.flushDeferredGC()
		close(sweepDone)
	}()
	select {
	case <-sweepEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("deferred sweep did not enter recycle")
	}

	hookDone := make(chan types.Result, 1)
	go func() { hookDone <- rt.CallVerb(0, "server_probe", nil, 0) }()
	select {
	case <-startAttempted:
	case <-time.After(5 * time.Second):
		t.Fatal("server hook did not attempt to acquire VM-start ownership")
	}
	select {
	case <-hookEntered:
		t.Fatal("server-hook VM began while sweep owned its root snapshot")
	default:
	}
	release()
	select {
	case <-sweepDone:
	case <-time.After(5 * time.Second):
		t.Fatal("deferred sweep did not finish after release")
	}
	select {
	case <-hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("server-hook VM did not begin after sweep released")
	}
	select {
	case result := <-hookDone:
		if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 1 {
			t.Fatalf("server-hook result = %+v, want return 1", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server-hook VM did not complete after sweep released")
	}
}
