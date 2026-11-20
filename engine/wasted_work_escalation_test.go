package engine

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// wastedWorkTicksLimit is far above escalateAfterWastedTicks so the long
// attempt below is never stopped by its own tick budget.
const wastedWorkTicksLimit = 10_000_000

// wastedWorkBody spins loops iterations, reads #0.v, calls conflict(), and
// writes #0.v.
func wastedWorkBody(loops int) string {
	return fmt.Sprintf(`
for i in [1..%d]
endfor
before = #0.v;
conflict();
#0.v = before + 1;
`, loops)
}

// wastedWorkFixture is a runtime whose conflict() builtin moves #0.v in the
// live store (a concurrent writer's commit landing after the read) whenever
// bump reports true for that call, so the attempt loses its commit. used holds
// the ticks each conflict() call saw used by its attempt.
type wastedWorkFixture struct {
	s     *Runtime
	store *dbstore.Store
	used  []int64
}

func newWastedWorkFixture(t *testing.T, bump func(call int, ctx *builtins.Execution) bool) *wastedWorkFixture {
	t.Helper()
	f := &wastedWorkFixture{store: newConflictTestStore(t)}
	// $server_options is #0 itself: forked tasks get bg_ticks, which must cover
	// the long attempt.
	for name, value := range map[string]types.Value{
		"server_options": types.NewObj(0),
		"bg_ticks":       types.NewInt(wastedWorkTicksLimit),
		"bg_seconds":     types.NewInt(120),
	} {
		if code := f.store.DirectTxn().DefineProperty(0, name, dbstore.NewProperty(value, 0, dbstore.PropRead, false, true)); code != types.E_NONE {
			t.Fatalf("DefineProperty(%s) failed: %s", name, code)
		}
	}
	conflict := builtins.BuiltinFunc(func(ctx *builtins.Execution, _ []types.Value) types.Result {
		f.used = append(f.used, wastedWorkTicksLimit-ctx.TicksRemaining)
		if bump(len(f.used), ctx) {
			if code := f.store.DirectTxn().SetPropertyValue(0, "v", types.NewInt(int64(len(f.used)*10))); code != types.E_NONE {
				return types.Err(code)
			}
		}
		return types.Ok(types.NewInt(0))
	})
	f.s = newTestRuntimeWithWorkersAndBuiltins(t, f.store, config.Options{}, 1, testBuiltinSlot("conflict", 0, 0, nil, &conflict))
	t.Cleanup(f.s.Stop)
	// Mirror server boot: the session loads the options its tasks run under.
	f.s.Session().LoadServerOptionsFromStore(f.store)
	if ticks, _ := backgroundTaskLimits(f.s.Session()); ticks != wastedWorkTicksLimit {
		t.Fatalf("bg_ticks=%d, want %d", ticks, wastedWorkTicksLimit)
	}
	return f
}

// runForked runs a parent that forks the wasted-work body and returns the
// forked child after it finishes; the child's first run is the attempt under
// test.
func (f *wastedWorkFixture) runForked(t *testing.T, loops int) *Task {
	t.Helper()
	owner := types.ObjID(8120)
	ticks, seconds := foregroundTaskLimits(f.s.Session())
	parent := NewTaskFull(3120, owner, compileTestProgram(t, f.s.registry, "fork (0)\n"+wastedWorkBody(loops)+"endfork\nreturn 0;\n"), ticks, seconds)
	parent.Context.IsWizard = true
	t.Cleanup(func() { removeTasksForOwner(f.s, owner) })
	if err := f.s.runTask(parent); err != nil {
		t.Fatalf("runTask(parent) failed: %v", err)
	}
	child := findForkedChild(f.s, owner)
	if child == nil {
		t.Fatal("fork did not create a child task")
	}
	runUntilTerminal(t, f.s, child)
	if child.GetState() != task.TaskCompleted || child.Result.Flow == types.FlowException {
		t.Fatalf("forked child = state %v flow %v err %v, want completed without exception", child.GetState(), child.Result.Flow, child.Result.Error)
	}
	return child
}

