package server

import (
	"barn/builtins"
	"barn/db"
	"barn/parser"
	"barn/task"
	"barn/trace"
	"barn/types"
	"barn/vm"
	"container/heap"
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Scheduler manages task execution
type Scheduler struct {
	tasks       map[int64]*task.Task
	waiting     *TaskQueue
	nextTaskID  int64
	evaluator   *vm.Evaluator
	registry    *builtins.Registry // Shared builtins registry for bytecode VMs
	store       *db.Store
	connManager *ConnectionManager
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewScheduler creates a new task scheduler
func NewScheduler(store *db.Store) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		tasks:      make(map[int64]*task.Task),
		waiting:    NewTaskQueue(),
		nextTaskID: 1,
		evaluator:  vm.NewEvaluatorWithStore(store),
		registry:   vm.BuildVMRegistry(store),
		store:      store,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Builtins like create()/recycle() need verb callbacks in VM mode.
	// Route builtin CallVerb() through scheduler CallVerb().
	s.registry.SetVerbCaller(func(objID types.ObjID, verbName string, args []types.Value, tc *types.TaskContext) types.Result {
		player := types.ObjNothing
		if tc != nil {
			player = tc.Player
			if player == types.ObjNothing {
				player = tc.Programmer
			}
		}
		return s.CallVerb(objID, verbName, args, player)
	})

	return s
}

// Start begins the scheduler loop
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

// GetEvaluator returns the scheduler's evaluator
func (s *Scheduler) GetEvaluator() *vm.Evaluator {
	return s.evaluator
}

// SetConnectionManager sets the connection manager for output flushing
func (s *Scheduler) SetConnectionManager(cm *ConnectionManager) {
	s.connManager = cm
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processReadyTasks()
		}
	}
}

