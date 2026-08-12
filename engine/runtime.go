// Package engine owns MOO evaluation, verb execution, task lifecycle, retry,
// and runtime garbage-collection coordination.
package engine

import (
	"container/heap"
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

// Runtime manages task and VM execution.
type Runtime struct {
	tasks                   map[int64]*task.Task
	taskManager             *task.Manager
	lifecycle               lifecycleCoordinator
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

// NewRuntime creates an execution runtime with default options.
func NewRuntime(store *dbstore.Store) *Runtime {
	return NewRuntimeWithOptions(store, config.DefaultOptions())
}

// NewRuntimeWithOptions creates an execution runtime with the supplied options.
func NewRuntimeWithOptions(store *dbstore.Store, options config.Options) *Runtime {
	return newRuntimeWithWorkerCount(store, options, runtime.GOMAXPROCS(0))
}

func newRuntimeWithWorkerCount(store *dbstore.Store, options config.Options, workerCount int) *Runtime {
	if workerCount < 1 {
		workerCount = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager := task.NewManager()
	registry := vm.BuildVMRegistry()
	registry.SetTaskManager(manager)

	s := &Runtime{
		tasks:       make(map[int64]*task.Task),
		taskManager: manager,
		lifecycle:   newLifecycleCoordinator(),
		waiting:     NewTaskQueue(),
		nextTaskID:  1,
		registry:    registry,
		store:       store,
		options:     options,
		taskWork:    make(chan taskWorkItem),
		workerCount: workerCount,
		ctx:         ctx,
		cancel:      cancel,
	}

	s.registry.SetVerbCaller(func(objID types.ObjID, verbName string, args []types.Value, execution *builtins.Execution) types.Result {
		var tc *kernel.TaskContext
		if execution != nil {
			tc = execution.TaskContext
		}
		player := types.ObjNothing
		if tc != nil {
			player = tc.Player
			if player == types.ObjNothing {
				player = tc.Programmer
			}
		}
		if tc != nil && tc.StoreTxn != nil && execution.Task != nil {
			return s.CallVerbInContext(objID, verbName, args, execution)
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
	s.registry.SetRunGCFunc(func(execution *builtins.Execution) error {
		ctx := execution.TaskContext
		// A recycle hook may call run_gc() while its owning sweep is still active.
		// Re-entry is a successful no-op; blocking here would self-deadlock.
		if !s.lifecycle.sweepMu.TryLock() {
			return nil
		}
		defer s.lifecycle.sweepMu.Unlock()
		s.lifecycle.vmStartMu.Lock()
		defer s.lifecycle.vmStartMu.Unlock()

		current := execution.Task
		if ownerID, claimed, attributable := s.executionContextClaim(ctx); claimed && !attributable {
			return nil
		} else if claimed {
			current = &task.Task{ID: ownerID}
		}
		siblingAnon, ok := s.collectExplicitGlobalGCSiblingRefs(current)
		if !ok {
			return nil
		}
		renewTransaction := func() error {
			if ctx == nil || ctx.StoreTxn == nil {
				return nil
			}
			next, publishedWrites, errCode := ctx.StoreTxn.CommitAndRenew()
			if errCode != types.E_NONE {
				return errors.New(errCode.String())
			}
			ctx.StoreTxn = next
			if publishedWrites || ctx.LiveStoreMutated {
				ctx.LiveStoreMutated = true
				next.MarkLiveMutated()
			}
			return nil
		}
		if err := renewTransaction(); err != nil {
			return err
		}
		s.acquireSweepContext(ctx)
		defer s.releaseSweepContext(ctx)
		vm.AutoRecycleOrphanAnonymousSince(store, s.registry, execution, 0, siblingAnon)
		return renewTransaction()
	})
	s.startWorkers()

	return s
}

func (s *Runtime) startWorkers() {
	for i := 0; i < s.workerCount; i++ {
		s.workersWG.Add(1)
		go s.workerLoop()
	}
}

func (s *Runtime) workerLoop() {
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

// Registry returns the runtime's builtin registry so the owning server can
// wire host capabilities (connection manager, lifecycle hooks) onto it.
func (s *Runtime) Registry() *builtins.Registry { return s.registry }

// newTaskID allocates the next task id. Every task in the server is born here,
// which makes it the one place worth counting them.
func (s *Runtime) newTaskID() int64 {
	metrics.TasksStarted.Add(1)
	return atomic.AddInt64(&s.nextTaskID, 1)
}

// LiveTaskCount reports how many tasks the runtime is currently holding.
func (s *Runtime) LiveTaskCount() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.tasks))
}

func (s *Runtime) acquireTaskExecution(t *task.Task) {
	if s.lifecycle.executionStartObserver != nil {
		s.lifecycle.executionStartObserver()
	}
	s.lifecycle.vmStartMu.Lock()
	defer s.lifecycle.vmStartMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lifecycle.executingTasks[t.ID]++
	t.SetState(task.TaskRunning)
}

func (s *Runtime) releaseTaskExecution(taskID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if count := s.lifecycle.executingTasks[taskID]; count > 1 {
		s.lifecycle.executingTasks[taskID] = count - 1
	} else {
		delete(s.lifecycle.executingTasks, taskID)
	}
}

// acquireInheritedTaskExecution records a synchronously nested standalone VM
// under an execution lease that already passed vmStartMu. It must not acquire
// vmStartMu again, but the extra refcount makes run_gc fail closed while both
// the outer and inner VMs both hold roots for the same task.
func (s *Runtime) acquireInheritedTaskExecution(taskID int64) {
	s.mu.Lock()
	s.lifecycle.executingTasks[taskID]++
	s.mu.Unlock()
}

func (s *Runtime) acquireExecutionContext(ctx *kernel.TaskContext, taskID int64) {
	if ctx == nil {
		return
	}
	s.mu.Lock()
	owners := s.lifecycle.executionContexts[ctx]
	if owners == nil {
		owners = make(map[int64]int)
		s.lifecycle.executionContexts[ctx] = owners
	}
	owners[taskID]++
	s.mu.Unlock()
}

func (s *Runtime) releaseExecutionContext(ctx *kernel.TaskContext, taskID int64) {
	if ctx == nil {
		return
	}
	s.mu.Lock()
	owners := s.lifecycle.executionContexts[ctx]
	if count := owners[taskID]; count > 1 {
		owners[taskID] = count - 1
	} else if count == 1 {
		delete(owners, taskID)
		if len(owners) == 0 {
			delete(s.lifecycle.executionContexts, ctx)
		}
	}
	s.mu.Unlock()
}

func (s *Runtime) executionContextOwner(ctx *kernel.TaskContext) (int64, bool) {
	ownerID, claimed, attributable := s.executionContextClaim(ctx)
	return ownerID, claimed && attributable
}

// executionContextClaim distinguishes an unowned context from one whose
// physical execution owner is ambiguous. Ambiguity makes global GC a
// successful no-op; treating it as absent could fall back to ctx.Task and
// exclude the wrong sole lease.
func (s *Runtime) executionContextClaim(ctx *kernel.TaskContext) (ownerID int64, claimed, attributable bool) {
	if ctx == nil {
		return 0, false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	owners := s.lifecycle.executionContexts[ctx]
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

func (s *Runtime) acquireSweepContext(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	s.mu.Lock()
	s.lifecycle.sweepContexts[ctx]++
	s.mu.Unlock()
}

func (s *Runtime) releaseSweepContext(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	s.mu.Lock()
	if count := s.lifecycle.sweepContexts[ctx]; count > 1 {
		s.lifecycle.sweepContexts[ctx] = count - 1
	} else {
		delete(s.lifecycle.sweepContexts, ctx)
	}
	s.mu.Unlock()
}

func (s *Runtime) isSweepOwnedContext(ctx *kernel.TaskContext) bool {
	if ctx == nil {
		return false
	}
	s.mu.Lock()
	owned := s.lifecycle.sweepContexts[ctx] > 0
	s.mu.Unlock()
	return owned
}

func (s *Runtime) populateTaskContextDependencies(ctx *kernel.TaskContext) {
	if ctx == nil {
		return
	}
	ctx.Store = s.store
	ctx.RuntimeOptions = s.options
	// With() formats these once per task rather than once per record.
	ctx.Log = slog.Default().With(
		slog.Int64("task_id", ctx.TaskID),
		slog.Int64("player", int64(ctx.Player)),
		slog.String("verb", ctx.Verb))
}

// Stop cancels runtime-owned task contexts.
func (s *Runtime) Stop() {
	s.cancel()
	s.workersWG.Wait()
}

// BeginShutdown closes ordinary finalization ownership and returns a channel
// that closes after every admitted producer has published either pending roots
// or a complete task-owned suspended state.
func (s *Runtime) BeginShutdown(exec *vm.VM) <-chan struct{} {
	var callerRoots []types.Value
	if exec != nil && (exec.Context == nil || !exec.Context.DeferredGC) {
		callerRoots = vm.CollectPendingFinalizationValues(s.store, exec)
	}
	return s.BeginShutdownWithRoots(callerRoots)
}

// BeginShutdownWithRoots closes ordinary finalization ownership using roots
// supplied by the active execution, without exposing its concrete VM to the
// server lifecycle layer.
func (s *Runtime) BeginShutdownWithRoots(callerRoots []types.Value) <-chan struct{} {
	s.lifecycle.mu.Lock()
	ready := s.lifecycle.shutdownReady
	if s.lifecycle.shutdownPublished {
		s.lifecycle.mu.Unlock()
		s.appendPendingFinalizations(callerRoots)
		return ready
	}
	s.lifecycle.pendingShutdownRoots = append(s.lifecycle.pendingShutdownRoots, callerRoots...)
	s.lifecycle.shutdownRequested = true
	publish := s.canPublishShutdownLocked()
	if publish {
		s.lifecycle.shutdownPublishing = true
	}
	s.lifecycle.mu.Unlock()
	if publish {
		s.publishShutdown()
	}
	return ready
}

func (s *Runtime) beginFinalizationProducer() {
	s.lifecycle.mu.Lock()
	s.lifecycle.activeFinalizationProducers++
	s.lifecycle.mu.Unlock()
}

func (s *Runtime) finishFinalizationProducer() {
	s.lifecycle.mu.Lock()
	if s.lifecycle.activeFinalizationProducers <= 0 {
		s.lifecycle.mu.Unlock()
		panic("engine finalization producer underflow")
	}
	s.lifecycle.activeFinalizationProducers--
	publish := s.canPublishShutdownLocked()
	if publish {
		s.lifecycle.shutdownPublishing = true
	}
	s.lifecycle.mu.Unlock()
	if publish {
		s.publishShutdown()
	}
}

func (s *Runtime) canPublishShutdownLocked() bool {
	return s.lifecycle.shutdownRequested && !s.lifecycle.shutdownPublishing && !s.lifecycle.shutdownPublished &&
		!s.lifecycle.gcRunning && s.lifecycle.activeFinalizationProducers == 0
}

func (s *Runtime) publishShutdown() {
	for {
		s.lifecycle.mu.Lock()
		if s.lifecycle.gcRunning || s.lifecycle.activeFinalizationProducers != 0 {
			s.lifecycle.shutdownPublishing = false
			s.lifecycle.mu.Unlock()
			return
		}
		pending := s.takeDeferredFinalizationRootsLocked()
		pending = append(pending, s.lifecycle.pendingShutdownRoots...)
		s.lifecycle.pendingShutdownRoots = nil
		s.lifecycle.mu.Unlock()
		s.appendPendingFinalizations(pending)

		s.lifecycle.mu.Lock()
		if len(s.lifecycle.pendingShutdownRoots) != 0 || len(s.lifecycle.pendingWaifs) != 0 || len(s.lifecycle.pendingAnonGC) != 0 {
			s.lifecycle.mu.Unlock()
			continue
		}
		if s.lifecycle.gcRunning || s.lifecycle.activeFinalizationProducers != 0 {
			s.lifecycle.shutdownPublishing = false
			s.lifecycle.mu.Unlock()
			return
		}
		s.lifecycle.shutdownPublished = true
		s.lifecycle.shutdownPublishing = false
		close(s.lifecycle.shutdownReady)
		s.lifecycle.mu.Unlock()
		return
	}
}

func (s *Runtime) appendPendingFinalizations(values []types.Value) {
	if len(values) != 0 && s.pendingFinalizationSink != nil {
		s.pendingFinalizationSink(values)
	}
}

// ShutdownRequested distinguishes lifecycle publication from generic runtime
// cancellation; only an explicit lifecycle request transfers task roots.
func (s *Runtime) ShutdownRequested() bool {
	s.lifecycle.mu.Lock()
	defer s.lifecycle.mu.Unlock()
	return s.lifecycle.shutdownRequested
}

func (s *Runtime) SetPendingFinalizationSink(sink func([]types.Value)) {
	s.pendingFinalizationSink = sink
}

func (s *Runtime) SetTaskLineSender(sender func(types.ObjID, string)) {
	s.taskLineSender = sender
}

func (s *Runtime) SetTracebackSender(sender func(types.ObjID, types.ErrorCode, []task.ActivationFrame)) {
	s.tracebackSender = sender
}

func (s *Runtime) SetTaskOutputFlusher(flusher func(types.ObjID, string)) {
	s.taskOutputFlusher = flusher
}

// ProcessReadyTasks executes at most one task that is ready to run.
//
// The input processor owns the outer scheduling loop. Returning after each
// task lets that loop service socket input between runnable tasks instead of
// draining an arbitrarily large ready snapshot first. A single MOO task still
// runs atomically until completion or suspension.
func (s *Runtime) ProcessReadyTasks() int {
	s.mu.Lock()

	now := time.Now()
	var readyTasks []*task.Task

	for s.waiting.Len() > 0 {
		t := s.waiting.Peek()
		if t.StartTime.After(now) {
			break
		}
		heap.Pop(s.waiting)
		if !t.TryClaimQueued() {
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
			if t.Resume(types.NewInt(0)) && t.TryClaimQueued() {
				readyTasks = append(readyTasks, t)
			}
			continue
		}

		if t.GetState() == task.TaskQueued && (t.StmtIndex > 0 || t.BytecodeVMValue() != nil) {
			if (t.WakeTime.IsZero() || !t.WakeTime.After(now)) && !t.StartTime.After(now) {
				if t.TryClaimQueued() {
					readyTasks = append(readyTasks, t)
				}
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

func (s *Runtime) runReadyTasks(readyTasks []*task.Task) {
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

func (s *Runtime) readyTaskBatches(readyTasks []*task.Task) [][]*task.Task {
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

func (s *Runtime) runTaskBatch(readyTasks []*task.Task) {
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

func (s *Runtime) YieldReadyTasks() int {
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
func (s *Runtime) collectAllGCRefs() (anonRefs map[types.ObjID]struct{}, waifRefs []types.Value, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lifecycle.executingTasks) != 0 {
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
func (s *Runtime) collectExplicitGlobalGCSiblingRefs(exclude *task.Task) (anonRefs map[types.ObjID]struct{}, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, count := range s.lifecycle.executingTasks {
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
func (s *Runtime) collectSiblingGCRefs(exclude *task.Task) (anonRefs map[types.ObjID]struct{}, waifRefs []types.Value, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, count := range s.lifecycle.executingTasks {
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

func (s *Runtime) isWizard(objID types.ObjID) bool {
	hasWizard, errCode := s.store.HasObjectFlag(objID, dbstore.FlagWizard)
	return errCode == types.E_NONE && hasWizard
}

var (
	ErrTicksExceeded = errors.New("tick limit exceeded")
	ErrNotSuspended  = errors.New("task not suspended")
	ErrResumeFailed  = errors.New("failed to resume task")
	ErrPermission    = errors.New("permission denied")
)
