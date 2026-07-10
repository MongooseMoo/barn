package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"barn/builtins"
	"barn/bytecode"
	"barn/command"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/parser"
	"barn/task"
	"barn/types"
	"barn/vm"
)

// maxConflictRetryAttempts bounds how many times a fresh AST task re-runs after an
// MVCC commit conflict. Conflicts only arise between tasks committing inside the same
// optimistic batch, and batches never exceed workerCount tasks, so retrying more than
// the worker count is enough to guarantee the loser eventually commits against every
// peer's writes. The bound exists only to prevent livelock under a pathological store.
const maxConflictRetryAttempts = 64

var ErrCommandVerbNoCode = errors.New("command verb has no code")
// runTask executes a task's code using the bytecode VM
func (s *Scheduler) runTask(t *task.Task) (retErr error) {
	// Recover from panics to avoid crashing the server
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in runTask(%d): %v", t.ID, r)
			t.SetState(task.TaskKilled)
			retErr = fmt.Errorf("internal panic: %v", r)
		}
	}()

	retryState := captureTaskRetryState(t)
	attempt := 0

retryAttempt:
	if attempt > 0 {
		retryState.restore(t)
	}
	// Publish the Running transition under s.mu, the lock collectSiblingGCRefs holds
	// while it reads sibling tasks' VMs. This guarantees a concurrent GC walk either
	// sees this task as Running (and skips its VM) or blocks here before this goroutine
	// touches the VM below — closing the popped-but-not-yet-running race window.
	s.mu.Lock()
	t.SetState(task.TaskRunning)
	s.mu.Unlock()

	ctx := t.Context
	if ctx == nil {
		t.SetState(task.TaskKilled)
		return errors.New("task has no context")
	}

	// Attach task to context so builtins can access task_local
	ctx.Task = t
	ctx.TaskID = t.ID
	ctx.Store = s.store
	// Release any txn left on this context from a previous attempt/run before
	// beginning a fresh one, so its readTS deregisters from the history-GC floor
	// promptly (the runtime finalizer is only a backstop).
	if old := ctx.StoreTxn; old != nil {
		old.Release()
	}
	ctx.StoreTxn = s.store.BeginReadOnly(0)
	ctx.LiveStoreMutated = false
	ctx.Registry = s.registry
	ctx.RuntimeOptions = s.options

	// A task resuming after suspend runs under background limits: Toast treats
	// resumed tasks as background tasks, and time spent suspended does not count
	// against the execution budget. Reset both the tick and second budgets (and
	// the start time used for the deadline below) so ticks_left()/seconds_left()
	// and the hard deadline reflect a fresh background slice.
	if savedVM, ok := t.BytecodeVMValue().(*vm.VM); ok && savedVM.IsYielded() {
		bgTicks, bgSeconds := backgroundTaskLimits()
		t.TicksLimit = bgTicks
		t.TicksUsed = 0
		t.SecondsLimit = bgSeconds
		t.SecondsUsed = 0
		t.StartTime = time.Now()
		savedVM.TickLimit = bgTicks
		savedVM.Ticks = 0
	}

	// Set up cancellation with deadline. The budget deadline must be anchored
	// to when the task actually starts running, not its (possibly long-past)
	// scheduled start time — otherwise a fork-delayed or checkpoint-restored
	// task whose StartTime has already elapsed (e.g. the server was down for
	// a while between checkpoint and restart) gets an already-expired
	// deadline and is killed instantly with context.DeadlineExceeded.
	// t.StartTime itself is left untouched since it's spec-visible via
	// queued_tasks() and used for scheduling order
	// (scheduler/task_queue.go's readyTime()).
	budgetAnchor := t.StartTime
	if now := time.Now(); budgetAnchor.Before(now) {
		budgetAnchor = now
	}
	deadline := budgetAnchor.Add(time.Duration(t.SecondsLimit * float64(time.Second)))
	taskCtx, cancel := context.WithDeadline(s.ctx, deadline)
	t.CancelFunc = cancel
	defer cancel()

	var result types.Result
	var bcVM *vm.VM
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
		if bcVM.IsYielded() {
			// If this task was read()-suspended, deliver the input line
			if !t.WakeValue.IsNone() {
				bcVM.SetResumeValue(t.WakeValue)
				t.WakeValue = types.None // Consume — don't leak into future suspends
			}
			// Resume after suspend
			result = bcVM.Resume()
		} else {
			// First run for forked child task (VM was pre-configured by CreateForkedTask)
			result = bcVM.ExecuteLoop()
		}
	} else {
		// First run - compile and execute
		code, ok := t.Code.([]parser.Stmt)
		if !ok || code == nil {
			t.SetState(task.TaskKilled)
			return errors.New("task has no code")
		}

		// Compile AST to bytecode
		compiler := bytecode.NewCompilerWithRegistry(s.registry)
		prog, compileErr := compiler.CompileStatements(code)
		if compileErr != nil {
			t.SetState(task.TaskKilled)
			return fmt.Errorf("compile error: %w", compileErr)
		}
		if t.VerbName != "" {
			if taskVerb, _, vErr := s.store.FindVerb(t.This, t.VerbName); vErr == nil && len(taskVerb.Code) > 0 {
				prog.Source = append([]string(nil), taskVerb.Code...)
			}
		}

		// Update TaskContext for permissions and builtins
		if t.VerbName != "" {
			ctx.Player = t.Owner
			ctx.Programmer = t.Programmer
			ctx.IsWizard = s.isWizard(t.Programmer)
			ctx.ThisObj = t.This
			ctx.Verb = t.VerbName

			// Push initial activation frame for traceback support
			t.PushFrame(task.ActivationFrame{
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
		bcVM = vm.NewVM(s.store, s.registry)
		bcVM.Context = ctx
		bcVM.TickLimit = t.TicksLimit
		configureVMStackLimit(bcVM)

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
			if taskVerb, _, vErr := s.store.FindVerb(t.This, t.VerbName); vErr == nil {
				frame.VerbDebug = taskVerb.Perms.Has(dbstore.VerbDebug)
				frame.StoredVerb = strings.Join(taskVerb.Names, " ")
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

	committed := true
	committedWrites := false
	if ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
		if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
			if errCode == types.E_INVARG && ctx.StoreTxn.ValidationFailed() && !ctx.LiveStoreMutated && retryState.canRetry && attempt < maxConflictRetryAttempts {
				s.discardCreatedForks(t)
				builtins.DiscardPendingNotifications(ctx)
				builtins.DiscardPendingConnectionSwitches(ctx)
				builtins.DiscardPendingBootPlayers(ctx)
				builtins.DiscardPendingServerOptions(ctx)
				s.store.NoteCommitRetry() // Phase A: count each actual conflict retry (observation-only)
				attempt++
				goto retryAttempt
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
		if errCode := builtins.FlushPendingServerOptions(ctx); errCode != types.E_NONE {
			result = types.Err(errCode)
			t.Result = result
		}
		if errCode := builtins.FlushPendingConnectionSwitches(ctx); errCode != types.E_NONE {
			result = types.Err(errCode)
			t.Result = result
		}
		if errCode := builtins.FlushPendingNotifications(ctx); errCode != types.E_NONE {
			result = types.Err(errCode)
			t.Result = result
		}
		if errCode := builtins.FlushPendingBootPlayers(ctx); errCode != types.E_NONE {
			result = types.Err(errCode)
			t.Result = result
		}
	} else {
		s.discardCreatedForks(t)
		builtins.DiscardPendingNotifications(ctx)
		builtins.DiscardPendingConnectionSwitches(ctx)
		builtins.DiscardPendingBootPlayers(ctx)
		builtins.DiscardPendingServerOptions(ctx)
	}
	if committed && committedWrites && ctx.StoreTxn != nil {
		ctx.StoreTxn.Release()
		ctx.StoreTxn = s.store.BeginReadOnly(0)
	}

	// Check context deadline
	select {
	case <-taskCtx.Done():
		if taskCtx.Err() == context.Canceled && bcVM != nil && s.pendingFinalizationSink != nil {
			if pending := vm.CollectPendingFinalizationValues(s.store, bcVM); len(pending) > 0 {
				s.pendingFinalizationSink(pending)
			}
		}
		t.SetState(task.TaskKilled)
		t.SetBytecodeVM(nil)
		return taskCtx.Err()
	default:
	}

	for zeroDelayYields := 0; result.Flow == types.FlowSuspend && t.IsForked && t.GetState() == task.TaskQueued && zeroDelayYields < 16; zeroDelayYields++ {
		t.SetBytecodeVM(bcVM)
		if !t.WakeValue.IsNone() {
			bcVM.SetResumeValue(t.WakeValue)
			t.WakeValue = types.None
		}
		result = bcVM.Resume()
		t.Result = result
	}
	if result.Flow != types.FlowSuspend && ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
		if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
			result = types.Err(errCode)
			t.Result = result
			s.discardCreatedForks(t)
			builtins.DiscardPendingNotifications(ctx)
			builtins.DiscardPendingConnectionSwitches(ctx)
			builtins.DiscardPendingBootPlayers(ctx)
			builtins.DiscardPendingServerOptions(ctx)
		} else {
			t.CreatedForks = nil
			if errCode := builtins.FlushPendingServerOptions(ctx); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
			}
			if errCode := builtins.FlushPendingConnectionSwitches(ctx); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
			}
			if errCode := builtins.FlushPendingNotifications(ctx); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
			}
			if errCode := builtins.FlushPendingBootPlayers(ctx); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
			}
			ctx.StoreTxn.Release()
			ctx.StoreTxn = s.store.BeginReadOnly(0)
		}
	}

	// Handle suspend
	if result.Flow == types.FlowSuspend {
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
		if ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
			if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
				t.SetState(task.TaskKilled)
				t.SetBytecodeVM(nil)
				s.discardCreatedForks(t)
				builtins.DiscardPendingNotifications(ctx)
				builtins.DiscardPendingConnectionSwitches(ctx)
				builtins.DiscardPendingBootPlayers(ctx)
				builtins.DiscardPendingServerOptions(ctx)
				return nil
			}
			// The commit published this slice's forks; they are the scheduler's now.
			// Leaving them on the task would let a later conflict-retry discard forks
			// that are already durable (yin() suspends mid-verb, so a retry can follow).
			t.CreatedForks = nil
			if errCode := builtins.FlushPendingServerOptions(ctx); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
			}
			if errCode := builtins.FlushPendingConnectionSwitches(ctx); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
			}
			if errCode := builtins.FlushPendingNotifications(ctx); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
			}
			if errCode := builtins.FlushPendingBootPlayers(ctx); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
			}
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
			if t.WakeTime.IsZero() {
				t.WakeTime = time.Now()
			}
			s.queueSeq++
			t.QueueSeq = s.queueSeq
			heap.Push(s.waiting, t)
		}
		s.mu.Unlock()
		// The task manager has already been notified via builtinSuspend
		// Just return without setting state to Completed
		return nil
	}

	// Handle completion
	if result.Flow == types.FlowException {
		t.SetState(task.TaskKilled)
		if t.IsForked && t.Result.Error == types.E_MAXREC && resultValueContains(t.Result.Val, "tick") {
			s.callTaskTimeoutHook(t, "ticks", types.NewStr("Task ran out of ticks"))
		}
		// Log traceback to server log (skip for forked tasks to match Toast behavior:
		// Toast does not log forked-task tracebacks to stderr)
		if !t.IsForked {
			s.logTraceback(t, result.Error)
		}
		// An uncaught error aborts the task; report it to the player the way
		// Toast does. (When a database's eval verb catches the error itself —
		// e.g. Test.db wraps results as {status, result} — the task completes
		// normally and never reaches this branch.) Tick exhaustion gets the
		// friendlier one-line message instead of a full traceback.
		if t.VerbName == "eval" && t.Result.Error == types.E_MAXREC && resultValueContains(t.Result.Val, "tick") {
			s.sendTaskLine(t.Owner, "Task ran out of ticks")
		} else if !t.IsForked {
			// Prefer the activation stack snapshotted at raise time (carried on
			// the result): the live call stack has already unwound, so it would
			// report the eval frame instead of the verb where the error occurred.
			if stack, ok := result.CallStack.([]task.ActivationFrame); ok && len(stack) > 0 {
				s.SendTracebackToPlayer(t.Owner, result.Error, stack)
			} else {
				s.sendTraceback(t, result.Error)
			}
		}
		// Clean up call stack after traceback has been sent
		for len(t.CallStack) > 0 {
			t.PopFrame()
		}
	} else {
		t.SetState(task.TaskCompleted)
	}

	// Match Toast lifecycle semantics at shutdown: preserve composite local
	// values that still carry anonymous references so the final checkpoint can
	// serialize them as pending finalization values. Outside shutdown, completed
	// tasks still trigger orphan-anonymous collection.
	if s.ctx.Err() != nil {
		if s.pendingFinalizationSink != nil && bcVM != nil {
			if pending := vm.CollectPendingFinalizationValues(s.store, bcVM); len(pending) > 0 {
				s.pendingFinalizationSink(pending)
			}
		}
	} else {
		// A per-task waif/anon sweep is prohibitive on large databases, so both are
		// deferred and settled by flushDeferredGC (which self-throttles once sweeps
		// get expensive, and stays prompt while they are cheap). The cheap guards
		// still apply: with no anonymous object created since the floor and no
		// pending waifs there is provably nothing to collect, so nothing is enqueued.
		//
		// This task's VM is released below, so its references are snapshotted now,
		// on the goroutine that owns it, rather than walked at flush time.
		if bcVM != nil {
			s.deferPendingWaifs(ctx, bcVM.TakePendingWaifs(), bcVM)
		}
		if s.store.AnonCreationCount() != anonFloor {
			s.deferAnonGC(ctx, anonGCFloor, bcVM)
		}
		if ctx.StoreTxn != nil && ctx.StoreTxn.HasWrites() {
			if errCode := ctx.StoreTxn.Commit(); errCode != types.E_NONE {
				result = types.Err(errCode)
				t.Result = result
				t.SetState(task.TaskKilled)
				s.discardCreatedForks(t)
				builtins.DiscardPendingNotifications(ctx)
				builtins.DiscardPendingConnectionSwitches(ctx)
				builtins.DiscardPendingBootPlayers(ctx)
				builtins.DiscardPendingServerOptions(ctx)
			} else {
				if errCode := builtins.FlushPendingServerOptions(ctx); errCode != types.E_NONE {
					result = types.Err(errCode)
					t.Result = result
				}
				if errCode := builtins.FlushPendingConnectionSwitches(ctx); errCode != types.E_NONE {
					result = types.Err(errCode)
					t.Result = result
				}
				if errCode := builtins.FlushPendingNotifications(ctx); errCode != types.E_NONE {
					result = types.Err(errCode)
					t.Result = result
				}
				if errCode := builtins.FlushPendingBootPlayers(ctx); errCode != types.E_NONE {
					result = types.Err(errCode)
					t.Result = result
				}
			}
		}

		// Settle the batches now so an orphan's :recycle stays observable by the very
		// next command, as it was when collection ran inline. If a sibling is still
		// running, the flush declines and the end-of-pass flush picks these up.
		s.flushDeferredGC()
	}

	t.SetBytecodeVM(nil) // Release VM after completion

	// Fire the terminal-completion callback (if any) exactly once. This branch
	// is only reached on terminal completion — a suspend returns earlier (the
	// FlowSuspend block above), so OnComplete never fires on a read() yield.
	if t.OnComplete != nil {
		cb := t.OnComplete
		t.OnComplete = nil
		cb(result)
	}
	return nil
}