// processReadyTasks executes tasks that are ready to run
func (s *Scheduler) processReadyTasks() {
	s.mu.Lock()

	now := time.Now()
	var readyTasks []*task.Task

	// Collect all ready tasks from waiting queue
	for s.waiting.Len() > 0 {
		t := s.waiting.Peek()
		if t.StartTime.After(now) {
			break // Tasks are ordered by start time
		}
		heap.Pop(s.waiting)
		if t.GetState() != task.TaskQueued {
			// Ignore tasks killed/suspended after enqueue.
			continue
		}
		readyTasks = append(readyTasks, t)
	}

	// Check for suspended/resumed tasks that need to be re-run.
	// These are tasks that were suspended and later resumed via resume() builtin
	for _, t := range s.tasks {
		// Timed suspension wake-up: suspend(seconds) resumes after deadline.
		if t.WakeDue(now) {
			if t.Resume(types.NewInt(0)) {
				readyTasks = append(readyTasks, t)
			}
			continue
		}

		// TaskQueued state means it was resumed and is ready to run again
		// We need to check if it's not already in readyTasks and not in waiting queue
		if t.GetState() == task.TaskQueued && (t.StmtIndex > 0 || t.BytecodeVM != nil) {
			// This is a resumed task (StmtIndex > 0 or BytecodeVM saved means it was partially executed)
			// Check if wake time has passed (or no wake time was set)
			if t.WakeTime.IsZero() || !t.WakeTime.After(now) {
				readyTasks = append(readyTasks, t)
			}
		}
	}

	s.mu.Unlock()

	// Execute ready tasks (outside the lock to allow concurrency)
	for _, t := range readyTasks {
		// Run task in a goroutine
		go func(t *task.Task) {
			err := s.runTask(t)
			if err != nil {
				log.Printf("Task %d (#%d:%s) error: %v", t.ID, t.This, t.VerbName, err)
			}

			// Flush output buffer for the player
			if s.connManager != nil {
				if conn := s.connManager.GetConnection(t.Owner); conn != nil {
					conn.Flush()
					// For raw command execution, emit framing suffix after task output.
					if t.CommandOutputSuffix != "" {
						_ = conn.Send(t.CommandOutputSuffix)
					}
				}
			}
		}(t)
	}
}

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
		compiler := vm.NewCompilerWithRegistry(s.registry)
		prog, compileErr := compiler.CompileStatements(code)
		if compileErr != nil {
			t.SetState(task.TaskKilled)
			return fmt.Errorf("compile error: %w", compileErr)
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

		if t.VerbName != "" {
			// Convert string args to Value list for verb context
			argList := make([]types.Value, len(t.Args))
			for i, arg := range t.Args {
				argList[i] = types.NewStr(arg)
			}

			// Prepare frame first, then set ALL variables before execution
			frame := bcVM.PrepareVerbFrame(prog, t.This, t.Owner, t.Caller, t.VerbName, t.VerbLoc, argList)

			// Set verb context variables
			vm.SetLocalByNamePublic(frame, prog, "this", types.NewObj(t.This))
			vm.SetLocalByNamePublic(frame, prog, "player", types.NewObj(t.Owner))
			vm.SetLocalByNamePublic(frame, prog, "caller", types.NewObj(t.Caller))
			vm.SetLocalByNamePublic(frame, prog, "verb", types.NewStr(t.VerbName))
			vm.SetLocalByNamePublic(frame, prog, "args", types.NewList(argList))

			// Set command-specific variables
			vm.SetLocalByNamePublic(frame, prog, "argstr", types.NewStr(t.Argstr))
			vm.SetLocalByNamePublic(frame, prog, "dobjstr", types.NewStr(t.Dobjstr))
			vm.SetLocalByNamePublic(frame, prog, "iobjstr", types.NewStr(t.Iobjstr))
			vm.SetLocalByNamePublic(frame, prog, "prepstr", types.NewStr(t.Prepstr))
			vm.SetLocalByNamePublic(frame, prog, "dobj", types.NewObj(t.Dobj))
			vm.SetLocalByNamePublic(frame, prog, "iobj", types.NewObj(t.Iobj))

			// Start execution
			result = bcVM.ExecuteLoop()
		} else {
			// Simple eval task (no verb context)
			result = bcVM.Run(prog)
		}
	}

	t.Result = result

	// Handle fork: create child task, set fork variable, resume parent
	for result.Flow == types.FlowFork {
		if t.ForkCreator != nil && result.ForkInfo != nil {
			childID := t.ForkCreator.CreateForkedTask(t, result.ForkInfo)
			bcVM.SetForkResult(childID)
		}
		// Resume the parent after handling the fork
		result = bcVM.Resume()
		t.Result = result
	}

	// Check context deadline
	select {
	case <-taskCtx.Done():
		t.SetState(task.TaskKilled)
		t.BytecodeVM = nil
		return taskCtx.Err()
	default:
	}

	// Handle suspend
	if result.Flow == types.FlowSuspend {
		// Save VM state for later Resume()
		t.BytecodeVM = bcVM
		// The task manager has already been notified via builtinSuspend
		// Just return without setting state to Completed
		return nil
	}

	// Handle completion
	if result.Flow == types.FlowException {
		t.SetState(task.TaskKilled)
		// Log traceback to server log
		s.logTraceback(t, result.Error)
		// Send traceback to player
		s.sendTraceback(t, result.Error)
		// Clean up call stack after traceback has been sent
		for len(t.CallStack) > 0 {
			t.PopFrame()
		}
	} else {
		t.SetState(task.TaskCompleted)
	}

	// Match Toast lifecycle semantics: orphan anonymous objects are collected
	// when a task completes (locals and stack references are gone).
	vm.AutoRecycleOrphanAnonymousSince(s.store, s.registry, ctx, anonGCFloor)

	t.BytecodeVM = nil // Release VM after completion
	return nil
}

// QueueTask adds a task to the scheduler
func (s *Scheduler) QueueTask(t *task.Task) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	t.SetState(task.TaskQueued)
	s.tasks[t.ID] = t
	heap.Push(s.waiting, t)

	// Also register with global task manager so builtins can find it
	task.GetManager().RegisterTask(t)

	return t.ID
}

