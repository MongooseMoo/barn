package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"barn/builtins"
	"barn/config"
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
	options                 config.Options
	mu                      sync.Mutex
	ctx                     context.Context
	cancel                  context.CancelFunc
}

// NewScheduler creates a task scheduler with default runtime options.
func NewScheduler(store *dbstore.Store) *Scheduler {
	return NewSchedulerWithOptions(store, config.DefaultOptions())
}

// NewSchedulerWithOptions creates a task scheduler with the supplied runtime options.
func NewSchedulerWithOptions(store *dbstore.Store, options config.Options) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		tasks:      make(map[int64]*task.Task),
		waiting:    NewTaskQueue(),
		nextTaskID: 1,
		registry:   vm.BuildVMRegistry(),
		store:      store,
		options:    options,
		ctx:        ctx,
		cancel:     cancel,
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

	return s
}

func (s *Scheduler) populateTaskContextDependencies(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	ctx.Store = s.store
	ctx.Registry = s.registry
	ctx.RuntimeOptions = s.options
}

// Stop cancels scheduler-owned task contexts.
func (s *Scheduler) Stop() {
	s.cancel()
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

	for _, t := range readyTasks {
		err := s.runTask(t)
		if err != nil {
			log.Printf("Task %d (#%d:%s) error: %v", t.ID, t.This, t.VerbName, err)
		}

		if s.taskOutputFlusher != nil {
			s.taskOutputFlusher(t.Owner, t.CommandOutputSuffix)
		}

		if t.Done != nil {
			close(t.Done)
		}
	}
	return len(readyTasks)
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
