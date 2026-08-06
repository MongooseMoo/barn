package scheduler

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
)

func TestRunTaskTransfersExecutionProvenanceAcrossConflictRetry(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	root.SetProperty("retry_value", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}

	scheduler := NewScheduler(store)
	defer scheduler.Stop()
	forceCalls := 0
	scheduler.registry.Register("force_retry_conflict", func(ctx *kernel.TaskContext, _ []types.Value) types.Result {
		forceCalls++
		if forceCalls == 1 {
			// Simulate a concurrent commit after this attempt read retry_value. Do
			// not mark LiveStoreMutated: this is deliberately a retryable conflict.
			if errCode := ctx.Store.SetPropertyValue(0, "retry_value", types.NewInt(10)); errCode != types.E_NONE {
				return types.Err(errCode)
			}
		}
		return types.Ok(types.NewInt(0))
	})

	var firstCtx, replacementCtx *kernel.TaskContext
	firstAttributed := false
	replacementAttributed := false
	firstRemovedDuringRetry := false
	observeCalls := 0
	scheduler.registry.Register("observe_retry_provenance", func(ctx *kernel.TaskContext, _ []types.Value) types.Result {
		observeCalls++
		ownerID, ok := scheduler.executionContextOwner(ctx)
		holder, _ := ctx.Task.(*task.Task)
		attributed := ok && holder != nil && ownerID == holder.ID
		if observeCalls == 1 {
			firstCtx = ctx
			firstAttributed = attributed
		} else if observeCalls == 2 {
			replacementCtx = ctx
			replacementAttributed = attributed
			_, claimed, _ := scheduler.executionContextClaim(firstCtx)
			firstRemovedDuringRetry = !claimed
		}
		return types.Ok(types.NewInt(0))
	})

	ticks, seconds := foregroundTaskLimits()
	running := task.NewTaskFull(94001, 0, compileTestProgram(t, scheduler.registry, `
before = #0.retry_value;
force_retry_conflict();
#0.retry_value = before + 1;
observe_retry_provenance();
return #0.retry_value;
`), ticks, seconds)
	running.Context.IsWizard = true
	if err := scheduler.runTask(running); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}

	if forceCalls != 2 || observeCalls != 2 {
		t.Fatalf("attempt calls = force %d, observe %d; want two conflict attempts", forceCalls, observeCalls)
	}
	if firstCtx == nil || replacementCtx == nil || firstCtx == replacementCtx {
		t.Fatalf("retry contexts = first %p replacement %p, want distinct pointers", firstCtx, replacementCtx)
	}
	if !firstAttributed || !replacementAttributed || !firstRemovedDuringRetry {
		t.Fatalf("provenance transfer = first %v replacement %v old-removed %v, want all true", firstAttributed, replacementAttributed, firstRemovedDuringRetry)
	}
	if _, claimed, _ := scheduler.executionContextClaim(firstCtx); claimed {
		t.Fatal("initial TaskContext retained an execution claim after runTask returned")
	}
	if _, claimed, _ := scheduler.executionContextClaim(replacementCtx); claimed {
		t.Fatal("replacement TaskContext retained an execution claim after runTask returned")
	}
	if running.Result.Flow != types.FlowReturn || running.Result.Val.Type() != types.TYPE_INT || running.Result.Val.Int() != 11 {
		t.Fatalf("retry result = %+v, want committed return 11", running.Result)
	}
	if retries := store.CommitRetries(); retries == 0 {
		t.Fatal("forced conflict did not record a commit retry")
	}
}
