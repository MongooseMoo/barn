package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestCallVerbForegroundBudget(t *testing.T) {
	for _, entry := range []string{"direct", "argstr", "owned_hook", "sweep_hook"} {
		t.Run(entry, func(t *testing.T) {
			store := newConflictTestStore(t)
			var observe builtins.BuiltinFunc
			rt := newTestRuntimeWithBuiltins(t, store, testBuiltinSlot("observe_hook_budget", 0, 0, nil, &observe))
			defer rt.Stop()
			ticks, seconds := foregroundTaskLimits(rt.session)
			var remaining []int64
			observe = func(ctx *builtins.Execution, _ []types.Value) types.Result {
				remaining = append(remaining, ctx.TicksRemaining)
				if got := ctx.Task.SecondsLeft(); got <= 0 || got > seconds {
					t.Errorf("seconds left = %v, want positive foreground budget <= %v", got, seconds)
				}
				if got := ctx.Task.TicksLeft(); got <= 0 || got > ticks {
					t.Errorf("task ticks left = %v, want positive foreground budget <= %v", got, ticks)
				}
				return types.Ok(types.NewInt(0))
			}
			admissionVerb(t, store, "budget_probe", "observe_hook_budget(); total = 0; for i in [1..2000] total = total + i; endfor observe_hook_budget(); return total;")
			var result types.Result
			switch entry {
			case "direct":
				result = rt.CallVerb(0, "budget_probe", nil, 0)
			case "argstr":
				result = rt.CallVerbWithArgstr(0, "budget_probe", nil, 0, "probe")
			case "sweep_hook":
				ctx := kernel.NewTaskContext()
				ctx.Player = 0
				ctx.StoreTxn = store.DirectTxn()
				rt.acquireSweepContext(ctx)
				defer rt.releaseSweepContext(ctx)
				result = rt.session.CallVerb(0, "budget_probe", nil, rt.session.NewExecution(ctx, nil))
			case "owned_hook":
				owner := &task.Task{ID: 91001, Owner: 0}
				if !rt.acquireTaskExecution(owner) {
					t.Fatal("could not acquire hook owner")
				}
				defer rt.releaseTaskExecution(owner.ID)
				ctx := kernel.NewTaskContext()
				ctx.Player = 0
				ctx.StoreTxn = store.DirectTxn()
				rt.acquireExecutionContext(ctx, owner.ID)
				defer rt.releaseExecutionContext(ctx, owner.ID)
				result = rt.session.CallVerb(0, "budget_probe", nil, rt.session.NewExecution(ctx, owner))
			}
			if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 2001000 {
				t.Fatalf("finite hook result = %+v, want return 2001000", result)
			}
			if len(remaining) != 2 || remaining[0]-remaining[1] < 1024 || remaining[1] <= 0 {
				t.Fatalf("remaining ticks = %v, want >1024 ticks consumed below foreground limit", remaining)
			}
		})
	}
}

func TestCallVerbExpiredDeadlineStillStopsExecution(t *testing.T) {
	store := newConflictTestStore(t)
	var expire builtins.BuiltinFunc
	rt := newTestRuntimeWithBuiltins(t, store, testBuiltinSlot("expire_hook_budget", 0, 0, nil, &expire))
	defer rt.Stop()
	expire = func(ctx *builtins.Execution, _ []types.Value) types.Result {
		ctx.Task.SetExecutionDeadline(time.Now().Add(-time.Second))
		return types.Ok(types.NewInt(0))
	}
	admissionVerb(t, store, "expired_probe", "expire_hook_budget(); for i in [1..2000] endfor return 42;")
	result := rt.CallVerb(0, "expired_probe", nil, 0)
	if result.Flow != types.FlowException || result.Error != types.E_MAXREC || result.Val.Str() != "E_MAXREC: seconds limit exceeded" {
		t.Fatalf("expired hook result = %+v, want seconds limit exceeded", result)
	}
}

func TestCallVerbInContextPreservesParentBudget(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "live"
		if expired {
			name = "expired"
		}
		t.Run(name, func(t *testing.T) {
			store := newConflictTestStore(t)
			rt := newTestRuntimeWithBuiltins(t, store)
			defer rt.Stop()
			admissionVerb(t, store, "nested_probe", "total = 0; for i in [1..2000] total = total + i; endfor return total;")
			parent := &task.Task{Owner: 0}
			anchor := time.Now()
			parent.ResetExecutionBudget(40000, 2, anchor)
			deadline := anchor.Add(2 * time.Second)
			if expired {
				deadline = anchor.Add(-time.Second)
			}
			parent.SetExecutionDeadline(deadline)
			ctx := kernel.NewTaskContext()
			ctx.Store = store
			ctx.StoreTxn = store.BeginSnapshot(0)
			defer ctx.StoreTxn.Release()
			ctx.Player = 0
			ctx.TicksRemaining = 40000
			result := rt.CallVerbInContext(0, "nested_probe", nil, rt.session.NewExecution(ctx, parent))
			if expired {
				if result.Flow != types.FlowException || result.Error != types.E_MAXREC || result.Val.Str() != "E_MAXREC: seconds limit exceeded" {
					t.Fatalf("expired nested hook = %+v, want inherited seconds limit exceeded", result)
				}
			} else if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_INT || result.Val.Int() != 2001000 {
				t.Fatalf("live nested hook = %+v, want return 2001000", result)
			}
			if gotAnchor, gotSeconds := parent.ExecutionBudget(); gotAnchor != anchor || gotSeconds != 2 {
				t.Fatalf("parent budget changed: anchor %v seconds %v", gotAnchor, gotSeconds)
			}
			if ctx.TicksRemaining >= 40000 || ctx.BuiltinTicksConsumed <= 0 {
				t.Fatalf("nested execution did not charge parent: remaining %d consumed %d", ctx.TicksRemaining, ctx.BuiltinTicksConsumed)
			}
		})
	}
}