// A forked task's first run that loses once after executing at least
// escalateAfterWastedTicks re-runs holding the exclusive commit gate, so it
// cannot lose again, and the hold is recorded under the wasted-work cause.
func TestForkedRunEscalatesAfterExpensiveLostAttempt(t *testing.T) {
	// Every optimistic attempt is beaten by the writer; only a gate-exempt
	// (escalated) attempt can commit.
	f := newWastedWorkFixture(t, func(_ int, ctx *builtins.Execution) bool {
		return !ctx.StoreTxn.IsCommitGateExempt()
	})
	child := f.runForked(t, 2*escalateAfterWastedTicks)
	if len(f.used) != 2 || f.used[0] < escalateAfterWastedTicks {
		t.Fatalf("conflict() calls=%d ticks used=%v, want 2 calls with the first at >= %d ticks", len(f.used), f.used, escalateAfterWastedTicks)
	}
	if f.store.CommitRetries() != 1 || f.store.CommitEscalations() != 1 {
		t.Fatalf("retries=%d escalations=%d, want 1 and 1", f.store.CommitRetries(), f.store.CommitEscalations())
	}
	if got := readRootV(t, f.store); got != 11 {
		t.Fatalf("#0.v = %d, want 11 (retry read the competing write)", got)
	}
	holds := f.s.GateHolds()
	if len(holds) != 1 || holds[0].This != child.This || holds[0].Verb != child.VerbName || holds[0].Cause != gateCauseWastedWork || holds[0].Count != 1 {
		t.Fatalf("holds=%+v, want one #%d:%s hold with cause %q", holds, child.This, child.VerbName, gateCauseWastedWork)
	}
	assertCommitGateReleased(t, f.store)
}

// A forked task's first run that loses after executing fewer than
// escalateAfterWastedTicks keeps retrying optimistically: its next attempt
// does not take the gate.
func TestForkedRunCheapLostAttemptRetriesOptimistically(t *testing.T) {
	f := newWastedWorkFixture(t, func(call int, _ *builtins.Execution) bool {
		return call == 1
	})
	f.runForked(t, 100)
	if f.store.CommitRetries() != 1 || f.store.CommitEscalations() != 0 {
		t.Fatalf("retries=%d escalations=%d, want 1 and 0", f.store.CommitRetries(), f.store.CommitEscalations())
	}
	if len(f.used) != 2 || f.used[0] >= escalateAfterWastedTicks {
		t.Fatalf("conflict() calls=%d ticks used=%v, want 2 calls with the first under %d ticks", len(f.used), f.used, escalateAfterWastedTicks)
	}
	if got := readRootV(t, f.store); got != 11 {
		t.Fatalf("#0.v = %d, want 11", got)
	}
	if holds := f.s.GateHolds(); len(holds) != 0 {
		t.Fatalf("holds=%+v, want none", holds)
	}
}

// A player command that loses after executing at least
// escalateAfterWastedTicks retries optimistically: escalating heavy commands
// serialized the server at 16 players.
func TestCommandExpensiveLostAttemptRetriesOptimistically(t *testing.T) {
	f := newWastedWorkFixture(t, func(call int, _ *builtins.Execution) bool {
		return call == 1
	})
	queued := NewTaskFull(8121, 0, compileTestProgram(t, f.s.registry, wastedWorkBody(2*escalateAfterWastedTicks)+"return before;\n"), wastedWorkTicksLimit, 120)
	queued.Context.IsWizard = true
	queued.This, queued.VerbName = 7, "long_job"
	runSQLiteTaskToCompletion(t, f.s, queued)
	if len(f.used) != 2 || f.used[0] < escalateAfterWastedTicks {
		t.Fatalf("conflict() calls=%d ticks used=%v, want 2 calls with the first at >= %d ticks", len(f.used), f.used, escalateAfterWastedTicks)
	}
	if f.store.CommitRetries() != 1 || f.store.CommitEscalations() != 0 {
		t.Fatalf("retries=%d escalations=%d, want 1 and 0", f.store.CommitRetries(), f.store.CommitEscalations())
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 10 || readRootV(t, f.store) != 11 {
		t.Fatalf("result=%+v v=%d, want return 10 and v=11", queued.Result, readRootV(t, f.store))
	}
	if holds := f.s.GateHolds(); len(holds) != 0 {
		t.Fatalf("holds=%+v, want none", holds)
	}
}
