package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/command"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// maxConflictRetryAttempts bounds how many times a fresh AST task re-runs after an
// MVCC commit conflict. Conflicts only arise between tasks committing inside the same
// optimistic batch, and batches never exceed workerCount tasks, so retrying more than
// the worker count is enough to guarantee the loser eventually commits against every
// peer's writes. The bound exists only to prevent livelock under a pathological store.
const maxConflictRetryAttempts = 64

// escalateAfterAttempts is the optimistic-loss budget before a task stops
// gambling and re-executes under the store's exclusive commit gate (a
// guaranteed win against every commit-based writer). With the threshold below
// maxConflictRetryAttempts, cap exhaustion — which surfaces a conflict-only
// E_INVARG to the user as a phantom "coding error" no serial execution
// produces — becomes impossible: the final attempt cannot lose.
//
// Tuning data (16p real-mongoose mix, experiments/2026-07-27): at 8 the gate
// serialized the server (goodput -45%, p50 6ms → 117ms); at 48 it traded
// ~13% goodput for ~25% max-latency; at 63 (escalate only on the last
// attempt) throughput and tail match no-gate within noise while keeping the
// cannot-lose guarantee. Escalation is correctness insurance, not a second
// commit path.
const escalateAfterAttempts = 63

var ErrCommandVerbNoCode = errors.New("command verb has no code")

// runTask executes a task's code using the bytecode VM
func (s *Runtime) runTask(t *task.Task) (retErr error) {
	pprof.Do(s.ctx, pprof.Labels("moo.task", fmt.Sprint(t.ID), "moo.verb", fmt.Sprintf("#%d:%s", t.This, t.VerbName)), func(context.Context) {
		retErr = s.runTaskSlice(t)
	})
	return retErr
}

