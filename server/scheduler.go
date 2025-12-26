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
	task := NewTask(taskID, player, code, 60000, 5*time.Second)
	task.StartTime = time.Now()
	return s.QueueTask(task)
}

// CreateBackgroundTask creates a background task (fork)
func (s *Scheduler) CreateBackgroundTask(player types.ObjID, code []parser.Stmt, delay time.Duration) int64 {
	taskID := atomic.AddInt64(&s.nextTaskID, 1)
	task := NewTask(taskID, player, code, 30000, 3*time.Second)
	task.StartTime = time.Now().Add(delay)
	return s.QueueTask(task)
}

// Fork creates a forked task with a delay
func (s *Scheduler) Fork(ctx *types.TaskContext, code []parser.Stmt, delay time.Duration) int64 {
	return s.CreateBackgroundTask(ctx.Player, code, delay)
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