type taskRetryState struct {
	canRetry     bool
	context      *kernel.TaskContext
	callStack    []task.ActivationFrame
	taskLocal    types.Value
	wakeValue    types.Value
	ticksLimit   int64
	secondsLimit float64
	code         interface{}
}

// taskIsConflictRetryable reports whether a task may be re-run from the top after
// an MVCC commit conflict. Only fresh AST tasks qualify: a resumed/forked task has
// a saved bytecode VM (or fork bookkeeping) whose mid-flight state cannot be
// reconstructed from the original statements. Conflict retry re-executes the whole
// body, so it is also the predicate for whether a task is safe to co-schedule
// optimistically with other tasks: if two such tasks happen to conflict at commit,
// the loser simply retries against the winner's committed writes.
func taskIsConflictRetryable(t *task.Task) bool {
	return t != nil && t.BytecodeVMValue() == nil && !t.IsForked && t.ForkInfo == nil && t.Code != nil
}

func captureTaskRetryState(t *task.Task) taskRetryState {
	if t == nil {
		return taskRetryState{}
	}
	state := taskRetryState{
		canRetry:     taskIsConflictRetryable(t),
		context:      cloneTaskContextForRetry(t.Context),
		callStack:    cloneActivationFramesForRetry(t.CallStack),
		taskLocal:    t.TaskLocal,
		wakeValue:    t.WakeValue,
		ticksLimit:   t.TicksLimit,
		secondsLimit: t.SecondsLimit,
		code:         t.Code,
	}
	return state
}