func (s *Runtime) runTaskSlice(t *task.Task) (retErr error) {
	started := time.Now()
	var bcVM *vm.VM
	var sliceTicks int64
	gateHeld := false
	defer func() {
		if elapsed := time.Since(started); elapsed >= 100*time.Millisecond {
			slog.Debug("slow task slice", slog.Int64("task_id", t.ID),
				slog.Int64("this", int64(t.This)), slog.String("verb", t.VerbName),
				slog.Int64("ticks", sliceTicks), slog.Bool("gate_held", gateHeld),
				slog.Duration("elapsed", elapsed), slog.Any("err", retErr))
		}
	}()
	s.beginFinalizationProducer()
	defer s.finishFinalizationProducer()
	// Logical TaskRunning ends as soon as a builtin records suspension, before
	// the VM has returned and before its resumable state is published. Keep a
	// runtime-owned physical execution lease across the entire invocation so
	// GC never walks or ignores a VM that this goroutine can still mutate.
	s.acquireTaskExecution(t)
	var executionCtx *kernel.TaskContext
	defer func() {
		// This is the singular runTask lifecycle boundary for every caller:
		// publish all task/VM state first, release the physical execution lease,
		// then settle deferred GC while the just-finished VM is safe to inspect.
		// If another task remains active the flush fails closed and that task's
		// corresponding lifecycle boundary will retry it.
		if bcVM != nil {
			sliceTicks = bcVM.Ticks
		}
		if executionCtx != nil {
			s.releaseExecutionContext(executionCtx, t.ID)
		}
		s.releaseTaskExecution(t.ID)
		s.flushDeferredGC()
	}()

	// Recover from panics to avoid crashing the server
	defer func() {
		if r := recover(); r != nil {
			metrics.PanicsRecovered.Add(1)
			slog.Error("panic in task",
				slog.Int64("task_id", t.ID),
				slog.Int64("this", int64(t.This)),
				slog.String("verb", t.VerbName),
				slog.String("panic", fmt.Sprint(r)),
				slog.String("go_stack", string(debug.Stack())))
			t.SetState(task.TaskKilled)
			retErr = fmt.Errorf("internal panic: %v", r)
		}
	}()

	retryState := s.captureTaskRetryState(t)
	defer func() {
		if ctx := t.ContextValue(); ctx != nil {
			ctx.StoreTxn.Release()
		}
	}()
	attempt := 0
	escalated := false
	// Backstop for every early return (suspend hand-off, deadline, panic): the
	// gate must never outlive this invocation. The common path releases it
	// explicitly right after the attempt's commit resolves.
	defer func() {
		if escalated {
			s.store.EscalationUnlock()
		}
	}()

retryAttempt:
	// Two reasons to run this attempt under the exclusive commit gate: the
	// optimistic-loss budget is spent, or the slice cannot be re-executed at all
	// (a resumed task's VM carries mid-flight state no retry can rebuild). A
	// slice that cannot retry has exactly one defence against a lost commit —
	// making the loss impossible — because the alternative is handing MOO code
	// a frameless E_INVARG no serial execution produces (issue #296).
	if (attempt >= escalateAfterAttempts || !retryState.canRetry) && !escalated {
		s.store.EscalationLock()
		escalated = true
		gateHeld = true
	}
	if attempt > 0 {
		retryState.restore(t)
		// A failed attempt may have recorded a logical suspend before its
		// transaction conflict was detected. The physical lease remains held,
		// while the retry begins a fresh logical running slice.
		s.mu.Lock()
		t.SetState(task.TaskRunning)
		s.mu.Unlock()
	}

	ctx := t.ContextValue()
	if ctx == nil {
		t.SetState(task.TaskKilled)
		return errors.New("task has no context")
	}
	if executionCtx != ctx {
		if executionCtx != nil {
			s.releaseExecutionContext(executionCtx, t.ID)
		}
		s.acquireExecutionContext(ctx, t.ID)
		executionCtx = ctx
	}

	ctx.TaskID = t.ID
	ctx.Store = s.store
	// Release any txn left on this context from a previous attempt/run before
	// beginning a fresh one, so its readTS deregisters from the history-GC floor
	// promptly (the runtime finalizer is only a backstop).
	ctx.StoreTxn.Release()
	ctx.StoreTxn = s.store.BeginReadOnly(0)
	if escalated {
		// Snapshot taken while holding the gate exclusively: no ordinary commit
		// can interleave before this attempt's own commit, so it cannot lose
		// validation to one. The txn must skip the shared gate or it would
		// deadlock against our own exclusive hold.
		ctx.StoreTxn.ExemptFromCommitGate()
	}
	ctx.LiveStoreMutated = false
	ctx.IrreversibleSideEffect = false
	ctx.ConflictRetryRequested = false
	// An irreversible external effect selected by a builtin descriptor makes the
	// rest of the attempt un-retryable: a commit conflict after it can no longer be
	// answered by re-running the task and would surface as an uncatchable E_INVARG
	// no serial execution produces, with the attempt's pending output discarded (a
	// Mongoose login task lost exactly this way at its read() after
	// set_connection_option, to a $logger commit that landed between its snapshot
	// and that call). So the effect is where the attempt stops gambling. Take the
	// exclusive commit gate, then commit-and-renew: the reads made so far are
	// validated against the now-frozen store, the writes made so far are
	// published, and the attempt continues on a read view taken under the gate
	// (still carrying the validated reads, so the final commit checks them too),
	// so nothing it reads from here on can be stale either. If the validation
	// fails the effect has not happened yet, so the task is re-run from the top.
	// Once the renew succeeds no ordinary commit can interleave until this
	// attempt's own commit, so it cannot lose. Coarse builtins that mutate the
	// live store directly cross the same boundary first (beforeCoarse), so a
	// live-mutated attempt cannot lose either. Publishing at the boundary
	// exposes the slice's first half to
	// concurrent readers before its second half exists; none of them can commit
	// on that view until this attempt has.
	ctx.BeforeIrreversibleEffect = func() bool {
		if escalated {
			return false
		}
		waitStart := time.Now()
		s.store.EscalationLock()
		gateWait := time.Since(waitStart)
		t.ExcludeExecutionWait(gateWait)
		escalated = true
		gateHeld = true
		ctx.StoreTxn.ExemptFromCommitGate()
		canRerun := retryState.canRetry && !ctx.LiveStoreMutated && attempt < maxConflictRetryAttempts
		next, publishedWrites, errCode := ctx.StoreTxn.CommitAndRenewCarryingReads()
		slog.Debug("irreversible-effect boundary",
			slog.Int64("task_id", t.ID), slog.String("verb", t.VerbName),
			slog.Duration("gate_wait", gateWait),
			slog.String("renew", types.NewErr(errCode).String()),
			slog.Bool("published_writes", publishedWrites),
			slog.Bool("can_rerun", canRerun), slog.Int("attempt", attempt))
		if errCode != types.E_NONE {
			// A validation loss with the effect still ahead is answered by re-running;
			// any other failure (a terminal preflight error, or a loss this task cannot
			// re-run) is left for the final commit to surface exactly as before.
			if ctx.StoreTxn.ValidationFailed() && canRerun {
				ctx.ConflictRetryRequested = true
				return true
			}
			return false
		}
		ctx.StoreTxn = next
		if publishedWrites {
			// Mirrors the suspend-time commit: the forks and effects recorded so far
			// are now durable, so a later discard must not touch them.
			t.CreatedForks = nil
			builtins.FlushPendingEffects(s.session.NewExecution(ctx, t))
		}
		return false
	}
	ctx.DeferredCheckpoint = false
	ctx.RuntimeOptions = s.options

	// releaseEscalation hands the commit gate back once this attempt's commits
	// are decided. Everything after it — completion hooks, the suspend hand-off,
	// a failure-path txn that lives on — takes the gate normally. A checkpoint
	// the attempt had to postpone (dump_database under the gate) runs here, now
	// that it can take the gate and run its hook tasks without deadlocking.
	releaseEscalation := func() {
		if !escalated {
			return
		}
		ctx.StoreTxn.ClearCommitGateExemption()
		s.store.EscalationUnlock()
		escalated = false
		s.runDeferredCheckpoint(ctx)
	}

	// A task resuming after suspend runs under background limits: Toast treats
	// resumed tasks as background tasks, and time spent suspended does not count
	// against the execution budget. Reset both the tick and second budgets (and
	// the start time used for the deadline below) so ticks_left()/seconds_left()
	// and the hard deadline reflect a fresh background slice.
	if savedVM, ok := t.BytecodeVMValue().(*vm.VM); ok && savedVM.IsYielded() {
		bgTicks, bgSeconds := backgroundTaskLimits(s.session)
		t.ResetExecutionBudget(bgTicks, bgSeconds, time.Now())
		savedVM.TickLimit = bgTicks
		savedVM.Ticks = 0
	}

	// Set up the VM execution deadline. The budget deadline must be anchored
	// to when the task actually starts running, not its (possibly long-past)
	// scheduled start time — otherwise a fork-delayed or checkpoint-restored
	// task whose StartTime has already elapsed (e.g. the server was down for
	// a while between checkpoint and restart) gets an already-expired
	// deadline and is killed instantly with context.DeadlineExceeded.
	// t.StartTime itself is left untouched since it's spec-visible via
	// queued_tasks() and used for scheduling order
	// (engine/task_queue.go's readyTime()).
	budgetAnchor, secondsLimit := t.ExecutionBudget()
	if now := time.Now(); budgetAnchor.Before(now) {
		budgetAnchor = now
	}
	deadline := budgetAnchor.Add(time.Duration(secondsLimit * float64(time.Second)))
	t.SetExecutionDeadline(deadline)
	// The VM owns the seconds deadline so commit-gate waits can extend it.
	// Cancellation separately handles shutdown and explicit task kills.
	taskCtx, cancel := context.WithCancel(s.ctx)
	t.SetCancelFunc(cancel)
	defer cancel()

	var result types.Result
	anonGCFloor := s.store.NextID()
	// Sample the global anon-creation counter at the SAME point as anonGCFloor so
	// the two are consistent. If it is unchanged at task end, no anonymous object
	// was created since the floor and the orphan-anon GC sweep can be skipped.
	anonFloor := s.store.AnonCreationCount()

	if savedVM := t.BytecodeVMValue(); savedVM != nil {
		// Retrieve saved VM -- could be resuming after suspend or running a forked child
		var ok bool
		bcVM, ok = savedVM.(*vm.VM)
		if !ok {
			t.SetState(task.TaskKilled)
			return errors.New("invalid saved VM state")
		}
		// Attach task context (may have been updated since VM was created)
		bcVM.Context = ctx
		bcVM.Task = t
		if bcVM.IsYielded() {
			// If this task was read()-suspended, deliver the input line
			if !t.WakeValue.IsNone() {
				bcVM.SetResumeValue(t.WakeValue, t.WakeErrorAsValue)
				t.WakeValue = types.None // Consume — don't leak into future suspends
				t.WakeErrorAsValue = false
			}
			// Resume after suspend
			result = bcVM.Resume()
		} else {
			// First run for forked child task (VM was pre-configured by CreateForkedTask)
			result = bcVM.ExecuteLoop()
		}
	} else {
		// First run - execute the program compiled at the source boundary.
		prog := t.Program
		if prog == nil {
			t.SetState(task.TaskKilled)
			return errors.New("task has no compiled program")
		}

		// Update TaskContext for permissions and builtins
		if t.VerbName != "" {
			ctx.Player = t.Owner
			ctx.Programmer = t.Programmer
			ctx.IsWizard = s.isWizard(t.Programmer)
			ctx.ThisObj = t.This
			ctx.Verb = t.VerbName

			// Push initial activation frame for traceback support
			t.PushFrame(types.ActivationFrame{
				This:       t.This,
				ThisValue:  types.None, // explicit None: zero Value{} is int 0 post-de-box; ToList would render this as 0
				Player:     t.Owner,
				Programmer: t.Programmer,
				Caller:     t.Caller,
				Verb:       t.VerbName,
				VerbLoc:    t.VerbLoc,
				LineNumber: 1,
			})
		}

		// Create bytecode VM
		bcVM = vm.NewVM(s.store, s.session)
		bcVM.Context = ctx
		bcVM.Task = t
		bcVM.TickLimit = t.TicksLimit
		configureVMStackLimit(bcVM, s.session)

		if t.VerbName != "" {
			// Command verbs derive args from raw words; server-initiated hooks can
			// provide fully-typed arguments directly.
			argList := append([]types.Value(nil), t.VerbArgsValues...)
			if argList == nil {
				argList = make([]types.Value, len(t.Args))
				for i, arg := range t.Args {
					argList[i] = types.NewStr(arg)
				}
			}

			// Prepare frame first, then set ALL variables before execution
			frame := bcVM.PrepareVerbFrame(prog, t.This, t.Owner, t.Caller, t.VerbName, t.VerbLoc, argList)

			// Set verb debug flag from the actual verb permissions, and record the
			// verb's stored name spec (incl. wildcards) for printed tracebacks.
			if taskVerb, _, vErr := ctx.StoreTxn.FindVerb(t.This, t.VerbName); vErr == nil {
				frame.VerbDebug = taskVerb.Perms.Has(dbstore.VerbDebug)
				frame.StoredVerbNames = taskVerb.Names
			}

			// Set verb context variables
			vm.SetLocalByName(frame, prog, "this", types.NewObj(t.This))
			vm.SetLocalByName(frame, prog, "player", types.NewObj(t.Owner))
			vm.SetLocalByName(frame, prog, "caller", types.NewObj(t.Caller))
			vm.SetLocalByName(frame, prog, "verb", types.NewStr(t.VerbName))
			vm.SetLocalByName(frame, prog, "args", types.NewList(argList))

			// Set command-specific variables
			vm.SetLocalByName(frame, prog, "argstr", types.NewStr(t.Argstr))
			vm.SetLocalByName(frame, prog, "dobjstr", types.NewStr(t.Dobjstr))
			vm.SetLocalByName(frame, prog, "iobjstr", types.NewStr(t.Iobjstr))
			vm.SetLocalByName(frame, prog, "prepstr", types.NewStr(t.Prepstr))
			vm.SetLocalByName(frame, prog, "dobj", types.NewObj(t.Dobj))
			vm.SetLocalByName(frame, prog, "iobj", types.NewObj(t.Iobj))

			// Start execution
			result = bcVM.ExecuteLoop()
		} else {
			// Simple eval task (no verb context)
			result = bcVM.Run(prog)
		}
	}

	t.Result = result

	// Handle fork yields: create child tasks and resume parent
	result = s.drainForks(t, bcVM, result)
	t.Result = result

	if ctx.ConflictRetryRequested {
		// The attempt stopped itself at its first irreversible effect because its
		// reads were already stale (BeforeIrreversibleEffect above). Nothing
		// external has happened and nothing was committed, so re-run it from the
		// top. The gate the hook took is released first: the retry gambles
		// optimistically again and re-checks at its own boundary, so the gate is
		// only ever held from a passed boundary to that attempt's commit. Holding
		// it across the re-execution would deadlock any ordinary commit the task
		// body issues before reaching the boundary. The bounded escalation above
		// remains the backstop against a writer that keeps winning.
		ctx.ConflictRetryRequested = false
		if escalated {
			ctx.StoreTxn.ClearCommitGateExemption()
			s.store.EscalationUnlock()
			escalated = false
		}
		s.discardCreatedForks(t)
		builtins.DiscardPendingEffects(s.session.NewExecution(ctx, t))
		s.store.NoteCommitRetry()
		attempt++
		goto retryAttempt
	}

	committed := true
	committedWrites := false
	if ctx.StoreTxn.HasWrites() {
		if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
			// A conflict surfaces as E_INVARG (a read-set version moved) OR E_INVIND (a
			// read-set object was recycled/renumbered out from under us since the
			// snapshot). Both set validationFail, and only the read-set validators set
			// it, so gating on ValidationFailed() plus these two codes retries exactly
			// the conflict cases — never a genuine execution-time E_INVIND (that never
			// sets validationFail). Re-running reproduces the correct builtin-level
			// error deterministically. Without the E_INVIND arm, a create-under-P racing
			// a recycle-of-P would surface a raw E_INVIND no serial ordering produces.
			if (errCode == types.E_INVARG || errCode == types.E_INVIND) && ctx.StoreTxn.ValidationFailed() && !ctx.LiveStoreMutated && !ctx.IrreversibleSideEffect && retryState.canRetry && attempt < maxConflictRetryAttempts {
				s.discardCreatedForks(t)
				builtins.DiscardPendingEffects(s.session.NewExecution(ctx, t))
				s.store.NoteCommitRetry() // Phase A: count each actual conflict retry (observation-only)
				if os.Getenv("BARN_DEBUG_RETRY") != "" {
					slog.Warn("DEBUG-RETRY",
						slog.Int64("task_id", t.ID),
						slog.String("verb", t.VerbName),
						slog.Int64("this", int64(t.This)),
						slog.Int64("player", int64(t.Owner)),
						slog.Int("attempt", attempt),
						slog.String("error", types.NewErr(errCode).String()))
				}
				attempt++
				goto retryAttempt
			}
			if ctx.StoreTxn.ValidationFailed() {
				// A conflict this attempt cannot answer by re-running: the error
				// reaches the task as an uncatchable exception no serial execution
				// would produce, so leave a trail for whoever reads the traceback.
				slog.Warn("commit conflict not retryable; surfacing as task error",
					slog.Int64("task_id", t.ID),
					slog.Int64("this", int64(t.This)),
					slog.String("verb", t.VerbName),
					slog.String("error", types.NewErr(errCode).String()),
					slog.Bool("retryable_task", retryState.canRetry),
					slog.Bool("live_store_mutated", ctx.LiveStoreMutated),
					slog.Bool("irreversible_side_effect", ctx.IrreversibleSideEffect),
					slog.Int("attempt", attempt))
			}
			result = types.Err(errCode)
			t.Result = result
			committed = false
		} else {
			committedWrites = true
		}
	}
	if committed {
		t.CreatedForks = nil
		builtins.FlushPendingEffects(s.session.NewExecution(ctx, t))
	} else {
		s.discardCreatedForks(t)
		builtins.DiscardPendingEffects(s.session.NewExecution(ctx, t))
	}
	// Refresh the committed view for lifecycle cleanup. Suspended tasks take
	// their next execution snapshot when the scheduler resumes them.
	if committed && committedWrites {
		ctx.StoreTxn.Release()
		ctx.StoreTxn = s.store.BeginReadOnly(0)
	}
	// Every suspension yields the commit gate along with execution.
	releaseEscalation()

	// Check context deadline
	select {
	case <-taskCtx.Done():
		s.settleCompletedTaskFinalizations(ctx, bcVM, anonGCFloor, s.store.AnonCreationCount() != anonFloor)
		t.SetState(task.TaskKilled)
		t.SetBytecodeVM(nil)
		return taskCtx.Err()
	default:
	}

	// Handle suspend
	if result.Flow == types.FlowSuspend {
		// A suspend is a waif liveness boundary as well as an anonymous-object
		// boundary. Values overwritten before the yield are no longer live in the
		// saved VM, so queue them now; the flush-time scan of the registered VM
		// protects any waifs that remain reachable when execution resumes.
		s.deferPendingWaifs(ctx, bcVM.TakePendingWaifs(), nil)
		// Match Toast lifecycle semantics more closely: a scheduler yield/suspend
		// is a GC boundary for newly-created orphan anonymous objects. The sweep is
		// deferred to the next quiescent flush; the suspended task's VM is registered
		// below (SetBytecodeVM), so the flush-time root scan still sees its locals.
		// Fast path retained: if no anonymous object was created since this task's
		// floor, the candidate set (anon ids >= floor) is provably empty and there is
		// nothing to enqueue.
		if s.store.AnonCreationCount() != anonFloor {
			s.deferAnonGC(ctx, anonGCFloor, nil)
		}
		// A terminal commit failure makes HasWrites false without discarding the
		// private view, preventing completion cleanup from recommitting it.
		if ctx.StoreTxn.HasWrites() {
			if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
				t.SetState(task.TaskKilled)
				t.SetBytecodeVM(nil)
				s.discardCreatedForks(t)
				builtins.DiscardPendingEffects(s.session.NewExecution(ctx, t))
				releaseEscalation()
				return nil
			}
			// The commit published this slice's forks; the runtime owns them now.
			// Leaving them on the task would let a later conflict-retry discard forks
			// that are already durable (yin() suspends mid-verb, so a retry can follow).
			t.CreatedForks = nil
			builtins.FlushPendingEffects(s.session.NewExecution(ctx, t))
			ctx.StoreTxn.Release()
			ctx.StoreTxn = s.store.BeginReadOnly(0)
		}
		// Save VM state for later Resume() via the thread-safe setter, so a
		// concurrently running sibling scanning saved VMs for orphan GC never races
		// the write. The s.mu critical section additionally guards the suspend(0)-
		// style heap re-queue below; lock order is s.mu then the task lock taken
		// inside SetBytecodeVM, matching collectSiblingGCRefs's read path.
		s.mu.Lock()
		t.SetBytecodeVM(bcVM)
		if t.GetState() == task.TaskQueued {
			// A suspend(0) re-queue carries no wake delay, so WakeTime is unset.
			// Stamp it with the suspend moment so the task's ready time reflects
			// when it yielded — otherwise it sorts by its original StartTime and
			// unfairly preempts tasks (e.g. a just-forked task) that became ready
			// while it was running.
			s.scheduler.RequeueYield(t, time.Now())
		}
		s.mu.Unlock()
		// The task manager has already been notified via builtinSuspend
		// Just return without setting state to Completed
		return nil
	}

	// Handle completion
	if result.Flow == types.FlowException {
		t.SetState(task.TaskKilled)
		handled := false
		if t.IsForked && t.Result.Error == types.E_MAXREC && resultValueContains(t.Result.Val, "tick") {
			handled = s.callTaskTimeoutHook(t, "ticks", types.NewStr("Task ran out of ticks"))
		} else if t.IsForked && t.Result.Error == types.E_MAXREC && resultValueContains(t.Result.Val, "seconds limit exceeded") {
			handled = s.callTaskTimeoutHook(t, "seconds", types.NewStr("Task ran out of seconds"))
		}
		// Prefer the activation stack snapshotted at raise time (carried on the
		// result): the live call stack has already unwound, so it would report the
		// eval frame instead of the verb where the error occurred, and it carries
		// no source lines. The log and the player see the same stack.
		stack := result.CallStack
		if len(stack) == 0 {
			stack = t.GetCallStack()
		}

		// Toast gives #0:handle_uncaught_error the first opportunity to handle
		// every uncaught task exception. A truthy return or a suspended handler
		// suppresses the fallback traceback. The handler itself runs with database
		// traceback dispatch disabled, so an error there falls back to the original
		// task's traceback instead of recursively invoking the same hook.
		isUncaughtHandler := t.Context.ServerInitiated && t.This == 0 && t.VerbName == "handle_uncaught_error"
		if !handled && !isUncaughtHandler {
			// Count every uncaught task exception here, before #0:handle_uncaught_error
			// gets its chance: a handled error is still an uncaught one, and on the real
			// Mongoose workload each one is a global write to $wiz_utils.traceback_log.
			metrics.UncaughtExceptions.Add(1)
			if os.Getenv("BARN_DEBUG_RETRY") != "" {
				top := ""
				if len(stack) > 0 {
					f := stack[len(stack)-1]
					top = fmt.Sprintf("#%d:%s line %d", f.VerbLoc, f.Verb, f.LineNumber)
				}
				caller := ""
				if len(stack) > 1 {
					f := stack[len(stack)-2]
					caller = fmt.Sprintf("#%d:%s line %d (this=%d)", f.VerbLoc, f.Verb, f.LineNumber, f.This)
				}
				msg := result.Error.Message()
				if result.Val.Type() == types.TYPE_LIST && result.Val.Len() >= 3 {
					if m := result.Val.Get(2); m.Type() == types.TYPE_STR {
						msg = m.Str()
					}
				}
				full := ""
				if result.Error == types.E_PROPNF {
					full = strings.Join(task.FormatTraceback(stack, result.Error), " || ")
				}
				slog.Warn("DEBUG-UNCAUGHT",
					slog.String("msg", msg),
					slog.String("full", full),
					slog.String("caller_frame", caller),
					slog.String("error", types.NewErr(result.Error).String()),
					slog.String("task_verb", t.VerbName),
					slog.String("top_frame", top),
					slog.Int("frames", len(stack)))
			}
			stackValues := make([]types.Value, 0, len(stack))
			for i := len(stack) - 1; i >= 0; i-- {
				frame := stack[i].ToList()
				if s.session.IncludeRTVars(t.Context) {
					frame = types.NewList(append(frame.Elements(), stack[i].RuntimeVariableMap()))
				}
				stackValues = append(stackValues, frame)
			}
			formattedLines := task.FormatTraceback(stack, result.Error)
			formattedValues := make([]types.Value, 0, len(formattedLines))
			for _, line := range formattedLines {
				formattedValues = append(formattedValues, types.NewStr(line))
			}
			handlerMessage := types.NewStr(result.Error.Message())
			handlerValue := types.NewInt(0)
			if result.Val.Type() == types.TYPE_LIST && result.Val.Len() >= 3 {
				if message := result.Val.Get(2); message.Type() == types.TYPE_STR {
					handlerMessage = message
				}
				handlerValue = result.Val.Get(3)
			}
			handlerResult, handlerErr := s.RunServerVerbTask(0, "handle_uncaught_error", []types.Value{
				types.NewErr(result.Error),
				handlerMessage,
				handlerValue,
				types.NewList(stackValues),
				types.NewList(formattedValues),
			}, t.Owner)
			if handlerErr == nil {
				handled = handlerResult.Flow == types.FlowSuspend || handlerResult.Val.Truthy()
			}
		}

		if !handled && !isUncaughtHandler {
			// Preserve the existing structured task log for foreground failures.
			// Toast does not write forked-task tracebacks directly to stderr.
			if !t.IsForked {
				s.logTraceback(t, result.Error, stack)
			}
			// When a database's eval verb catches the error itself — e.g. Test.db
			// wraps results as {status, result} — the task completes normally and
			// never reaches this branch. Tick exhaustion keeps its friendlier line.
			if t.VerbName == "eval" && t.Result.Error == types.E_MAXREC && resultValueContains(t.Result.Val, "tick") {
				s.sendTaskLine(t.Owner, "Task ran out of ticks")
			} else {
				s.SendTracebackToPlayer(t.Owner, result.Error, stack)
			}
		}
		// Clean up call stack after traceback has been sent
		t.ClearCallStack()
	} else {
		t.SetState(task.TaskCompleted)
	}

	// Match Toast lifecycle semantics at shutdown: transfer completed-task roots
	// only after an explicit shutdown request. Generic cancellation still runs
	// ordinary finalization rather than fabricating pending checkpoint roots.
	if !s.settleCompletedTaskFinalizations(ctx, bcVM, anonGCFloor, s.store.AnonCreationCount() != anonFloor) {
		// A per-task waif/anon sweep is prohibitive on large databases, so both are
		// deferred and settled by flushDeferredGC (which self-throttles once sweeps
		// get expensive, and stays prompt while they are cheap). The cheap guards
		// still apply: with no anonymous object created since the floor and no
		// pending waifs there is provably nothing to collect, so nothing is enqueued.
		//
		// This task's VM is released below, so its references are snapshotted now,
		// on the goroutine that owns it, rather than walked at flush time.
		if ctx.StoreTxn.HasWrites() {
			if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
				t.SetState(task.TaskKilled)
				s.discardCreatedForks(t)
				builtins.DiscardPendingEffects(s.session.NewExecution(ctx, t))
			} else {
				builtins.FlushPendingEffects(s.session.NewExecution(ctx, t))
			}
		}

	}

	t.SetBytecodeVM(nil) // Release VM after completion

	// Fire the terminal-completion callback (if any) exactly once. This branch
	// is only reached on terminal completion — a suspend returns earlier (the
	// FlowSuspend block above), so OnComplete never fires on a read() yield.
	if cb := t.TakeOnComplete(); cb != nil {
		cb(result)
	}
	return nil
}

