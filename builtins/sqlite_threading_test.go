package builtins

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Toast's thread callbacks return sqlite3_errmsg() verbatim (sqlite.cc
// sqlite_query_thread_callback / sqlite_execute_thread_callback). The driver's
// decorated "SQL logic error: ... (1)" form must not leak into MOO values.
func TestSqliteErrorStringsMatchToastErrmsg(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	ctx := sqliteWizardCtx()
	handle := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})))
	defer sqliteCloseAllHandles(ctx)
	h := types.NewInt(handle)

	sqliteMustResult(t, builtinSqliteQuery(ctx, []types.Value{h, types.NewStr("BEGIN TRANSACTION;")}))
	nested := sqliteMustString(t, sqliteMustResult(t, builtinSqliteQuery(ctx, []types.Value{h, types.NewStr("BEGIN TRANSACTION;")})))
	if nested != "cannot start a transaction within a transaction" {
		t.Fatalf("nested BEGIN = %q, want sqlite3_errmsg text", nested)
	}
	sqliteMustResult(t, builtinSqliteQuery(ctx, []types.Value{h, types.NewStr("ROLLBACK;")}))

	missing := sqliteMustString(t, sqliteMustResult(t, builtinSqliteQuery(ctx, []types.Value{h, types.NewStr("SELECT * FROM nosuch")})))
	if missing != "no such table: nosuch" {
		t.Fatalf("query on missing table = %q, want sqlite3_errmsg text", missing)
	}
	missingExec := sqliteMustString(t, sqliteMustResult(t, builtinSqliteExecute(ctx, []types.Value{
		h, types.NewStr("INSERT INTO nosuch VALUES (?)"), types.NewList([]types.Value{types.NewInt(1)}),
	})))
	if missingExec != "no such table: nosuch" {
		t.Fatalf("execute on missing table = %q, want sqlite3_errmsg text", missingExec)
	}
}

// With set_thread_mode(0) Toast's background_thread() runs the callback inline
// and returns its value; the task never suspends (background.cc).
func TestSqliteUnthreadedModeRunsInlineWithoutSuspending(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	ctx, taskValue := sqliteAsyncCtx()
	ctx.ThreadMode = false
	handle := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})))
	defer sqliteCloseAllHandles(ctx)
	h := types.NewInt(handle)

	rows := builtinSqliteQuery(ctx, []types.Value{h, types.NewStr("SELECT 7")})
	if rows.Flow == types.FlowSuspend || rows.IsError() {
		t.Fatalf("unthreaded query = %+v, want a direct value", rows)
	}
	if got := rows.Val.String(); got != "{{7}}" {
		t.Fatalf("unthreaded query = %s, want {{7}}", got)
	}
	if got := taskValue.GetState(); got != task.TaskRunning {
		t.Fatalf("task state = %v, want still running (no suspend)", got)
	}
	if len(ctx.PendingEffects) != 0 {
		t.Fatalf("unthreaded query queued %d pending effects, want none", len(ctx.PendingEffects))
	}
	if ctx.IrreversibleSideEffect {
		t.Fatal("an inline SELECT flagged an irreversible side effect; a retry replays it harmlessly")
	}

	create := builtinSqliteQuery(ctx, []types.Value{h, types.NewStr("CREATE TABLE t(x INTEGER)")})
	if create.Flow == types.FlowSuspend || create.IsError() {
		t.Fatalf("unthreaded CREATE = %+v, want a direct value", create)
	}
	if !ctx.IrreversibleSideEffect {
		t.Fatal("an inline write did not flag an irreversible side effect; a conflict retry would run it twice")
	}
}

// The threaded statement is an external effect. It must not start until the
// slice that issued it commits (FlushPendingEffects); a discarded attempt must
// never run it, and the resume it produces is bound to the suspension it was
// started for.
func TestSqliteThreadedStartIsDeferredToCommit(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	ctx, taskValue := sqliteAsyncCtx()
	handle := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})))
	defer sqliteCloseAllHandles(ctx)
	h := types.NewInt(handle)
	// A task-less execution on the same session runs statements inline.
	inline := ctx.Session.NewExecution(kernel.NewTaskContext(), nil)
	inline.IsWizard = true
	sqliteMustResult(t, builtinSqliteQuery(inline, []types.Value{h, types.NewStr("CREATE TABLE t(x INTEGER)")}))

	// Attempt 1 issues an INSERT, then loses its commit: the effect log is discarded.
	first := builtinSqliteExecute(ctx, []types.Value{h, types.NewStr("INSERT INTO t(x) VALUES (?)"), types.NewList([]types.Value{types.NewInt(1)})})
	if first.Flow != types.FlowSuspend {
		t.Fatalf("threaded execute flow = %v, want suspend", first.Flow)
	}
	if got := taskValue.GetState(); got != task.TaskSuspended {
		t.Fatalf("task state = %v, want suspended", got)
	}
	if ctx.IrreversibleSideEffect {
		t.Fatal("threaded execute flagged an irreversible side effect before anything ran")
	}
	if len(ctx.PendingEffects) != 1 {
		t.Fatalf("pending effects = %d, want the deferred start", len(ctx.PendingEffects))
	}
	time.Sleep(20 * time.Millisecond)
	if got := taskValue.GetState(); got != task.TaskSuspended {
		t.Fatalf("task state = %v before the commit, want still suspended (nothing started)", got)
	}
	DiscardPendingEffects(ctx)

	// Attempt 2 re-runs from the top and suspends for its own INSERT; only that
	// one may run, and only its completion may wake the task.
	taskValue.SetState(task.TaskRunning)
	second := builtinSqliteExecute(ctx, []types.Value{h, types.NewStr("INSERT INTO t(x) VALUES (?)"), types.NewList([]types.Value{types.NewInt(2)})})
	if second.Flow != types.FlowSuspend {
		t.Fatalf("threaded execute flow = %v, want suspend", second.Flow)
	}
	FlushPendingEffects(ctx)
	waitForSQLiteResume(t, taskValue)
	if taskValue.WakeValue.Type() != types.TYPE_LIST || taskValue.WakeValue.Len() != 0 {
		t.Fatalf("wake value = %v, want the INSERT's empty row list", taskValue.WakeValue)
	}

	rows := sqliteMustResult(t, builtinSqliteQuery(inline, []types.Value{h, types.NewStr("SELECT x FROM t ORDER BY x")}))
	if got := rows.String(); got != "{{2}}" {
		t.Fatalf("rows after retry = %s, want only attempt 2's row", got)
	}
}
