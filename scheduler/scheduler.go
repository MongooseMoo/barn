package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"barn/builtins"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
	"barn/vm"
)

// Scheduler manages task and VM execution.
type Scheduler struct {
	tasks                   map[int64]*task.Task
	waiting                 *TaskQueue
	nextTaskID              int64
	queueSeq                int64
	registry                *builtins.Registry
	store                   *dbstore.Store
	pendingFinalizationSink func([]types.Value)
	taskLineSender          func(types.ObjID, string)
	tracebackSender         func(types.ObjID, types.ErrorCode, []task.ActivationFrame)
	taskOutputFlusher       func(types.ObjID, string)
	promoteNumbers          bool
	taskWork                chan taskWorkItem
	workersWG               sync.WaitGroup
	workerCount             int
	mu                      sync.Mutex
	ctx                     context.Context
	cancel                  context.CancelFunc
}

type taskWorkItem struct {
	task    *task.Task
	results chan<- taskRunResult
}

type taskRunResult struct {
	task *task.Task
	err  error
}

// NewScheduler creates a task scheduler with strict (default) numeric semantics.
func NewScheduler(store *dbstore.Store) *Scheduler {
	return NewSchedulerWithOptions(store, false)
}

// NewSchedulerWithOptions creates a task scheduler. When promoteNumbers is true,
// mixed int/float arithmetic and comparison auto-promote (ToastStunt mongoose
// PROMOTE_NUMBERS); when false, strict E_TYPE behavior is used.
func NewSchedulerWithOptions(store *dbstore.Store, promoteNumbers bool) *Scheduler {
	return newSchedulerWithWorkerCount(store, promoteNumbers, 1)
}

func newSchedulerWithWorkerCount(store *dbstore.Store, promoteNumbers bool, workerCount int) *Scheduler {
	if workerCount < 1 {
		workerCount = 1
	}
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		tasks:          make(map[int64]*task.Task),
		waiting:        NewTaskQueue(),
		nextTaskID:     1,
		registry:       vm.BuildVMRegistry(),
		store:          store,
		promoteNumbers: promoteNumbers,
		taskWork:       make(chan taskWorkItem),
		workerCount:    workerCount,
		ctx:            ctx,
		cancel:         cancel,
	}

	s.registry.SetVerbCaller(func(objID types.ObjID, verbName string, args []types.Value, tc *kernel.TaskContext) types.Result {
		player := types.ObjNothing
		if tc != nil {
			player = tc.Player
			if player == types.ObjNothing {
				player = tc.Programmer
			}
		}
		return s.CallVerb(objID, verbName, args, player)
	})
	builtins.SetRunGCFunc(func(ctx *kernel.TaskContext) error {
		vm.AutoRecycleOrphanAnonymousWith(store, s.registry, ctx)
		return nil
	})
	s.startWorkers()

	return s
}

func (s *Scheduler) startWorkers() {
	for i := 0; i < s.workerCount; i++ {
		s.workersWG.Add(1)
		go s.workerLoop()
	}
}

func (s *Scheduler) workerLoop() {
	defer s.workersWG.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case work := <-s.taskWork:
			work.results <- taskRunResult{
				task: work.task,
				err:  s.runTask(work.task),
			}
		}
	}
}

func (s *Scheduler) populateTaskContextDependencies(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	ctx.Store = s.store
	ctx.Registry = s.registry
	ctx.PromoteNumbers = s.promoteNumbers
}

// Stop cancels scheduler-owned task contexts.
func (s *Scheduler) Stop() {
	s.cancel()
	s.workersWG.Wait()
}

func (s *Scheduler) SetPendingFinalizationSink(sink func([]types.Value)) {
	s.pendingFinalizationSink = sink
}

func (s *Scheduler) SetTaskLineSender(sender func(types.ObjID, string)) {
	s.taskLineSender = sender
}

func (s *Scheduler) SetTracebackSender(sender func(types.ObjID, types.ErrorCode, []task.ActivationFrame)) {
	s.tracebackSender = sender
}

func (s *Scheduler) SetTaskOutputFlusher(flusher func(types.ObjID, string)) {
	s.taskOutputFlusher = flusher
}

// ProcessReadyTasks executes tasks that are ready to run.
func (s *Scheduler) ProcessReadyTasks() int {
	s.mu.Lock()

	now := time.Now()
	var readyTasks []*task.Task

	for s.waiting.Len() > 0 {
		t := s.waiting.Peek()
		if t.StartTime.After(now) {
			break
		}
		heap.Pop(s.waiting)
		if t.GetState() != task.TaskQueued {
			continue
		}
		readyTasks = append(readyTasks, t)
	}

	heapReady := make(map[int64]bool, len(readyTasks))
	for _, t := range readyTasks {
		heapReady[t.ID] = true
	}

	for _, t := range s.tasks {
		if heapReady[t.ID] {
			continue
		}

		if t.WakeDue(now) {
			if t.Resume(types.NewInt(0)) {
				readyTasks = append(readyTasks, t)
			}
			continue
		}

		if t.GetState() == task.TaskQueued && (t.StmtIndex > 0 || t.BytecodeVM != nil) {
			if (t.WakeTime.IsZero() || !t.WakeTime.After(now)) && !t.StartTime.After(now) {
				readyTasks = append(readyTasks, t)
			}
		}
	}

	s.mu.Unlock()

	s.runReadyTasks(readyTasks)
	return len(readyTasks)
}

func (s *Scheduler) runReadyTasks(readyTasks []*task.Task) {
	if len(readyTasks) == 0 {
		return
	}

	results := make(chan taskRunResult, len(readyTasks))
	for _, t := range readyTasks {
		s.taskWork <- taskWorkItem{task: t, results: results}
	}

	byID := make(map[int64]taskRunResult, len(readyTasks))
	for range readyTasks {
		result := <-results
		byID[result.task.ID] = result
	}

	for _, t := range readyTasks {
		result := byID[t.ID]
		if result.err != nil {
			log.Printf("Task %d (#%d:%s) error: %v", t.ID, t.This, t.VerbName, result.err)
		}

		if s.taskOutputFlusher != nil {
			s.taskOutputFlusher(t.Owner, t.CommandOutputSuffix)
		}

		if t.Done != nil {
			close(t.Done)
		}
	}
}

func (s *Scheduler) YieldReadyTasks() int {
	return s.ProcessReadyTasks()
}

func (s *Scheduler) liveTaskVMs(exclude *task.Task) []*vm.VM {
	s.mu.Lock()
	defer s.mu.Unlock()

	var roots []*vm.VM
	for _, queued := range s.tasks {
		if queued == nil || (exclude != nil && queued.ID == exclude.ID) {
			continue
		}
		state := queued.GetState()
		if state == task.TaskCompleted || state == task.TaskKilled {
			continue
		}
		if exec, ok := queued.BytecodeVM.(*vm.VM); ok && exec != nil {
			roots = append(roots, exec)
		}
	}
	return roots
}

func (s *Scheduler) isWizard(objID types.ObjID) bool {
	hasWizard, errCode := s.store.HasObjectFlag(objID, dbstore.FlagWizard)
	return errCode == types.E_NONE && hasWizard
}

var (
	ErrTicksExceeded = errors.New("tick limit exceeded")
	ErrNotSuspended  = errors.New("task not suspended")
	ErrResumeFailed  = errors.New("failed to resume task")
	ErrPermission    = errors.New("permission denied")
)