type taskRetryState struct {
	canRetry bool
	// rebuildVM is set for a forked task's first run: it rebuilds the
	// pre-configured child VM so the attempt can start over from the fork
	// statement. Nil for a fresh task, whose retry recompiles nothing and simply
	// re-runs t.Program.
	rebuildVM    func() *vm.VM
	context      *kernel.TaskContext
	callStack    []types.ActivationFrame
	taskLocal    types.Value
	wakeValue    types.Value
	ticksLimit   int64
	secondsLimit float64
}

// taskIsConflictRetryable reports whether a task may be re-run from the top after
// an MVCC commit conflict. Only fresh AST tasks qualify: a resumed/forked task has
// a saved bytecode VM (or fork bookkeeping) whose mid-flight state cannot be
// reconstructed from the original statements. Conflict retry re-executes the whole
// body, so it is also the predicate for whether a task is safe to co-schedule
// optimistically with other tasks: if two such tasks happen to conflict at commit,
// the loser simply retries against the winner's committed writes.
//
// The runtime's own retry decision is wider than this scheduling predicate: a
// forked task that has not yet run is also re-executable (forkFirstRunRebuilder),
// but it keeps its solo batch here so co-scheduling behaviour is unchanged.
func taskIsConflictRetryable(t *task.Task) bool {
	return t != nil && t.BytecodeVMValue() == nil && !t.IsForked && t.ForkInfo == nil && t.Program != nil
}

