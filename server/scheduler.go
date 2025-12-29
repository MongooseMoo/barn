package server

import (
	"barn/db"
	"barn/parser"
	"barn/task"
	"barn/types"
	"barn/vm"
	"container/heap"
	"context"
	"errors"
	"fmt"
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

	return &Scheduler{
		tasks:      make(map[int64]*task.Task),
		waiting:    NewTaskQueue(),
		nextTaskID: 1,
		evaluator:  vm.NewEvaluatorWithStore(store),
		store:      store,
		ctx:        ctx,
		cancel:     cancel,
	}
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

	// Collect all ready tasks
	for s.waiting.Len() > 0 {
		t := s.waiting.Peek()
		if t.StartTime.After(now) {
			break // Tasks are ordered by start time
		}
		heap.Pop(s.waiting)
		readyTasks = append(readyTasks, t)
	}

	s.mu.Unlock()

	// Execute ready tasks (outside the lock to allow concurrency)
	for _, t := range readyTasks {
		// Run task in a goroutine
		go func(t *task.Task) {
			err := s.runTask(t)
			if err != nil {
				// Log error (in real implementation)
				_ = err
			}

			// Flush output buffer for the player
			if s.connManager != nil {
				if conn := s.connManager.GetConnection(t.Owner); conn != nil {
					conn.Flush()
				}
			}
		}(t)
	}
}

// runTask executes a task's code
func (s *Scheduler) runTask(t *task.Task) error {
	t.SetState(task.TaskRunning)

	// Get code and context
	code, ok := t.Code.([]parser.Stmt)
	if !ok || code == nil {
		t.SetState(task.TaskKilled)
		return errors.New("task has no code")
	}

	ctx := t.Context
	if ctx == nil {
		t.SetState(task.TaskKilled)
		return errors.New("task has no context")
	}

	// Get or create evaluator
	evaluator := s.evaluator
	if t.Evaluator != nil {
		if eval, ok := t.Evaluator.(*vm.Evaluator); ok {
			evaluator = eval
		}
	}

	// Set up verb context if this is a verb task
	if t.VerbName != "" {
		evaluator.SetVerbContext(&vm.VerbContext{
			Player:  t.Owner,
			This:    t.This,
			Caller:  t.Caller,
			Verb:    t.VerbName,
			Args:    t.Args,
			Argstr:  t.Argstr,
			Dobj:    t.Dobj,
			Dobjstr: t.Dobjstr,
			Iobj:    t.Iobj,
			Iobjstr: t.Iobjstr,
			Prepstr: t.Prepstr,
		})
		// Also update TaskContext for permissions and builtins
		ctx.ThisObj = t.This
		ctx.Verb = t.VerbName

		// Push initial activation frame for traceback support
		t.PushFrame(task.ActivationFrame{
			This:       t.This,
			Player:     t.Owner,
			Programmer: t.Programmer,
			Caller:     t.Caller,
			Verb:       t.VerbName,
			VerbLoc:    t.VerbLoc, // Object where verb is defined
			LineNumber: 1,
		})
	}

	// Set up cancellation with deadline
	deadline := t.StartTime.Add(time.Duration(t.SecondsLimit * float64(time.Second)))
	taskCtx, cancel := context.WithDeadline(s.ctx, deadline)
	t.CancelFunc = cancel
	defer cancel()

	// Execute code
	for _, stmt := range code {
		select {
		case <-taskCtx.Done():
			t.SetState(task.TaskKilled)
			return taskCtx.Err()
		default:
		}

		// Check tick limit
		if ctx.TicksRemaining <= 0 {
			t.SetState(task.TaskKilled)
			return ErrTicksExceeded
		}

		// Execute statement
		result := evaluator.EvalStmt(stmt, ctx)
		t.Result = result

		// Handle control flow
		if result.Flow == types.FlowFork {
			// Fork statement - create child task via ForkCreator
			if t.ForkCreator != nil && result.ForkInfo != nil {
				childID := t.ForkCreator.CreateForkedTask(t, result.ForkInfo)

				// If named fork, store child ID in parent's variable
				if result.ForkInfo.VarName != "" {
					evaluator.GetEnvironment().Set(result.ForkInfo.VarName, types.NewInt(childID))
				}
			}
			// Parent continues execution
			continue
		}

		if result.Flow == types.FlowReturn || result.Flow == types.FlowException {
			if result.Flow == types.FlowException {
				t.SetState(task.TaskKilled)
				// Send traceback to player
				s.sendTraceback(t, result.Error)
				// Clean up call stack after traceback has been sent
				for len(t.CallStack) > 0 {
					t.PopFrame()
				}
			} else {
				t.SetState(task.TaskCompleted)
			}
			return nil
		}
	}

	t.SetState(task.TaskCompleted)
	return nil
}

