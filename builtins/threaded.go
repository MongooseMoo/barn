package builtins

import (
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/types"
)

// Threaded builtins mirror ToastStunt's background_thread() (background.cc).
// While the calling activation's thread mode is on (the default), Toast runs
// the builtin's work on a background thread and suspends the task until it
// finishes, so every call is a point where other tasks run. The task resumes
// through resume_from_previous_vm() (execute.cc) as a background task with a
// fresh bg_ticks/bg_seconds budget, and an error value from the work is raised
// there. With set_thread_mode(0) the work runs inline and its value, an error
// value included, is returned as is.
//
// Barn suspends only where the suspension is honored: a builtin called from a
// nested VM (create()'s :initialize, for example) runs its work inline instead.

// threadedCall reports whether a threaded builtin called now suspends the task.
func threadedCall(ctx *Execution) bool {
	return ctx.Task != nil && ctx.CanSuspend && ctx.ThreadMode
}

// backgroundValue runs the pure work of a threaded builtin (sort,
// all_members), which returns its result or an error value.
func backgroundValue(ctx *Execution, work func() types.Value) types.Result {
	if threadedCall(ctx) {
		return runInBackground(ctx, func() types.Result { return types.Ok(work()) })
	}
	value := work()
	if ctx.TaskContext != nil && !ctx.ThreadMode {
		return types.Ok(value)
	}
	// Toast would suspend here but this call cannot, so it gets what the
	// resumed task would: an error value is raised.
	if value.Type() == types.TYPE_ERR {
		return types.Err(value.ErrCode())
	}
	return types.Ok(value)
}

// runInBackground runs an external operation (SQLite, curl) off the
// scheduler's task goroutines. Every completion, including an error, resumes
// the suspended task exactly once. When the call is not threaded the operation
// runs inline and the task never suspends; the caller must cross the
// irreversible-effect boundary first.
//
// The threaded operation is an external effect, so it must not start until this
// slice's transaction has been published. The suspend commits the transaction;
// if that commit loses validation the runtime discards the slice and re-executes
// the task from the top, and an operation already in flight would both leak its
// effect (a BEGIN or INSERT executed once per attempt) and deliver its completion
// into the retried attempt's own suspension. Launching through the commit-gated
// effect log runs the operation at most once per published slice, and because
// nothing ran, the attempt stays eligible for conflict retry. The generation
// check makes any completion that no longer belongs to the suspension it was
// started for a no-op.
func runInBackground(ctx *Execution, operation func() types.Result) types.Result {
	if !threadedCall(ctx) {
		return operation()
	}
	t := ctx.Task
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	mgr.SuspendTask(t, -1)
	gen := t.SuspendGeneration()
	start := func() {
		go func() {
			result := operation()
			if result.IsError() {
				_ = t.ResumeGeneration(gen, types.NewErr(result.Error))
				return
			}
			_ = t.ResumeGeneration(gen, result.Val)
		}()
	}
	if readTxn(ctx).IsDirect() {
		// A direct transaction (EvalCommandOutput, the dbtool) has no commit
		// boundary and no conflict retry, so there is nothing to defer to:
		// start now, exactly as notify() sends immediately on this path.
		start()
	} else {
		enqueuePendingEffect(ctx, kernel.PendingEffect{Kind: kernel.PendingEffectAsyncStart, Start: start})
	}
	return types.Suspend(-1)
}