// forkFirstRunRebuilder returns a constructor for the pre-configured VM of a
// forked task that has not yet run, or nil when t is not such a task. A forked
// first run is as re-executable as a fresh task: its VM is a pure function of
// t.ForkInfo, and nothing it does before a lost commit is published. Only a
// slice whose VM has yielded (a resumed task) carries mid-flight state that
// cannot be rebuilt. Without this, every `fork (0) ... endfork` body that lost a
// commit surfaced a frameless E_INVARG to MOO code (issue #296).
func (s *Runtime) forkFirstRunRebuilder(t *task.Task, ticks int64) func() *vm.VM {
	if t == nil || !t.IsForked || t.ForkInfo == nil {
		return nil
	}
	saved, ok := t.BytecodeVMValue().(*vm.VM)
	if !ok || saved == nil || saved.IsYielded() {
		return nil
	}
	forkProg := forkBodyProgram(t.ForkInfo)
	if forkProg == nil {
		return nil
	}
	forkInfo := t.ForkInfo
	taskID := t.ID
	return func() *vm.VM {
		child := s.newForkVM(taskID, forkInfo, forkProg, ticks)
		child.Task = t
		return child
	}
}

func (s *Runtime) captureTaskRetryState(t *task.Task) taskRetryState {
	if t == nil {
		return taskRetryState{}
	}
	ctx, stack, local, wake, ticks, seconds := t.RetryStateSnapshot()
	state := taskRetryState{
		canRetry:     taskIsConflictRetryable(t),
		context:      cloneTaskContextForRetry(ctx),
		callStack:    cloneActivationFramesForRetry(stack),
		taskLocal:    local,
		wakeValue:    wake,
		ticksLimit:   ticks,
		secondsLimit: seconds,
	}
	if rebuild := s.forkFirstRunRebuilder(t, ticks); rebuild != nil {
		state.canRetry = true
		state.rebuildVM = rebuild
	}
	return state
}