// CreateForegroundTask creates a foreground task (user command)
func (s *Scheduler) CreateForegroundTask(player types.ObjID, code []parser.Stmt) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	t := task.NewTaskFull(taskID, player, code, 300000, 5.0)
	t.StartTime = time.Now()
	t.ForkCreator = s // Give task access to scheduler for forks
	// Set wizard flag based on player
	t.Context.IsWizard = s.isWizard(player)
	return s.QueueTask(t)
}

// CreateVerbTask creates a task to execute a verb
func (s *Scheduler) CreateVerbTask(player types.ObjID, match *VerbMatch, cmd *ParsedCommand, outputSuffix string) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	t := task.NewTaskFull(taskID, player, match.Verb.Program.Statements, 300000, 5.0)
	t.StartTime = time.Now()
	// Task runs with verb owner permissions (MOO programmer semantics).
	t.Programmer = match.Verb.Owner
	t.Context.Programmer = match.Verb.Owner
	t.Context.IsWizard = s.isWizard(match.Verb.Owner)

	// Set up verb context
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
	t.ForkCreator = s // Give task access to scheduler for forks

	return s.QueueTask(t)
}

// CreateBackgroundTask creates a background task (fork)
func (s *Scheduler) CreateBackgroundTask(player types.ObjID, code []parser.Stmt, delay time.Duration) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	t := task.NewTaskFull(taskID, player, code, 300000, 3.0)
	t.StartTime = time.Now().Add(delay)
	t.ForkCreator = s // Give task access to scheduler for forks
	// Set wizard flag based on player
	t.Context.IsWizard = s.isWizard(player)
	return s.QueueTask(t)
}

// Fork creates a forked task with a delay
func (s *Scheduler) Fork(ctx *types.TaskContext, code []parser.Stmt, delay time.Duration) int64 {
	return s.CreateBackgroundTask(ctx.Player, code, delay)
}

// CreateForkedTask creates a forked child task from fork statement
// Implements task.ForkCreator interface
// Handles both bytecode VM forks (Body is [3]interface{}{*Program, IP, Len})
// and tree-walker forks (Body is []parser.Stmt).
func (s *Scheduler) CreateForkedTask(parent *task.Task, forkInfo *types.ForkInfo) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)

	// Determine the fork body type
	var t *task.Task

	if bcFork, ok := forkInfo.Body.([3]interface{}); ok {
		// Bytecode VM fork: Body is [3]interface{}{*Program, bodyIP, bodyLen}
		parentProg, ok1 := bcFork[0].(*vm.Program)
		bodyIP, ok2 := bcFork[1].(int)
		bodyLen, ok3 := bcFork[2].(int)
		if !ok1 || !ok2 || !ok3 {
			return 0 // Invalid fork info
		}

		// Extract the fork body as a sub-program
		forkProg := parentProg.ExtractForkBody(bodyIP, bodyLen)

		// Create child task -- Code stays nil since we'll use BytecodeVM path
		t = task.NewTaskFull(taskID, forkInfo.Player, nil, 300000, 3.0)

		// Create a pre-configured VM for the child
		childVM := vm.NewVM(s.store, s.registry)
		childVM.TickLimit = 300000

		// Set up the child frame with inherited variables
		frame := childVM.PrepareVerbFrame(forkProg,
			forkInfo.ThisObj, forkInfo.Player, forkInfo.Caller,
			forkInfo.Verb, types.ObjNothing, nil)

		// Copy inherited variable values from the parent
		for varName, varVal := range forkInfo.Variables {
			vm.SetLocalByNamePublic(frame, forkProg, varName, varVal)
		}

		// The child VM is stored on the task and will be executed via ExecuteLoop
		// when runTask picks it up. We need a special code path for this.
		// Store the VM as BytecodeVM so runTask's resume path handles it.
		t.BytecodeVM = childVM

	} else if body, ok := forkInfo.Body.([]parser.Stmt); ok {
		// Tree-walker fork: Body is []parser.Stmt
		t = task.NewTaskFull(taskID, forkInfo.Player, body, 300000, 3.0)

		// Create evaluator with copied variable environment
		childEnv := vm.NewEnvironment()
		for k, v := range forkInfo.Variables {
			childEnv.Set(k, v)
		}
		t.Evaluator = vm.NewEvaluatorWithEnvAndStore(childEnv, s.store)
	} else {
		return 0 // Unknown fork body type
	}

	t.StartTime = time.Now().Add(forkInfo.Delay)
	t.Kind = task.TaskForked
	t.IsForked = true
	t.ForkInfo = forkInfo
	t.Programmer = parent.Programmer // Inherit permissions
	t.This = forkInfo.ThisObj
	t.Caller = forkInfo.Caller
	t.VerbName = forkInfo.Verb
	t.ForkCreator = s                   // Give child access to scheduler for nested forks
	t.TaskLocal = parent.GetTaskLocal() // Copy parent's task_local to child

	// Set up child's context
	t.Context.ThisObj = forkInfo.ThisObj
	t.Context.Player = forkInfo.Player
	t.Context.Programmer = parent.Programmer
	t.Context.Verb = forkInfo.Verb
	t.Context.IsWizard = s.isWizard(parent.Programmer)
	t.Context.Task = t // Attach task to context for task_local access

	return s.QueueTask(t)
}

