package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"barn/builtins"
	"barn/config"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/metrics"
	"barn/task"
	"barn/types"
	"barn/vm"
)

// Scheduler manages task and VM execution.
type Scheduler struct {
	tasks map[int64]*task.Task
	// executingTasks counts physical VM-execution leases by task ID. Counts, rather
	// than a boolean set, remain truthful if the duplicate-dispatch defect tracked
	// separately in #30 runs the same task twice. Logical task state may become
	// suspended before runTask finishes mutating and publishing its VM. Every access
	// is protected by mu.
	executingTasks map[int64]int
	// gcSweepMu serializes every anonymous/waif sweep. vmStartMu closes the
	// root-snapshot gap by preventing a new VM execution lease from being
	// published until the sweep and its recycle hooks are complete. The strict
	// lock order is gcSweepMu -> vmStartMu -> mu -> task locks.
	gcSweepMu sync.Mutex
	vmStartMu sync.Mutex
	// Context ownership is scheduler-private provenance for every leased VM path.
	// It lets nested verb calls inherit an existing execution lease or sweep
	// barrier instead of acquiring vmStartMu recursively (which would deadlock
	// during recycle hooks).
	executionOwnedContexts  map[*kernel.TaskContext]map[int64]int
	sweepOwnedContexts      map[*kernel.TaskContext]int
	executionStartObserver  func() // test-only observation before vmStartMu acquisition
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

const ambiguousExecutionOwnerID int64 = -1 << 63

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
		tasks:                  make(map[int64]*task.Task),
		executingTasks:         make(map[int64]int),
		executionOwnedContexts: make(map[*kernel.TaskContext]map[int64]int),
		sweepOwnedContexts:     make(map[*kernel.TaskContext]int),
		waiting:                NewTaskQueue(),
		nextTaskID:             1,
		registry:               vm.BuildVMRegistry(),
		store:                  store,
		options:                options,
		taskWork:               make(chan taskWorkItem),
		workerCount:            workerCount,
		ctx:                    ctx,
		cancel:                 cancel,
	}

	s.registry.SetVerbCaller(func(objID types.ObjID, verbName string, args []types.Value, tc *kernel.TaskContext) types.Result {
		player := types.ObjNothing
		if tc != nil {
			player = tc.Player
			if player == types.ObjNothing {
				player = tc.Programmer
			}
		}
		if tc != nil && tc.StoreTxn != nil && tc.Task != nil {
			return s.CallVerbInContext(objID, verbName, args, tc)
		}
		// A no-transaction recycle context needs standalone server-hook semantics,
		// but inherits the sweep barrier instead of acquiring vmStartMu. Transaction
		// contexts retain CallVerbInContext's shared transaction/caller semantics;
		// that path handles sweep ownership itself.
		if s.isSweepOwnedContext(tc) {
			return s.callVerbWithArgstr(objID, verbName, args, player, "", vmOwnershipSweep, 0)
		}
		if ownerID, claimed, attributable := s.executionContextClaim(tc); claimed {
			if !attributable {
				ownerID = ambiguousExecutionOwnerID
			}
			return s.callVerbWithArgstr(objID, verbName, args, player, "", vmOwnershipExecution, ownerID)
		}
		return s.CallVerb(objID, verbName, args, player)
	})
	s.registry.SetRunGCFunc(func(ctx *kernel.TaskContext) error {
		// A recycle hook may call run_gc() while its owning sweep is still active.
		// Re-entry is a successful no-op; blocking here would self-deadlock.
		if !s.gcSweepMu.TryLock() {
			return nil
		}
		defer s.gcSweepMu.Unlock()
		s.vmStartMu.Lock()
		defer s.vmStartMu.Unlock()

		var current *task.Task
		if ctx != nil {
			current, _ = ctx.Task.(*task.Task)
		}
		if ownerID, claimed, attributable := s.executionContextClaim(ctx); claimed && !attributable {
			return nil
		} else if claimed {
			current = &task.Task{ID: ownerID}
		}
		siblingAnon, ok := s.collectExplicitGlobalGCSiblingRefs(current)
		if !ok {
			return nil
		}
		s.acquireSweepContext(ctx)
		defer s.releaseSweepContext(ctx)
		vm.AutoRecycleOrphanAnonymousSince(store, s.registry, ctx, 0, siblingAnon)
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

// newTaskID allocates the next task id. Every task in the server is born here,
// which makes it the one place worth counting them.
func (s *Scheduler) newTaskID() int64 {
	metrics.TasksStarted.Add(1)
	return atomic.AddInt64(&s.nextTaskID, 1)
}

// LiveTaskCount reports how many tasks the scheduler is currently holding.
func (s *Scheduler) LiveTaskCount() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.tasks))
}