func (state taskRetryState) restore(t *task.Task) {
	if t == nil || !state.canRetry {
		return
	}
	t.Code = state.code
	t.SetBytecodeVM(nil)
	t.Result = types.Result{}
	t.CallStack = cloneActivationFramesForRetry(state.callStack)
	t.TaskLocal = state.taskLocal
	t.WakeValue = state.wakeValue
	t.CreatedForks = nil
	t.TicksLimit = state.ticksLimit
	t.TicksUsed = 0
	t.SecondsLimit = state.secondsLimit
	t.SecondsUsed = 0
	t.StartTime = time.Now()
	t.Context = cloneTaskContextForRetry(state.context)
	if t.Context != nil {
		t.Context.Task = t
		t.Context.TaskID = t.ID
	}
}

func cloneTaskContextForRetry(ctx *kernel.TaskContext) *kernel.TaskContext {
	if ctx == nil {
		return nil
	}
	clone := *ctx
	clone.StoreTxn = nil
	clone.PendingNotifications = nil
	clone.PendingConnectionSwitches = nil
	clone.PendingBootPlayers = nil
	clone.PendingServerOptions = nil
	clone.CallerVM = nil
	return &clone
}

func cloneActivationFramesForRetry(frames []task.ActivationFrame) []task.ActivationFrame {
	if len(frames) == 0 {
		return nil
	}
	cloned := make([]task.ActivationFrame, len(frames))
	for i, frame := range frames {
		cloned[i] = frame
		cloned[i].Args = append([]types.Value(nil), frame.Args...)
	}
	return cloned
}

