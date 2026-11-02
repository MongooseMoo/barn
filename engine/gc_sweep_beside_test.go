package engine

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// besideFixture runs one task that keeps executing MOO code (and so keeps
// reaching its tick checkpoints) until stop is set, while the deferred-GC
// maintenance worker sweeps beside it.
type besideFixture struct {
	t         *testing.T
	rt        *Runtime
	store     *dbstore.Store
	started   chan types.Value
	stop      atomic.Bool
	finalized atomic.Int64
	spinner   *Task
	done      chan error
}

func newBesideFixture(t *testing.T) *besideFixture {
	t.Helper()
	f := &besideFixture{t: t, store: newConflictTestStore(t), started: make(chan types.Value, 1)}
	var spinStarted, spinGo, waifRecycled, recycle builtins.BuiltinFunc
	spinStarted = func(_ *builtins.Execution, args []types.Value) types.Result {
		f.started <- args[0]
		return types.Ok(types.NewInt(0))
	}
	spinGo = func(_ *builtins.Execution, _ []types.Value) types.Result {
		if f.stop.Load() {
			return types.Ok(types.NewInt(0))
		}
		return types.Ok(types.NewInt(1))
	}
	waifRecycled = func(_ *builtins.Execution, _ []types.Value) types.Result {
		f.finalized.Add(1)
		return types.Ok(types.NewInt(0))
	}
	recycle = func(_ *builtins.Execution, args []types.Value) types.Result {
		if err := f.store.Recycle(args[0].Obj()); err != nil {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.NewInt(0))
	}
	f.rt = newTestRuntimeWithWorkersAndBuiltins(t, f.store, config.DefaultOptions(), 4,
		testBuiltinSlot("spin_started", 1, 1, []int64{-1}, &spinStarted),
		testBuiltinSlot("spin_go", 0, 0, nil, &spinGo),
		testBuiltinSlot("waif_recycled", 0, 0, nil, &waifRecycled),
		testBuiltinSlot("recycle", 1, 1, []int64{-1}, &recycle))
	t.Cleanup(func() {
		f.stop.Store(true)
		if f.done != nil {
			select {
			case <-f.done:
			case <-time.After(5 * time.Second):
				t.Error("spinning task did not stop")
			}
		}
		f.rt.Stop()
	})
	f.run(`c = create(-1);
add_verb(c, {#0, "xd", "new"}, {"this", "none", "this"});
set_verb_code(c, "new", {"return new_waif();"});
add_verb(c, {#0, "xd", ":recycle"}, {"this", "none", "this"});
set_verb_code(c, ":recycle", {"waif_recycled();"});
add_property(#0, "maker", c, {#0, "rw"});
add_property(#0, "holder", 0, {#0, "rw"});
return 0;`)
	return f
}

var besideTaskID atomic.Int64

func (f *besideFixture) run(src string) *Task {
	f.t.Helper()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	tk := NewTaskFull(7000+besideTaskID.Add(1), 0, compileTestProgram(f.t, f.rt.registry, src), ticks, seconds)
	tk.Context.IsWizard = true
	if err := f.rt.runTask(tk); err != nil {
		f.t.Fatal(err)
	}
	runUntilTerminal(f.t, f.rt, tk)
	if tk.Result.Flow == types.FlowException {
		f.t.Fatalf("setup raised %v %v", tk.Result.Error, tk.Result.Val)
	}
	return tk
}

// spin starts src on its own goroutine and returns the value it passed to
// spin_started() once it is executing.
func (f *besideFixture) spin(src string) types.Value {
	f.t.Helper()
	f.spinner = NewTaskFull(7000+besideTaskID.Add(1), 0, compileTestProgram(f.t, f.rt.registry, src), 1<<40, 60)
	f.spinner.Context.IsWizard = true
	f.done = make(chan error, 1)
	go func() { f.done <- f.rt.runTask(f.spinner) }()
	select {
	case value := <-f.started:
		return value
	case <-time.After(5 * time.Second):
		f.t.Fatal("spinning task did not start")
		return types.None
	}
}

