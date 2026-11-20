package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestRecycleContinuationKeepsGuardWhenValidationRequestsRetry(t *testing.T) {
	ctx := newTestExecution()
	if !ctx.Session.startRecycle(345) {
		t.Fatal("fixture did not reserve object")
	}
	defer ctx.Session.EndRecycle(types.NewAnon(345))
	ctx.BeforeIrreversibleEffect = func(string) bool { ctx.ConflictRetryRequested = true; return true }
	result := FinishRecycleLifecycle(ctx, RecycleLifecycleRequest{Object: types.NewAnon(345)}, types.Err(types.E_INVARG))
	if result.Flow != types.FlowAbortAttempt || ctx.IrreversibleSideEffect {
		t.Fatalf("result=%+v irreversible=%v", result, ctx.IrreversibleSideEffect)
	}
	if ctx.Session.startRecycle(345) {
		t.Fatal("abandoned attempt released the prior committed reservation")
	}
}
