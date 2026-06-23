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

	"barn/bytecode"
	"barn/command"
	dbstore "barn/db/store"
	"barn/parser"
	"barn/task"
	"barn/types"
	"barn/vm"
)

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

	t.SetState(task.TaskRunning)

	ctx := t.Context
	if ctx == nil {
		t.SetState(task.TaskKilled)
		return errors.New("task has no context")
	}

	// Attach task to context so builtins can access task_local
	ctx.Task = t
	ctx.TaskID = t.ID
	ctx.Store = s.store
	ctx.Registry = s.registry

	// A task resuming after suspend runs under background limits: Toast treats
	// resumed tasks as background tasks, and time spent suspended does not count
	// against the execution budget. Reset both the tick and second budgets (and
	// the start time used for the deadline below) so ticks_left()/seconds_left()
	// and the hard deadline reflect a fresh background slice.
	if savedVM, ok := t.BytecodeVM.(*vm.VM); ok && savedVM.IsYielded() {
		bgTicks, bgSeconds := backgroundTaskLimits()
		t.TicksLimit = bgTicks
		t.TicksUsed = 0
		t.SecondsLimit = bgSeconds
		t.SecondsUsed = 0
		t.StartTime = time.Now()
		savedVM.TickLimit = bgTicks
		savedVM.Ticks = 0
	}

	// Set up cancellation with deadline
	deadline := t.StartTime.Add(time.Duration(t.SecondsLimit * float64(time.Second)))
	taskCtx, cancel := context.WithDeadline(s.ctx, deadline)
	t.CancelFunc = cancel
	defer cancel()

	var result types.Result
	var bcVM *vm.VM
	anonGCFloor := s.store.NextID()

	if t.BytecodeVM != nil {
		// Retrieve saved VM -- could be resuming after suspend or running a forked child
		var ok bool
		bcVM, ok = t.BytecodeVM.(*vm.VM)
		if !ok {
			t.SetState(task.TaskKilled)
			return errors.New("invalid saved VM state")
		}
		// Attach task context (may have been updated since VM was created)
		bcVM.Context = ctx
		if bcVM.IsYielded() {
			// If this task was read()-suspended, deliver the input line
			if t.WakeValue != nil {
				bcVM.SetResumeValue(t.WakeValue)
				t.WakeValue = nil // Consume — don't leak into future suspends
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

	// Check context deadline
	select {
	case <-taskCtx.Done():
		if taskCtx.Err() == context.Canceled && bcVM != nil && s.pendingFinalizationSink != nil {
			if pending := vm.CollectPendingFinalizationValues(s.store, bcVM); len(pending) > 0 {
				s.pendingFinalizationSink(pending)
			}
		}
		t.SetState(task.TaskKilled)
		t.BytecodeVM = nil
		return taskCtx.Err()
	default:
	}

	for zeroDelayYields := 0; result.Flow == types.FlowSuspend && t.IsForked && t.GetState() == task.TaskQueued && zeroDelayYields < 16; zeroDelayYields++ {
		t.BytecodeVM = bcVM
		if t.WakeValue != nil {
			bcVM.SetResumeValue(t.WakeValue)
			t.WakeValue = nil
		}
		result = bcVM.Resume()
		t.Result = result
	}

	// Handle suspend
	if result.Flow == types.FlowSuspend {
		// Match Toast lifecycle semantics more closely: a scheduler yield/suspend
		// is a GC boundary for newly-created orphan anonymous objects.
		liveVMs := append([]*vm.VM{bcVM}, s.liveTaskVMs(t)...)
		vm.AutoRecycleOrphanAnonymousSince(s.store, s.registry, ctx, anonGCFloor, liveVMs...)

		// Save VM state for later Resume()
		t.BytecodeVM = bcVM
		if t.GetState() == task.TaskQueued {
			s.mu.Lock()
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
			s.mu.Unlock()
		}
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
		liveVMs := s.liveTaskVMs(t)
		if bcVM != nil {
			s.finalizePendingWaifs(ctx, bcVM.TakePendingWaifs(), liveVMs...)
		}
		vm.AutoRecycleOrphanAnonymousSince(s.store, s.registry, ctx, anonGCFloor, liveVMs...)
	}

	t.BytecodeVM = nil // Release VM after completion

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
	if value == nil {
		return false
	}
	return strings.Contains(strings.ToLower(value.String()), strings.ToLower(text))
}

func (s *Scheduler) sendTaskLine(player types.ObjID, line string) {
	if s.taskLineSender != nil {
		s.taskLineSender(player, line)
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

// ExecuteVerbTaskSync creates and immediately runs a verb task on the scheduler goroutine.
func (s *Scheduler) ExecuteVerbTaskSync(player types.ObjID, match *command.VerbMatch, cmd *command.ParsedCommand, outputSuffix string) {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	ticks, seconds := foregroundTaskLimits()
	t := task.NewTaskFull(taskID, player, match.Statements, ticks, seconds)
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
}
