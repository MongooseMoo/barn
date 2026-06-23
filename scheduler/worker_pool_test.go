package scheduler

import (
	"sync/atomic"
	"testing"
	"time"

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
