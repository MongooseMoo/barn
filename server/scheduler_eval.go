package server

import (
	"barn/parser"
	"barn/task"
	"barn/types"
	"barn/vm"
	"fmt"
	"log"
	"time"
)

// evalConnection is the interface needed for eval command output
type evalConnection interface {
	Send(string) error
	GetOutputPrefix() string
	GetOutputSuffix() string
}

// EvalCommand evaluates MOO code directly (for ; commands)
// Executes synchronously and sends the result back to the connection
func (s *Scheduler) EvalCommand(player types.ObjID, code string, conn interface{}) {
	// Type assert to get full eval connection interface
	c, ok := conn.(evalConnection)
	if !ok {
		return // Can't send output without proper connection
	}

	// Recover from panics in compile/execute to avoid crashing the server
	defer func() {
		if r := recover(); r != nil {
			prefix := c.GetOutputPrefix()
			suffix := c.GetOutputSuffix()
			if prefix != "" {
				c.Send(prefix)
			}
			c.Send(fmt.Sprintf("{0, {\"Internal error: %v\"}}", r))
			if suffix != "" {
				c.Send(suffix)
			}
			log.Printf("PANIC in EvalCommand: %v", r)
		}
	}()

	// Parse the code
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()

	// Get prefix/suffix for response framing
	prefix := c.GetOutputPrefix()
	suffix := c.GetOutputSuffix()

	if err != nil {
		// Send parse error in ToastStunt eval format: {0, {"error message"}}
		if prefix != "" {
			c.Send(prefix)
		}
		errMsg := fmt.Sprintf("{0, {\"Parse error: %s\"}}", err)
		c.Send(errMsg)
		if suffix != "" {
			c.Send(suffix)
		}
		return
	}

	// Execute the code synchronously
	ctx := types.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = player
	ctx.IsWizard = s.isWizard(player)
	ctx.Store = s.store
	ctx.Registry = s.registry

	// Create and register a real task so task_id()/resume()/task_local()
	// semantics match normal task execution.
	mgr := task.GetManager()
	ticks, secondsLimit := foregroundTaskLimits()
	t := mgr.CreateTask(player, ticks, secondsLimit)
	defer mgr.RemoveTask(t.ID)
	t.Programmer = player
	t.ForkCreator = s // Enable fork support in eval commands
	ctx.Task = t
	ctx.TaskID = t.ID

	// Compile AST to bytecode
	compiler := vm.NewCompilerWithRegistry(s.registry)
	prog, compileErr := compiler.CompileStatements(stmts)
	if compileErr != nil {
		// Compilation failed - send error
		if prefix != "" {
			c.Send(prefix)
		}
		errMsg := fmt.Sprintf("{0, {\"Compile error: %s\"}}", compileErr)
		c.Send(errMsg)
		if suffix != "" {
			c.Send(suffix)
		}
		return
	}

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
	result := bcVM.ExecuteLoop()

	// Handle yielded control flow (fork/suspend) until the eval completes.
	for result.Flow == types.FlowFork || result.Flow == types.FlowSuspend {
		result = s.drainForks(t, bcVM, result)

		if result.Flow != types.FlowSuspend {
			continue
		}

		// suspend(seconds): sleep for seconds then resume.
		// suspend(0): scheduler-yield then resume quickly.
		// suspend() (encoded as -1): wait for explicit resume(task_id, ...).
		seconds := 0.0
		switch v := result.Val.(type) {
		case types.FloatValue:
			seconds = v.Val
		case types.IntValue:
			seconds = float64(v.Val)
		}

		switch {
		case seconds < 0:
			deadline := time.Now().Add(10 * time.Second)
			for t.GetState() != task.TaskQueued && time.Now().Before(deadline) {
				// Process ready tasks while waiting for explicit resume().
				// Since we're on the scheduler goroutine, processReadyTasks
				// won't fire from the ticker, so we must drive it here.
				s.processReadyTasks()
				time.Sleep(10 * time.Millisecond)
			}
			if t.GetState() != task.TaskQueued {
				result = types.Result{Flow: types.FlowException, Error: types.E_INVARG}
				break
			}
		case seconds == 0:
			// Process immediate ready tasks before resuming. Nested zero-delay
			// forks and suspend(0) resumes may need multiple scheduler passes.
			idlePasses := 0
			deadline := time.Now().Add(2 * time.Second)
			for idlePasses < 8 && time.Now().Before(deadline) {
				if s.processReadyTasks() == 0 {
					idlePasses++
					time.Sleep(5 * time.Millisecond)
				} else {
					idlePasses = 0
				}
			}
		default:
			sleepEnd := time.Now().Add(time.Duration(seconds * float64(time.Second)))
			for time.Now().Before(sleepEnd) {
				s.processReadyTasks()
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
		if t.WakeValue != nil {
			bcVM.SetResumeValue(t.WakeValue)
			t.WakeValue = nil // Consume — don't leak into future suspends
		}
		result = bcVM.Resume()
	}

	// Match Toast lifecycle semantics for eval: orphan anonymous objects are
	// collected once evaluation completes and locals are out of scope.
	liveVMs := s.liveTaskVMs(t)
	s.finalizePendingWaifs(ctx, bcVM.TakePendingWaifs(), liveVMs...)
	vm.AutoRecycleOrphanAnonymousSince(s.store, s.registry, ctx, anonGCFloor, liveVMs...)

	// Send result wrapped with prefix/suffix in ToastStunt eval format:
	// Success: {1, value}
	// Runtime error: {2, {E_TYPE, "message", value}}
	if prefix != "" {
		c.Send(prefix)
	}
	var resultStr string
	if result.Flow == types.FlowException {
		// Runtime error: {2, {E_TYPE, "message", value}}
		errCode := types.NewErr(result.Error).String()
		errMsg := result.Error.Message()
		resultStr = fmt.Sprintf("{2, {%s, \"%s\", 0}}", errCode, errMsg)
	} else if result.Val != nil {
		// Success: {1, value}
		resultStr = fmt.Sprintf("{1, %s}", result.Val.String())
	} else {
		// Success with no return value: {1, 0}
		resultStr = "{1, 0}"
	}
	c.Send(resultStr)
	if suffix != "" {
		c.Send(suffix)
	}
}
