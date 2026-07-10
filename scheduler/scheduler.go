package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"log"
	"runtime"
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
	taskWork                chan taskWorkItem
	workersWG               sync.WaitGroup
	workerCount             int
	mu                      sync.Mutex
	ctx                     context.Context
	cancel                  context.CancelFunc

	// Deferred GC: task completion/suspend enqueue their pending waifs and
	// orphan-anonymous collection requests here instead of paying a full-db
	// sweep per task; flushDeferredGC settles both batches on an interval.
	pendingWaifMu    sync.Mutex
	pendingWaifBatch []pendingWaifEntry
	pendingAnonGC    []vm.AnonGCRequest
	lastGCSweep      time.Time
	lastGCCost       time.Duration
}

type taskWorkItem struct {
	task    *task.Task
	results chan<- taskRunResult
}

type taskRunResult struct {
	task *task.Task
	err  error
}

// NewScheduler creates a task scheduler with default runtime options.
func NewScheduler(store *dbstore.Store) *Scheduler {
	return NewSchedulerWithOptions(store, config.DefaultOptions())
}

// NewSchedulerWithOptions creates a task scheduler with the supplied runtime options.
func NewSchedulerWithOptions(store *dbstore.Store, options config.Options) *Scheduler {
	return newSchedulerWithWorkerCount(store, options, runtime.GOMAXPROCS(0))
}

func newSchedulerWithWorkerCount(store *dbstore.Store, options config.Options, workerCount int) *Scheduler {
	if workerCount < 1 {
		workerCount = 1
	}
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		tasks:       make(map[int64]*task.Task),
		waiting:     NewTaskQueue(),
		nextTaskID:  1,
		registry:    vm.BuildVMRegistry(),
		store:       store,
		options:     options,
		taskWork:    make(chan taskWorkItem),
		workerCount: workerCount,
		ctx:         ctx,
		cancel:      cancel,
	}

	s.registry.SetVerbCaller(func(objID types.ObjID, verbName string, args []types.Value, tc *kernel.TaskContext) types.Result {
		if tc != nil && tc.StoreTxn != nil && tc.Task != nil {
			return s.CallVerbInContext(objID, verbName, args, tc)
		}
		player := types.ObjNothing
		if tc != nil {
			player = tc.Player
			if player == types.ObjNothing {
				player = tc.Programmer
			}
		}
		return s.CallVerb(objID, verbName, args, player)
	})
	s.registry.SetRunGCFunc(func(ctx *kernel.TaskContext) error {
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

// Registry returns the scheduler's builtin registry so the owning server can
// wire host capabilities (connection manager, lifecycle hooks) onto it.
func (s *Scheduler) Registry() *builtins.Registry { return s.registry }

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

		if t.GetState() == task.TaskQueued && (t.StmtIndex > 0 || t.BytecodeVMValue() != nil) {
			if (t.WakeTime.IsZero() || !t.WakeTime.After(now)) && !t.StartTime.After(now) {
				readyTasks = append(readyTasks, t)
			}
		}
	}

	s.mu.Unlock()

	s.runReadyTasks(readyTasks)
	// Every task in the pass has joined by now (runTaskBatch waits on all of them),
	// so no task VM is being mutated. This is the point where the deferred-GC
	// batches are guaranteed to see complete roots.
	s.flushDeferredGC()
	return len(readyTasks)
}

func (s *Scheduler) runReadyTasks(readyTasks []*task.Task) {
	if len(readyTasks) == 0 {
		return
	}
	for _, batch := range s.readyTaskBatches(readyTasks) {
		s.runTaskBatch(batch)
	}
}

type readyTaskCandidate struct {
	task      *task.Task
	footprint accessFootprint
	// optimistic reports whether this task may be co-scheduled with others even
	// without a proven-commuting footprint, relying on commit-time conflict
	// detection and retry. Only fresh AST tasks (taskIsConflictRetryable) qualify.
	optimistic bool
}

func (s *Scheduler) readyTaskBatches(readyTasks []*task.Task) [][]*task.Task {
	if s.workerCount <= 1 {
		batches := make([][]*task.Task, 0, len(readyTasks))
		for _, t := range readyTasks {
			batches = append(batches, []*task.Task{t})
		}
		return batches
	}

	var batches [][]*task.Task
	var current []readyTaskCandidate
	flush := func() {
		if len(current) == 0 {
			return
		}
		batch := make([]*task.Task, 0, len(current))
		for _, candidate := range current {
			batch = append(batch, candidate.task)
		}
		batches = append(batches, batch)
		current = nil
	}

	for _, t := range readyTasks {
		candidate := readyTaskCandidate{
			task:       t,
			footprint:  analyzeTaskAccessFootprint(t),
			optimistic: taskIsConflictRetryable(t),
		}
		if !candidateCanJoinBatch(candidate, current) {
			flush()
		}
		current = append(current, candidate)
		if len(current) >= s.workerCount {
			flush()
		}
	}
	flush()
	return batches
}

