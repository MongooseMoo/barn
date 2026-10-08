package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/types"
)

func queueDueAnonymousGC(t *testing.T, rt *Runtime) types.ObjID {
	t.Helper()
	id, code := rt.store.DirectTxn().CreateObject([]types.ObjID{0}, 0, true)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	rt.lifecycle.Mu.Lock()
	rt.lifecycle.LastGCCost = cheapGCSweep
	rt.lifecycle.LastGCSweep = time.Now().Add(-gcSweepInterval)
	rt.lifecycle.Mu.Unlock()
	rt.AdoptPendingFinalizations([]types.Value{types.NewAnon(id)})
	return id
}

func TestDeferredGCMaintenanceCancellationReleasesAdmission(t *testing.T) {
	rt := NewRuntime(newConflictTestStore(t))
	defer rt.Stop()
	owner, err := rt.enterInput(0, false)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Finish()
	queueDueAnonymousGC(t, rt)
	probeCtx, cancelProbe := context.WithCancel(context.Background())
	defer cancelProbe()
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		for {
			scope, err := rt.admission.Enter(probeCtx, inputAdmissionKey(1))
			if err != nil {
				return
			}
			scope.Finish()
		}
	}()
	defer func() { cancelProbe(); <-probeDone }()
	waitAdmission(t, rt, 1)
	rt.cancel()
	select {
	case <-rt.lifecycle.MaintenanceDone:
	case <-time.After(time.Second):
		t.Fatal("maintenance waited for its admission drain after cancellation")
	}
	// Use a separate context to prove the pause was released, independently of
	// the runtime's cancellation of ordinary input admission.
	checkCtx, cancelCheck := context.WithTimeout(context.Background(), time.Second)
	defer cancelCheck()
	scope, err := rt.admission.Enter(checkCtx, inputAdmissionKey(2))
	if err != nil {
		t.Fatalf("cancelled maintenance left admission paused: %v", err)
	}
	scope.Finish()
}

func TestDeferredGCMaintenanceDoesNotWaitBehindCheckpointBarrier(t *testing.T) {
	rt := NewRuntime(newConflictTestStore(t))
	defer rt.Stop()
	// Checkpoint holds the same two barriers. The maintenance worker must leave
	// a retry deadline and release its pause instead of blocking Runtime.Stop.
	rt.lifecycle.SweepMu.Lock()
	defer rt.lifecycle.SweepMu.Unlock()
	rt.lifecycle.VMStartMu.Lock()
	defer rt.lifecycle.VMStartMu.Unlock()
	id := queueDueAnonymousGC(t, rt)
	deadline := time.Now().Add(time.Second)
	for {
		rt.lifecycle.Mu.Lock()
		retry := rt.lifecycle.RetryAfter
		rt.lifecycle.Mu.Unlock()
		if !retry.IsZero() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("maintenance did not release the contended checkpoint boundary")
		}
		time.Sleep(time.Millisecond)
	}
	if !rt.store.DirectTxn().Valid(id) {
		t.Fatal("maintenance swept despite the checkpoint's root barrier")
	}
	rt.cancel()
	select {
	case <-rt.lifecycle.MaintenanceDone:
	case <-time.After(time.Second):
		t.Fatal("maintenance cancellation waited for checkpoint barriers")
	}
}

func TestDeferredGCMaintenanceStartupHoldAndShutdownPreservePending(t *testing.T) {
	rt := NewRuntime(newConflictTestStore(t))
	defer rt.Stop()
	rt.HoldFinalizationUntilStarted()
	id := queueDueAnonymousGC(t, rt)
	if deadline := rt.deferredGCDeadline(); !deadline.IsZero() {
		t.Fatalf("startup-held collection has a maintenance deadline: %v", deadline)
	}
	var roots []types.Value
	rt.SetPendingFinalizationSink(func(values []types.Value) { roots = append(roots, values...) })
	ready := rt.BeginShutdown(nil)
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("held collection prevented shutdown publication")
	}
	if !rt.store.DirectTxn().Valid(id) || len(roots) != 1 || roots[0].ID() != id {
		t.Fatalf("shutdown pending roots = %v, want unrecycled #%d", roots, id)
	}
	if deadline := rt.deferredGCDeadline(); !deadline.IsZero() {
		t.Fatalf("shutdown collection has a maintenance deadline: %v", deadline)
	}
}

func TestDeferredGCMaintenancePreservesSuspendedAnonymousRoot(t *testing.T) {
	store := newConflictTestStore(t)
	var recycled atomic.Int64
	var recycle builtins.BuiltinFunc
	rt := newTestRuntimeWithBuiltins(t, store, testBuiltinSlot("recycle", 1, 1, []int64{-1}, &recycle))
	defer rt.Stop()
	defer removeTasksForOwner(rt, 0)
	recycle = func(_ *builtins.Execution, args []types.Value) types.Result {
		if err := store.Recycle(args[0].Obj()); err != nil {
			return types.Err(types.E_INVARG)
		}
		recycled.Add(1)
		return types.Ok(types.NewInt(0))
	}
	program := compileTestProgram(t, rt.registry, "held = create(#0, 1); suspend(); return held;")
	id := rt.CreateBackgroundTask(0, program, 0)
	held := store.NextID()
	if err := rt.runTask(rt.GetTask(id)); err != nil {
		t.Fatal(err)
	}
	orphan := queueDueAnonymousGC(t, rt)
	deadline := time.Now().Add(time.Second)
	for store.DirectTxn().Valid(orphan) {
		if time.Now().After(deadline) {
			t.Fatal("maintenance did not settle its orphan")
		}
		time.Sleep(time.Millisecond)
	}
	if !store.DirectTxn().Valid(held) || recycled.Load() != 1 {
		t.Fatalf("suspended root valid=%t, recycle count=%d", store.DirectTxn().Valid(held), recycled.Load())
	}
}
