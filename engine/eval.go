package engine

import (
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/MongooseMoo/barn/compiler"
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
	return s.Eval(player, strings.Split(code, "\n")).CommandOutput()
}

func (outcome EvalOutcome) CommandOutput() string {
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

// StartEval runs the first slice on the input caller and hands every suspension
// to the normal task scheduler. Completion is delivered once, after VM ownership
// and admission have been released.
func (s *Runtime) StartEval(player types.ObjID, source []string, complete func(EvalOutcome)) *task.Task {
	var running *task.Task
	var once sync.Once
	finish := func(out EvalOutcome) {
		once.Do(func() {
			if running != nil {
				running.TakeOnComplete()
				running.TakeOnFailure()
				s.taskManager.RemoveTask(running.ID)
			}
			complete(out)
		})
	}
	defer func() {
		if r := recover(); r != nil {
			metrics.PanicsRecovered.Add(1)
			slog.Error("panic in eval", slog.Int64("player", int64(player)), slog.String("panic", fmt.Sprint(r)), slog.String("go_stack", string(debug.Stack())))
			finish(EvalOutcome{Panic: fmt.Errorf("%v", r)})
		}
	}()
	scope, err := s.enterInput(player, false)
	if err != nil {
		finish(EvalOutcome{Panic: err})
		return nil
	}
	defer scope.Finish()
	prog, diagnostics := s.registry.Compiler().CompileMOO(source)
	if len(diagnostics) != 0 {
		scope.Finish()
		finish(EvalOutcome{Diagnostics: diagnostics})
		return nil
	}
	ticks, seconds := foregroundTaskLimits(s.session)
	running = task.NewTaskFull(s.newTaskID(), player, prog, ticks, seconds)
	s.populateTaskContextDependencies(running.Context)
	running.Context.IsWizard = s.isWizard(player)
	running.IntrinsicEval = true
	running.This = types.ObjNothing
	running.Caller = player
	running.VerbLoc = types.ObjNothing
	running.ForkCreator = s
	running.SetOnComplete(func(result types.Result) { finish(EvalOutcome{Result: result}) })
	running.SetOnFailure(func(err error) {
		finish(EvalOutcome{Panic: fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "internal panic: "))})
	})
	running.SetState(task.TaskQueued)
	s.taskManager.RegisterTask(running)
	if err := s.runTaskAdmitted(running, scope, true); err != nil {
		finish(EvalOutcome{Panic: fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "internal panic: "))})
	}
	return running
}

// Eval uses the same resumable lifecycle as a connection, driving scheduled work
// while a synchronous API caller waits for the terminal result.
func (s *Runtime) Eval(player types.ObjID, source []string) EvalOutcome {
	done := make(chan EvalOutcome, 1)
	running := s.StartEval(player, source, func(out EvalOutcome) { done <- out })
	var indefiniteSince time.Time
	for {
		select {
		case out := <-done:
			return out
		default:
		}
		if running == nil {
			return EvalOutcome{Panic: fmt.Errorf("eval has no task")}
		}
		if running.CancellationRequested() {
			// Kill intentionally suppresses task completion callbacks. Leave its
			// registered roots to normal cleanup if a final slice is still exiting.
			return EvalOutcome{Result: types.Err(types.E_INVARG)}
		}
		if running.GetState() == task.TaskSuspended && running.ReadyDeadline(time.Now()).IsZero() {
			if indefiniteSince.IsZero() {
				indefiniteSince = time.Now()
			}
			if time.Since(indefiniteSince) >= 10*time.Second {
				running.Kill()
				s.taskManager.RemoveTask(running.ID)
				return EvalOutcome{Result: types.Err(types.E_INVARG)}
			}
		} else {
			indefiniteSince = time.Time{}
		}
		if s.ProcessReadyTasks() != 0 {
			continue
		}
		select {
		case out := <-done:
			return out
		case <-s.ctx.Done():
			running.Kill()
			s.taskManager.RemoveTask(running.ID)
			return EvalOutcome{Panic: s.ctx.Err()}
		case <-time.After(time.Millisecond):
		}
	}
}

func prepareIntrinsicEval(machine *vm.VM, t *task.Task) {
	prog, player := t.Program, t.Owner
	frame := machine.PrepareVerbFrame(prog, types.ObjNothing, player, player, "", types.ObjNothing, []types.Value{})
	vm.SetLocalByName(frame, prog, "this", types.NewObj(types.ObjNothing))
	vm.SetLocalByName(frame, prog, "player", types.NewObj(player))
	vm.SetLocalByName(frame, prog, "caller", types.NewObj(player))
	vm.SetLocalByName(frame, prog, "verb", types.NewStr(""))
	vm.SetLocalByName(frame, prog, "args", types.NewList([]types.Value{}))
	for _, name := range []string{"argstr", "dobjstr", "iobjstr", "prepstr"} {
		vm.SetLocalByName(frame, prog, name, types.NewStr(""))
	}
	vm.SetLocalByName(frame, prog, "dobj", types.NewObj(types.ObjNothing))
	vm.SetLocalByName(frame, prog, "iobj", types.NewObj(types.ObjNothing))
	t.PushFrame(types.ActivationFrame{
		This: types.ObjNothing, ThisValue: types.None, Player: player,
		Programmer: player, Caller: types.ObjNothing, Verb: "",
		VerbLoc: types.ObjNothing, LineNumber: 1, IsEvalFrame: true,
	})
}