func (s *Scheduler) acquireTaskExecution(t *task.Task) {
	if s.executionStartObserver != nil {
		s.executionStartObserver()
	}
	s.vmStartMu.Lock()
	defer s.vmStartMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executingTasks[t.ID]++
	t.SetState(task.TaskRunning)
}

func (s *Scheduler) releaseTaskExecution(taskID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if count := s.executingTasks[taskID]; count > 1 {
		s.executingTasks[taskID] = count - 1
	} else {
		delete(s.executingTasks, taskID)
	}
}

// acquireInheritedTaskExecution records a synchronously nested standalone VM
// under an execution lease that already passed vmStartMu. It must not acquire
// vmStartMu again, but the extra refcount makes run_gc fail closed while both
// the outer and inner VMs hold roots and TaskContext.CallerVM names only one.
func (s *Scheduler) acquireInheritedTaskExecution(taskID int64) {
	s.mu.Lock()
	s.executingTasks[taskID]++
	s.mu.Unlock()
}

func (s *Scheduler) acquireExecutionContext(ctx *kernel.TaskContext, taskID int64) {
	if ctx == nil {
		return
	}
	s.mu.Lock()
	owners := s.executionOwnedContexts[ctx]
	if owners == nil {
		owners = make(map[int64]int)
		s.executionOwnedContexts[ctx] = owners
	}
	owners[taskID]++
	s.mu.Unlock()
}

func (s *Scheduler) releaseExecutionContext(ctx *kernel.TaskContext, taskID int64) {
	if ctx == nil {
		return
	}
	s.mu.Lock()
	owners := s.executionOwnedContexts[ctx]
	if count := owners[taskID]; count > 1 {
		owners[taskID] = count - 1
	} else if count == 1 {
		delete(owners, taskID)
		if len(owners) == 0 {
			delete(s.executionOwnedContexts, ctx)
		}
	}
	s.mu.Unlock()
}

func (s *Scheduler) executionContextOwner(ctx *kernel.TaskContext) (int64, bool) {
	ownerID, claimed, attributable := s.executionContextClaim(ctx)
	return ownerID, claimed && attributable
}

