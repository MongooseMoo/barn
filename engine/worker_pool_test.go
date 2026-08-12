package engine

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func compileTestProgram(t *testing.T, registry *builtins.Registry, code string) *bytecode.Program {
	t.Helper()
	prog, diagnostics := registry.Compiler().CompileMOO(strings.Split(code, "\n"))
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO() failed: %v", diagnostics[0])
	}
	return prog
}

func newReadyTestTask(t *testing.T, registry *builtins.Registry, id int64, owner types.ObjID) *task.Task {
	t.Helper()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(id, owner, compileTestProgram(t, registry, "return 1;"), ticks, seconds)
	queued.StartTime = time.Now().Add(-time.Second)
	queued.Done = make(chan struct{})
	return queued
}

func TestRuntimeWorkerPoolStopsCleanly(t *testing.T) {
	s := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{}, 2)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop() did not stop worker pool")
	}
}

func TestRunTaskBatchRunsConfiguredWorkersInParallel(t *testing.T) {
	s := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{}, 2)
	defer s.Stop()

	var entered atomic.Int32
	release := make(chan struct{})
	s.registry.Register("parallel_gate", func(ctx *builtins.Execution, args []types.Value) types.Result {
		if entered.Add(1) == 2 {
			close(release)
		}
		select {
		case <-release:
			return types.Ok(types.NewInt(1))
		case <-time.After(500 * time.Millisecond):
			return types.Err(types.E_QUOTA)
		}
	})

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	first := task.NewTaskFull(1101, 7, compileTestProgram(t, s.registry, "parallel_gate(); return 1;"), ticks, seconds)
	first.StartTime = time.Now().Add(-time.Second)
	first.Done = make(chan struct{})
	second := task.NewTaskFull(1102, 7, compileTestProgram(t, s.registry, "parallel_gate(); return 2;"), ticks, seconds)
	second.StartTime = time.Now().Add(-time.Second)
	second.Done = make(chan struct{})
	s.QueueTask(first)
	s.QueueTask(second)
	defer s.taskManager.RemoveTask(first.ID)
	defer s.taskManager.RemoveTask(second.ID)

	done := make(chan int, 1)
	go func() {
		s.runTaskBatch([]*task.Task{first, second})
		done <- 2
	}()

	select {
	case got := <-done:
		if got != 2 {
			t.Fatalf("ProcessReadyTasks() = %d, want 2", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ProcessReadyTasks() did not complete; ready tasks did not run in parallel")
	}

	if got := entered.Load(); got != 2 {
		t.Fatalf("parallel_gate entries = %d, want 2", got)
	}
	for _, queued := range []*task.Task{first, second} {
		if queued.Result.Flow != types.FlowReturn {
			t.Fatalf("task %d flow = %v err=%v, want return", queued.ID, queued.Result.Flow, queued.Result.Error)
		}
	}
}

func TestReadyTaskBatchesGroupCommutingPropertyWrites(t *testing.T) {
	s := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{}, 2)
	defer s.Stop()

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	first := task.NewTaskFull(1201, 7, compileTestProgram(t, s.registry, "#1.a = 2;"), ticks, seconds)
	second := task.NewTaskFull(1202, 7, compileTestProgram(t, s.registry, "#1.b = 3;"), ticks, seconds)

	batches := s.scheduler.Plan([]*task.Task{first, second})

	if len(batches) != 1 {
		t.Fatalf("batch count = %d, want 1", len(batches))
	}
	if len(batches[0]) != 2 {
		t.Fatalf("batch size = %d, want 2", len(batches[0]))
	}
}