// runDeferredCheckpoint performs a dump_database() that a gate-holding attempt
// had to postpone (see builtinDumpDatabase). It runs only after the gate is
// released, so the checkpoint can take the gate itself and run its hook tasks.
func (s *Runtime) runDeferredCheckpoint(ctx *kernel.TaskContext) {
	if ctx == nil || !ctx.DeferredCheckpoint {
		return
	}
	ctx.DeferredCheckpoint = false
	dump := s.session.Host().Checkpoint
	if dump == nil {
		return
	}
	if err := dump(); err != nil {
		slog.Error("deferred dump_database() failed",
			slog.Int64("task_id", ctx.TaskID),
			slog.Any("err", err))
	}
}

func (state taskRetryState) restore(t *task.Task) {
	if t == nil || !state.canRetry {
		return
	}
	if state.rebuildVM != nil {
		t.SetBytecodeVM(state.rebuildVM())
	} else {
		t.SetBytecodeVM(nil)
	}
	ctx := cloneTaskContextForRetry(state.context)
	if ctx != nil {
		ctx.TaskID = t.ID
	}
	t.RestoreRetryState(ctx, cloneActivationFramesForRetry(state.callStack), state.taskLocal, state.wakeValue, state.ticksLimit, state.secondsLimit)
}

func cloneTaskContextForRetry(ctx *kernel.TaskContext) *kernel.TaskContext {
	if ctx == nil {
		return nil
	}
	clone := *ctx
	clone.StoreTxn = clone.Store.DirectTxn()
	clone.PendingEffects = nil
	return &clone
}

