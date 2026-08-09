package engine

import (
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"github.com/MongooseMoo/barn/compiler"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// EvalCommandOutput evaluates MOO code directly for the intrinsic EVAL command
// and returns its single result record. The server input boundary owns framing.
func (s *Runtime) EvalCommandOutput(player types.ObjID, code string) (line string) {
	s.beginFinalizationProducer()
	defer s.finishFinalizationProducer()
	var executionTask *task.Task
	var executionCtx *kernel.TaskContext
	// Recover from panics in compile/execute to avoid crashing the server
	defer func() {
		if r := recover(); r != nil {
			line = fmt.Sprintf("{0, {\"Internal error: %v\"}}", r)
			metrics.PanicsRecovered.Add(1)
			slog.Error("panic in eval",
				slog.Int64("player", int64(player)),
				slog.String("panic", fmt.Sprint(r)),
				slog.String("go_stack", string(debug.Stack())))
		}
		// Recovery must finish before this direct VM relinquishes ownership. Then
		// release the physical lease and let the lifecycle flush retry anything an
		// inline floor sweep had to defer.
		if executionCtx != nil {
			s.releaseExecutionContext(executionCtx, executionTask.ID)
		}
		if executionTask != nil {
			s.releaseTaskExecution(executionTask.ID)
			s.flushDeferredGC()
		}
	}()

	prog, diagnostics := compiler.CompileMOO(strings.Split(code, "\n"), s.registry)
	if len(diagnostics) > 0 {
		kind := "Compile error"
		if diagnostics[0].Stage == compiler.SyntaxStage {
			kind = "Parse error"
		}
		errMsg := fmt.Sprintf("{0, {\"%s: %s\"}}", kind, diagnostics[0].Message)
		return errMsg
	}

	// Execute the code synchronously
	ctx := kernel.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = player
	ctx.IsWizard = s.isWizard(player)
	ctx.Store = s.store
	ctx.Registry = s.registry
	ctx.RuntimeOptions = s.options

	// Create and register a real task so task_id()/resume()/task_local()
	// semantics match normal task execution.
	mgr := s.taskManager
	ticks, secondsLimit := foregroundTaskLimits()
	t := task.NewTask(s.newTaskID(), player, ticks, secondsLimit)
	mgr.RegisterTask(t)
	defer mgr.RemoveTask(t.ID)
	t.Programmer = player
	t.ForkCreator = s // Enable fork support in eval commands
	ctx.Task = t
	ctx.TaskID = t.ID
	s.acquireTaskExecution(t)
	s.acquireExecutionContext(ctx, t.ID)
	executionTask = t
	executionCtx = ctx

	// Create bytecode VM and execute
	bcVM := vm.NewVM(s.store, s.registry)
	bcVM.Context = ctx
	bcVM.TickLimit = ticks
	configureVMStackLimit(bcVM)

	// Top-level eval still has intrinsic command variables in Toast:
	// player/caller/this/verb/args and command parser placeholders.
	frame := bcVM.PrepareVerbFrame(
		prog,
		types.ObjNothing,
		player,
		player,
		"",
		types.ObjNothing,
		[]types.Value{},
	)
	vm.SetLocalByName(frame, prog, "this", types.NewObj(types.ObjNothing))
	vm.SetLocalByName(frame, prog, "player", types.NewObj(player))
	vm.SetLocalByName(frame, prog, "caller", types.NewObj(player))
	vm.SetLocalByName(frame, prog, "verb", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "args", types.NewList([]types.Value{}))
	vm.SetLocalByName(frame, prog, "argstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "dobjstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "iobjstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "prepstr", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "dobj", types.NewObj(types.ObjNothing))
	vm.SetLocalByName(frame, prog, "iobj", types.NewObj(types.ObjNothing))

	anonGCFloor := s.store.NextID()
	// Sample the global anon-creation counter consistently with anonGCFloor so the
	// orphan-anon GC sweep can be skipped when no anonymous object was created.
	anonFloor := s.store.AnonCreationCount()
	result := bcVM.ExecuteLoop()

	// Handle yielded control flow (fork/suspend) until the eval completes.
resumeLoop:
	for result.Flow == types.FlowFork || result.Flow == types.FlowSuspend {
		result = s.drainForks(t, bcVM, result)

		if result.Flow != types.FlowSuspend {
			continue
		}

		// suspend(seconds): sleep for seconds then resume.
		// suspend(0): scheduler-yield then resume quickly.
		// suspend() (encoded as -1): wait for explicit resume(task_id, ...).
		seconds := 0.0
		switch result.Val.Type() {
		case types.TYPE_FLOAT:
			seconds = result.Val.Float()
		case types.TYPE_INT:
			seconds = float64(result.Val.Int())
		}

		switch {
		case seconds < 0:
			deadline := time.Now().Add(10 * time.Second)
			for t.GetState() != task.TaskQueued && time.Now().Before(deadline) {
				// Process ready tasks while waiting for explicit resume().
				// Since we're on the runtime goroutine, the ticker cannot
				// drive ready tasks while eval is waiting for resume().
				// won't fire from the ticker, so we must drive it here.
				s.ProcessReadyTasks()
				time.Sleep(10 * time.Millisecond)
			}
			if t.GetState() != task.TaskQueued {
				result = types.Result{Flow: types.FlowException, Error: types.E_INVARG, Val: types.None}
				break resumeLoop
			}
		case seconds == 0:
			// Process immediate ready tasks before resuming. Nested zero-delay
			// forks and suspend(0) resumes may need multiple runtime passes.
			idlePasses := 0
			deadline := time.Now().Add(2 * time.Second)
			for idlePasses < 8 && time.Now().Before(deadline) {
				if s.ProcessReadyTasks() == 0 {
					idlePasses++
					time.Sleep(5 * time.Millisecond)
				} else {
					idlePasses = 0
				}
			}
		default:
			sleepEnd := time.Now().Add(time.Duration(seconds * float64(time.Second)))
			for time.Now().Before(sleepEnd) {
				s.ProcessReadyTasks()
				remaining := time.Until(sleepEnd)
				if remaining <= 0 {
					break
				}
				if remaining > 10*time.Millisecond {
					remaining = 10 * time.Millisecond
				}
				time.Sleep(remaining)
			}
		}

		// Inject wake value before resuming (read() sets WakeValue to
		// the input string; default suspend uses 0).
		if !t.WakeValue.IsNone() {
			bcVM.SetResumeValue(t.WakeValue, t.WakeErrorAsValue)
			t.WakeValue = types.None // Consume — don't leak into future suspends
			t.WakeErrorAsValue = false
		}
		result = bcVM.Resume()
	}

	// Match Toast lifecycle semantics for eval: orphan anonymous objects are
	// collected once evaluation completes and locals are out of scope.
	// Take pending waifs first to preserve that side effect/ordering, then do the
	// s.mu sibling scan and the O(N) anon reachability sweep only when there is
	// something to GC (an anon was created since the floor, or pending waifs).
	pending := bcVM.TakePendingWaifs()
	anonCreated := s.store.AnonCreationCount() != anonFloor
	if anonCreated || len(pending) > 0 {
		func() {
			s.gcSweepMu.Lock()
			defer s.gcSweepMu.Unlock()
			s.vmStartMu.Lock()
			defer s.vmStartMu.Unlock()

			siblingAnon, siblingWaifs, quiescent := s.collectSiblingGCRefs(t)
			if !quiescent {
				if len(pending) > 0 {
					s.deferPendingWaifs(ctx, pending, bcVM)
				}
				if anonCreated {
					s.deferAnonGC(ctx, anonGCFloor, bcVM)
				}
				return
			}

			s.acquireSweepContext(ctx)
			defer s.releaseSweepContext(ctx)
			if len(pending) > 0 {
				s.finalizePendingWaifs(ctx, pending, siblingWaifs, bcVM)
			}
			if anonCreated {
				vm.AutoRecycleOrphanAnonymousSince(s.store, s.registry, ctx, anonGCFloor, siblingAnon, bcVM)
			}
		}()
	}

	// Return one result record in ToastStunt eval format:
	// Success: {1, value}
	// Runtime error: {2, {E_TYPE, "message", value}}
	var resultStr string
	if result.Flow == types.FlowException {
		// Runtime error: {2, {E_TYPE, "message", value}}
		errCode := types.NewErr(result.Error).String()
		errMsg := result.Error.Message()
		resultStr = fmt.Sprintf("{2, {%s, \"%s\", 0}}", errCode, errMsg)
	} else if !result.Val.IsNone() {
		// Success: {1, value}
		resultStr = fmt.Sprintf("{1, %s}", result.Val.String())
	} else {
		// Success with no return value: {1, 0}
		resultStr = "{1, 0}"
	}
	return resultStr
}