// CallVerb synchronously executes a verb on an object and returns the result
// This is used for server hooks like do_login_command, user_connected, etc.
// Returns a Result with a call stack for traceback formatting
func (s *Scheduler) CallVerb(objID types.ObjID, verbName string, args []types.Value, player types.ObjID) (result types.Result) {
	// Recover from panics in compile/execute to avoid crashing the server
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in CallVerb(%v:%s): %v", objID, verbName, r)
			result = types.Err(types.E_NONE)
		}
	}()

	// Trace verb call
	trace.VerbCall(objID, verbName, args, player, player)

	// Create a lightweight task FIRST for call stack tracking
	// This ensures we have a stack even if verb lookup fails
	t := &task.Task{
		Owner:      player,
		Programmer: player, // Will be updated to verb owner if verb found
		CallStack:  make([]task.ActivationFrame, 0),
		TaskLocal:  types.NewEmptyMap(), // Initialize task_local to empty map
	}

	// Look up the verb to get its owner for programmer permissions
	verb, defObjID, err := s.store.FindVerb(objID, verbName)
	if err != nil || verb == nil {
		// Verb not found
		result := types.Result{
			Flow:  types.FlowException,
			Error: types.E_VERBNF,
		}
		// Don't log E_VERBNF for optional hooks
		return result
	}

	// Compile verb to bytecode
	prog, compileErr := vm.CompileVerbBytecode(verb, s.registry)
	if compileErr != nil {
		log.Printf("[COMPILE ERROR] Failed to compile verb %s on #%d: %v", verbName, defObjID, compileErr)
		return types.Result{
			Flow:  types.FlowException,
			Error: types.E_VERBNF,
		}
	}

	// Update programmer to verb owner now that we found the verb
	t.Programmer = verb.Owner

	thisVal := types.Value(types.NewObj(objID))
	var frameThisValue types.Value
	if target := s.store.Get(objID); target != nil && target.Anonymous {
		anon := types.NewAnon(objID)
		thisVal = anon
		frameThisValue = anon
	}

	ctx := types.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = verb.Owner           // Programmer is verb owner, not player
	ctx.IsWizard = s.isWizard(verb.Owner) // Set wizard flag based on verb owner
	ctx.ThisObj = objID
	ctx.ThisValue = frameThisValue
	ctx.Verb = verbName
	ctx.ServerInitiated = true // Mark as server-initiated
	ctx.Task = t               // Attach task so VM can track frames

	// Push activation frame for traceback support
	t.PushFrame(task.ActivationFrame{
		This:            objID,
		ThisValue:       frameThisValue,
		Player:          player,
		Programmer:      verb.Owner,
		Caller:          player, // For server hooks, caller is the player
		Verb:            verbName,
		VerbLoc:         defObjID,
		Args:            args,
		LineNumber:      1,
		ServerInitiated: true,
	})

	// Create bytecode VM and set up initial frame variables
	bcVM := vm.NewVM(s.store, s.registry)
	bcVM.Context = ctx
	bcVM.TickLimit = 300000

	// Build the initial verb frame explicitly so we can preserve ANON `this`.
	frame := bcVM.PrepareVerbFrame(prog, objID, player, player, verbName, defObjID, args)
	vm.SetLocalByNamePublic(frame, prog, "this", thisVal)
	vm.SetLocalByNamePublic(frame, prog, "player", types.NewObj(player))
	vm.SetLocalByNamePublic(frame, prog, "caller", types.NewObj(player))
	vm.SetLocalByNamePublic(frame, prog, "verb", types.NewStr(verbName))
	vm.SetLocalByNamePublic(frame, prog, "args", types.NewList(args))
	result = bcVM.ExecuteLoop()

	// Extract call stack BEFORE popping frames
	if result.Flow == types.FlowException {
		result.CallStack = t.GetCallStack()
		// Log traceback to server log
		s.logCallVerbTraceback(objID, verbName, result.Error, t.GetCallStack(), player)
		// Trace exception
		trace.Exception(objID, verbName, result.Error)
	} else {
		// Trace return value
		trace.VerbReturn(objID, verbName, result.Val)
	}

	// Clean up call stack
	if len(t.CallStack) > 0 {
		t.PopFrame()
	}

	return result
}

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

	// Create and register a real task so task_id()/resume()/task_local()
	// semantics match normal task execution.
	mgr := task.GetManager()
	t := mgr.CreateTask(player, 300000, 5.0)
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
	bcVM.TickLimit = 300000

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
	vm.SetLocalByNamePublic(frame, prog, "this", types.NewObj(types.ObjNothing))
	vm.SetLocalByNamePublic(frame, prog, "player", types.NewObj(player))
	vm.SetLocalByNamePublic(frame, prog, "caller", types.NewObj(player))
	vm.SetLocalByNamePublic(frame, prog, "verb", types.NewStr(""))
	vm.SetLocalByNamePublic(frame, prog, "args", types.NewList([]types.Value{}))
	vm.SetLocalByNamePublic(frame, prog, "argstr", types.NewStr(""))
	vm.SetLocalByNamePublic(frame, prog, "dobjstr", types.NewStr(""))
	vm.SetLocalByNamePublic(frame, prog, "iobjstr", types.NewStr(""))
	vm.SetLocalByNamePublic(frame, prog, "prepstr", types.NewStr(""))
	vm.SetLocalByNamePublic(frame, prog, "dobj", types.NewObj(types.ObjNothing))
	vm.SetLocalByNamePublic(frame, prog, "iobj", types.NewObj(types.ObjNothing))

	anonGCFloor := s.store.NextID()
	result := bcVM.ExecuteLoop()

	// Handle yielded control flow (fork/suspend) until the eval completes.
	for result.Flow == types.FlowFork || result.Flow == types.FlowSuspend {
		for result.Flow == types.FlowFork {
			if t.ForkCreator != nil && result.ForkInfo != nil {
				childID := t.ForkCreator.CreateForkedTask(t, result.ForkInfo)
				bcVM.SetForkResult(childID)
			}
			result = bcVM.Resume()
		}

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
				time.Sleep(10 * time.Millisecond)
			}
			if t.GetState() != task.TaskQueued {
				result = types.Result{Flow: types.FlowException, Error: types.E_INVARG}
				break
			}
		case seconds == 0:
			time.Sleep(10 * time.Millisecond)
		default:
			time.Sleep(time.Duration(seconds * float64(time.Second)))
		}

		result = bcVM.Resume()
	}

	// Match Toast lifecycle semantics for eval: orphan anonymous objects are
	// collected once evaluation completes and locals are out of scope.
	vm.AutoRecycleOrphanAnonymousSince(s.store, s.registry, ctx, anonGCFloor)

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
		resultStr = fmt.Sprintf("{2, {%s, \"\", 0}}", errCode)
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

