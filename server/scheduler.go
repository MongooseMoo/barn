package server

import (
	"barn/db"
	"barn/vm"
	"barn/parser"
	"barn/types"
	"container/heap"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Scheduler manages task execution
type Scheduler struct {
	tasks       map[int64]*Task
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
		tasks:      make(map[int64]*Task),
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
	var readyTasks []*Task

	// Collect all ready tasks
	for s.waiting.Len() > 0 {
		task := s.waiting.Peek()
		if task.StartTime.After(now) {
			break // Tasks are ordered by start time
		}
		heap.Pop(s.waiting)
		readyTasks = append(readyTasks, task)
	}

	s.mu.Unlock()

	// Execute ready tasks (outside the lock to allow concurrency)
	for _, task := range readyTasks {
		// Run task in a goroutine
		go func(t *Task) {
			err := t.Run(s.ctx, s.evaluator)
			if err != nil {
				// Log error (in real implementation)
				_ = err
			}

			// Flush output buffer for the player
			if s.connManager != nil {
				if conn := s.connManager.GetConnection(t.Player); conn != nil {
					conn.Flush()
				}
			}
		}(task)
	}
}

// QueueTask adds a task to the scheduler
func (s *Scheduler) QueueTask(task *Task) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	task.State = TaskWaiting
	s.tasks[task.ID] = task
	heap.Push(s.waiting, task)

	return task.ID
}

// CreateForegroundTask creates a foreground task (user command)
func (s *Scheduler) CreateForegroundTask(player types.ObjID, code []parser.Stmt) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	task := NewTask(taskID, player, code, 300000, 5*time.Second)
	task.StartTime = time.Now()
	task.Scheduler = s // Give task access to scheduler for forks
	return s.QueueTask(task)
}

// CreateVerbTask creates a task to execute a verb
func (s *Scheduler) CreateVerbTask(player types.ObjID, match *VerbMatch, cmd *ParsedCommand) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	task := NewTask(taskID, player, match.Verb.Program.Statements, 300000, 5*time.Second)
	task.StartTime = time.Now()

	// Set up verb context
	task.VerbName = match.Verb.Name
	task.This = match.This
	task.Caller = player
	task.Argstr = cmd.Argstr
	task.Args = cmd.Args
	task.Dobjstr = cmd.Dobjstr
	task.Dobj = cmd.Dobj
	task.Prepstr = cmd.Prepstr
	task.Iobjstr = cmd.Iobjstr
	task.Iobj = cmd.Iobj
	task.Scheduler = s // Give task access to scheduler for forks

	return s.QueueTask(task)
}

// CreateBackgroundTask creates a background task (fork)
func (s *Scheduler) CreateBackgroundTask(player types.ObjID, code []parser.Stmt, delay time.Duration) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	task := NewTask(taskID, player, code, 300000, 3*time.Second)
	task.StartTime = time.Now().Add(delay)
	task.Scheduler = s // Give task access to scheduler for forks
	return s.QueueTask(task)
}

// Fork creates a forked task with a delay
func (s *Scheduler) Fork(ctx *types.TaskContext, code []parser.Stmt, delay time.Duration) int64 {
	return s.CreateBackgroundTask(ctx.Player, code, delay)
}

// CreateForkedTask creates a forked child task from fork statement
func (s *Scheduler) CreateForkedTask(parent *Task, forkInfo *types.ForkInfo) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)

	// Cast Body back to []parser.Stmt
	body, ok := forkInfo.Body.([]parser.Stmt)
	if !ok {
		// Should not happen - return 0 as error indicator
		return 0
	}

	// Create child task with forked task limits
	task := NewTask(taskID, forkInfo.Player, body, 300000, 3*time.Second)
	task.StartTime = time.Now().Add(forkInfo.Delay)
	task.IsForked = true
	task.ForkInfo = forkInfo
	task.Programmer = parent.Programmer // Inherit permissions
	task.This = forkInfo.ThisObj
	task.Caller = forkInfo.Caller
	task.VerbName = forkInfo.Verb
	task.Scheduler = s // Give child access to scheduler for nested forks

	// Set up child's context
	task.Context.ThisObj = forkInfo.ThisObj
	task.Context.Player = forkInfo.Player
	task.Context.Programmer = parent.Programmer
	task.Context.Verb = forkInfo.Verb

	// Create evaluator with copied variable environment
	childEnv := vm.NewEnvironment()
	for k, v := range forkInfo.Variables {
		childEnv.Set(k, v)
	}
	task.Evaluator = vm.NewEvaluatorWithEnvAndStore(childEnv, s.store)

	return s.QueueTask(task)
}

// CallVerb synchronously executes a verb on an object and returns the result
// This is used for server hooks like do_login_command, user_connected, etc.
func (s *Scheduler) CallVerb(objID types.ObjID, verbName string, args []types.Value, player types.ObjID) types.Result {
	// Look up the verb to get its owner for programmer permissions
	verb, _, err := s.store.FindVerb(objID, verbName)
	if err != nil || verb == nil {
		return types.Result{
			Flow:  types.FlowException,
			Error: types.E_VERBNF,
		}
	}

	ctx := types.NewTaskContext()
	ctx.Player = player
	ctx.Programmer = verb.Owner // Programmer is verb owner, not player
	ctx.IsWizard = s.isWizard(verb.Owner) // Set wizard flag based on verb owner
	ctx.ThisObj = objID
	ctx.Verb = verbName

	return s.evaluator.CallVerb(objID, verbName, args, ctx)
}

// ResumeTask resumes a suspended task
func (s *Scheduler) ResumeTask(taskID int64, value types.Value) error {
	s.mu.Lock()
	task, exists := s.tasks[taskID]
	s.mu.Unlock()

	if !exists {
		return ErrNotSuspended
	}

	return task.Resume(value)
}

// KillTask kills a running task
func (s *Scheduler) KillTask(taskID int64, killerID types.ObjID) error {
	s.mu.Lock()
	task, exists := s.tasks[taskID]
	s.mu.Unlock()

	if !exists {
		return ErrNotSuspended
	}

	// Permission check
	if task.Player != killerID && !s.isWizard(killerID) {
		return ErrPermission
	}

	task.Kill()
	return nil
}

// GetTask retrieves a task by ID
func (s *Scheduler) GetTask(taskID int64) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[taskID]
}

// QueuedTasks returns list of queued tasks
func (s *Scheduler) QueuedTasks() []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]*Task, 0)
	for _, task := range s.tasks {
		if task.GetState() == TaskWaiting {
			tasks = append(tasks, task)
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

// TaskQueue is a priority queue for tasks ordered by start time
type TaskQueue []*Task

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
	*tq = append(*tq, x.(*Task))
}

func (tq *TaskQueue) Pop() interface{} {
	old := *tq
	n := len(old)
	item := old[n-1]
	*tq = old[0 : n-1]
	return item
}

func (tq TaskQueue) Peek() *Task {
	if len(tq) == 0 {
		return nil
	}
	return tq[0]
}