func (s *Scheduler) callTaskTimeoutHook(t *task.Task, resource string, message types.Value) {
	stack := t.GetCallStack()
	stackValues := make([]types.Value, 0, len(stack))
	for _, frame := range stack {
		stackValues = append(stackValues, frame.ToList())
	}
	traceLines := task.FormatTraceback(stack, t.Result.Error)
	traceValues := make([]types.Value, 0, len(traceLines))
	for i, line := range traceLines {
		if i == 0 {
			line = "Task ran out of ticks"
		}
		traceValues = append(traceValues, types.NewStr(line))
	}
	if len(traceValues) == 0 {
		traceValues = append(traceValues, message)
	}
	_ = s.CallVerb(0, "handle_task_timeout", []types.Value{
		types.NewStr(resource),
		types.NewList(stackValues),
		types.NewList(traceValues),
	}, t.Owner)
}

func resultValueContains(value types.Value, text string) bool {
	if value.IsNone() {
		return false
	}
	return strings.Contains(strings.ToLower(value.String()), strings.ToLower(text))
}

func (s *Scheduler) sendTaskLine(player types.ObjID, line string) {
	if s.taskLineSender != nil {
		s.taskLineSender(player, line)
	}
}

func (s *Scheduler) discardCreatedForks(parent *task.Task) {
	if parent == nil || len(parent.CreatedForks) == 0 {
		return
	}
	created := append([]int64(nil), parent.CreatedForks...)
	parent.CreatedForks = nil

	s.mu.Lock()
	for _, id := range created {
		if child := s.tasks[id]; child != nil {
			child.Kill()
			delete(s.tasks, id)
		}
	}
	s.mu.Unlock()

	mgr := task.GetManager()
	for _, id := range created {
		if child := mgr.GetTask(id); child != nil {
			child.Kill()
		}
		mgr.RemoveTask(id)
	}
}

