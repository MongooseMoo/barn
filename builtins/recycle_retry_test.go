package builtins

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestRecycleContinuationKeepsGuardWhenValidationRequestsRetry(t *testing.T) {
	ctx := newTestExecution()
	if !beginRecycle(ctx, 345) {
		t.Fatal("fixture did not reserve object")
	}
	defer endRecycle(ctx, 345)
	ctx.BeforeIrreversibleEffect = func() bool { ctx.ConflictRetryRequested = true; return true }
	result := FinishRecycleLifecycle(ctx, RecycleLifecycleRequest{Object: types.NewObj(345)}, types.Err(types.E_INVARG))
	if result.Flow != types.FlowAbortAttempt || ctx.IrreversibleSideEffect {
		t.Fatalf("result=%+v irreversible=%v", result, ctx.IrreversibleSideEffect)
	}
	if beginRecycle(ctx, 345) {
		t.Fatal("abandoned attempt released the prior committed reservation")
	}
}