// sweepBeside queues values for an overdue deferred sweep together with a
// fresh orphan, and waits until the maintenance worker has recycled that
// orphan while the spinning task is still executing.
func (f *besideFixture) sweepBeside(values ...types.Value) {
	f.t.Helper()
	orphan, code := f.store.DirectTxn().CreateObject([]types.ObjID{0}, 0, true)
	if code != types.E_NONE {
		f.t.Fatal(code)
	}
	f.rt.lifecycle.Mu.Lock()
	f.rt.lifecycle.LastGCCost = cheapGCSweep
	f.rt.lifecycle.LastGCSweep = time.Now().Add(-gcSweepInterval)
	f.rt.lifecycle.Mu.Unlock()
	f.rt.AdoptPendingFinalizations(append(values, types.NewAnon(orphan)))
	deadline := time.Now().Add(3 * time.Second)
	for f.store.DirectTxn().Valid(orphan) {
		if time.Now().After(deadline) {
			f.t.Fatal("deferred sweep did not run while another task's slice was executing")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-f.done:
		f.t.Fatalf("spinning task ended (%v) before the sweep finished; the test proves nothing", err)
	default:
	}
}

func (f *besideFixture) finish() types.Value {
	f.t.Helper()
	f.stop.Store(true)
	select {
	case err := <-f.done:
		f.done = nil
		if err != nil {
			f.t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		f.t.Fatal("spinning task did not finish")
	}
	return f.spinner.Result.Val
}

// (a) The overdue sweep does not wait for another task's slice to end.
func TestDeferredGCSweepsBesideRunningSlice(t *testing.T) {
	f := newBesideFixture(t)
	f.spin(`spin_started(0); while (spin_go()) endwhile; return 0;`)
	f.sweepBeside()
	f.finish()
}

// (b) A WAIF held only in a running VM's local survives the sweep.
func TestDeferredGCSweepKeepsWaifInRunningVMLocal(t *testing.T) {
	f := newBesideFixture(t)
	w := f.spin(`w = #0.maker:new(); spin_started(w); while (spin_go()) endwhile; return w;`)
	f.sweepBeside(w)
	if n := f.finalized.Load(); n != 0 {
		t.Fatalf("WAIF held by a running VM's local was finalized (%d :recycle calls)", n)
	}
	if got := f.finish(); !got.Equal(w) {
		t.Fatalf("spinner returned %v, want its WAIF", got)
	}
}

// (c) A WAIF held only in a running task's staged property write survives.
func TestDeferredGCSweepKeepsWaifInRunningStagedWrite(t *testing.T) {
	f := newBesideFixture(t)
	w := f.spin(`#0.holder = #0.maker:new(); spin_started(#0.holder); while (spin_go()) endwhile; return 0;`)
	f.sweepBeside(w)
	if n := f.finalized.Load(); n != 0 {
		t.Fatalf("WAIF held by a running task's staged write was finalized (%d :recycle calls)", n)
	}
	f.finish()
	if got, _ := f.store.DirectTxn().PropertyValue(0, "holder"); !got.Equal(w) {
		t.Fatalf("#0.holder = %v after commit, want the staged WAIF", got)
	}
}

// (d) A WAIF reachable only through a history version a running snapshot can
// still read survives.
func TestDeferredGCSweepKeepsWaifInRetainedHistory(t *testing.T) {
	f := newBesideFixture(t)
	f.run(`#0.holder = #0.maker:new(); return 0;`)
	w, code := f.store.DirectTxn().PropertyValue(0, "holder")
	if code != types.E_NONE || w.Type() != types.TYPE_WAIF {
		t.Fatalf("#0.holder = %v (%v), want a WAIF", w, code)
	}
	f.spin(`spin_started(0); while (spin_go()) endwhile; return #0.holder;`)
	if code := f.store.DirectTxn().SetPropertyValue(0, "holder", types.NewInt(0)); code != types.E_NONE {
		t.Fatal(code)
	}
	f.sweepBeside(w)
	if n := f.finalized.Load(); n != 0 {
		t.Fatalf("WAIF a running snapshot can still read was finalized (%d :recycle calls)", n)
	}
	if got := f.finish(); !got.Equal(w) {
		t.Fatalf("spinner read #0.holder = %v, want the WAIF its snapshot holds", got)
	}
}
