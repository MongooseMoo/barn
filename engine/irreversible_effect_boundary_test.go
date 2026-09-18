package engine

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// The irreversible-effect boundary: a task whose reads went stale before its
// first irreversible builtin is re-run from the top instead of performing the
// effect and then losing its commit as an uncatchable E_INVARG. This is the
// shape of the Mongoose login task that died at its read() after
// set_connection_option when a concurrent writer moved a property it had read.

func newBoundaryTestStore(t *testing.T) *dbstore.Store {
	t.Helper()
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
	return store
}

// bumpReadValueLiveOnce describes a builtin standing in for a concurrent
// writer: the first call moves #0.read_value in the live store (bypassing the
// task's transaction, like another task's commit landing between this task's
// snapshot and its irreversible effect); later calls do nothing.
func bumpReadValueLiveOnce(t *testing.T, store *dbstore.Store) (builtins.Descriptor, *int) {
	t.Helper()
	calls := 0
	var callback builtins.BuiltinFunc = func(ctx *builtins.Execution, args []types.Value) types.Result {
		calls++
		if calls > 1 {
			return types.Ok(types.NewInt(0))
		}
		value, errCode := store.DirectTxn().PropertyValue(0, "read_value")
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if errCode := store.DirectTxn().SetPropertyValue(0, "read_value", types.NewInt(value.Int()+1)); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		return types.Ok(types.NewInt(0))
	}
	return testBuiltinSlot("bump_read_value_live_once", 0, 0, []int64{}, &callback), &calls
}

func assertCommitGateReleased(t *testing.T, store *dbstore.Store) {
	t.Helper()
	acquired := make(chan struct{})
	go func() {
		store.EscalationLock()
		store.EscalationUnlock()
		close(acquired)
	}()
	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("commit gate still held after the task finished its slice")
	}
}

func TestRunTaskRetriesStaleReadsBeforeFirstIrreversibleEffect(t *testing.T) {
	store := newBoundaryTestStore(t)
	descriptor, _ := bumpReadValueLiveOnce(t, store)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1, descriptor)
	defer s.Stop()

	var logs bytes.Buffer
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3101, 0, compileTestProgram(t, s.registry, `
before = #0.read_value;
bump_read_value_live_once();
server_log("irreversible-once");
#0.write_value = before + 10;
return before;
`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.Context.Log = slog.New(slog.NewTextHandler(&logs, nil))

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 1 {
		t.Fatalf("result = flow %v val %v, want return of the re-read value 1", queued.Result.Flow, queued.Result.Val)
	}
	if got := strings.Count(logs.String(), "irreversible-once"); got != 1 {
		t.Fatalf("server_log executions = %d, want exactly 1 (none on the aborted attempt)", got)
	}
	if got := store.CommitRetries(); got != 1 {
		t.Fatalf("commit retries = %d, want 1", got)
	}
	if got := store.CommitEscalations(); got == 0 {
		t.Fatal("commit escalations = 0, want the boundary to have taken the gate")
	}
	written, errCode := store.DirectTxn().PropertyValue(0, "write_value")
	if errCode != types.E_NONE || written.Int() != 11 {
		t.Fatalf("write_value = %v (%v), want 11 committed from the re-run", written, errCode)
	}
	assertCommitGateReleased(t, store)
}

