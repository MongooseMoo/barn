package engine

import (
	"context"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/admission"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func waitAdmission(t *testing.T, rt *Runtime, queued int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if rt.AdmissionStats().Queued == queued {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("admission = %+v, want queued %d", rt.AdmissionStats(), queued)
}

func admissionVerb(t *testing.T, store *dbstore.Store, name string, source ...string) {
	t.Helper()
	if _, e := store.AddVerb(0, dbstore.NewVerb(name, []string{name}, 0, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, source)); e != types.E_NONE {
		t.Fatal(e)
	}
}

func TestAdmissionCapOneEvalYieldsToChild(t *testing.T) {
	rt := newRuntimeWithWorkerCount(newConflictTestStore(t), config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	done := make(chan EvalOutcome, 1)
	go func() { done <- rt.Eval(0, []string{"fork (0) #0.v = 42; endfork; suspend(0); return #0.v;"}) }()
	select {
	case got := <-done:
		if got.Panic != nil || got.Result.Val.Int() != 42 {
			t.Fatalf("eval = %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("eval retained admission across suspension")
	}
	if got := rt.AdmissionStats(); got.Active != 0 || got.Queued != 0 || got.Service <= 0 {
		t.Fatalf("settlement = %+v", got)
	}
}

func TestAdmissionCapOneCompletionStartsIndependentHook(t *testing.T) {
	store := newConflictTestStore(t)
	admissionVerb(t, store, "login", "return 1;")
	admissionVerb(t, store, "connected", "return 42;")
	rt := newRuntimeWithWorkerCount(store, config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	done := make(chan error, 1)
	var result types.Result
	var active int
	go func() {
		_, err := rt.CreateLoginHookTask(0, "login", nil, -7, "", nil, func(types.Result) {
			active = rt.AdmissionStats().Active
			result, _ = rt.RunServerVerbTask(0, "connected", nil, 0)
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil || active != 0 || result.Val.Int() != 42 {
			t.Fatalf("completion err=%v active=%d result=%+v", err, active, result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("completion retained predecessor admission")
	}
}

func TestAdmissionCapOneErrorHookBorrowsReservation(t *testing.T) {
	store := newConflictTestStore(t)
	var observe builtins.BuiltinFunc
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{AdmissionLimit: 1}, 1, testBuiltinSlot("observe_admission", 0, 0, nil, &observe))
	defer rt.Stop()
	var seen *admission.Scope
	observe = func(ctx *builtins.Execution, _ []types.Value) types.Result {
		seen = ctx.Admission
		if got := rt.AdmissionStats().Active; got != 1 {
			t.Errorf("nested active=%d", got)
		}
		return types.Ok(types.NewInt(1))
	}
	admissionVerb(t, store, "handle_uncaught_error", "return observe_admission();")
	admissionVerb(t, store, "fail", "raise(E_INVARG);")
	done := make(chan error, 1)
	go func() { _, err := rt.RunServerVerbTaskWithArgstr(0, "fail", nil, 0, "", nil); done <- err }()
	select {
	case err := <-done:
		if err != nil || seen == nil {
			t.Fatalf("error=%v borrowed=%p", err, seen)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("error hook attempted a second admission")
	}
	if got := rt.AdmissionStats().Active; got != 0 {
		t.Fatalf("leaked reservation: %d", got)
	}
}

func TestAdmissionQueuedKillDoesNotClaimOrResurrectTask(t *testing.T) {
	rt := newRuntimeWithWorkerCount(newConflictTestStore(t), config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	held, _ := rt.admission.Acquire(context.Background(), admission.Key{Principal: 99})
	defer held.Finish(0, 0)
	id := rt.CreateBackgroundTask(0, compileTestProgram(t, rt.registry, "#0.v = 99;"), 0)
	queued := rt.GetTask(id)
	done := make(chan int, 1)
	go func() { done <- rt.ProcessReadyBatch() }()
	waitAdmission(t, rt, 1)
	if state := queued.GetState(); state != task.TaskQueued {
		t.Fatalf("waiter state=%v", state)
	}
	rt.lifecycle.Mu.Lock()
	producers := rt.lifecycle.ActiveFinalizationProducers
	rt.lifecycle.Mu.Unlock()
	if producers != 0 {
		t.Fatalf("waiting finalization producers=%d", producers)
	}
	queued.Kill()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("kill did not cancel admission")
	}
	held.Finish(0, 0)
	if queued.GetState() != task.TaskKilled || readRootV(t, rt.store) == 99 {
		t.Fatal("killed waiter executed")
	}
	if got := rt.AdmissionStats(); got.Active != 0 || got.Queued != 0 {
		t.Fatalf("leak=%+v", got)
	}
}

func TestAdmissionWaitingCallPinsRootsWithoutExecutionLease(t *testing.T) {
	store := newConflictTestStore(t)
	admissionVerb(t, store, "probe", "return args[1];")
	anon, e := store.DirectTxn().CreateObject(nil, 0, true)
	if e != types.E_NONE {
		t.Fatal(e)
	}
	rt := newRuntimeWithWorkerCount(store, config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	held, _ := rt.admission.Acquire(context.Background(), admission.Key{Principal: 99})
	defer held.Finish(0, 0)
	done := make(chan types.Result, 1)
	go func() { done <- rt.CallVerb(0, "probe", []types.Value{types.NewAnon(anon)}, 0) }()
	waitAdmission(t, rt, 1)
	roots, _, quiescent := rt.collectAllGCRefs()
	if _, ok := roots[anon]; !ok || !quiescent {
		t.Fatalf("roots=%v quiescent=%v", roots, quiescent)
	}
	held.Finish(0, 0)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("call did not finish")
	}
	roots, _, _ = rt.collectAllGCRefs()
	if _, ok := roots[anon]; ok {
		t.Fatal("admission root pin leaked")
	}
}

func TestAdmissionClosingInputStillAllowsLifecycleHooks(t *testing.T) {
	store := newConflictTestStore(t)
	admissionVerb(t, store, "shutdown_started", "return 42;")
	rt := newRuntimeWithWorkerCount(store, config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	held, _ := rt.admission.Acquire(context.Background(), admission.Key{Principal: 99})
	defer held.Finish(0, 0)
	done := make(chan EvalOutcome, 1)
	go func() { done <- rt.Eval(0, []string{"return 1;"}) }()
	waitAdmission(t, rt, 1)
	rt.CloseInputAdmission()
	select {
	case got := <-done:
		if got.Panic == nil {
			t.Fatal("input not cancelled")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("input close blocked")
	}
	held.Finish(0, 0)
	got, err := rt.RunServerVerbTask(0, "shutdown_started", nil, 0)
	if err != nil || got.Val.Int() != 42 {
		t.Fatalf("lifecycle after input close: %+v %v", got, err)
	}
}

func TestAdmissionKilledGateWaiterDoesNotReleaseGateOwner(t *testing.T) {
	rt := newRuntimeWithWorkerCount(newConflictTestStore(t), config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	id := rt.CreateBackgroundTask(0, compileTestProgram(t, rt.registry, "suspend(0); #0.v=99;"), 0)
	queued := rt.GetTask(id)
	if err := rt.runTask(queued); err != nil {
		t.Fatal(err)
	}
	held, err := rt.store.AcquireExclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	done := make(chan int, 1)
	go func() { done <- rt.ProcessReadyBatch() }()
	deadline := time.Now().Add(3 * time.Second)
	for queued.GetState() != task.TaskRunning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if queued.GetState() != task.TaskRunning {
		t.Fatal("continuation did not reach gate wait")
	}
	queued.Kill()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled gate waiter did not finish")
	}
	if !held.Release() {
		t.Fatal("waiter released another owner's gate")
	}
	if queued.GetState() != task.TaskKilled || readRootV(t, rt.store) == 99 {
		t.Fatal("killed continuation executed")
	}
	if got := rt.AdmissionStats(); got.Active != 0 || got.Queued != 0 {
		t.Fatalf("leak=%+v", got)
	}
}

func TestAdmissionBackgroundOrderingUsesInputServiceDebt(t *testing.T) {
	rt := newRuntimeWithWorkerCount(newConflictTestStore(t), config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	// Keep the lower-service principal eligible while the other accumulates
	// service, so idle-watermark normalization cannot erase their difference.
	held, _ := rt.admission.Acquire(context.Background(), admission.Key{Principal: 10})
	waiting := rt.admission.Enqueue(admission.Key{Principal: 20})
	held.Finish(100*time.Millisecond, 0)
	(<-waiting.Ready()).Finish(time.Millisecond, 0)
	first := rt.CreateBackgroundTask(10, compileTestProgram(t, rt.registry, "return 10;"), 0)
	second := rt.CreateBackgroundTask(20, compileTestProgram(t, rt.registry, "return 20;"), 0)
	if n := rt.ProcessReadyBatch(); n != 1 {
		t.Fatalf("batch size=%d", n)
	}
	if rt.GetTask(second).GetState() != task.TaskCompleted || rt.GetTask(first).GetState() != task.TaskQueued {
		t.Fatal("background order ignored shared principal debt")
	}
}

func TestAdmissionRuntimeStopPreservesUnstartedContinuation(t *testing.T) {
	rt := newRuntimeWithWorkerCount(newConflictTestStore(t), config.Options{AdmissionLimit: 1}, 1)
	held, _ := rt.admission.Acquire(context.Background(), admission.Key{Principal: 99})
	defer held.Finish(0, 0)
	id := rt.CreateBackgroundTask(0, compileTestProgram(t, rt.registry, "return 1;"), 0)
	done := make(chan int, 1)
	go func() { done <- rt.ProcessReadyBatch() }()
	waitAdmission(t, rt, 1)
	rt.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not drain admission waiter")
	}
	if state := rt.GetTask(id).GetState(); state != task.TaskQueued {
		t.Fatalf("unstarted task state=%v", state)
	}
}

func TestAdmissionCompletionRetainsResultRootsUntilCallbackReturns(t *testing.T) {
	store := newConflictTestStore(t)
	admissionVerb(t, store, "login", "return create(#0, 1);")
	rt := newRuntimeWithWorkerCount(store, config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	valid := false
	_, err := rt.CreateLoginHookTask(0, "login", nil, -7, "", nil, func(result types.Result) {
		valid = result.Val.IsAnonymous() && store.DirectTxn().Valid(result.Val.Obj())
	})
	if err != nil || !valid {
		t.Fatalf("callback received reclaimed result: valid=%v err=%v", valid, err)
	}
}

// A cooperative background slice that never suspends must not hold the only
// admission slot for its whole run: input queued behind it has to be serviced
// within a bounded quantum, as Go preemption provided before admission existed.
func TestAdmissionCapOneInputNotStarvedByLongBackgroundSlice(t *testing.T) {
	store := serverOptionsVerbStore(t, `add_property(#1, "bg_ticks", 1000000000, {player, "r"});`+
		`add_property(#1, "bg_seconds", 30, {player, "r"});`+
		`load_server_options();`)
	rt := newRuntimeWithWorkerCount(store, config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	if r := rt.CallVerb(1, "go", nil, 2); r.Flow == types.FlowException {
		t.Fatalf("setup raised %s", r.Error)
	}
	const spin = time.Second
	id := rt.CreateBackgroundTask(2, compileTestProgram(t, rt.registry,
		"t = ftime(1); while (ftime(1) - t < 1.0) endwhile return 7;"), 0)
	batch := make(chan int, 1)
	go func() { batch <- rt.ProcessReadyBatch() }()
	deadline := time.Now().Add(3 * time.Second)
	for rt.AdmissionStats().Active != 1 || rt.GetTask(id).GetState() != task.TaskRunning {
		if time.Now().After(deadline) {
			t.Fatalf("background task never started: %+v", rt.AdmissionStats())
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	got := rt.Eval(2, []string{"return 1;"})
	latency := time.Since(start)
	if got.Panic != nil || got.Result.Val.Int() != 1 {
		t.Fatalf("eval = %+v", got)
	}
	<-batch
	if tk := rt.GetTask(id); tk.GetState() != task.TaskCompleted || tk.Result.Val.Int() != 7 {
		t.Fatalf("background task state=%v result=%+v", tk.GetState(), tk.Result)
	}
	if latency > spin/4 {
		t.Fatalf("input waited %v behind a nonsuspending background slice", latency)
	}
	t.Logf("input latency %v; admission %+v", latency, rt.AdmissionStats())
}

// A resumed slice runs escalated, holding the exclusive commit gate. It must not
// lend its reservation: a committing input admitted in its place would wait on
// the gate while the gate holder waited for readmission.
func TestAdmissionCapOneEscalatedSliceDoesNotYieldIntoDeadlock(t *testing.T) {
	store := serverOptionsVerbStore(t, `add_property(#1, "bg_ticks", 1000000000, {player, "r"});`+
		`add_property(#1, "bg_seconds", 30, {player, "r"});`+
		`add_property(#1, "v", 0, {player, "rw"});`+
		`load_server_options();`)
	rt := newRuntimeWithWorkerCount(store, config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	if r := rt.CallVerb(1, "go", nil, 2); r.Flow == types.FlowException {
		t.Fatalf("setup raised %s", r.Error)
	}
	id := rt.CreateBackgroundTask(2, compileTestProgram(t, rt.registry,
		"suspend(0); t = ftime(1); while (ftime(1) - t < 0.5) endwhile #1.v = #1.v + 1; return 7;"), 0)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for rt.GetTask(id).GetState() != task.TaskCompleted {
			rt.ProcessReadyBatch()
			time.Sleep(time.Millisecond)
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for rt.GetTask(id).GetState() != task.TaskRunning || rt.GetTask(id).BytecodeVMValue() == nil {
		if time.Now().After(deadline) {
			t.Fatal("resumed slice never started")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	evalDone := make(chan EvalOutcome, 1)
	go func() { evalDone <- rt.Eval(2, []string{"#1.v = #1.v + 10; return #1.v;"}) }()
	select {
	case got := <-evalDone:
		if got.Panic != nil || got.Result.Flow == types.FlowException {
			t.Fatalf("eval = %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("deadlock: admission %+v", rt.AdmissionStats())
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("background task never completed")
	}
	if v := rt.GetTask(id).Result.Val.Int(); v != 7 {
		t.Fatalf("background result %d", v)
	}
}
