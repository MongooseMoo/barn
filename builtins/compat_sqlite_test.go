package builtins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func resetSQLiteTestState(t *testing.T) { t.Helper() }

// TestSqliteOpenRefusesSandboxEscape confirms the sole sqlite path entry point
// (sqlite_open) rejects a traversal escape with E_INVARG, matching the fileio
// builtins (and Toast's file_resolve_path -> file_verify_path NULL -> E_INVARG,
// toaststunt/src/sqlite.cc:241-246).
func TestSqliteOpenRefusesSandboxEscape(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	ctx := sqliteWizardCtx()
	for _, escape := range []string{"../escape.db", "../../etc/escape.db"} {
		res := builtinSqliteOpen(ctx, []types.Value{types.NewStr(escape)})
		if !res.IsError() || res.Error != types.E_INVARG {
			t.Fatalf("sqlite_open(%q): want E_INVARG, got result=%v err=%v",
				escape, res.Val, res.Error)
		}
	}
}

func sqliteWizardCtx() *Execution {
	ctx := newTestExecution()
	ctx.IsWizard = true
	ctx.Player = 3
	ctx.Programmer = 3
	return ctx
}

func sqliteMustResult(t *testing.T, result types.Result) types.Value {
	t.Helper()
	if result.IsError() {
		t.Fatalf("unexpected error %v", result.Error)
	}
	return result.Val
}

func sqliteMustInt(t *testing.T, value types.Value) int64 {
	t.Helper()
	if value.Type() != types.TYPE_INT {
		t.Fatalf("expected int, got %T", value)
	}
	return value.Int()
}

func sqliteMustList(t *testing.T, value types.Value) types.Value {
	t.Helper()
	if value.Type() != types.TYPE_LIST {
		t.Fatalf("expected list, got %T", value)
	}
	return value
}

func sqliteMustMap(t *testing.T, value types.Value) types.Value {
	t.Helper()
	if value.Type() != types.TYPE_MAP {
		t.Fatalf("expected map, got %T", value)
	}
	return value
}

func sqliteMustString(t *testing.T, value types.Value) string {
	t.Helper()
	if value.Type() != types.TYPE_STR {
		t.Fatalf("expected string, got %T", value)
	}
	return value.Str()
}

func sqliteMapGet(t *testing.T, m types.Value, key string) types.Value {
	t.Helper()
	value, ok := m.MapGet(types.NewStr(key))
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	return value
}

func sqliteAsyncCtx() (*Execution, *task.Task) {
	ctx := sqliteWizardCtx()
	taskValue := task.NewTask(1, ctx.Programmer, 1000, 1)
	taskValue.SetState(task.TaskRunning)
	ctx.Task = taskValue
	wireTestTaskManager(ctx)
	return ctx, taskValue
}

func waitForSQLiteResume(t *testing.T, taskValue *task.Task) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for taskValue.GetState() == task.TaskSuspended && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := taskValue.GetState(); got != task.TaskQueued {
		t.Fatalf("task state = %v, want queued after SQLite completion", got)
	}
}

func TestSqliteAsyncErrorAlwaysResumesTask(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	ctx, taskValue := sqliteAsyncCtx()
	handle := newSQLiteHandle(1, ":memory:", nil, nil)
	handle.closed = true

	result := sqliteExecOrQueryAsync(ctx, handle, "SELECT 1", nil, false)
	if result.Flow != types.FlowSuspend {
		t.Fatalf("result flow = %v, want suspend", result.Flow)
	}
	waitForSQLiteResume(t, taskValue)
	if taskValue.WakeValue.Type() != types.TYPE_ERR || taskValue.WakeValue.ErrCode() != types.E_INVARG {
		t.Fatalf("wake value = %v, want E_INVARG", taskValue.WakeValue)
	}
}

func TestSqliteCloseWaitsOffTaskGoroutine(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	ctx, taskValue := sqliteAsyncCtx()
	handle := newSQLiteHandle(1, ":memory:", nil, nil)
	handle.activeOps = 1
	ctx.Session.runtime.sqlite.mu.Lock()
	ctx.Session.runtime.sqlite.handles[handle.id] = handle
	ctx.Session.runtime.sqlite.mu.Unlock()

	returned := make(chan types.Result, 1)
	go func() {
		returned <- builtinSqliteClose(ctx, []types.Value{types.NewInt(handle.id)})
	}()
	select {
	case result := <-returned:
		if result.Flow != types.FlowSuspend {
			t.Fatalf("result flow = %v, want suspend", result.Flow)
		}
	case <-time.After(time.Second):
		t.Fatal("sqlite_close blocked the task goroutine")
	}

	if got := taskValue.GetState(); got != task.TaskSuspended {
		t.Fatalf("task state = %v, want suspended while close waits", got)
	}
	handle.mu.Lock()
	handle.activeOps = 0
	handle.cond.Broadcast()
	handle.mu.Unlock()
	waitForSQLiteResume(t, taskValue)
	if taskValue.WakeValue.Type() != types.TYPE_INT || taskValue.WakeValue.Int() != 0 {
		t.Fatalf("wake value = %v, want 0", taskValue.WakeValue)
	}
}

