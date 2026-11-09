package engine

import (
	"context"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/internal/admission"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestDeferredGCProgressAfterThrottledRuntimeBecomesIdle(t *testing.T) {
	store := newConflictTestStore(t)
	var recycle builtins.BuiltinFunc
	rt := newTestRuntimeWithBuiltins(t, store, testBuiltinSlot("recycle", 1, 1, []int64{-1}, &recycle))
	defer rt.Stop()
	candidate, code := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, true)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	recycled := make(chan struct{})
	recycle = func(_ *builtins.Execution, args []types.Value) types.Result {
		if err := store.Recycle(args[0].Obj()); err != nil {
			return types.Err(types.E_INVARG)
		}
		close(recycled)
		return types.Ok(types.NewInt(0))
	}
	// An expensive preceding sweep leaves this request throttled. No task or
	// runtime pass will arrive after the explicit attempt below.
	rt.lifecycle.Mu.Lock()
	rt.lifecycle.LastGCCost = cheapGCSweep
	rt.lifecycle.LastGCSweep = time.Now().Add(-gcSweepInterval + 100*time.Millisecond)
	rt.lifecycle.Mu.Unlock()
	ctx := task.NewContext()
	ctx.Store = store
	ctx.IsWizard = true
	rt.deferAnonGC(ctx, candidate, nil)
	rt.flushDeferredGC()
	select {
	case <-recycled:
	case <-time.After(3 * time.Second):
		t.Fatal("throttled anonymous collection never ran after the runtime became idle")
	}
	if store.DirectTxn().Valid(candidate) {
		t.Fatal("idle collection left its orphan valid")
	}
}

func TestDeferredGCProgressDrainsOverlappingExecution(t *testing.T) {
	store := newConflictTestStore(t)
	var hold, recycle builtins.BuiltinFunc
	opts := config.DefaultOptions()
	opts.AdmissionLimit = 4
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, opts, 4,
		testBuiltinSlot("gc_hold", 0, 0, nil, &hold),
		testBuiltinSlot("recycle", 1, 1, []int64{-1}, &recycle))
	defer rt.Stop()
	defer removeTasksForOwner(rt, 0)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	entered := make(chan struct{}, 2)
	hold = func(_ *builtins.Execution, _ []types.Value) types.Result {
		entered <- struct{}{}
		<-release
		return types.Ok(types.NewInt(0))
	}
	recycled := make(chan struct{})
	recycle = func(_ *builtins.Execution, args []types.Value) types.Result {
		if err := store.Recycle(args[0].Obj()); err != nil {
			return types.Err(types.E_INVARG)
		}
		close(recycled)
		return types.Ok(types.NewInt(0))
	}
	program := compileTestProgram(t, rt.registry, "gc_hold(); return 0;")
	finished := make(chan error, 2)
	for range 2 {
		id := rt.CreateBackgroundTask(0, program, 0)
		go func() { finished <- rt.runTask(rt.GetTask(id)) }()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("overlapping VM did not start")
		}
	}
	candidate, code := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, true)
	if code != types.E_NONE {
		t.Fatal(code)
	}
	ctx := task.NewContext()
	ctx.Store = store
	ctx.IsWizard = true
	// Already due, so maintenance must exclude fresh owners rather than
	// waiting forever for an accidental gap between the two executing VMs.
	rt.lifecycle.Mu.Lock()
	rt.lifecycle.LastGCCost = cheapGCSweep
	rt.lifecycle.LastGCSweep = time.Now().Add(-gcSweepInterval)
	rt.lifecycle.Mu.Unlock()
	rt.deferAnonGC(ctx, candidate, nil)
	probeCtx, cancel := context.WithCancel(rt.ctx)
	defer cancel()
	probeDone := make(chan struct{})
	var background atomic.Int64
	go func() {
		defer close(probeDone)
		for {
			scope, err := rt.admission.Enter(probeCtx, admission.Key{Principal: 1, Class: admission.Background})
			if err != nil {
				return
			}
			scope.Finish()
			background.Add(1)
			rt.flushDeferredGC()
		}
	}()
	// A player's commands keep arriving for the whole test.
	var commands atomic.Int64
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		for {
			scope, err := rt.admission.Enter(probeCtx, inputAdmissionKey(2))
			if err != nil {
				return
			}
			scope.Finish()
			commands.Add(1)
			time.Sleep(100 * time.Microsecond)
		}
	}()
	defer func() { cancel(); <-probeDone; <-inputDone }()
	// The two VMs are inside a builtin, where they cannot park, and may be for
	// seconds. The overdue collection must not hold admission for them: input
	// and fresh background work keep flowing through repeated attempts.
	admitted, started := commands.Load(), background.Load()
	for deadline := time.Now().Add(3 * time.Second); commands.Load() < admitted+20 || background.Load() < started+20; {
		if time.Now().After(deadline) {
			t.Fatalf("admitted %d inputs and %d background segments beside overdue collection, want both to keep flowing", commands.Load()-admitted, background.Load()-started)
		}
		time.Sleep(time.Millisecond)
	}
	// Wait for a rendezvous to meet the unparked VMs and fail closed.
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(time.Millisecond) {
		rt.lifecycle.Mu.Lock()
		retry := rt.lifecycle.RetryAfter
		rt.lifecycle.Mu.Unlock()
		if !retry.IsZero() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("overdue collection never attempted a rendezvous")
		}
	}
	select {
	case <-recycled:
		t.Fatal("collection ran while VMs that cannot park held execution leases")
	default:
	}
	unblock()
	for range 2 {
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("execution could not hand off during maintenance")
		}
	}
	select {
	case <-recycled:
	case <-time.After(3 * time.Second):
		t.Fatal("collection did not settle after draining overlapping VMs")
	}
}

