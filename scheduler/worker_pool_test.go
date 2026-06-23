package scheduler

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"barn/builtins"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/parser"
	"barn/task"
	"barn/types"
)

func parseTestStatements(t *testing.T, code string) []parser.Stmt {
	t.Helper()
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("ParseProgram() failed: %v", err)
	}
	return stmts
}

func newReadyTestTask(t *testing.T, id int64, owner types.ObjID) *task.Task {
	t.Helper()
	ticks, seconds := foregroundTaskLimits()
	queued := task.NewTaskFull(id, owner, parseTestStatements(t, "return 1;"), ticks, seconds)
	queued.StartTime = time.Now().Add(-time.Second)
	queued.Done = make(chan struct{})
	return queued
}

func TestSchedulerWorkerPoolStopsCleanly(t *testing.T) {
	s := newSchedulerWithWorkerCount(dbstore.NewStore(), false, 2)

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

func TestNewSchedulerWithOptionsUsesGOMAXPROCSWorkers(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(oldProcs)

	s := NewSchedulerWithOptions(dbstore.NewStore(), false)
	defer s.Stop()

	if s.workerCount != 2 {
		t.Fatalf("workerCount = %d, want 2", s.workerCount)
	}
}

func TestProcessReadyTasksRunsDefaultWorkersInParallel(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(oldProcs)

	s := NewSchedulerWithOptions(dbstore.NewStore(), false)
	defer s.Stop()

	var entered atomic.Int32
	release := make(chan struct{})
	s.registry.Register("parallel_gate", func(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

	ticks, seconds := foregroundTaskLimits()
	first := task.NewTaskFull(1101, 7, parseTestStatements(t, "parallel_gate(); return 1;"), ticks, seconds)
	first.StartTime = time.Now().Add(-time.Second)
	first.Done = make(chan struct{})
	second := task.NewTaskFull(1102, 7, parseTestStatements(t, "parallel_gate(); return 2;"), ticks, seconds)
	second.StartTime = time.Now().Add(-time.Second)
	second.Done = make(chan struct{})
	s.QueueTask(first)
	s.QueueTask(second)
	defer task.GetManager().RemoveTask(first.ID)
	defer task.GetManager().RemoveTask(second.ID)

	done := make(chan int, 1)
	go func() {
		done <- s.ProcessReadyTasks()
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

func TestProcessReadyTasksRunsTaskOnceAndClosesDoneOnce(t *testing.T) {
	s := newSchedulerWithWorkerCount(dbstore.NewStore(), false, 2)
	defer s.Stop()

	var completes atomic.Int32
	queued := newReadyTestTask(t, 1001, 7)
	queued.OnComplete = func(types.Result) {
		completes.Add(1)
	}
	s.QueueTask(queued)
	defer task.GetManager().RemoveTask(queued.ID)

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
	s := newSchedulerWithWorkerCount(dbstore.NewStore(), false, 2)
	defer s.Stop()

	var flushed []string
	s.SetTaskOutputFlusher(func(_ types.ObjID, suffix string) {
		flushed = append(flushed, suffix)
	})

	first := newReadyTestTask(t, 2001, 7)
	first.CommandOutputSuffix = "first"
	second := newReadyTestTask(t, 2002, 7)
	second.CommandOutputSuffix = "second"
	s.QueueTask(first)
	s.QueueTask(second)
	defer task.GetManager().RemoveTask(first.ID)
	defer task.GetManager().RemoveTask(second.ID)

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
	root.SetProperty("snapshot_value", dbstore.NewProperty("snapshot_value", types.NewStr("old"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newSchedulerWithWorkerCount(store, false, 1)
	defer s.Stop()
	s.registry.Register("mutate_snapshot_value", func(ctx *kernel.TaskContext, args []types.Value) types.Result {
		if ctx.StoreTxn == nil {
			t.Fatal("task context did not have a store read transaction")
		}
		if errCode := ctx.Store.SetPropertyValue(0, "snapshot_value", types.NewStr("new")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	ticks, seconds := foregroundTaskLimits()
	queued := task.NewTaskFull(3001, 0, parseTestStatements(t, `
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
	values, ok := queued.Result.Val.(types.ListValue)
	if !ok {
		t.Fatalf("result value = %T, want list", queued.Result.Val)
	}
	if values.Len() != 2 {
		t.Fatalf("result len = %d, want 2", values.Len())
	}
	for i := 1; i <= values.Len(); i++ {
		str, ok := values.Get(i).(types.StrValue)
		if !ok {
			t.Fatalf("result[%d] = %T, want string", i, values.Get(i))
		}
		if str.Value() != "old" {
			t.Fatalf("result[%d] = %q, want old", i, str.Value())
		}
	}

	liveValue, errCode := store.PropertyValue(0, "snapshot_value")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue failed: %s", errCode)
	}
	if got := liveValue.(types.StrValue).Value(); got != "new" {
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

	s := newSchedulerWithWorkerCount(store, false, 1)
	defer s.Stop()

	owner := types.ObjID(7702)
	ticks, seconds := foregroundTaskLimits()
	queued := task.NewTaskFull(3002, owner, parseTestStatements(t, `
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
	childID, ok := queued.Result.Val.(types.IntValue)
	if !ok {
		t.Fatalf("result value = %T, want child task id", queued.Result.Val)
	}
	if len(queued.CreatedForks) != 0 {
		t.Fatalf("created forks after successful commit = %#v, want none", queued.CreatedForks)
	}
	child := task.GetManager().GetTask(childID.Val)
	if child == nil {
		t.Fatalf("child task %d was not registered", childID.Val)
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
	root.SetProperty("yield_order", dbstore.NewProperty("yield_order", types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newSchedulerWithWorkerCount(store, false, 1)
	defer s.Stop()
	builtins.SetTaskYielder(s)
	t.Cleanup(func() { builtins.SetTaskYielder(nil) })

	owner := types.ObjID(7710)
	ticks, seconds := foregroundTaskLimits()
	queued := task.NewTaskFull(3010, owner, parseTestStatements(t, `
#0.yield_order = {"main-before"};
fork (0)
  #0.yield_order = listappend(#0.yield_order, "fork");
endfork
yin(0, 59999, 4);
#0.yield_order = listappend(#0.yield_order, "main-after");
return #0.yield_order;
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	got, ok := queued.Result.Val.(types.ListValue)
	if !ok {
		t.Fatalf("result value = %T, want list", queued.Result.Val)
	}
	want := []string{"main-before", "fork", "main-after"}
	if got.Len() != len(want) {
		t.Fatalf("result len = %d, want %d: %s", got.Len(), len(want), got.String())
	}
	for i, wantValue := range want {
		value, ok := got.Get(i + 1).(types.StrValue)
		if !ok || value.Value() != wantValue {
			t.Fatalf("result[%d] = %v, want %q in %s", i+1, got.Get(i+1), wantValue, got.String())
		}
	}
}

func TestForkedSuspendZeroRefreshesAfterPreResumeCommit(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("yield_progress", dbstore.NewProperty("yield_progress", types.NewStr("before"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newSchedulerWithWorkerCount(store, false, 1)
	defer s.Stop()

	owner := types.ObjID(7711)
	ticks, seconds := backgroundTaskLimits()
	queued := task.NewTaskFull(3011, owner, parseTestStatements(t, `
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
	got, ok := value.(types.StrValue)
	if !ok || got.Value() != "after-yield" {
		t.Fatalf("yield_progress = %v, want after-yield", value)
	}
}

func TestRunTaskRollsBackForksOnTransactionConflict(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("snapshot_value", dbstore.NewProperty("snapshot_value", types.NewStr("old"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newSchedulerWithWorkerCount(store, false, 1)
	defer s.Stop()
	s.registry.Register("mutate_snapshot_value", func(ctx *kernel.TaskContext, args []types.Value) types.Result {
		if errCode := ctx.Store.SetPropertyValue(0, "snapshot_value", types.NewStr("live")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})
	s.registry.Register("stage_snapshot_value", func(ctx *kernel.TaskContext, args []types.Value) types.Result {
		if ctx.StoreTxn == nil {
			t.Fatal("task context did not have a store transaction")
		}
		if errCode := ctx.StoreTxn.SetPropertyValue(0, "snapshot_value", types.NewStr("task")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	owner := types.ObjID(7703)
	ticks, seconds := foregroundTaskLimits()
	queued := task.NewTaskFull(3003, owner, parseTestStatements(t, `
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
	for _, task := range task.GetManager().GetQueuedTasks() {
		if task.Owner == owner {
			t.Fatalf("conflicted fork task %d remained queued", task.ID)
		}
	}
}

func TestRunTaskRetriesFreshTaskOnValidationConflict(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("snapshot_value", dbstore.NewProperty("snapshot_value", types.NewStr("old"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	s := newSchedulerWithWorkerCount(store, false, 1)
	defer s.Stop()
	mutateCalls := 0
	s.registry.Register("mutate_snapshot_value_once", func(ctx *kernel.TaskContext, args []types.Value) types.Result {
		mutateCalls++
		if mutateCalls == 1 {
			if errCode := ctx.Store.SetPropertyValue(0, "snapshot_value", types.NewStr("live")); errCode != types.E_NONE {
				return types.Err(errCode)
			}
		}
		return types.Ok(types.NewInt(0))
	})
	s.registry.Register("stage_snapshot_value", func(ctx *kernel.TaskContext, args []types.Value) types.Result {
		if ctx.StoreTxn == nil {
			t.Fatal("task context did not have a store transaction")
		}
		if errCode := ctx.StoreTxn.SetPropertyValue(0, "snapshot_value", types.NewStr("task")); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	})

	ticks, seconds := foregroundTaskLimits()
	queued := task.NewTaskFull(3004, 0, parseTestStatements(t, `
before = #0.snapshot_value;
mutate_snapshot_value_once();
stage_snapshot_value();
return before;
`), ticks, seconds)
	queued.Context.IsWizard = true

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn {
		t.Fatalf("result flow = %v err=%v, want return", queued.Result.Flow, queued.Result.Error)
	}
	if mutateCalls != 2 {
		t.Fatalf("mutate calls = %d, want 2 attempts", mutateCalls)
	}
	liveValue, errCode := store.PropertyValue(0, "snapshot_value")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue failed: %s", errCode)
	}
	if got := liveValue.(types.StrValue).Value(); got != "task" {
		t.Fatalf("live store value = %q, want task", got)
	}
}

func removeTasksForOwner(s *Scheduler, owner types.ObjID) {
	var ids []int64
	s.mu.Lock()
	for id, task := range s.tasks {
		if task != nil && task.Owner == owner {
			task.Kill()
			ids = append(ids, id)
			delete(s.tasks, id)
		}
	}
	s.mu.Unlock()

	mgr := task.GetManager()
	for _, task := range mgr.GetAllTasks() {
		if task != nil && task.Owner == owner {
			task.Kill()
			ids = append(ids, task.ID)
		}
	}
	for _, id := range ids {
		mgr.RemoveTask(id)
	}
}