func TestSqliteOpenInfoAndHandles(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	ctx := sqliteWizardCtx()
	handleValue := sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")}))
	handleID := sqliteMustInt(t, handleValue)

	info := sqliteMustMap(t, sqliteMustResult(t, builtinSqliteInfo(ctx, []types.Value{types.NewInt(handleID)})))
	if got := sqliteMustString(t, sqliteMapGet(t, info, "path")); got != ":memory:" {
		t.Fatalf("path = %q, want :memory:", got)
	}
	if got := sqliteMustInt(t, sqliteMapGet(t, info, "parse_types")); got != 1 {
		t.Fatalf("parse_types = %d, want 1", got)
	}
	if got := sqliteMustInt(t, sqliteMapGet(t, info, "parse_objects")); got != 1 {
		t.Fatalf("parse_objects = %d, want 1", got)
	}
	if got := sqliteMustInt(t, sqliteMapGet(t, info, "sanitize_strings")); got != 0 {
		t.Fatalf("sanitize_strings = %d, want 0", got)
	}
	if got := sqliteMustInt(t, sqliteMapGet(t, info, "locks")); got != 0 {
		t.Fatalf("locks = %d, want 0", got)
	}

	handles := sqliteMustList(t, sqliteMustResult(t, builtinSqliteHandles(ctx, nil)))
	if handles.Len() != 1 || sqliteMustInt(t, handles.Get(1)) != handleID {
		t.Fatalf("unexpected handles %v", handles)
	}
}

func TestSqliteExecuteAndQueryShapes(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	ctx := sqliteWizardCtx()
	handleID := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})))

	sqliteMustResult(t, builtinSqliteQuery(ctx, []types.Value{
		types.NewInt(handleID),
		types.NewStr("CREATE TABLE t(id INTEGER PRIMARY KEY, n INTEGER, x REAL, obj TEXT, label TEXT)"),
	}))
	inserted := sqliteMustList(t, sqliteMustResult(t, builtinSqliteExecute(ctx, []types.Value{
		types.NewInt(handleID),
		types.NewStr("INSERT INTO t(n, x, obj, label) VALUES (?, ?, ?, ?)"),
		types.NewList([]types.Value{types.NewInt(42), types.NewFloat(3.5), types.NewObj(0), types.NewStr("alpha")}),
	})))
	if inserted.Len() != 0 {
		t.Fatalf("insert result = %v, want {}", inserted)
	}

	lastID := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteLastInsertRowID(ctx, []types.Value{types.NewInt(handleID)})))
	if lastID != 1 {
		t.Fatalf("last insert row id = %d, want 1", lastID)
	}

	rows := sqliteMustList(t, sqliteMustResult(t, builtinSqliteQuery(ctx, []types.Value{
		types.NewInt(handleID),
		types.NewStr("SELECT id, n, x, obj, label FROM t"),
	})))
	if rows.Len() != 1 {
		t.Fatalf("row count = %d, want 1", rows.Len())
	}
	row := sqliteMustList(t, rows.Get(1))
	if sqliteMustInt(t, row.Get(1)) != 1 || sqliteMustInt(t, row.Get(2)) != 42 {
		t.Fatalf("unexpected numeric columns %v", row)
	}
	if got := row.Get(3).Float(); got != 3.5 {
		t.Fatalf("float column = %v, want 3.5", got)
	}
	if got := sqliteMustString(t, row.Get(4)); got != "#0" {
		t.Fatalf("obj column = %q, want #0", got)
	}
	if got := sqliteMustString(t, row.Get(5)); got != "alpha" {
		t.Fatalf("label column = %q, want alpha", got)
	}

	headers := sqliteMustList(t, sqliteMustResult(t, builtinSqliteQuery(ctx, []types.Value{
		types.NewInt(handleID),
		types.NewStr("SELECT n AS first, label FROM t"),
		types.NewInt(1),
	})))
	headerRow := sqliteMustList(t, headers.Get(1))
	firstPair := sqliteMustList(t, headerRow.Get(1))
	if sqliteMustString(t, firstPair.Get(1)) != "first" || sqliteMustInt(t, firstPair.Get(2)) != 42 {
		t.Fatalf("unexpected header row %v", headerRow)
	}
}

func TestSqliteLimitNameAndNumberParity(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })

	ctx := sqliteWizardCtx()
	handleID := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})))

	nameLimit := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteLimit(ctx, []types.Value{
		types.NewInt(handleID),
		types.NewStr("LIMIT_COLUMN"),
		types.NewInt(-1),
	})))
	numberLimit := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteLimit(ctx, []types.Value{
		types.NewInt(handleID),
		types.NewInt(2),
		types.NewInt(-1),
	})))
	if nameLimit != numberLimit || nameLimit <= 0 {
		t.Fatalf("limit mismatch: name=%d number=%d", nameLimit, numberLimit)
	}

	prev := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteLimit(ctx, []types.Value{
		types.NewInt(handleID),
		types.NewInt(2),
		types.NewInt(nameLimit - 1),
	})))
	if prev != nameLimit {
		t.Fatalf("previous limit = %d, want %d", prev, nameLimit)
	}

	current := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteLimit(ctx, []types.Value{
		types.NewInt(handleID),
		types.NewStr("LIMIT_COLUMN"),
		types.NewInt(-1),
	})))
	if current != nameLimit-1 {
		t.Fatalf("current limit = %d, want %d", current, nameLimit-1)
	}
}