// ResumeTask resumes a suspended task
func (s *Scheduler) ResumeTask(taskID int64, value types.Value) error {
	s.mu.Lock()
	t, exists := s.tasks[taskID]
	s.mu.Unlock()

	if !exists {
		return ErrNotSuspended
	}

	if !t.Resume(value) {
		return ErrNotSuspended
	}
	return nil
}

// KillTask kills a running task
func (s *Scheduler) KillTask(taskID int64, killerID types.ObjID) error {
	s.mu.Lock()
	t, exists := s.tasks[taskID]
	s.mu.Unlock()

	if !exists {
		return ErrNotSuspended
	}

	// Permission check
	if t.Owner != killerID && !s.isWizard(killerID) {
		return ErrPermission
	}

	t.Kill()
	return nil
}

// GetTask retrieves a task by ID
func (s *Scheduler) GetTask(taskID int64) *task.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[taskID]
}

// QueuedTasks returns list of queued tasks
func (s *Scheduler) QueuedTasks() []*task.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]*task.Task, 0)
	for _, t := range s.tasks {
		if t.GetState() == task.TaskQueued {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// SuspendedTasks returns list of suspended tasks
func (s *Scheduler) SuspendedTasks() []*task.Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]*task.Task, 0)
	for _, t := range s.tasks {
		if t.GetState() == task.TaskSuspended {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// isWizard checks if an object has wizard permissions
func (s *Scheduler) isWizard(objID types.ObjID) bool {
	obj := s.store.Get(objID)
	if obj == nil {
		return false
	}
	return obj.Flags.Has(db.FlagWizard)
}

// logTraceback logs a formatted traceback to the server log for a task
func (s *Scheduler) logTraceback(t *task.Task, err types.ErrorCode) {
	stack := t.GetCallStack()
	lines := task.FormatTraceback(stack, err, t.Owner)
	log.Printf("TRACEBACK: Task %d (#%d:%s) uncaught exception %s",
		t.ID, t.This, t.VerbName, types.NewErr(err).String())
	for _, line := range lines {
		log.Printf("TRACEBACK:   %s", line)
	}
}

// logCallVerbTraceback logs a formatted traceback to the server log for a synchronous verb call
// E_VERBNF is not logged because it's the normal case for optional hook verbs
func (s *Scheduler) logCallVerbTraceback(objID types.ObjID, verbName string, err types.ErrorCode, stack []task.ActivationFrame, player types.ObjID) {
	if err == types.E_VERBNF {
		return // Verb not found is expected for optional hooks
	}
	lines := task.FormatTraceback(stack, err, player)
	log.Printf("TRACEBACK: #%d:%s uncaught exception %s (player #%d)",
		objID, verbName, types.NewErr(err).String(), player)
	for _, line := range lines {
		log.Printf("TRACEBACK:   %s", line)
	}
}

// sendTraceback sends a formatted traceback to the player
func (s *Scheduler) sendTraceback(t *task.Task, err types.ErrorCode) {
	if s.connManager == nil {
		return
	}

	conn := s.connManager.GetConnection(t.Owner)
	if conn == nil {
		return
	}

	// Format and send the traceback
	lines := task.FormatTraceback(t.GetCallStack(), err, t.Owner)
	for _, line := range lines {
		conn.Send(line)
	}
}

// TaskQueue is a priority queue for tasks ordered by start time
type TaskQueue []*task.Task

func NewTaskQueue() *TaskQueue {
	tq := make(TaskQueue, 0)
	heap.Init(&tq)
	return &tq
}

func (tq TaskQueue) Len() int { return len(tq) }

func (tq TaskQueue) Less(i, j int) bool {
	return tq[i].StartTime.Before(tq[j].StartTime)
}

func (tq TaskQueue) Swap(i, j int) {
	tq[i], tq[j] = tq[j], tq[i]
}

func (tq *TaskQueue) Push(x interface{}) {
	*tq = append(*tq, x.(*task.Task))
}

func (tq *TaskQueue) Pop() interface{} {
	old := *tq
	n := len(old)
	item := old[n-1]
	*tq = old[0 : n-1]
	return item
}

func (tq TaskQueue) Peek() *task.Task {
	if len(tq) == 0 {
		return nil
	}
	return tq[0]
}

// Error definitions
var (
	ErrTicksExceeded = errors.New("tick limit exceeded")
	ErrNotSuspended  = errors.New("task not suspended")
	ErrResumeFailed  = errors.New("failed to resume task")
	ErrPermission    = errors.New("permission denied")
)