func cloneActivationFramesForRetry(frames []types.ActivationFrame) []types.ActivationFrame {
	if len(frames) == 0 {
		return nil
	}
	cloned := make([]types.ActivationFrame, len(frames))
	for i, frame := range frames {
		cloned[i] = frame
		cloned[i].Args = append([]types.Value(nil), frame.Args...)
	}
	return cloned
}

func (s *Runtime) callTaskTimeoutHook(t *task.Task, resource string, message types.Value) bool {
	stack := t.GetCallStack()
	stackValues := make([]types.Value, 0, len(stack))
	for _, frame := range stack {
		stackValues = append(stackValues, frame.ToList())
	}
	traceLines := task.FormatTraceback(stack, t.Result.Error)
	traceValues := make([]types.Value, 0, len(traceLines))
	for i, line := range traceLines {
		if i == 0 {
			line = "Task ran out of " + resource
		}
		traceValues = append(traceValues, types.NewStr(line))
	}
	if len(traceValues) == 0 {
		traceValues = append(traceValues, message)
	}
	result := s.CallVerb(0, "handle_task_timeout", []types.Value{
		types.NewStr(resource),
		types.NewList(stackValues),
		types.NewList(traceValues),
	}, t.Owner)
	return result.Flow == types.FlowSuspend || (result.Flow != types.FlowException && result.Val.Truthy())
}