// executionContextClaim distinguishes an unowned context from one whose
// physical execution owner is ambiguous. Ambiguity makes global GC a
// successful no-op; treating it as absent could fall back to ctx.Task and
// exclude the wrong sole lease.
func (s *Scheduler) executionContextClaim(ctx *kernel.TaskContext) (ownerID int64, claimed, attributable bool) {
	if ctx == nil {
		return 0, false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	owners := s.executionOwnedContexts[ctx]
	if len(owners) == 0 {
		return 0, false, false
	}
	if len(owners) != 1 {
		return 0, true, false
	}
	for id := range owners {
		return id, true, true
	}
	return 0, false, false
}

func (s *Scheduler) acquireSweepContext(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	s.mu.Lock()
	s.sweepOwnedContexts[ctx]++
	s.mu.Unlock()
}

func (s *Scheduler) releaseSweepContext(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	s.mu.Lock()
	if count := s.sweepOwnedContexts[ctx]; count > 1 {
		s.sweepOwnedContexts[ctx] = count - 1
	} else {
		delete(s.sweepOwnedContexts, ctx)
	}
	s.mu.Unlock()
}

func (s *Scheduler) isSweepOwnedContext(ctx *kernel.TaskContext) bool {
	if ctx == nil {
		return false
	}
	s.mu.Lock()
	owned := s.sweepOwnedContexts[ctx] > 0
	s.mu.Unlock()
	return owned
}

func (s *Scheduler) populateTaskContextDependencies(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	ctx.Store = s.store
	ctx.Registry = s.registry
	ctx.RuntimeOptions = s.options
	// With() formats these once per task rather than once per record.
	ctx.Log = slog.Default().With(
		slog.Int64("task_id", ctx.TaskID),
		slog.Int64("player", int64(ctx.Player)),
		slog.String("verb", ctx.Verb))
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

// ProcessReadyTasks executes at most one task that is ready to run.
//
// The input processor owns the outer scheduling loop. Returning after each
// task lets that loop service socket input between runnable tasks instead of
// draining an arbitrarily large ready snapshot first. A single MOO task still
// runs atomically until completion or suspension.
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
			slog.Error("task error",
				slog.Int64("task_id", t.ID),
				slog.Int64("this", int64(t.This)),
				slog.String("verb", t.VerbName),
				slog.Any("err", result.err))
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
// An actively executing task is therefore fatal rather than skippable. Its VM is being
// mutated on another goroutine, so it can neither be walked here nor ignored (its locals
// are roots). Logical TaskRunning is insufficient because suspend records a suspended
// state before VM handoff completes. When any physical execution lease is active, the
// caller leaves the batches queued for the next quiescent pass.
func (s *Scheduler) collectAllGCRefs() (anonRefs map[types.ObjID]struct{}, waifRefs []types.Value, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.executingTasks) != 0 {
		return nil, nil, false
	}
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

// collectExplicitGlobalGCSiblingRefs snapshots the anonymous references held by
// every live task other than the run_gc() caller. A global sweep cannot safely
// inspect an actively executing sibling's mutating VM, and unlike per-task floor
// GC it also cannot prove that the sibling has no reference to an existing
// candidate. It therefore fails closed on any other physical execution lease.
func (s *Scheduler) collectExplicitGlobalGCSiblingRefs(exclude *task.Task) (anonRefs map[types.ObjID]struct{}, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, count := range s.executingTasks {
		if count > 0 && (exclude == nil || id != exclude.ID || count != 1) {
			return nil, false
		}
	}
	anonRefs = make(map[types.ObjID]struct{})
	for _, sibling := range s.tasks {
		if sibling == nil || (exclude != nil && sibling.ID == exclude.ID) {
			continue
		}
		switch sibling.GetState() {
		case task.TaskCompleted, task.TaskKilled:
			continue
		case task.TaskRunning:
			return nil, false
		}
		if exec, isVM := sibling.BytecodeVMValue().(*vm.VM); isVM && exec != nil {
			vm.CollectAnonymousRefsFromVM(exec, anonRefs)
		}
	}
	return anonRefs, true
}

// collectSiblingGCRefs snapshots the anonymous-object and waif references held by
// every OTHER task's saved VM, for use when `exclude` runs orphan GC / waif
// finalization. The references are collected here, under s.mu, rather than by handing
// VM pointers back to be walked after the lock is dropped — that is the whole point:
//
//   - A physically executing sibling may be mutating its VM on another goroutine even
//     after it has logically suspended, so the collection fails closed. Skipping it
//     would omit roots it acquired after this caller's floor was sampled.
//   - A queued/suspended sibling's VM is read here while s.mu is held. Because task
//     dispatch and physical execution-lease acquisition both take s.mu, no sibling
//     can begin executing — and thus begin mutating its VM — while we read it. Walking
//     the pointers after releasing s.mu would race exactly that transition.
func (s *Scheduler) collectSiblingGCRefs(exclude *task.Task) (anonRefs map[types.ObjID]struct{}, waifRefs []types.Value, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, count := range s.executingTasks {
		if count > 0 && (exclude == nil || id != exclude.ID || count != 1) {
			return nil, nil, false
		}
	}
	anonRefs = make(map[types.ObjID]struct{})
	for _, queued := range s.tasks {
		if queued == nil || (exclude != nil && queued.ID == exclude.ID) {
			continue
		}
		state := queued.GetState()
		if state == task.TaskCompleted || state == task.TaskKilled {
			continue
		}
		if state == task.TaskRunning {
			return nil, nil, false
		}
		if exec, ok := queued.BytecodeVMValue().(*vm.VM); ok && exec != nil {
			vm.CollectAnonymousRefsFromVM(exec, anonRefs)
			vm.CollectWaifsFromVM(exec, &waifRefs)
		}
	}
	return anonRefs, waifRefs, true
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
