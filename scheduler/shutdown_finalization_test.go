package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"barn/compiler"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
	"barn/vm"
)

func TestBeginShutdownWaitsForClaimedDeferredGCBeforePublishing(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anonID, errCode := store.CreateObject([]types.ObjID{0}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}
	scheduler := NewScheduler(store)
	t.Cleanup(scheduler.Stop)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	scheduler.registry.Register("recycle", func(*kernel.TaskContext, []types.Value) types.Result {
		close(entered)
		<-release
		return types.Ok(types.None)
	})
	scheduler.pendingAnonGC = []vm.AnonGCRequest{{
		Ctx:   kernel.NewTaskContext(),
		MinID: anonID,
	}}

	flushDone := make(chan struct{})
	go func() {
		scheduler.flushDeferredGC()
		close(flushDone)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("deferred GC did not claim the queued anonymous request")
	}

	shutdownDone := make(chan struct{})
	go func() {
		scheduler.BeginShutdown(nil)
		close(shutdownDone)
	}()
	select {
	case <-shutdownDone:
		t.Fatal("BeginShutdown published while claimed deferred GC was still executing")
	case <-time.After(20 * time.Millisecond):
	}
	if scheduler.isShuttingDown() {
		t.Fatal("shutdown state published before the claimed deferred GC completed")
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case <-flushDone:
	case <-time.After(time.Second):
		t.Fatal("deferred GC did not finish after release")
	}
	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("BeginShutdown did not publish after deferred GC finished")
	}
	if !scheduler.isShuttingDown() {
		t.Fatal("shutdown state was not published after deferred GC finished")
	}
}