func resultValueContains(value types.Value, text string) bool {
	if value.IsNone() {
		return false
	}
	return strings.Contains(strings.ToLower(value.String()), strings.ToLower(text))
}

func (s *Runtime) sendTaskLine(player types.ObjID, line string) {
	if s.taskLineSender != nil {
		s.taskLineSender(player, line)
	}
}

func (s *Runtime) discardCreatedForks(parent *task.Task) {
	if parent == nil || len(parent.CreatedForks) == 0 {
		return
	}
	created := append([]int64(nil), parent.CreatedForks...)
	parent.CreatedForks = nil

	for _, id := range created {
		if child := s.taskManager.GetTask(id); child != nil {
			child.Kill()
			s.taskManager.RemoveTaskIf(id, child)
		}
	}
}

// drainForks handles FlowFork yields from the VM by creating child tasks
// and resuming the parent until no more forks are pending.
func (s *Runtime) drainForks(t *task.Task, bcVM *vm.VM, result types.Result) types.Result {
	for result.Flow == types.FlowFork {
		var childID int64
		if result.ForkInfo != nil {
			childID = s.CreateForkedTask(t, result.ForkInfo)
		}
		bcVM.SetForkResult(childID)
		result = bcVM.Resume()
	}
	return result
}

// ExecuteVerbTaskSync creates and immediately runs a command verb task on the runtime goroutine.
func (s *Runtime) ExecuteVerbTaskSync(player types.ObjID, match *command.VerbMatch, cmd *command.ParsedCommand, outputSuffix string) error {
	return s.ExecuteVerbTaskSyncWithStart(player, match, cmd, outputSuffix, nil)
}

