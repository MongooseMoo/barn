package engine

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/types"
)

// The statement of the conformance test
// builtins/recycle_keeps_shared_anonymous.yaml::anonymous_held_only_by_recycled_object_is_destroyed.
// Toast queues the orphan when recycle(h) drops its last reference, and its main
// loop runs the queued :recycle (server.cc recycle_anonymous_objects) before the
// next run_ready_tasks(), so the task resumed from suspend(0) reads hits == 1.
const recycleOrphanThenSuspend = `k = create(#-1);
add_property(k, "hits", 0, {#0, "rw"});
p = create(#-1);
add_property(p, "keeper", k, {#0, "r"});
add_verb(p, {#0, "rxd", "recycle"}, {"this", "none", "this"});
set_verb_code(p, "recycle", {"this.keeper.hits = this.keeper.hits + 1;"});
h = create(#-1);
add_property(h, "ref", create(p, 1), {#0, "rw"});
recycle(h);
suspend(0);
result = k.hits;
recycle(p);
recycle(k);
return result;`

var suspendOrderTaskID atomic.Int64

func runSuspendOrderTask(t *testing.T, rt *Runtime, src string) *Task {
	t.Helper()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	tk := NewTaskFull(9100+suspendOrderTaskID.Add(1), 0, compileTestProgram(t, rt.registry, src), ticks, seconds)
	tk.Context.IsWizard = true
	tk.Context.Player = 0
	tk.Context.Programmer = 0
	if err := rt.runTask(tk); err != nil {
		t.Fatal(err)
	}
	runUntilTerminal(t, rt, tk)
	if tk.Result.Flow == types.FlowException {
		t.Fatalf("task raised %v %v", tk.Result.Error, tk.Result.Val)
	}
	return tk
}

func wantHits(t *testing.T, tk *Task) {
	t.Helper()
	if got := tk.Result.Val; got.Type() != types.TYPE_INT || got.Int() != 1 {
		t.Fatalf("k.hits after suspend(0) = %v, want 1: the orphan's :recycle must run before the suspended task resumes", got)
	}
}

// The slice-boundary flush skips the sweep while the last one was expensive and
// recent (waif_lifecycle.go, cheapGCSweep). The resumed task must still observe
// the recycle its own suspend queued.
func TestSuspendResumesAfterQueuedOrphanRecycleWhenSweepThrottled(t *testing.T) {
	store := newConflictTestStore(t)
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.DefaultOptions(), 4)
	rt.lifecycle.Mu.Lock()
	rt.lifecycle.LastGCCost = cheapGCSweep
	rt.lifecycle.LastGCSweep = time.Now()
	rt.lifecycle.Mu.Unlock()
	wantHits(t, runSuspendOrderTask(t, rt, recycleOrphanThenSuspend))
}

// The slice-boundary flush skips the sweep while another task's VM is
// executing (collectAllGCRefs not quiescent).
func TestSuspendResumesAfterQueuedOrphanRecycleBesideRunningTask(t *testing.T) {
	wantHits(t, runBesideSpinningTask(t, recycleOrphanThenSuspend))
}

// runBesideSpinningTask runs src to completion while another task keeps
// executing MOO code.
func runBesideSpinningTask(t *testing.T, src string) *Task {
	t.Helper()
	store := newConflictTestStore(t)
	started := make(chan struct{}, 1)
	var stop atomic.Bool
	var spinStarted, spinGo builtins.BuiltinFunc
	spinStarted = func(_ *builtins.Execution, _ []types.Value) types.Result {
		started <- struct{}{}
		return types.Ok(types.NewInt(0))
	}
	spinGo = func(_ *builtins.Execution, _ []types.Value) types.Result {
		if stop.Load() {
			return types.Ok(types.NewInt(0))
		}
		return types.Ok(types.NewInt(1))
	}
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.DefaultOptions(), 4,
		testBuiltinSlot("spin_started", 0, 0, nil, &spinStarted),
		testBuiltinSlot("spin_go", 0, 0, nil, &spinGo))
	spinner := NewTaskFull(9100+suspendOrderTaskID.Add(1), 0,
		compileTestProgram(t, rt.registry, `spin_started(); while (spin_go()) endwhile; return 0;`), 1<<40, 60)
	spinner.Context.IsWizard = true
	done := make(chan error, 1)
	go func() { done <- rt.runTask(spinner) }()
	t.Cleanup(func() {
		stop.Store(true)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("spinning task did not stop")
		}
	})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("spinning task did not start")
	}
	tk := runSuspendOrderTask(t, rt, src)
	select {
	case err := <-done:
		t.Fatalf("spinning task ended (%v) before the test task; the test proves nothing", err)
	default:
	}
	return tk
}

// The WAIF case of the same order: Toast's recycle_waifs() also runs before the
// next run_ready_tasks(), so a WAIF dropped before suspend(0) has had its
// :recycle called when the task resumes.
// The shape of the conformance test
// audit/divergence_waif_statement_semantics.yaml::waif_recycle_verb_called_on_release.
const releaseWaifThenSuspend = `c = create(#-1);
add_property(c, "hits", 0, {#0, "rw"});
add_verb(c, {#0, "xd", "new"}, {"this", "none", "this"});
set_verb_code(c, "new", {"return new_waif();"});
add_verb(c, {#0, "xd", ":recycle"}, {"this", "none", "this"});
set_verb_code(c, ":recycle", {"this.class.hits = this.class.hits + 1;"});
w = c:new();
w = 0;
suspend(0);
result = c.hits;
recycle(c);
return result;`

func TestSuspendResumesAfterQueuedWaifRecycleWhenSweepThrottled(t *testing.T) {
	store := newConflictTestStore(t)
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.DefaultOptions(), 4)
	rt.lifecycle.Mu.Lock()
	rt.lifecycle.LastGCCost = cheapGCSweep
	rt.lifecycle.LastGCSweep = time.Now()
	rt.lifecycle.Mu.Unlock()
	wantHits(t, runSuspendOrderTask(t, rt, releaseWaifThenSuspend))
}

func TestSuspendResumesAfterQueuedWaifRecycleBesideRunningTask(t *testing.T) {
	wantHits(t, runBesideSpinningTask(t, releaseWaifThenSuspend))
}