// This opt-in measurement has the same workload on the baseline and candidate.
// It reports retention instead of requiring the candidate's progress policy.
func TestDeferredGCProgressMeasurement(t *testing.T) {
	if os.Getenv("BARN_GC_PROGRESS_MEASURE") != "1" {
		t.Skip("set BARN_GC_PROGRESS_MEASURE=1 for paired retention measurements")
	}
	store := newConflictTestStore(t)
	rt := NewRuntime(store)
	defer rt.Stop()
	rt.HoldFinalizationUntilStarted()
	if got := rt.EvalCommandOutput(0, "for i in [1..1000] create(#0, 1); endfor; return 1;"); got != "{1, 1}" {
		t.Fatal(got)
	}
	before := len(store.AnonymousRecycleCandidates(store.PersistentAnonymousReachability(), 0))
	if before != 1000 {
		t.Fatalf("workload produced %d collectible objects, want 1000", before)
	}
	rt.lifecycle.Mu.Lock()
	rt.lifecycle.LastGCCost = cheapGCSweep
	rt.lifecycle.LastGCSweep = time.Now().Add(-gcSweepInterval + 100*time.Millisecond)
	rt.lifecycle.Mu.Unlock()
	start := time.Now()
	rt.ReleaseStartupFinalization()
	// No MOO work or runtime passes arrive during this observation window.
	time.Sleep(500 * time.Millisecond)
	after := len(store.AnonymousRecycleCandidates(store.PersistentAnonymousReachability(), 0))
	t.Logf("gc_progress allocations=%d retained_after_idle=%d observation_ms=%d", before, after, time.Since(start).Milliseconds())
}

func BenchmarkDeferredGCCommands(b *testing.B) {
	for _, workload := range []struct{ name, code string }{
		{"plain", "return 1;"},
		{"anonymous", "create(#0, 1); return 1;"},
	} {
		b.Run(workload.name, func(b *testing.B) {
			store := dbstore.NewStore()
			root := dbstore.NewObjectBuilder(0)
			root.SetOwner(0)
			root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
			if err := store.Add(root.Build()); err != nil {
				b.Fatal(err)
			}
			rt := NewRuntime(store)
			defer rt.Stop()
			if got := rt.EvalCommandOutput(0, workload.code); got != "{1, 1}" {
				b.Fatal(got)
			}
			latencies := make([]int64, b.N)
			b.ResetTimer()
			for i := range b.N {
				start := time.Now()
				if got := rt.EvalCommandOutput(0, workload.code); got != "{1, 1}" {
					b.Fatal(got)
				}
				latencies[i] = time.Since(start).Nanoseconds()
			}
			b.StopTimer()
			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			b.ReportMetric(float64(latencies[(len(latencies)-1)*99/100]), "p99-ns")
		})
	}
}