// ExecuteVerbTaskSyncWithStart is ExecuteVerbTaskSync with a hook invoked after
// the task is registered and before any of its code runs.
func (s *Runtime) ExecuteVerbTaskSyncWithStart(player types.ObjID, match *command.VerbMatch, cmd *command.ParsedCommand, outputSuffix string, onStart func(int64)) error {
	program, diagnostics := s.registry.Compiler().CompileMOOWithKey(match.Verb.Code, match.Verb.CodeKey)
	if len(diagnostics) > 0 {
		return fmt.Errorf("verb compile error: %s", diagnostics[0].Error())
	}
	if len(match.Verb.Code) == 0 {
		return ErrCommandVerbNoCode
	}

	taskID := s.newTaskID()
	ticks, seconds := foregroundTaskLimits(s.session)
	t := task.NewTaskFull(taskID, player, program, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)
	t.StartTime = time.Now()
	t.Programmer = match.Verb.Owner
	t.Context.Programmer = match.Verb.Owner
	t.Context.IsWizard = s.isWizard(match.Verb.Owner)

	t.VerbName = cmd.Verb
	t.VerbLoc = match.VerbLoc
	t.This = match.This
	t.Caller = player
	t.Argstr = cmd.Argstr
	t.Args = cmd.Args
	t.Dobjstr = cmd.Dobjstr
	t.Dobj = cmd.Dobj
	t.Prepstr = cmd.Prepstr
	t.Iobjstr = cmd.Iobjstr
	t.Iobj = cmd.Iobj
	t.CommandOutputSuffix = outputSuffix
	t.FromCommand = true
	t.ForkCreator = s

	// Register task
	t.SetState(task.TaskQueued)
	s.taskManager.RegisterTask(t)
	if onStart != nil {
		onStart(t.ID)
	}

	// Run synchronously on the runtime goroutine.
	err := s.runTask(t)
	if err != nil {
		slog.Error("task error",
			slog.Int64("task_id", t.ID),
			slog.Int64("this", int64(t.This)),
			slog.String("verb", t.VerbName),
			slog.Any("err", err))
	}

	// Flush output buffer for the player
	if s.taskOutputFlusher != nil {
		s.taskOutputFlusher(t.Owner, t.CommandOutputSuffix)
	}
	return err
}