func TestRunTaskSuspendAfterIrreversibleEffectCommitsInsteadOfPhantomError(t *testing.T) {
	store := newBoundaryTestStore(t)
	descriptor, _ := bumpReadValueLiveOnce(t, store)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1, descriptor)
	defer s.Stop()

	var logs bytes.Buffer
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	owner := types.ObjID(7711)
	queued := task.NewTaskFull(3102, owner, compileTestProgram(t, s.registry, `
before = #0.read_value;
bump_read_value_live_once();
server_log("irreversible-once");
#0.write_value = before + 10;
suspend(0);
return before;
`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.Context.Log = slog.New(slog.NewTextHandler(&logs, nil))
	defer removeTasksForOwner(s, owner)

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowSuspend {
		t.Fatalf("result = flow %v err %v, want the slice to end in a suspend, not an exception", queued.Result.Flow, queued.Result.Error)
	}
	if got := strings.Count(logs.String(), "irreversible-once"); got != 1 {
		t.Fatalf("server_log executions = %d, want exactly 1", got)
	}
	if got := store.CommitRetries(); got != 1 {
		t.Fatalf("commit retries = %d, want 1", got)
	}
	written, errCode := store.DirectTxn().PropertyValue(0, "write_value")
	if errCode != types.E_NONE || written.Int() != 11 {
		t.Fatalf("write_value = %v (%v), want 11 committed at the suspend", written, errCode)
	}
	assertCommitGateReleased(t, store)
}

func TestRunTaskIrreversibleEffectWithFreshReadsRunsOnce(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := newRuntimeWithWorkerCount(store, config.Options{}, 1)
	defer s.Stop()

	var logs bytes.Buffer
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3103, 0, compileTestProgram(t, s.registry, `
before = #0.read_value;
server_log("irreversible-once");
#0.write_value = before + 10;
return before;
`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.Context.Log = slog.New(slog.NewTextHandler(&logs, nil))

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 0 {
		t.Fatalf("result = flow %v val %v, want return 0", queued.Result.Flow, queued.Result.Val)
	}
	if got := strings.Count(logs.String(), "irreversible-once"); got != 1 {
		t.Fatalf("server_log executions = %d, want 1", got)
	}
	if got := store.CommitRetries(); got != 0 {
		t.Fatalf("commit retries = %d, want 0", got)
	}
	assertCommitGateReleased(t, store)
}

// A coarse builtin mutates the live store directly, which a whole-task re-run
// could not undo either, so it crosses the same boundary before touching
// anything: stale reads are answered by re-running, the verb is added exactly
// once, and no E_INVARG reaches the task.
func TestRunTaskRetriesStaleReadsBeforeFirstCoarseBuiltin(t *testing.T) {
	store := newBoundaryTestStore(t)
	descriptor, _ := bumpReadValueLiveOnce(t, store)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1, descriptor)
	defer s.Stop()

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3104, 0, compileTestProgram(t, s.registry, `
before = #0.read_value;
bump_read_value_live_once();
add_verb(#0, {#0, "rxd", "coarse_probe"}, {"this", "none", "this"});
#0.write_value = before + 10;
return {before, verbs(#0)};
`), ticks, seconds)
	queued.Context.IsWizard = true

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.String() != `{1, {"coarse_probe"}}` {
		t.Fatalf("result = flow %v val %v err %v, want return {1, {\"coarse_probe\"}} (re-read value; verb added once)", queued.Result.Flow, queued.Result.Val, queued.Result.Error)
	}
	if got := store.CommitRetries(); got != 1 {
		t.Fatalf("commit retries = %d, want 1", got)
	}
	if got := store.CommitEscalations(); got == 0 {
		t.Fatal("commit escalations = 0, want the boundary to have taken the gate")
	}
	written, errCode := store.DirectTxn().PropertyValue(0, "write_value")
	if errCode != types.E_NONE || written.Int() != 11 {
		t.Fatalf("write_value = %v (%v), want 11 committed from the re-run", written, errCode)
	}
	assertCommitGateReleased(t, store)
}

// The boundary also stops an attempt from inside a nested verb-call VM (a
// create() running :initialize here): the nested VM's result goes back to the
// builtin that ran it, so the stop is carried by the context flag and the
// outer VM unwinds at its next builtin boundary. The effect runs exactly once.
func TestRunTaskRetriesStaleReadsAtIrreversibleEffectInsideNestedVM(t *testing.T) {
	store := newBoundaryTestStore(t)
	initialize := dbstore.NewVerb("initialize", []string{"initialize"}, 0, dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{`server_log("nested-once");`, "return 1;"})
	if _, ec := store.AddVerb(0, initialize); ec != types.E_NONE {
		t.Fatalf("AddVerb initialize: %v", ec)
	}
	descriptor, _ := bumpReadValueLiveOnce(t, store)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1, descriptor)
	defer s.Stop()

	var logs bytes.Buffer
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3105, 0, compileTestProgram(t, s.registry, `
before = #0.read_value;
bump_read_value_live_once();
c = create(#0);
#0.write_value = before + 10;
return {before, valid(c)};
`), ticks, seconds)
	queued.Context.IsWizard = true
	queued.Context.Log = slog.New(slog.NewTextHandler(&logs, nil))

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.String() != "{1, 1}" {
		t.Fatalf("result = flow %v val %v err %v, want return {1, 1}", queued.Result.Flow, queued.Result.Val, queued.Result.Error)
	}
	if got := strings.Count(logs.String(), "nested-once"); got != 1 {
		t.Fatalf("server_log executions = %d, want exactly 1 (none on the aborted attempt)", got)
	}
	if got := store.CommitRetries(); got != 1 {
		t.Fatalf("commit retries = %d, want 1", got)
	}
	written, errCode := store.DirectTxn().PropertyValue(0, "write_value")
	if errCode != types.E_NONE || written.Int() != 11 {
		t.Fatalf("write_value = %v (%v), want 11 committed from the re-run", written, errCode)
	}
	assertCommitGateReleased(t, store)
}