// candidateCanJoinBatch decides whether a ready task may run in parallel with the
// tasks already gathered for the current batch. Two regimes:
//
//   - Both sides have a known property footprint: require PROVEN commutativity, so
//     these tasks never conflict at commit (the conflict-free fast path).
//   - Either side is "unknown" (a verb call, fork, opaque builtin, or dynamic
//     property target — i.e. almost every real command): fall back to OPTIMISTIC
//     co-scheduling, which is only sound when every task in the batch is conflict
//     -retryable. If two of them happen to write the same datum, the loser's commit
//     fails read-set validation and it re-runs against the winner's writes. A
//     non-retryable task (resumed/forked) cannot re-run, so it stays solo.
func candidateCanJoinBatch(candidate readyTaskCandidate, batch []readyTaskCandidate) bool {
	if len(batch) == 0 {
		return true
	}
	if candidate.footprint.unknown || batchHasUnknownFootprint(batch) {
		if !candidate.optimistic {
			return false
		}
		for _, existing := range batch {
			if !existing.optimistic {
				return false
			}
		}
		return true
	}
	for _, existing := range batch {
		if !accessFootprintsCommute(candidate.footprint, existing.footprint) {
			return false
		}
	}
	return true
}

func batchHasUnknownFootprint(batch []readyTaskCandidate) bool {
	for _, existing := range batch {
		if existing.footprint.unknown {
			return true
		}
	}
	return false
}

func (s *Scheduler) runTaskBatch(readyTasks []*task.Task) {
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

		// runTask returns nil for both suspend/yield and terminal completion.
		// Only signal Done when the task has actually terminated (Completed or
		// Killed); a merely suspended task is still alive and will be closed
		// later when it truly finishes. CloseDone guards against double-close.
		if state := t.GetState(); state == task.TaskCompleted || state == task.TaskKilled {
			t.CloseDone()
		}
	}
}

func (s *Scheduler) YieldReadyTasks() int {
	return s.ProcessReadyTasks()
}

// collectAllGCRefs snapshots the anonymous-object and waif references held by every
// live task's saved VM, for the deferred-GC flush. It is the flush-time analogue of
// collectSiblingGCRefs, with one stricter requirement: the flush must see EVERY live
// task's locals as roots, not merely the ones it happens to be able to read.
//
// A running task is therefore fatal rather than skippable. Its VM is being mutated on
// another goroutine, so it can neither be walked here (the data race collectSiblingGCRefs
// exists to avoid) nor ignored (its locals are roots). The per-task inline sweep could
// skip running siblings because a task's own uncommitted creations are not yet visible
// to them; a deferred batch has no such argument, because the tasks that enqueued it
// have since committed. So when any task is running we report ok=false and the caller
// leaves the batches queued for the next quiescent pass.
func (s *Scheduler) collectAllGCRefs() (anonRefs map[types.ObjID]struct{}, waifRefs []types.Value, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	anonRefs = make(map[types.ObjID]struct{})
	for _, t := range s.tasks {
		if t == nil {
			continue
		}
		switch t.GetState() {
		case task.TaskCompleted, task.TaskKilled:
			continue
		case task.TaskRunning:
			return nil, nil, false
		}
		if exec, isVM := t.BytecodeVMValue().(*vm.VM); isVM && exec != nil {
			vm.CollectAnonymousRefsFromVM(exec, anonRefs)
			vm.CollectWaifsFromVM(exec, &waifRefs)
		}
	}
	return anonRefs, waifRefs, true
}

// collectSiblingGCRefs snapshots the anonymous-object and waif references held by
// every OTHER task's saved VM, for use when `exclude` runs orphan GC / waif
// finalization. The references are collected here, under s.mu, rather than by handing
// VM pointers back to be walked after the lock is dropped — that is the whole point:
//
//   - A running sibling is actively mutating its VM on another goroutine, so it is
//     skipped (orphan GC never recycles objects a concurrent sibling could hold, since
//     this task's current-slice creations are not yet committed or handed off).
//   - A queued/suspended sibling's VM is read here while s.mu is held. Because task
//     dispatch (ProcessReadyTasks) and the Running transition in runTask both take
//     s.mu, no sibling can begin executing — and thus begin mutating its VM — while we
//     read it. Walking the pointers after releasing s.mu would race exactly that
//     transition.
func (s *Scheduler) collectSiblingGCRefs(exclude *task.Task) (anonRefs map[types.ObjID]struct{}, waifRefs []types.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()

	anonRefs = make(map[types.ObjID]struct{})
	for _, queued := range s.tasks {
		if queued == nil || (exclude != nil && queued.ID == exclude.ID) {
			continue
		}
		state := queued.GetState()
		if state == task.TaskCompleted || state == task.TaskKilled || state == task.TaskRunning {
			continue
		}
		if exec, ok := queued.BytecodeVMValue().(*vm.VM); ok && exec != nil {
			vm.CollectAnonymousRefsFromVM(exec, anonRefs)
			vm.CollectWaifsFromVM(exec, &waifRefs)
		}
	}
	return anonRefs, waifRefs
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
