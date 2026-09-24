package engine

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A lost commit discards private WAIF writes together with object writes.
func TestLostCommitRetryDoesNotReapplyWaifWrite(t *testing.T) {
	store := newConflictTestStore(t)
	var conflictOnce builtins.BuiltinFunc
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1, testBuiltinSlot("conflict_once", 0, 0, []int64{}, &conflictOnce))
	defer s.Stop()
	calls := 0
	conflictOnce = func(ctx *builtins.Execution, args []types.Value) types.Result {
		calls++
		if calls == 1 {
			if errCode := store.DirectTxn().SetPropertyValue(0, "v", types.NewInt(50)); errCode != types.E_NONE {
				return types.Err(errCode)
			}
		}
		return types.Ok(types.NewInt(0))
	}
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	run := func(id int64, src string) *task.Task {
		tk := task.NewTaskFull(id, 0, compileTestProgram(t, s.registry, src), ticks, seconds)
		tk.Context.IsWizard = true
		if err := s.runTask(tk); err != nil {
			t.Fatalf("runTask %d failed: %v", id, err)
		}
		runUntilTerminal(t, s, tk)
		if tk.Result.Flow == types.FlowException {
			t.Fatalf("task %d raised %v %v", id, tk.Result.Error, tk.Result.Val)
		}
		return tk
	}
	run(5201, `c = create(-1);
add_property(c, ":count", 0, {#0, "rw"});
add_verb(c, {#0, "xd", "new"}, {"this", "none", "this"});
set_verb_code(c, "new", {"return new_waif();"});
add_property(#0, "holder", c:new(), {#0, "rw"});
return 0;`)
	got := run(5202, `w = #0.holder;
w.count = w.count + 1;
conflict_once();
#0.v = #0.v + 1;
return w.count;`)
	after := run(5203, `return #0.holder.count;`)
	if calls != 2 {
		t.Fatalf("attempts = %d, want one lost commit and one retry", calls)
	}
	if got.Result.Val.Int() != 1 || after.Result.Val.Int() != 1 {
		t.Fatalf("WAIF write from the discarded attempt survived: returned %v, stored %v", got.Result.Val, after.Result.Val)
	}
}

func TestConcurrentWaifIncrementsRetryWithoutDirtyReads(t *testing.T) {
	store := newConflictTestStore(t)
	w := types.NewWaif(0, 0).SetProperty("count", types.NewInt(0))
	if ec := store.DirectTxn().DefineProperty(0, "holder", dbstore.NewProperty(w, 0, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	var barrier builtins.BuiltinFunc
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2, testBuiltinSlot("waif_barrier", 0, 0, []int64{}, &barrier))
	defer s.Stop()
	arrivals := make(chan struct{}, 2)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var calls atomic.Int32
	barrier = func(ctx *builtins.Execution, args []types.Value) types.Result {
		if calls.Add(1) <= 2 {
			arrivals <- struct{}{}
			<-release
		}
		return types.Ok(types.NewInt(0))
	}
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	program := compileTestProgram(t, s.registry, `w = #0.holder; n = w.count; w.count = n + 1; waif_barrier(); return w.count;`)
	tasks := []*task.Task{task.NewTaskFull(5301, 0, program, ticks, seconds), task.NewTaskFull(5302, 0, program, ticks, seconds)}
	done := make(chan error, 2)
	for _, tk := range tasks {
		tk.Context.IsWizard = true
		go func(tk *task.Task) { done <- s.runTask(tk) }(tk)
	}
	for range tasks {
		select {
		case <-arrivals:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent WAIF attempts did not reach barrier")
		}
	}
	if got, _ := w.GetProperty("count"); got.Int() != 0 {
		t.Fatalf("uncommitted WAIF write escaped: %v", got)
	}
	once.Do(func() { close(release) })
	for range tasks {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("WAIF attempts did not finish")
		}
	}
	for _, tk := range tasks {
		runUntilTerminal(t, s, tk)
		if tk.Result.Flow != types.FlowReturn {
			t.Fatalf("task result %v", tk.Result)
		}
	}
	if got, _ := w.GetProperty("count"); got.Int() != 2 {
		t.Fatalf("concurrent increments lost: %v", got)
	}
	if calls.Load() != 3 {
		t.Fatalf("attempts = %d, want two first attempts and one retry", calls.Load())
	}
}