// Conflicting property writes to the same cell are no longer separated into
// ordered batches by static footprint analysis — that analysis is stubbed to
// "unknown" (see access_footprint.go). Two fresh retryable tasks are instead
// co-scheduled optimistically in one batch; the commit-time read/write-set
// validation serializes them and re-runs the loser (see
// TestOptimisticConflictingWritersAreSerializable).
func TestReadyTaskBatchesCoScheduleConflictingRetryableWrites(t *testing.T) {
	s := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{}, 2)
	defer s.Stop()

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	first := task.NewTaskFull(1211, 7, compileTestProgram(t, s.registry, "#1.a = 2;"), ticks, seconds)
	second := task.NewTaskFull(1212, 7, compileTestProgram(t, s.registry, "#1.a = 3;"), ticks, seconds)

	batches := s.scheduler.Plan([]*task.Task{first, second})

	if len(batches) != 1 {
		t.Fatalf("batch count = %d, want 1", len(batches))
	}
	if len(batches[0]) != 2 {
		t.Fatalf("batch size = %d, want 2", len(batches[0]))
	}
}

// A task with an opaque ("unknown") footprint — here a verb-dispatching notify() —
// no longer forces serialization on its own. As long as both tasks are fresh AST
// tasks (conflict-retryable), they are co-scheduled optimistically and any real
// conflict is caught and retried at commit time.
func TestReadyTaskBatchesGroupRetryableUnknownTasks(t *testing.T) {
	s := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{}, 2)
	defer s.Stop()

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	first := task.NewTaskFull(1221, 7, compileTestProgram(t, s.registry, "notify(player, \"x\");"), ticks, seconds)
	second := task.NewTaskFull(1222, 7, compileTestProgram(t, s.registry, "#1.a = 3;"), ticks, seconds)

	batches := s.scheduler.Plan([]*task.Task{first, second})

	if len(batches) != 1 {
		t.Fatalf("batch count = %d, want 1", len(batches))
	}
	if len(batches[0]) != 2 {
		t.Fatalf("batch size = %d, want 2", len(batches[0]))
	}
}

// A non-retryable task (resumed/forked: its mid-flight state cannot be re-run from
// the original statements) must stay solo when its footprint is unknown, because an
// optimistic conflict could not be recovered by retry.
func TestReadyTaskBatchesKeepNonRetryableUnknownSolo(t *testing.T) {
	s := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{}, 2)
	defer s.Stop()

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	first := task.NewTaskFull(1221, 7, compileTestProgram(t, s.registry, "notify(player, \"x\");"), ticks, seconds)
	first.IsForked = true // not conflict-retryable
	second := task.NewTaskFull(1222, 7, compileTestProgram(t, s.registry, "#1.a = 3;"), ticks, seconds)

	batches := s.scheduler.Plan([]*task.Task{first, second})

	if len(batches) != 2 {
		t.Fatalf("batch count = %d, want 2", len(batches))
	}
	if batches[0][0] != first || batches[1][0] != second {
		t.Fatal("non-retryable unknown task was not kept solo in order")
	}
}

func TestProcessReadyTasksRunsTaskOnceAndClosesDoneOnce(t *testing.T) {
	s := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{}, 2)
	defer s.Stop()

	var completes atomic.Int32
	queued := newReadyTestTask(t, s.registry, 1001, 7)
	queued.OnComplete = func(types.Result) {
		completes.Add(1)
	}
	s.QueueTask(queued)
	defer s.taskManager.RemoveTask(queued.ID)

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("ProcessReadyTasks() = %d, want 1", got)
	}

	select {
	case <-queued.Done:
	case <-time.After(time.Second):
		t.Fatal("task Done channel was not closed")
	}
	if got := completes.Load(); got != 1 {
		t.Fatalf("OnComplete calls = %d, want 1", got)
	}
	if state := queued.GetState(); state != task.TaskCompleted {
		t.Fatalf("task state = %s, want completed", state)
	}
	if got := s.ProcessReadyTasks(); got != 0 {
		t.Fatalf("second ProcessReadyTasks() = %d, want 0", got)
	}
}