// sqliteCloseAllHandles releases every open database so Windows can delete the
// test's temporary files; sqlite_close itself completes off-task.
func sqliteCloseAllHandles(ctx *Execution) {
	rt := ctx.Session.runtime.sqlite
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for id, h := range rt.handles {
		if h.conn != nil {
			_ = h.conn.Close()
		}
		if h.db != nil {
			_ = h.db.Close()
		}
		delete(rt.handles, id)
	}
}

// TestSqliteOpenFilePathReportedUnderFiles pins the MOO-visible path of a
// file-backed database to Toast's file_resolve_path output: file_subdir plus
// the caller's spelling with one leading "/" removed, forward slashes on every
// platform (toaststunt/src/fileio.cc:318-335, sqlite.cc:279,351). Mongoose's
// #3882::is_open compares sqlite_info()["path"] with "files/" + path, so a
// platform-native separator makes every wrapped query raise "not open".
func TestSqliteOpenFilePathReportedUnderFiles(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })
	t.Chdir(t.TempDir())

	ctx := sqliteWizardCtx()
	t.Cleanup(func() { sqliteCloseAllHandles(ctx) })
	for spelled, want := range map[string]string{
		"probe.sqlite":        "files/probe.sqlite",
		"/slashed.sqlite":     "files/slashed.sqlite",
		"nested/dir.sqlite":   "files/nested/dir.sqlite",
		"/nested/dir2.sqlite": "files/nested/dir2.sqlite",
	} {
		if strings.Contains(spelled, "nested") {
			if err := os.MkdirAll("files/nested", 0o755); err != nil {
				t.Fatal(err)
			}
		}
		handleID := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(spelled)})))
		info := sqliteMustMap(t, sqliteMustResult(t, builtinSqliteInfo(ctx, []types.Value{types.NewInt(handleID)})))
		if got := sqliteMustString(t, sqliteMapGet(t, info, "path")); got != want {
			t.Fatalf("sqlite_open(%q): info path = %q, want %q", spelled, got, want)
		}
		if _, err := os.Stat(filepath.FromSlash(want)); err != nil {
			t.Fatalf("sqlite_open(%q): database file not at %q: %v", spelled, want, err)
		}
	}
}

// TestSqliteOpenSamePathTwiceRaisesWithExistingHandle pins Toast's
// database_already_open check (toaststunt/src/sqlite.cc:140-150, 249-256): a
// second open of the same resolved path raises E_INVARG carrying the message
// "Database already open with handle: N" and the existing handle as the value,
// allocates nothing, and ":memory:" databases never collide.
func TestSqliteOpenSamePathTwiceRaisesWithExistingHandle(t *testing.T) {
	resetSQLiteTestState(t)
	t.Cleanup(func() { resetSQLiteTestState(t) })
	t.Chdir(t.TempDir())

	ctx := sqliteWizardCtx()
	t.Cleanup(func() { sqliteCloseAllHandles(ctx) })
	first := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr("dup.sqlite")})))
	before := sqliteMustList(t, sqliteMustResult(t, builtinSqliteHandles(ctx, nil))).Len()

	for _, respelled := range []string{"dup.sqlite", "/dup.sqlite"} {
		res := builtinSqliteOpen(ctx, []types.Value{types.NewStr(respelled)})
		if res.Flow != types.FlowException || res.Error != types.E_INVARG {
			t.Fatalf("sqlite_open(%q) second open: want raised E_INVARG, got flow=%v err=%v val=%v", respelled, res.Flow, res.Error, res.Val)
		}
		if res.Val.Type() != types.TYPE_LIST || res.Val.Len() != 3 {
			t.Fatalf("sqlite_open(%q): raise payload = %v, want {code, message, value}", respelled, res.Val)
		}
		wantMsg := fmt.Sprintf("Database already open with handle: %d", first)
		if got := res.Val.Get(2).Str(); got != wantMsg {
			t.Fatalf("sqlite_open(%q): message = %q, want %q", respelled, got, wantMsg)
		}
		if got := sqliteMustInt(t, res.Val.Get(3)); got != first {
			t.Fatalf("sqlite_open(%q): value = %d, want existing handle %d", respelled, got, first)
		}
	}
	if after := sqliteMustList(t, sqliteMustResult(t, builtinSqliteHandles(ctx, nil))).Len(); after != before {
		t.Fatalf("duplicate open allocated a handle: %d -> %d", before, after)
	}

	m1 := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})))
	m2 := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})))
	if m1 == m2 {
		t.Fatalf(":memory: opened twice returned the same handle %d", m1)
	}
}
