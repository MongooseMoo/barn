package server

import (
	"barn/builtins"
	"barn/db"
	"barn/parser"
	"barn/task"
	"barn/types"
	"barn/vm"
	"container/heap"
	"fmt"
	"sync/atomic"
	"time"
)

func foregroundTaskLimits() (int64, float64) {
	return builtins.GetTaskLimits(false)
}

func backgroundTaskLimits() (int64, float64) {
	return builtins.GetTaskLimits(true)
}

func configureVMStackLimit(machine *vm.VM) {
	machine.MaxStackDepth = builtins.GetMaxStackDepth()
}

// QueueTask adds a task to the scheduler
func (s *Scheduler) QueueTask(t *task.Task) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	t.SetState(task.TaskQueued)
	s.tasks[t.ID] = t
	s.queueSeq++
	t.QueueSeq = s.queueSeq
	heap.Push(s.waiting, t)

	// Also register with global task manager so builtins can find it
	task.GetManager().RegisterTask(t)

	return t.ID
}

// CreateForegroundTask creates a foreground task (user command)
func (s *Scheduler) CreateForegroundTask(player types.ObjID, code []parser.Stmt) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	ticks, seconds := foregroundTaskLimits()
	t := task.NewTaskFull(taskID, player, code, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)
	t.StartTime = time.Now()
	t.ForkCreator = s // Give task access to scheduler for forks
	// Set wizard flag based on player
	t.Context.IsWizard = s.isWizard(player)
	return s.QueueTask(t)
}

// CreateVerbTask creates a task to execute a verb
func (s *Scheduler) CreateVerbTask(player types.ObjID, match *VerbMatch, cmd *ParsedCommand, outputSuffix string) <-chan struct{} {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	ticks, seconds := foregroundTaskLimits()
	t := task.NewTaskFull(taskID, player, match.Verb.Program.Statements, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)
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

	// Create done channel so callers can wait for task completion
	t.Done = make(chan struct{})

	s.QueueTask(t)
	return t.Done
}

// CreateServerVerbTask queues a server-initiated hook verb so it runs through
// the normal scheduler/task machinery and can suspend, fork, and shut down.
func (s *Scheduler) CreateServerVerbTask(objID types.ObjID, verbName string, args []types.Value, player types.ObjID) (int64, error) {
	verb, defObjID, err := s.store.FindVerb(objID, verbName)
	if err != nil || verb == nil {
		return 0, fmt.Errorf("find verb %s on #%d: %w", verbName, objID, err)
	}

	if verb.Program == nil && len(verb.Code) > 0 {
		program, errors := db.CompileVerb(verb.Code)
		if len(errors) > 0 {
			return 0, fmt.Errorf("compile %s on #%d: %v", verbName, defObjID, errors[0])
		}
		verb.Program = program
	}
	if verb.Program == nil {
		return 0, fmt.Errorf("verb %s on #%d has no code", verbName, defObjID)
	}

	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	ticks, seconds := foregroundTaskLimits()
	t := task.NewTaskFull(taskID, player, verb.Program.Statements, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)
	t.StartTime = time.Now()
	t.Programmer = verb.Owner
	t.Context.Programmer = verb.Owner
	t.Context.IsWizard = s.isWizard(verb.Owner)
	t.Context.ServerInitiated = true
	t.VerbName = verbName
	t.VerbLoc = defObjID
	t.This = objID
	t.Caller = player
	t.VerbArgsValues = append([]types.Value(nil), args...)
	t.ForkCreator = s

	return s.QueueTask(t), nil
}

// CreateBackgroundTask creates a background task (fork)
func (s *Scheduler) CreateBackgroundTask(player types.ObjID, code []parser.Stmt, delay time.Duration) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	ticks, seconds := backgroundTaskLimits()
	t := task.NewTaskFull(taskID, player, code, ticks, seconds)
	s.populateTaskContextDependencies(t.Context)
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
		ticks, seconds := backgroundTaskLimits()
		t = task.NewTaskFull(taskID, forkInfo.Player, nil, ticks, seconds)
		s.populateTaskContextDependencies(t.Context)

		// Create a pre-configured VM for the child
		childVM := vm.NewVM(s.store, s.registry)
		childVM.TickLimit = ticks
		configureVMStackLimit(childVM)

		// Set up the child frame with inherited variables
		frame := childVM.PrepareVerbFrame(forkProg,
			forkInfo.ThisObj, forkInfo.Player, forkInfo.Caller,
			forkInfo.Verb, forkInfo.VerbLoc, nil)
		// Mark as verb-call so syncTaskLineNumbers includes this frame
		// when syncing line numbers to the task's CallStack.
		frame.IsVerbCall = true
		// Inherit verb debug flag from the parent verb
		if forkVerb, _, vErr := s.store.FindVerb(forkInfo.ThisObj, forkInfo.Verb); vErr == nil && forkVerb != nil {
			frame.VerbDebug = forkVerb.Perms.Has(db.VerbDebug)
		}

		// Copy inherited variable values from the parent
		for varName, varVal := range forkInfo.Variables {
			vm.SetLocalByName(frame, forkProg, varName, varVal)
		}

		// The child VM is stored on the task and will be executed via ExecuteLoop
		// when runTask picks it up. We need a special code path for this.
		// Store the VM as BytecodeVM so runTask's resume path handles it.
		t.BytecodeVM = childVM

	} else if body, ok := forkInfo.Body.([]parser.Stmt); ok {
		// Tree-walker fork: Body is []parser.Stmt
		ticks, seconds := backgroundTaskLimits()
		t = task.NewTaskFull(taskID, forkInfo.Player, body, ticks, seconds)
		s.populateTaskContextDependencies(t.Context)

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
	t.VerbLoc = forkInfo.VerbLoc
	t.ForkCreator = s
	t.TaskLocal = types.NewEmptyMap()

	// Set up child's context
	t.Context.ThisObj = forkInfo.ThisObj
	t.Context.ThisValue = forkInfo.ThisValue
	t.Context.Player = forkInfo.Player
	t.Context.Programmer = parent.Programmer
	t.Context.Verb = forkInfo.Verb
	t.Context.IsWizard = s.isWizard(parent.Programmer)
	t.Context.Task = t // Attach task to context for task_local access

	// Push initial activation frame for the fork body.
	// This matches Toast: forked tasks include a frame for the verb
	// context in which the fork statement appeared.
	t.PushFrame(task.ActivationFrame{
		This:       forkInfo.ThisObj,
		ThisValue:  forkInfo.ThisValue,
		Player:     forkInfo.Player,
		Programmer: parent.Programmer,
		Caller:     forkInfo.Caller,
		Verb:       forkInfo.Verb,
		VerbLoc:    forkInfo.VerbLoc,
		LineNumber: 1,
	})

	return s.QueueTask(t)
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
