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

// EvalOutcome is what evaluating MOO source as a player produced. Exactly one
// of the three fields is meaningful: Diagnostics when the source did not
// compile (nothing ran), Panic when execution crashed inside the server (a
// Barn bug, already logged and counted), Result otherwise.
type EvalOutcome struct {
	Diagnostics []compiler.Diagnostic
	Result      types.Result
	Panic       error
}

// EvalCommandOutput evaluates MOO code directly for the intrinsic EVAL command
// and returns its single result record. The server input boundary owns framing.
func (s *Runtime) EvalCommandOutput(player types.ObjID, code string) string {
	outcome := s.Eval(player, strings.Split(code, "\n"))
	switch {
	case outcome.Panic != nil:
		return fmt.Sprintf("{0, {\"Internal error: %v\"}}", outcome.Panic)
	case len(outcome.Diagnostics) > 0:
		kind := "Compile error"
		if outcome.Diagnostics[0].Stage == compiler.SyntaxStage {
			kind = "Parse error"
		}
		return fmt.Sprintf("{0, {\"%s: %s\"}}", kind, outcome.Diagnostics[0].Message)
	}
	// Return one result record in ToastStunt eval format:
	// Success: {1, value}
	// Runtime error: {2, {E_TYPE, "message", value}}
	result := outcome.Result
	if result.Flow == types.FlowException {
		errCode := types.NewErr(result.Error).String()
		errMsg := result.Error.Message()
		return fmt.Sprintf("{2, {%s, \"%s\", 0}}", errCode, errMsg)
	}
	if !result.Val.IsNone() {
		return fmt.Sprintf("{1, %s}", result.Val.String())
	}
	// Success with no return value: {1, 0}
	return "{1, 0}"
}

// Eval compiles source as a statement list and runs it synchronously as
// player, the way the intrinsic ";" command does: in a registered task with
// the eval activation frame and intrinsic variables Toast gives eval'd code,
// so callers(), task_id(), protected-builtin redirection, fork and suspend
// behave as they do for a live connection. It is the one eval implementation;
// the server's ";" and the dbtool's -eval/-eval-file both go through it.
func (s *Runtime) Eval(player types.ObjID, source []string) (outcome EvalOutcome) {
	scope, err := s.enterInput(player, false)
	if err != nil {
		outcome.Panic = err
		return
	}
	defer scope.Finish()
	s.beginFinalizationProducer()
	defer s.finishFinalizationProducer()
	var executionTask *task.Task
	var executionCtx *kernel.TaskContext
	// Recover from panics in compile/execute to avoid crashing the server
	defer func() {
		if r := recover(); r != nil {
			outcome.Panic = fmt.Errorf("%v", r)
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
			scope.Finish()
			s.flushDeferredGC()
		}
	}()

	prog, diagnostics := s.registry.Compiler().CompileMOO(source)
	if len(diagnostics) > 0 {
		outcome.Diagnostics = diagnostics
		return outcome
	}

	// Execute the code synchronously
	ctx := kernel.NewTaskContext()
	ctx.Admission = scope
	ctx.Player = player
	ctx.Programmer = player
	ctx.IsWizard = s.isWizard(player)
	ctx.Store = s.store
	ctx.StoreTxn = s.store.DirectTxn()
	ctx.RuntimeOptions = s.options

	// Create and register a real task so task_id()/resume()/task_local()
	// semantics match normal task execution.
	mgr := s.taskManager
	ticks, secondsLimit := foregroundTaskLimits(s.session)
	t := task.NewTask(s.newTaskID(), player, ticks, secondsLimit)
	mgr.RegisterTask(t)
	defer mgr.RemoveTask(t.ID)
	t.Programmer = player
	t.ForkCreator = s // Enable fork support in eval commands
	ctx.TaskID = t.ID
	if !s.acquireTaskExecution(t) {
		return
	}
	scope.Start()
	s.acquireExecutionContext(ctx, t.ID)
	executionTask = t
	executionCtx = ctx

	// Create bytecode VM and execute
	bcVM := vm.NewVM(s.store, s.session)
	bcVM.Context = ctx
	bcVM.Task = t
	bcVM.TickLimit = ticks
	configureVMStackLimit(bcVM, s.session)

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

	// The eval'd code is an activation of its own, as it is when the eval()
	// builtin runs it (vm/registry.go): a verb it calls sees this frame and
	// the eval wrappers in callers(), and like every eval frame it stays out
	// of tracebacks. It is the root of this task, so nothing pops it.
	t.PushFrame(types.ActivationFrame{
		This:        types.ObjNothing,
		ThisValue:   types.None,
		Player:      player,
		Programmer:  player,
		Caller:      types.ObjNothing,
		Verb:        "",
		VerbLoc:     types.ObjNothing,
		LineNumber:  1,
		IsEvalFrame: true,
	})

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
		// A real suspension releases service, even though this synchronous eval
		// retains its physical root lease while it drives background work.
		scope.Finish()

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
		if err := scope.ResumeBackground(s.ctx, int64(ctx.Programmer)); err != nil {
			outcome.Panic = err
			return
		}
		scope.Start()
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
			s.lifecycle.SweepMu.Lock()
			defer s.lifecycle.SweepMu.Unlock()
			s.lifecycle.VMStartMu.Lock()
			defer s.lifecycle.VMStartMu.Unlock()

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
				vm.AutoRecycleOrphanAnonymousSince(s.store, s.session, s.session.NewExecution(ctx, t), anonGCFloor, siblingAnon, bcVM)
			}
		}()
	}

	outcome.Result = result
	return outcome
}
