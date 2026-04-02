package builtins

import (
	"barn/types"
	"testing"
)

func resetSQLiteTestState(t *testing.T) {
	t.Helper()

	sqliteState.mu.Lock()
	handles := make([]*sqliteHandle, 0, len(sqliteState.handles))
	for _, handle := range sqliteState.handles {
		handles = append(handles, handle)
	}
	sqliteState.handles = make(map[int64]*sqliteHandle)
	sqliteState.nextID = 1
	sqliteState.mu.Unlock()

	for _, handle := range handles {
		if handle.conn != nil {
			_ = handle.conn.Close()
		}
		if handle.db != nil {
			_ = handle.db.Close()
		}
	}
}

func sqliteWizardCtx() *types.TaskContext {
	ctx := types.NewTaskContext()
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
	n, ok := value.(types.IntValue)
	if !ok {
		t.Fatalf("expected int, got %T", value)
	}
	return n.Val
}

func sqliteMustList(t *testing.T, value types.Value) types.ListValue {
	t.Helper()
	list, ok := value.(types.ListValue)
	if !ok {
		t.Fatalf("expected list, got %T", value)
	}
	return list
}

func sqliteMustMap(t *testing.T, value types.Value) types.MapValue {
	t.Helper()
	m, ok := value.(types.MapValue)
	if !ok {
		t.Fatalf("expected map, got %T", value)
	}
	return m
}

func sqliteMustString(t *testing.T, value types.Value) string {
	t.Helper()
	s, ok := value.(types.StrValue)
	if !ok {
		t.Fatalf("expected string, got %T", value)
	}
	return s.Value()
}

func sqliteMapGet(t *testing.T, m types.MapValue, key string) types.Value {
	t.Helper()
	value, ok := m.Get(types.NewStr(key))
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	return value
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
	if got := row.Get(3).(types.FloatValue).Val; got != 3.5 {
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
