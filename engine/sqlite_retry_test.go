package engine

import (
	"fmt"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestInlineSQLiteRetriesBeforeExecutingStatement(t *testing.T) {
	for _, tc := range []struct {
		name, statement, readback, want string
	}{
		{"query_cte", `sqlite_query(#0.h, "WITH v(x) AS (SELECT 7) INSERT INTO t SELECT x FROM v");`, "SELECT x FROM t", "{{7}}"},
		{"execute_cte", `sqlite_execute(#0.h, "WITH v(x) AS (SELECT ?) INSERT INTO t SELECT x FROM v", {7});`, "SELECT x FROM t", "{{7}}"},
		{"pragma", `sqlite_query(#0.h, "PRAGMA user_version = 17");`, "PRAGMA user_version", "{{17}}"},
		{"select", `sqlite_query(#0.h, "SELECT 7");`, "SELECT 7", "{{7}}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newBoundaryTestStore(t)
			if ec := store.DirectTxn().DefineProperty(0, "h", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			var inlineGateHeldBuiltin builtins.BuiltinFunc
			descriptor, calls := bumpReadValueLiveOnce(t, store)
			rt := newTestRuntimeWithBuiltins(t, store, descriptor, testBuiltinSlot("inline_gate_held", 0, 0, []int64{}, &inlineGateHeldBuiltin))
			defer rt.Stop()
			inlineGateHeldBuiltin = func(ctx *builtins.Execution, _ []types.Value) types.Result {
				if ctx.StoreTxn.IsCommitGateExempt() {
					return types.Ok(types.NewInt(1))
				}
				return types.Ok(types.NewInt(0))
			}
			ticks, seconds := foregroundTaskLimits(newTestRegistry())
			setup := task.NewTaskFull(95010, 0, compileTestProgram(t, rt.registry, `
#0.h = sqlite_open(":memory:");
sqlite_query(#0.h, "CREATE TABLE t(x INTEGER)");
return 1;
`), ticks, seconds)
			setup.Context.IsWizard = true
			runSQLiteTaskToCompletion(t, rt, setup)
			if setup.Result.Flow != types.FlowReturn {
				t.Fatalf("setup: %+v", setup.Result)
			}
			defer rt.EvalCommandOutput(0, "return sqlite_close(#0.h);")
			running := task.NewTaskFull(95011, 0, compileTestProgram(t, rt.registry, fmt.Sprintf(`
set_thread_mode(0);
before = #0.read_value;
bump_read_value_live_once();
%s
held = inline_gate_held();
#0.write_value = before + 10;
return {before, sqlite_query(#0.h, %q), held};
`, tc.statement, tc.readback)), ticks, seconds)
			running.Context.IsWizard = true
			retries := store.CommitRetries()
			runSQLiteTaskToCompletion(t, rt, running)
			want := "{1, " + tc.want + ", 1}"
			if running.Result.Flow != types.FlowReturn || running.Result.Val.String() != want {
				t.Fatalf("result = %+v, want %s (one SQL effect under the gate)", running.Result, want)
			}
			if *calls != 2 || store.CommitRetries()-retries != 1 {
				t.Fatalf("attempts=%d retries=%d, want 2 and 1", *calls, store.CommitRetries()-retries)
			}
			assertCommitGateReleased(t, store)
		})
	}
}

// runSQLiteTaskToCompletion drives a task through its threaded SQLite suspends:
// each completion re-queues the task and the scheduler pass resumes it.
func runSQLiteTaskToCompletion(t *testing.T, rt *Runtime, tk *task.Task) {
	t.Helper()
	tk.SetState(task.TaskQueued)
	rt.taskManager.RegisterTask(tk)
	if err := rt.runTask(tk); err != nil {
		t.Fatalf("runTask: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		switch tk.GetState() {
		case task.TaskCompleted, task.TaskKilled:
			return
		case task.TaskQueued:
			rt.ProcessReadyTasks()
		default:
			time.Sleep(time.Millisecond)
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %d did not complete: state %v, result %+v", tk.ID, tk.GetState(), tk.Result)
		}
	}
}

// Issue #298: a task that suspends for a threaded sqlite statement commits its
// slice at the suspend. When that commit loses validation the task is re-run from
// the top. The statement must run exactly once (for the attempt that commits) and
// the retried attempt must observe only its own completion.
func TestSqliteThreadedStatementRunsOnceAcrossSuspendCommitRetry(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	root.SetProperty("retry_value", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	root.SetProperty("h", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}

	var forceRetryConflictBuiltin, attemptNoBuiltin builtins.BuiltinFunc
	rt := newTestRuntimeWithBuiltins(t, store, testBuiltinSlot("force_retry_conflict", 0, 0, []int64{}, &forceRetryConflictBuiltin), testBuiltinSlot("attempt_no", 0, 0, []int64{}, &attemptNoBuiltin))
	defer rt.Stop()
	forceCalls := 0
	forceRetryConflictBuiltin = func(ctx *builtins.Execution, _ []types.Value) types.Result {
		forceCalls++
		if forceCalls == 1 {
			// A concurrent commit after this attempt read retry_value: a retryable
			// read-set conflict, detected when the suspend commits the slice.
			if errCode := ctx.Store.DirectTxn().SetPropertyValue(0, "retry_value", types.NewInt(10)); errCode != types.E_NONE {
				return types.Err(errCode)
			}
		}
		return types.Ok(types.NewInt(0))
	}
	attempts := 0
	attemptNoBuiltin = func(_ *builtins.Execution, _ []types.Value) types.Result {
		attempts++
		return types.Ok(types.NewInt(int64(attempts)))
	}

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	setup := task.NewTaskFull(94010, 0, compileTestProgram(t, rt.registry, `
h = sqlite_open(":memory:");
sqlite_query(h, "CREATE TABLE t(attempt INTEGER)");
#0.h = h;
return h;
`), ticks, seconds)
	setup.Context.IsWizard = true
	runSQLiteTaskToCompletion(t, rt, setup)
	if setup.Result.Flow != types.FlowReturn {
		t.Fatalf("setup result = %+v, want a handle", setup.Result)
	}

	running := task.NewTaskFull(94011, 0, compileTestProgram(t, rt.registry, `
n = attempt_no();
before = #0.retry_value;
force_retry_conflict();
#0.retry_value = before + 1;
sqlite_execute(#0.h, "INSERT INTO t(attempt) VALUES (?)", {n});
rows = sqlite_query(#0.h, "SELECT attempt FROM t ORDER BY attempt");
return {n, rows};
`), ticks, seconds)
	running.Context.IsWizard = true
	retriesBefore := store.CommitRetries()
	runSQLiteTaskToCompletion(t, rt, running)

	if running.Result.Flow != types.FlowReturn {
		t.Fatalf("task result = %+v, want a return (the conflict must be retried, not surfaced)", running.Result)
	}
	if store.CommitRetries() == retriesBefore {
		t.Fatal("forced conflict did not record a commit retry")
	}
	if forceCalls != 2 || attempts != 2 {
		t.Fatalf("attempt calls = force %d, attempt_no %d; want two attempts", forceCalls, attempts)
	}
	if got := running.Result.Val.String(); got != "{2, {{2}}}" {
		t.Fatalf("result = %s, want {2, {{2}}}: only the committing attempt's INSERT ran and only its completion resumed the task", got)
	}
	if got, errCode := store.DirectTxn().PropertyValue(0, "retry_value"); errCode != types.E_NONE || got.Int() != 11 {
		t.Fatalf("retry_value = %v (%v), want 11 committed by the retried attempt", got, errCode)
	}
}