// drainForks handles FlowFork yields from the VM by creating child tasks
// and resuming the parent until no more forks are pending.
func (s *Scheduler) drainForks(t *task.Task, bcVM *vm.VM, result types.Result) types.Result {
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

// ExecuteVerbTaskSync creates and immediately runs a command verb task on the scheduler goroutine.
func (s *Scheduler) ExecuteVerbTaskSync(player types.ObjID, match *command.VerbMatch, cmd *command.ParsedCommand, outputSuffix string) error {
	program, compileErrors := bytecode.CompileVerb(match.Verb.Code)
	if len(compileErrors) > 0 {
		return fmt.Errorf("Verb compile error: %s", compileErrors[0])
	}
	if len(program.Statements) == 0 {
		return ErrCommandVerbNoCode
	}

	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	ticks, seconds := foregroundTaskLimits()
	t := task.NewTaskFull(taskID, player, program.Statements, ticks, seconds)
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
	s.mu.Lock()
	s.tasks[t.ID] = t
	s.mu.Unlock()
	task.GetManager().RegisterTask(t)

	// Run synchronously on the scheduler goroutine
	err := s.runTask(t)
	if err != nil {
		log.Printf("Task %d (#%d:%s) error: %v", t.ID, t.This, t.VerbName, err)
	}

	// Flush output buffer for the player
	if s.taskOutputFlusher != nil {
		s.taskOutputFlusher(t.Owner, t.CommandOutputSuffix)
	}
	return nil
}