func TestProcessReadyTasksFlushesInReadyOrder(t *testing.T) {
	s := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{}, 2)
	defer s.Stop()

	var flushed []string
	s.SetTaskOutputFlusher(func(_ types.ObjID, suffix string) {
		flushed = append(flushed, suffix)
	})

	first := newReadyTestTask(t, s.registry, 2001, 7)
	first.CommandOutputSuffix = "first"
	second := newReadyTestTask(t, s.registry, 2002, 7)
	second.CommandOutputSuffix = "second"
	s.QueueTask(first)
	s.QueueTask(second)
	defer s.taskManager.RemoveTask(first.ID)
	defer s.taskManager.RemoveTask(second.ID)

	if got := s.ProcessReadyTasks(); got != 2 {
		t.Fatalf("ProcessReadyTasks() = %d, want 2", got)
	}

	if len(flushed) != 2 {
		t.Fatalf("flush count = %d, want 2", len(flushed))
	}
	if flushed[0] != "first" || flushed[1] != "second" {
		t.Fatalf("flush order = %#v, want [first second]", flushed)
	}
}

func TestRunTaskUsesStableReadTransaction(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("snapshot_value", dbstore.NewProperty(types.NewStr("old"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	s.registry.Register("mutate_snapshot_value", func(ctx *builtins.Execution, args []types.Value) types.Result {
		if ctx.StoreTxn == nil {
			t.Fatal("task context did not have a store read transaction")
		}
		if errCode := ctx.Store.SetPropertyValue(0, "snapshot_value", types.NewStr("new")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3001, 0, compileTestProgram(t, s.registry, `
first = #0.snapshot_value;
mutate_snapshot_value();
return {first, #0.snapshot_value};
`), ticks, seconds)
	queued.Context.IsWizard = true

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn {
		t.Fatalf("result flow = %v, want return", queued.Result.Flow)
	}
	values := queued.Result.Val
	if values.Type() != types.TYPE_LIST {
		t.Fatalf("result value = %T, want list", queued.Result.Val)
	}
	if values.Len() != 2 {
		t.Fatalf("result len = %d, want 2", values.Len())
	}
	for i := 1; i <= values.Len(); i++ {
		if values.Get(i).Type() != types.TYPE_STR {
			t.Fatalf("result[%d] = %T, want string", i, values.Get(i))
		}
		if values.Get(i).Str() != "old" {
			t.Fatalf("result[%d] = %q, want old", i, values.Get(i).Str())
		}
	}

	liveValue, errCode := store.PropertyValue(0, "snapshot_value")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue failed: %s", errCode)
	}
	if got := liveValue.Str(); got != "new" {
		t.Fatalf("live store value = %q, want new", got)
	}
}

func TestRunTaskKeepsForksAfterSuccessfulCommit(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()

	owner := types.ObjID(7702)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3002, owner, compileTestProgram(t, s.registry, `
fork child (30)
  suspend(5);
endfork
return child;
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn {
		t.Fatalf("result flow = %v, want return", queued.Result.Flow)
	}
	childID := queued.Result.Val
	if childID.Type() != types.TYPE_INT {
		t.Fatalf("result value = %T, want child task id", queued.Result.Val)
	}
	if len(queued.CreatedForks) != 0 {
		t.Fatalf("created forks after successful commit = %#v, want none", queued.CreatedForks)
	}
	child := s.taskManager.GetTask(childID.Int())
	if child == nil {
		t.Fatalf("child task %d was not registered", childID.Int())
	}
	if child.Owner != owner || child.GetState() != task.TaskQueued {
		t.Fatalf("child owner/state = #%d/%s, want #%d/queued", child.Owner, child.GetState(), owner)
	}
}

func TestYinCommitsAndRefreshesAroundReadyTasks(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("yield_order", dbstore.NewProperty(types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	s.Registry().SetTaskYielder(s)

	owner := types.ObjID(7710)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	// yin() yields when FEWER than min_ticks remain (Toast bf_yield_if_needed), and
	// min_ticks must stay under the foreground limit, so burn a few ticks first to
	// put the budget below the threshold.
	queued := task.NewTaskFull(3010, owner, compileTestProgram(t, s.registry, `
#0.yield_order = {"main-before"};
fork (0)
  #0.yield_order = listappend(#0.yield_order, "fork");
endfork
for i in [1..50]
endfor
yin(0, 59999, 4);
#0.yield_order = listappend(#0.yield_order, "main-after");
return #0.yield_order;
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	// yin() suspends (Toast bf_yield_if_needed), so the first slice ends at the
	// yield. Its commit must have published both the pre-yin write and the fork,
	// letting the fork run before the main task resumes on a later pass.
	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if len(queued.CreatedForks) != 0 {
		t.Fatalf("forks still owned by task after yin commit = %#v, want handed off", queued.CreatedForks)
	}
	deadline := time.Now().Add(2 * time.Second)
	for queued.GetState() != task.TaskCompleted && time.Now().Before(deadline) {
		s.ProcessReadyTasks()
	}
	if state := queued.GetState(); state != task.TaskCompleted {
		t.Fatalf("task state after yin resume = %v, want completed", state)
	}

	got := queued.Result.Val
	if got.Type() != types.TYPE_LIST {
		t.Fatalf("result value = %T, want list", queued.Result.Val)
	}
	want := []string{"main-before", "fork", "main-after"}
	if got.Len() != len(want) {
		t.Fatalf("result len = %d, want %d: %s", got.Len(), len(want), got.String())
	}
	for i, wantValue := range want {
		value := got.Get(i + 1)
		if value.Type() != types.TYPE_STR || value.Str() != wantValue {
			t.Fatalf("result[%d] = %v, want %q in %s", i+1, got.Get(i+1), wantValue, got.String())
		}
	}
}

func TestYinFlushesCommittedForksBeforeLaterConflict(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("snapshot_value", dbstore.NewProperty(types.NewStr("old"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	s.Registry().SetTaskYielder(s)
	s.registry.Register("mutate_snapshot_value", func(ctx *builtins.Execution, args []types.Value) types.Result {
		if errCode := ctx.Store.SetPropertyValue(0, "snapshot_value", types.NewStr("live")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		ctx.LiveStoreMutated = true
		return types.Ok(types.NewInt(0))
	})
	s.registry.Register("stage_snapshot_value", func(ctx *builtins.Execution, args []types.Value) types.Result {
		if ctx.StoreTxn == nil {
			t.Fatal("task context did not have a store transaction")
		}
		if errCode := ctx.StoreTxn.SetPropertyValue(0, "snapshot_value", types.NewStr("task")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	owner := types.ObjID(7712)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	// The `for` loop drops the tick budget below min_ticks so yin() actually
	// yields: Toast's yin yields when FEWER than min_ticks remain, and min_ticks
	// must stay under the foreground limit.
	queued := task.NewTaskFull(3012, owner, compileTestProgram(t, s.registry, `
#0.snapshot_value = "before-yin";
fork child (30)
  suspend(5);
endfork
for i in [1..50]
endfor
yin(0, 59999, 4);
before = #0.snapshot_value;
mutate_snapshot_value();
stage_snapshot_value();
return child;
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	// First slice ends at the yin suspend, whose commit publishes the fork.
	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if len(queued.CreatedForks) != 0 {
		t.Fatalf("created forks after yin commit = %#v, want none", queued.CreatedForks)
	}

	// Resuming runs the rest, which mutates the live store and then stages a
	// conflicting write: that commit fails with E_INVARG. The fork committed by
	// yin must survive it.
	deadline := time.Now().Add(2 * time.Second)
	for queued.GetState() != task.TaskKilled && queued.GetState() != task.TaskCompleted && time.Now().Before(deadline) {
		s.ProcessReadyTasks()
	}
	if queued.Result.Flow != types.FlowException || queued.Result.Error != types.E_INVARG {
		t.Fatalf("result = flow %v err %v, want E_INVARG exception", queued.Result.Flow, queued.Result.Error)
	}
	if len(queued.CreatedForks) != 0 {
		t.Fatalf("created forks after failed post-yin commit = %#v, want none", queued.CreatedForks)
	}
	for _, child := range s.taskManager.GetQueuedTasks() {
		if child.Owner == owner {
			return
		}
	}
	t.Fatalf("fork committed before yin was discarded after later conflict")
}

func TestForkedSuspendZeroRefreshesAfterPreResumeCommit(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("yield_progress", dbstore.NewProperty(types.NewStr("before"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()

	owner := types.ObjID(7711)
	ticks, seconds := backgroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3011, owner, compileTestProgram(t, s.registry, `
#0.yield_progress = "after-long-suspend";
suspend(0);
#0.yield_progress = "after-yield";
return 0;
`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.IsForked = true
	queued.Kind = task.TaskForked
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	value, errCode := store.PropertyValue(0, "yield_progress")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %s", errCode)
	}
	if value.Type() != types.TYPE_STR || value.Str() != "after-yield" {
		t.Fatalf("yield_progress = %v, want after-yield", value)
	}
}

func TestRunTaskRollsBackForksOnTransactionConflict(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("snapshot_value", dbstore.NewProperty(types.NewStr("old"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	s.registry.Register("mutate_snapshot_value", func(ctx *builtins.Execution, args []types.Value) types.Result {
		if errCode := ctx.Store.SetPropertyValue(0, "snapshot_value", types.NewStr("live")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		ctx.LiveStoreMutated = true
		return types.Ok(types.NewInt(0))
	})
	s.registry.Register("stage_snapshot_value", func(ctx *builtins.Execution, args []types.Value) types.Result {
		if ctx.StoreTxn == nil {
			t.Fatal("task context did not have a store transaction")
		}
		if errCode := ctx.StoreTxn.SetPropertyValue(0, "snapshot_value", types.NewStr("task")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	owner := types.ObjID(7703)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3003, owner, compileTestProgram(t, s.registry, `
before = #0.snapshot_value;
fork child (30)
  suspend(5);
endfork
mutate_snapshot_value();
stage_snapshot_value();
return child;
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowException || queued.Result.Error != types.E_INVARG {
		t.Fatalf("result = flow %v err %v, want E_INVARG exception", queued.Result.Flow, queued.Result.Error)
	}
	if len(queued.CreatedForks) != 0 {
		t.Fatalf("created forks after conflict = %#v, want none", queued.CreatedForks)
	}
	for _, task := range s.taskManager.GetQueuedTasks() {
		if task.Owner == owner {
			t.Fatalf("conflicted fork task %d remained queued", task.ID)
		}
	}
}

func TestRunTaskDoesNotRetryAfterLiveMutationConflict(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("snapshot_value", dbstore.NewProperty(types.NewStr("old"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	mutateCalls := 0
	s.registry.Register("mutate_snapshot_value_once", func(ctx *builtins.Execution, args []types.Value) types.Result {
		mutateCalls++
		if errCode := ctx.Store.SetPropertyValue(0, "snapshot_value", types.NewStr("live")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		ctx.LiveStoreMutated = true
		return types.Ok(types.NewInt(0))
	})
	s.registry.Register("stage_snapshot_value", func(ctx *builtins.Execution, args []types.Value) types.Result {
		if ctx.StoreTxn == nil {
			t.Fatal("task context did not have a store transaction")
		}
		if errCode := ctx.StoreTxn.SetPropertyValue(0, "snapshot_value", types.NewStr("task")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3004, 0, compileTestProgram(t, s.registry, `
before = #0.snapshot_value;
mutate_snapshot_value_once();
stage_snapshot_value();
return before;
`), ticks, seconds)
	queued.Context.IsWizard = true

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowException || queued.Result.Error != types.E_INVARG {
		t.Fatalf("result = flow %v err %v, want E_INVARG exception", queued.Result.Flow, queued.Result.Error)
	}
	if mutateCalls != 1 {
		t.Fatalf("mutate calls = %d, want one non-retried attempt", mutateCalls)
	}
	liveValue, errCode := store.PropertyValue(0, "snapshot_value")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue failed: %s", errCode)
	}
	if got := liveValue.Str(); got != "live" {
		t.Fatalf("live store value = %q, want live", got)
	}
}

func TestRunTaskDoesNotRetryAfterIrreversibleSideEffect(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("read_value", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	root.SetProperty("write_value", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	s.registry.Register("mutate_read_value", func(ctx *builtins.Execution, args []types.Value) types.Result {
		value, errCode := ctx.Store.PropertyValue(0, "read_value")
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if errCode := ctx.Store.SetPropertyValue(0, "read_value", types.NewInt(value.Int()+1)); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	var logs bytes.Buffer
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3005, 0, compileTestProgram(t, s.registry, `
before = #0.read_value;
server_log("irreversible-once");
mutate_read_value();
#0.write_value = 1;
return before;
`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.Context.Log = slog.New(slog.NewTextHandler(&logs, nil))

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowException || queued.Result.Error != types.E_INVARG {
		t.Fatalf("result = flow %v err %v, want E_INVARG exception", queued.Result.Flow, queued.Result.Error)
	}
	if got := strings.Count(logs.String(), "irreversible-once"); got != 1 {
		t.Fatalf("server_log executions = %d, want 1", got)
	}
	if got := store.CommitRetries(); got != 0 {
		t.Fatalf("commit retries = %d, want 0", got)
	}
}

func TestRunTaskFlushesBufferedEffectsInCallOrder(t *testing.T) {
	store := dbstore.NewStore()
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	conn := &evalCommandStubConn{}
	manager := &evalCommandStubConnManager{
		player:           7,
		conn:             conn,
		disconnectOnBoot: true,
	}
	s.Registry().SetConnectionManager(manager)

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3006, 7, compileTestProgram(t, s.registry, `
boot_player(#7);
notify(#7, "after boot");
return 0;
`), ticks, seconds)
	queued.Context.IsWizard = true

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn {
		t.Fatalf("result = flow %v err %v, want return", queued.Result.Flow, queued.Result.Error)
	}
	if len(conn.sent) != 1 || conn.sent[0] != "*** Disconnected ***" {
		t.Fatalf("connection output = %#v, want only the earlier boot", conn.sent)
	}
}

func TestRunTaskReleasesTerminalStoreTransaction(t *testing.T) {
	for _, test := range []struct {
		name string
		code string
	}{
		{name: "read only", code: "return #0.value;"},
		{name: "write", code: "#0.value = 1; return 1;"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := dbstore.NewStore()
			root := dbstore.NewObjectBuilder(0)
			root.SetOwner(0)
			root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
			root.SetProperty("value", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
			if err := store.Add(root.Build()); err != nil {
				t.Fatalf("store.Add failed: %v", err)
			}

			s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
			defer s.Stop()
			ticks, seconds := foregroundTaskLimits(newTestRegistry())
			queued := task.NewTaskFull(3007, 0, compileTestProgram(t, s.registry, test.code), ticks, seconds)
			queued.Context.IsWizard = true

			if err := s.runTask(queued); err != nil {
				t.Fatalf("runTask failed: %v", err)
			}
			if queued.Result.Flow != types.FlowReturn {
				t.Fatalf("result = flow %v err %v, want return", queued.Result.Flow, queued.Result.Error)
			}
			if got := store.ActiveReadTransactions(); got != 0 {
				t.Fatalf("active read transactions after terminal task = %d, want 0", got)
			}
		})
	}
}

func TestRunTaskDoesNotReplaceResultAfterDeferredEffectFailure(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("value", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()
	conn := &evalCommandStubConn{sendErr: errors.New("send failed")}
	s.Registry().SetConnectionManager(&evalCommandStubConnManager{player: 7, conn: conn})
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3008, 7, compileTestProgram(t, s.registry, `
#0.value = 1;
notify(#7, "deferred failure");
return 42;
`), ticks, seconds)
	queued.Context.IsWizard = true

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 42 {
		t.Fatalf("result = flow %v value %v err %v, want return 42", queued.Result.Flow, queued.Result.Val, queued.Result.Error)
	}
	value, errCode := store.PropertyValue(0, "value")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := value.Int(); got != 1 {
		t.Fatalf("committed value = %d, want 1", got)
	}
}

func removeTasksForOwner(s *Runtime, owner types.ObjID) {
	for _, task := range s.taskManager.Snapshot() {
		if task != nil && task.Owner == owner {
			task.Kill()
			s.taskManager.RemoveTaskIf(task.ID, task)
		}
	}
}