func TestWaifOnlyWritesPublishAtSuspendAndNestedEvalSharesView(t *testing.T) {
	store := newConflictTestStore(t)
	if ec := store.DirectTxn().SetObjectFlag(0, dbstore.FlagProgrammer, true); ec != types.E_NONE {
		t.Fatal(ec)
	}
	w := types.NewWaif(0, 0).SetProperty("count", types.NewInt(0))
	if ec := store.DirectTxn().DefineProperty(0, "holder", dbstore.NewProperty(w, 0, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	program := compileTestProgram(t, s.registry, `w = #0.holder; w.count = 1; eval("#0.holder.count = #0.holder.count + 1;"); suspend(0); w.count = w.count + 1; return w.count;`)
	tk := task.NewTaskFull(5401, 0, program, ticks, seconds)
	tk.IntrinsicEval = true
	tk.Context.IsWizard = true
	s.taskManager.RegisterTask(tk)
	if err := s.runTask(tk); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.GetProperty("count"); got.Int() != 2 {
		t.Fatalf("WAIF-only slice did not publish nested writes at suspension: %v", got)
	}
	runUntilTerminal(t, s, tk)
	if tk.Result.Flow != types.FlowReturn || tk.Result.Val.Int() != 3 {
		t.Fatalf("resumed WAIF result: %v", tk.Result)
	}
}

func TestWaifConflictRetriesBeforeIrreversibleEffect(t *testing.T) {
	store := newConflictTestStore(t)
	w := types.NewWaif(0, 0).SetProperty("count", types.NewInt(0))
	if ec := store.DirectTxn().DefineProperty(0, "holder", dbstore.NewProperty(w, 0, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	attempts, effects := 0, 0
	var interfere, effect builtins.BuiltinFunc
	interfere = func(ctx *builtins.Execution, args []types.Value) types.Result {
		attempts++
		if attempts == 1 {
			if ec := store.DirectTxn().SetWaifProperty(w, "count", types.NewInt(10)); ec != types.E_NONE {
				return types.Err(ec)
			}
		}
		return types.Ok(types.NewInt(0))
	}
	effect = func(ctx *builtins.Execution, args []types.Value) types.Result {
		effects++
		if got, _ := w.GetProperty("count"); got.Int() != 11 {
			t.Errorf("irreversible effect observed uncommitted or stale WAIF: %v", got)
		}
		return types.Ok(types.NewInt(0))
	}
	descriptor := testBuiltinSlot("waif_effect", 0, 0, []int64{}, &effect)
	descriptor.Effect = builtins.Irreversible
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1, testBuiltinSlot("waif_interfere", 0, 0, []int64{}, &interfere), descriptor)
	defer s.Stop()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	program := compileTestProgram(t, s.registry, `w = #0.holder; w.count = w.count + 1; waif_interfere(); waif_effect(); w.count = w.count + 1; return w.count;`)
	tk := task.NewTaskFull(5501, 0, program, ticks, seconds)
	tk.Context.IsWizard = true
	if err := s.runTask(tk); err != nil {
		t.Fatal(err)
	}
	if tk.Result.Flow != types.FlowReturn || tk.Result.Val.Int() != 12 || attempts != 2 || effects != 1 {
		t.Fatalf("result %v, attempts %d, effects %d", tk.Result, attempts, effects)
	}
	assertCommitGateReleased(t, store)
}

func TestReadOnlyWaifCacheDoesNotKeepDroppedLocalAlive(t *testing.T) {
	for _, suspend := range []bool{false, true} {
		name := "completion"
		if suspend {
			name = "suspension"
		}
		t.Run(name, func(t *testing.T) {
			store := newConflictTestStore(t)
			if ec := store.DirectTxn().DefineProperty(0, ":n", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			for name, code := range map[string][]string{"new": {"return new_waif();"}, ":recycle": {"#0.v = #0.v + 1;"}} {
				verb := dbstore.NewVerb(name, []string{name}, 0, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, code)
				if _, ec := store.AddVerb(0, verb); ec != types.E_NONE {
					t.Fatal(ec)
				}
			}
			s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
			defer s.Stop()
			ticks, seconds := foregroundTaskLimits(newTestRegistry())
			source := `w = #0:new(); n = w.n; w = 0; `
			if suspend {
				source += `suspend(0); `
			}
			program := compileTestProgram(t, s.registry, source+`return n;`)
			tk := task.NewTaskFull(5601, 0, program, ticks, seconds)
			tk.IntrinsicEval = true
			tk.Context.IsWizard = true
			s.taskManager.RegisterTask(tk)
			if err := s.runTask(tk); err != nil {
				t.Fatal(err)
			}
			s.flushDeferredGC()
			if !suspend && tk.Result.Flow != types.FlowReturn {
				t.Fatalf("result %v", tk.Result)
			}
			if got := readRootV(t, store); got != 1 {
				t.Fatalf("recycle calls = %d, want 1 after read-only local leaves scope", got)
			}
			if suspend {
				runUntilTerminal(t, s, tk)
			}
		})
	}
}