// QueueTask adds a task to the scheduler
func (s *Scheduler) QueueTask(t *task.Task) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	t.SetState(task.TaskQueued)
	s.tasks[t.ID] = t
	heap.Push(s.waiting, t)

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
func (s *Scheduler) CreateVerbTask(player types.ObjID, match *VerbMatch, cmd *ParsedCommand) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	t := task.NewTaskFull(taskID, player, match.Verb.Program.Statements, 300000, 5.0)
	t.StartTime = time.Now()
	// Set wizard flag based on player
	t.Context.IsWizard = s.isWizard(player)

	// Set up verb context
	t.VerbName = match.Verb.Name
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
func (s *Scheduler) CreateForkedTask(parent *task.Task, forkInfo *types.ForkInfo) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)

	// Cast Body back to []parser.Stmt
	body, ok := forkInfo.Body.([]parser.Stmt)
	if !ok {
		// Should not happen - return 0 as error indicator
		return 0
	}

	// Create child task with forked task limits
	t := task.NewTaskFull(taskID, forkInfo.Player, body, 300000, 3.0)
	t.StartTime = time.Now().Add(forkInfo.Delay)
	t.Kind = task.TaskForked
	t.IsForked = true
	t.ForkInfo = forkInfo
	t.Programmer = parent.Programmer // Inherit permissions
	t.This = forkInfo.ThisObj
	t.Caller = forkInfo.Caller
	t.VerbName = forkInfo.Verb
	t.ForkCreator = s // Give child access to scheduler for nested forks

	// Set up child's context
	t.Context.ThisObj = forkInfo.ThisObj
	t.Context.Player = forkInfo.Player
	t.Context.Programmer = parent.Programmer
	t.Context.Verb = forkInfo.Verb
	t.Context.IsWizard = s.isWizard(parent.Programmer)

	// Create evaluator with copied variable environment
	childEnv := vm.NewEnvironment()
	for k, v := range forkInfo.Variables {
		childEnv.Set(k, v)
	}
	t.Evaluator = vm.NewEvaluatorWithEnvAndStore(childEnv, s.store)

	return s.QueueTask(t)
}

// CallVerb synchronously executes a verb on an object and returns the result
// This is used for server hooks like do_login_command, user_connected, etc.
// Returns a Result with a call stack for traceback formatting
func (s *Scheduler) CallVerb(objID types.ObjID, verbName string, args []types.Value, player types.ObjID) types.Result {
	// Create a lightweight task FIRST for call stack tracking
	// This ensures we have a stack even if verb lookup fails
	t := &task.Task{
		Owner:      player,
		Programmer: player, // Will be updated to verb owner if verb found
		CallStack:  make([]task.ActivationFrame, 0),
		TaskLocal:  types.NewEmptyMap(), // Initialize task_local to empty map
	}

	// Look up the verb to get its owner for programmer permissions
	verb, _, err := s.store.FindVerb(objID, verbName)
	if err != nil || verb == nil {
		// Verb not found - but we still want to track the attempted call
		ctx := types.NewTaskContext()
		ctx.Player = player
		ctx.Programmer = player
		ctx.IsWizard = s.isWizard(player)
		ctx.ThisObj = objID
		ctx.Verb = verbName
		ctx.Task = t // Attach task so CallVerb can track the failed call

		result := s.evaluator.CallVerb(objID, verbName, args, ctx)
		// Extract call stack BEFORE popping frames
		if result.Flow == types.FlowException {
			result.CallStack = t.GetCallStack()
		}
		// Clean up call stack
		if len(t.CallStack) > 0 {
			t.PopFrame()
		}
		return result
	}

	// Update programmer to verb owner now that we found the verb
	t.Programmer = verb.Owner

	ctx := types.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = verb.Owner // Programmer is verb owner, not player
	ctx.IsWizard = s.isWizard(verb.Owner) // Set wizard flag based on verb owner
	ctx.ThisObj = objID
	ctx.Verb = verbName
	ctx.Task = t // Attach task so CallVerb can track frames

	result := s.evaluator.CallVerb(objID, verbName, args, ctx)

	// Extract call stack BEFORE popping frames
	// If there was an exception, attach the call stack to the result
	if result.Flow == types.FlowException {
		result.CallStack = t.GetCallStack()
	}

	// Now clean up the call stack for successful calls
	// For errors, we've already extracted the stack, so popping is safe
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
	// Parse the code
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()

	// Type assert to get full eval connection interface
	c, ok := conn.(evalConnection)
	if !ok {
		return // Can't send output without proper connection
	}

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

	// Create a task for call stack tracking and pass() support
	t := &task.Task{
		Owner:      player,
		Programmer: player,
		CallStack:  make([]task.ActivationFrame, 0),
		TaskLocal:  types.NewEmptyMap(), // Initialize task_local to empty map
	}
	ctx.Task = t

	// Create evaluator for execution
	eval := vm.NewEvaluatorWithStore(s.store)
	result := eval.EvalStatements(stmts, ctx)

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

// isWizard checks if an object has wizard permissions
func (s *Scheduler) isWizard(objID types.ObjID) bool {
	obj := s.store.Get(objID)
	if obj == nil {
		return false
	}
	return obj.Flags.Has(db.FlagWizard)
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