func TestOrdinaryCompletionRecyclesPoppedWaifDespiteShutdownOnlyRoots(t *testing.T) {
	resetServerVerbTaskManager(t)
	t.Cleanup(func() { resetServerVerbTaskManager(t) })
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	if errCode := store.DefineProperty(0, "recycled", dbstore.NewProperty(types.NewInt(0), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define recycle marker: %v", errCode)
	}
	store.AddVerb(0, dbstore.NewVerb(":recycle", []string{":recycle"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"#0.recycled = #0.recycled + 1;"}))
	store.AddVerb(0, dbstore.NewVerb("ordinary", []string{"ordinary"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"w = new_waif();"}))

	scheduler := NewScheduler(store)
	t.Cleanup(scheduler.Stop)
	if _, err := scheduler.RunServerVerbTask(0, "ordinary", nil, 0); err != nil {
		t.Fatalf("run ordinary task: %v", err)
	}
	got, errCode := store.PropertyValue(0, "recycled")
	if errCode != types.E_NONE || got.Type() != types.TYPE_INT || got.Int() != 1 {
		t.Fatalf("recycle marker = %v, err=%v, want 1", got, errCode)
	}
}

func TestBeginShutdownTransfersPreexistingDeferredFinalizations(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(dbstore.NewObject(0, 0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anonID, errCode := store.CreateObject([]types.ObjID{0}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous object: %v", errCode)
	}
	waif := types.NewWaif(0, 0)
	scheduler := NewScheduler(store)
	t.Cleanup(scheduler.Stop)
	var handedOff []types.Value
	scheduler.SetPendingFinalizationSink(func(values []types.Value) {
		handedOff = append(handedOff, values...)
	})
	scheduler.pendingWaifBatch = []pendingWaifEntry{{waif: waif, ctx: kernel.NewTaskContext()}}
	scheduler.pendingAnonGC = []vm.AnonGCRequest{{
		Ctx:     kernel.NewTaskContext(),
		MinID:   anonID,
		OwnRefs: map[types.ObjID]struct{}{anonID: {}},
	}}

	scheduler.BeginShutdown(nil)
	scheduler.flushDeferredGC()
	if len(handedOff) != 2 || !handedOff[0].Equal(types.NewAnon(anonID)) || !handedOff[1].Equal(waif) {
		t.Fatalf("handoff = %v, want anonymous root then WAIF identity %p", handedOff, waif.WaifIdentity())
	}
	if len(scheduler.pendingWaifBatch) != 0 || len(scheduler.pendingAnonGC) != 0 {
		t.Fatalf("deferred batches remain after shutdown handoff: waifs=%d anons=%d", len(scheduler.pendingWaifBatch), len(scheduler.pendingAnonGC))
	}
}

func TestCanceledSchedulerContextDoesNotInferShutdownHandoff(t *testing.T) {
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	program, diagnostics := compiler.CompileMOO([]string{"a = create(#0, #0, 1);"}, vm.BuildVMRegistry())
	if len(diagnostics) > 0 {
		t.Fatalf("compile: %v", diagnostics)
	}
	scheduler := NewScheduler(store)
	t.Cleanup(scheduler.Stop)
	var handedOff []types.Value
	scheduler.SetPendingFinalizationSink(func(values []types.Value) { handedOff = append(handedOff, values...) })
	tk := task.NewTaskFull(1, 0, program, 100000, 1)
	scheduler.cancel()
	if err := scheduler.runTask(tk); err != context.Canceled {
		t.Fatalf("runTask error = %v, want context.Canceled", err)
	}
	if len(handedOff) != 0 {
		t.Fatalf("generic cancellation handed off roots %v without BeginShutdown", handedOff)
	}
	if store.HasAnonymousAtOrAbove(1) {
		t.Fatal("generic cancellation left its anonymous creation live instead of running ordinary cleanup")
	}
}

func TestShutdownCompletionPreservesAnonymousRootsForCheckpoint(t *testing.T) {
	tests := []struct {
		name      string
		code      []string
		wantAnons int
	}{
		{
			name: "single local",
			code: []string{
				"a = create(#0, #2, 1);",
			},
			wantAnons: 1,
		},
		{
			name: "local cycle",
			code: []string{
				"a = create(#0, #2, 1);",
				"b = create(#0, #2, 1);",
				"a.next = b;",
				"b.next = a;",
			},
			wantAnons: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetServerVerbTaskManager(t)
			t.Cleanup(func() { resetServerVerbTaskManager(t) })
			store := dbstore.NewStore()
			addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
			addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
			if errCode := store.DefineProperty(0, "next", dbstore.NewProperty(types.NewObj(types.ObjNothing), 2, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
				t.Fatalf("define next property: %v", errCode)
			}
			store.AddVerb(0, dbstore.NewVerb("shutdown_started", []string{"shutdown_started"}, 2,
				dbstore.VerbRead|dbstore.VerbExecute,
				dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, tc.code))

			scheduler := NewScheduler(store)
			t.Cleanup(scheduler.Stop)
			scheduler.SetPendingFinalizationSink(store.AppendPendingFinalizations)
			scheduler.BeginShutdown(nil)
			if _, err := scheduler.RunServerVerbTask(0, "shutdown_started", nil, 0); err != nil {
				t.Fatalf("run shutdown_started: %v", err)
			}

			snapshot := store.Snapshot()
			if got := len(snapshot.AnonymousObjects); got != tc.wantAnons {
				t.Fatalf("checkpoint anonymous objects = %d, want %d (pending=%v)", got, tc.wantAnons, snapshot.PendingFinalizations)
			}
			if got := len(snapshot.PendingFinalizations); got != 1 {
				t.Fatalf("checkpoint pending roots = %v, want one canonical root", snapshot.PendingFinalizations)
			}
		})
	}
}

func TestShutdownCompletionPreservesTaskLocalWaif(t *testing.T) {
	resetServerVerbTaskManager(t)
	t.Cleanup(func() { resetServerVerbTaskManager(t) })
	store := dbstore.NewStore()
	addServerVerbTestObject(t, store, 0, dbstore.FlagWizard)
	addServerVerbTestObject(t, store, 2, dbstore.FlagUser|dbstore.FlagWizard)
	store.AddVerb(0, dbstore.NewVerb("shutdown_started", []string{"shutdown_started"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"set_task_local(new_waif());"}))

	scheduler := NewScheduler(store)
	t.Cleanup(scheduler.Stop)
	scheduler.SetPendingFinalizationSink(store.AppendPendingFinalizations)
	scheduler.BeginShutdown(nil)
	if _, err := scheduler.RunServerVerbTask(0, "shutdown_started", nil, 0); err != nil {
		t.Fatalf("run shutdown_started: %v", err)
	}

	pending := store.Snapshot().PendingFinalizations
	if len(pending) != 1 || pending[0].Type() != types.TYPE_WAIF {
		t.Fatalf("checkpoint pending roots = %v, want task-local WAIF", pending)
	}
}
