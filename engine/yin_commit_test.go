package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// These tests cover YIN_COMMIT_TICKS: with it set to K, a yin() on the task's
// root VM after at least K ticks in the current slice yields for 0 seconds even
// though the tick and second budgets are far from yin()'s thresholds.

func newYinCommitStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	for _, name := range []string{"read_value", "write_value", "yield_order", "after_yin"} {
		root.SetProperty(name, dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	}
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}
	return store
}

func newYinCommitRuntime(t *testing.T, store *dbstore.Store, k int, descriptors ...builtins.Descriptor) *Runtime {
	t.Helper()
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{YinCommitTicks: k}, 1, descriptors...)
	configureTestHost(s.Session(), func(host *builtins.Host) { host.TaskYielder = s })
	return s
}

// runYinTaskToCompletion runs the task's first slice, then drives the queue
// until it completes, and returns the first slice's end state.
func runYinTaskToCompletion(t *testing.T, s *Runtime, queued *Task) task.TaskState {
	t.Helper()
	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	firstSlice := queued.GetState()
	deadline := time.Now().Add(2 * time.Second)
	for queued.GetState() != task.TaskCompleted && time.Now().Before(deadline) {
		s.ProcessReadyTasks()
	}
	if state := queued.GetState(); state != task.TaskCompleted {
		t.Fatalf("task state = %v, want completed (result %+v)", state, queued.Result)
	}
	return firstSlice
}

// The loop burns a few hundred ticks; yin() with no arguments is far from its
// 2000-tick and 2-second thresholds, so only YIN_COMMIT_TICKS can make it yield.
const yinOrderProgram = `
#0.yield_order = {"main-before"};
fork (0)
  #0.yield_order = listappend(#0.yield_order, "fork");
endfork
for i in [1..300]
endfor
yin();
#0.yield_order = listappend(#0.yield_order, "main-after");
return #0.yield_order;
`

func runYinOrder(t *testing.T, k int, taskID int64) (string, task.TaskState) {
	t.Helper()
	store := newYinCommitStore(t)
	s := newYinCommitRuntime(t, store, k)
	defer s.Stop()
	owner := types.ObjID(7800 + taskID%100)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(taskID, owner, compileTestProgram(t, s.registry, yinOrderProgram), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)
	firstSlice := runYinTaskToCompletion(t, s, queued)
	return queued.Result.Val.String(), firstSlice
}

func TestYinCommitTicksYieldsAfterKTicks(t *testing.T) {
	got, firstSlice := runYinOrder(t, 100, 3201)
	if firstSlice == task.TaskCompleted {
		t.Fatal("first slice completed; want it to end at the yin")
	}
	if want := `{"main-before", "fork", "main-after"}`; got != want {
		t.Fatalf("order = %s, want %s (fork runs at the yin)", got, want)
	}
}

func TestYinCommitTicksDoesNotYieldBelowK(t *testing.T) {
	got, firstSlice := runYinOrder(t, 50000, 3202)
	if firstSlice != task.TaskCompleted {
		t.Fatalf("first slice ended in %v; want the task to complete without yielding", firstSlice)
	}
	if want := `{"main-before", "main-after"}`; got != want {
		t.Fatalf("order = %s, want %s (no yield)", got, want)
	}
}

func TestYinCommitTicksOffDoesNotYield(t *testing.T) {
	got, firstSlice := runYinOrder(t, 0, 3203)
	if firstSlice != task.TaskCompleted {
		t.Fatalf("first slice ended in %v; want the task to complete without yielding", firstSlice)
	}
	if want := `{"main-before", "main-after"}`; got != want {
		t.Fatalf("order = %s, want %s (no yield)", got, want)
	}
}

// A builtin-started verb runs on a nested VM, where a suspend is not honored,
// so yin() there must not yield: the rest of the nested verb runs and the task
// finishes in one slice.
func TestYinCommitTicksDoesNotYieldInNestedVM(t *testing.T) {
	store := newYinCommitStore(t)
	nested := dbstore.NewVerb([]string{"nested"}, 0, dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"for i in [1..300]", "endfor", "yin();", `#0.after_yin = "ran";`, "return 1;"})
	if _, ec := store.AddVerb(0, nested); ec != types.E_NONE {
		t.Fatalf("AddVerb nested: %v", ec)
	}
	var callNested builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
		return ctx.Session.CallVerb(0, "nested", nil, ctx)
	}
	s := newYinCommitRuntime(t, store, 100, testBuiltinSlot("call_nested", 0, 0, []int64{}, &callNested))
	defer s.Stop()

	owner := types.ObjID(7804)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(3204, owner, compileTestProgram(t, s.registry, `
for i in [1..300]
endfor
r = call_nested();
return {r, #0.after_yin};
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)
	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if state := queued.GetState(); state != task.TaskCompleted {
		t.Fatalf("task state after one slice = %v, want completed", state)
	}
	if got, want := queued.Result.Val.String(), `{1, "ran"}`; got != want {
		t.Fatalf("result = %s (flow %v err %v), want %s", got, queued.Result.Flow, queued.Result.Error, want)
	}
}

// The yield commits the slice: its write is visible to another transaction
// while the yielder is still queued.
func TestYinCommitTicksPublishesWritesBeforeResume(t *testing.T) {
	store := newYinCommitStore(t)
	s := newYinCommitRuntime(t, store, 100)
	defer s.Stop()

	owner := types.ObjID(7805)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(3205, owner, compileTestProgram(t, s.registry, `
#0.write_value = 42;
for i in [1..300]
endfor
yin();
#0.write_value = 43;
return 1;
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)
	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if state := queued.GetState(); state == task.TaskCompleted {
		t.Fatal("task completed in its first slice; want it queued at the yin")
	}
	if got, ec := store.DirectTxn().PropertyValue(0, "write_value"); ec != types.E_NONE || got.Int() != 42 {
		t.Fatalf("write_value while yielder queued = %v (%v), want 42", got, ec)
	}
}

// A conflict after the yin re-runs the slice that began at the yin, not the
// task: the counting builtin before the yin runs once.
func TestYinCommitTicksConflictRerunsFromTheYin(t *testing.T) {
	store := newYinCommitStore(t)
	bump, _ := bumpReadValueLiveOnce(t, store)
	countCalls := 0
	var count builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
		countCalls++
		return types.Ok(types.NewInt(0))
	}
	s := newYinCommitRuntime(t, store, 100, bump, testBuiltinSlot("count_before_yin", 0, 0, []int64{}, &count))
	defer s.Stop()

	owner := types.ObjID(7806)
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := NewTaskFull(3206, owner, compileTestProgram(t, s.registry, `
count_before_yin();
for i in [1..300]
endfor
yin();
before = #0.read_value;
bump_read_value_live_once();
#0.write_value = before + 10;
return before;
`), ticks, seconds)
	queued.Context.IsWizard = true
	defer removeTasksForOwner(s, owner)
	runYinTaskToCompletion(t, s, queued)

	if store.CommitRetries() == 0 {
		t.Fatal("no commit retry; the test did not produce a conflict after the yin")
	}
	if countCalls != 1 {
		t.Fatalf("builtin before the yin ran %d times, want 1", countCalls)
	}
	if got := queued.Result.Val; got.Type() != types.TYPE_INT || got.Int() != 1 {
		t.Fatalf("result = %v, want 1 from the re-run", got)
	}
	if written, ec := store.DirectTxn().PropertyValue(0, "write_value"); ec != types.E_NONE || written.Int() != 11 {
		t.Fatalf("write_value = %v (%v), want 11", written, ec)
	}
}
