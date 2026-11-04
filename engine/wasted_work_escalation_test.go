package engine

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// wastedWorkTicksLimit is far above escalateAfterWastedTicks so the long
// attempt below is never stopped by its own tick budget.
const wastedWorkTicksLimit = 10_000_000

// runWastedWorkTask runs a task that spins loops iterations, reads #0.v, calls
// conflict(), and writes #0.v. conflict() moves #0.v in the live store (a
// concurrent writer's commit landing after the read) whenever bump reports
// true for that call, so the attempt loses its commit. It returns the number
// of conflict() calls and the ticks each call saw used by its attempt.
func runWastedWorkTask(t *testing.T, loops int, bump func(call int, ctx *builtins.Execution) bool) (*Runtime, *dbstore.Store, *Task, []int64) {
	t.Helper()
	store := newConflictTestStore(t)
	var used []int64
	conflict := builtins.BuiltinFunc(func(ctx *builtins.Execution, _ []types.Value) types.Result {
		used = append(used, wastedWorkTicksLimit-ctx.TicksRemaining)
		if bump(len(used), ctx) {
			if code := store.DirectTxn().SetPropertyValue(0, "v", types.NewInt(int64(len(used)*10))); code != types.E_NONE {
				return types.Err(code)
			}
		}
		return types.Ok(types.NewInt(0))
	})
	s := newTestRuntimeWithBuiltins(t, store, testBuiltinSlot("conflict", 0, 0, nil, &conflict))
	t.Cleanup(s.Stop)
	queued := NewTaskFull(8120, 0, compileTestProgram(t, s.registry, fmt.Sprintf(`
for i in [1..%d]
endfor
before = #0.v;
conflict();
#0.v = before + 1;
return before;
`, loops)), wastedWorkTicksLimit, 120)
	queued.Context.IsWizard = true
	queued.This, queued.VerbName = 7, "long_job"
	runSQLiteTaskToCompletion(t, s, queued)
	return s, store, queued, used
}

// A slice that loses once after executing at least escalateAfterWastedTicks
// re-runs holding the exclusive commit gate, so it cannot lose again, and the
// hold is recorded under the wasted-work cause.
func TestRunTaskEscalatesAfterExpensiveLostAttempt(t *testing.T) {
	// Every optimistic attempt is beaten by the writer; only a gate-exempt
	// (escalated) attempt can commit.
	s, store, queued, used := runWastedWorkTask(t, 2*escalateAfterWastedTicks, func(_ int, ctx *builtins.Execution) bool {
		return !ctx.StoreTxn.IsCommitGateExempt()
	})
	if len(used) != 2 || used[0] < escalateAfterWastedTicks {
		t.Fatalf("conflict() calls=%d ticks used=%v, want 2 calls with the first at >= %d ticks", len(used), used, escalateAfterWastedTicks)
	}
	if store.CommitRetries() != 1 || store.CommitEscalations() != 1 {
		t.Fatalf("retries=%d escalations=%d, want 1 and 1", store.CommitRetries(), store.CommitEscalations())
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 10 || readRootV(t, store) != 11 {
		t.Fatalf("result=%+v v=%d, want return 10 and v=11", queued.Result, readRootV(t, store))
	}
	holds := s.GateHolds()
	if len(holds) != 1 || holds[0].This != 7 || holds[0].Verb != "long_job" || holds[0].Cause != gateCauseWastedWork || holds[0].Count != 1 {
		t.Fatalf("holds=%+v, want one #7:long_job hold with cause %q", holds, gateCauseWastedWork)
	}
	assertCommitGateReleased(t, store)
}

// A slice that loses after executing fewer than escalateAfterWastedTicks keeps
// retrying optimistically: its next attempt does not take the gate.
func TestRunTaskCheapLostAttemptRetriesOptimistically(t *testing.T) {
	s, store, queued, used := runWastedWorkTask(t, 100, func(call int, _ *builtins.Execution) bool {
		return call == 1
	})
	if store.CommitRetries() != 1 || store.CommitEscalations() != 0 {
		t.Fatalf("retries=%d escalations=%d, want 1 and 0", store.CommitRetries(), store.CommitEscalations())
	}
	if len(used) != 2 || used[0] >= escalateAfterWastedTicks {
		t.Fatalf("conflict() calls=%d ticks used=%v, want 2 calls with the first under %d ticks", len(used), used, escalateAfterWastedTicks)
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 10 || readRootV(t, store) != 11 {
		t.Fatalf("result=%+v v=%d, want return 10 and v=11", queued.Result, readRootV(t, store))
	}
	if holds := s.GateHolds(); len(holds) != 0 {
		t.Fatalf("holds=%+v, want none", holds)
	}
}
