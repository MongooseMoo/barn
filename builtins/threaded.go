package builtins

import (
	"sort"
	"sync"
	"time"

	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func runtimeIdentity(ctx *Execution) metrics.Identity {
	if ctx.Task == nil {
		return metrics.Identity{}
	}
	id := metrics.Identity{TaskID: ctx.Task.ID, This: int64(ctx.Task.This), Verb: ctx.Task.VerbName}
	if ctx.CommandDiagnostic != nil {
		id.CommandID = ctx.CommandDiagnostic.ID
	}
	return id
}

// backgroundHandles mirrors ToastStunt's background_process_table
// (background.cc): each threaded builtin whose work is running off the task
// holds a handle from when the work starts until it finishes, killed task or
// not. Handles count up from 1 and restart at 1 once the table is empty.
// threads() lists them.
var backgroundHandles = struct {
	sync.Mutex
	next   int64
	active map[int64]struct{}
}{next: 1, active: map[int64]struct{}{}}

// acquireBackgroundHandle registers one piece of background work and returns
// the function that releases its handle when the work has finished.
func acquireBackgroundHandle() (release func()) {
	backgroundHandles.Lock()
	handle := backgroundHandles.next
	backgroundHandles.next++
	backgroundHandles.active[handle] = struct{}{}
	backgroundHandles.Unlock()
	return func() {
		backgroundHandles.Lock()
		delete(backgroundHandles.active, handle)
		if len(backgroundHandles.active) == 0 {
			backgroundHandles.next = 1
		}
		backgroundHandles.Unlock()
	}
}

// activeBackgroundHandles returns the handles of background work still
// running, in ascending order.
func activeBackgroundHandles() []int64 {
	backgroundHandles.Lock()
	handles := make([]int64, 0, len(backgroundHandles.active))
	for handle := range backgroundHandles.active {
		handles = append(handles, handle)
	}
	backgroundHandles.Unlock()
	sort.Slice(handles, func(i, j int) bool { return handles[i] < handles[j] })
	return handles
}

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
// synchronous host VM runs its work inline instead.

// threadedCall reports whether a threaded builtin called now suspends the task.
func threadedCall(ctx *Execution) bool {
	return ctx.Task != nil && ctx.CanSuspend && ctx.ThreadMode
}

// backgroundValue runs the pure work of a threaded builtin (sort,
// all_members), which returns its result or an error value. held are the
// values the work reads, which its goroutine holds while the task is
// suspended.
func backgroundValue(ctx *Execution, work func() types.Value, held ...types.Value) types.Result {
	if threadedCall(ctx) {
		return runInBackground(ctx, func() types.Result { return types.Ok(work()) }, held...)
	}
	value := work()
	if ctx.Context != nil && !ctx.ThreadMode {
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
//
// held are the values operation reads. Its goroutine holds them while the task
// is suspended, where the task's own state no longer shows them, so they
// escape (escapeFinalizers).
func runInBackground(ctx *Execution, operation func() types.Result, held ...types.Value) types.Result {
	if !threadedCall(ctx) {
		return operation()
	}
	escapeFinalizers(ctx, held...)
	t := ctx.Task
	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	t.SuspendSeconds(-1)
	gen := t.SuspendGeneration()
	id := runtimeIdentity(ctx) // Capture before the task can resume on another goroutine.
	start := func() {
		release := acquireBackgroundHandle()
		go func() {
			defer release()
			metrics.AsyncStarted.Add(1)
			metrics.RuntimeEvent("start", id, gen, false)
			started := time.Now()
			result := operation()
			metrics.ObserveRuntime(metrics.AsyncWork, id, time.Since(started), 0)
			value := result.Val
			if result.IsError() {
				value = types.NewErr(result.Error)
			}
			accepted := t.ResumeGeneration(gen, value)
			metrics.AsyncCompleted.Add(1)
			if !accepted {
				metrics.AsyncRejected.Add(1)
			}
			metrics.RuntimeEvent("complete", id, gen, accepted)
		}()
	}
	if readTxn(ctx).IsDirect() {
		// A direct transaction (EvalCommandOutput, the dbtool) has no commit
		// boundary and no conflict retry, so there is nothing to defer to:
		// start now, exactly as notify() sends immediately on this path.
		start()
	} else {
		enqueuePendingEffect(ctx, task.PendingEffect{Kind: task.PendingEffectAsyncStart, Start: start})
	}
	return types.Suspend(-1)
}
