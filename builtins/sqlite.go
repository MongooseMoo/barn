package builtins

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/MongooseMoo/barn/types"

	_ "modernc.org/sqlite"
)

type sqliteHandle struct {
	id               int64
	path             string
	db               *sql.DB
	conn             *sql.Conn
	limits           map[int64]int64
	mu               sync.Mutex
	cond             *sync.Cond
	activeOps        int
	currentCancel    context.CancelFunc
	pendingInterrupt bool
	closed           bool
}

var sqliteLimitNames = map[string]int64{
	"LIMIT_LENGTH":              0,
	"LIMIT_SQL_LENGTH":          1,
	"LIMIT_COLUMN":              2,
	"LIMIT_EXPR_DEPTH":          3,
	"LIMIT_COMPOUND_SELECT":     4,
	"LIMIT_VDBE_OP":             5,
	"LIMIT_FUNCTION_ARG":        6,
	"LIMIT_ATTACHED":            7,
	"LIMIT_LIKE_PATTERN_LENGTH": 8,
	"LIMIT_VARIABLE_NUMBER":     9,
	"LIMIT_TRIGGER_DEPTH":       10,
	"LIMIT_WORKER_THREADS":      11,
}

func defaultSQLiteLimits() map[int64]int64 {
	return map[int64]int64{
		0:  1000000000,
		1:  1000000000,
		2:  2000,
		3:  1000,
		4:  500,
		5:  250000000,
		6:  127,
		7:  10,
		8:  50000,
		9:  32766,
		10: 1000,
		11: 0,
	}
}

func newSQLiteHandle(id int64, path string, db *sql.DB, conn *sql.Conn) *sqliteHandle {
	handle := &sqliteHandle{
		id:     id,
		path:   path,
		db:     db,
		conn:   conn,
		limits: defaultSQLiteLimits(),
	}
	handle.cond = sync.NewCond(&handle.mu)
	return handle
}

func getSQLiteHandle(ctx *Execution, v types.Value) (*sqliteHandle, types.ErrorCode) {
	if v.Type() != types.TYPE_INT {
		return nil, types.E_TYPE
	}

	ctx.Registry.runtime.sqlite.mu.Lock()
	handle := ctx.Registry.runtime.sqlite.handles[v.Int()]
	ctx.Registry.runtime.sqlite.mu.Unlock()
	if handle == nil {
		return nil, types.E_INVARG
	}
	return handle, types.E_NONE
}

func beginSQLiteOperation(handle *sqliteHandle) (context.Context, bool) {
	handle.mu.Lock()
	defer handle.mu.Unlock()

	for handle.activeOps > 0 && !handle.closed {
		handle.cond.Wait()
	}
	if handle.closed {
		return nil, false
	}

	handle.activeOps = 1
	opCtx, cancel := context.WithCancel(context.Background())
	if handle.pendingInterrupt {
		cancel()
		handle.pendingInterrupt = false
	}
	handle.currentCancel = cancel
	return opCtx, true
}

func endSQLiteOperation(handle *sqliteHandle) {
	handle.mu.Lock()
	cancel := handle.currentCancel
	handle.currentCancel = nil
	handle.activeOps = 0
	handle.cond.Broadcast()
	handle.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func sqliteReturnsRows(sqlText string) bool {
	trimmed := strings.TrimSpace(sqlText)
	if trimmed == "" {
		return false
	}
	upper := strings.ToUpper(trimmed)
	return strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "WITH") ||
		strings.HasPrefix(upper, "PRAGMA") ||
		strings.HasPrefix(upper, "VALUES") ||
		strings.HasPrefix(upper, "EXPLAIN")
}

func sqliteParamValue(v types.Value) any {
	switch v.Type() {
	case types.TYPE_INT:
		return v.Int()
	case types.TYPE_FLOAT:
		return v.Float()
	case types.TYPE_STR:
		return v.Str()
	case types.TYPE_OBJ, types.TYPE_ANON:
		return v.String()
	default:
		return v.String()
	}
}

func sqliteRowValue(v any) types.Value {
	switch value := v.(type) {
	case nil:
		return types.NewStr("NULL")
	case int64:
		return types.NewInt(value)
	case float64:
		return types.NewFloat(value)
	case string:
		return types.NewStr(value)
	case []byte:
		return types.NewStr(string(value))
	case bool:
		if value {
			return types.NewInt(1)
		}
		return types.NewInt(0)
	default:
		return types.NewStr("")
	}
}

func sqliteErrorResult(err error) types.Result {
	if err == nil {
		return types.Ok(types.NewEmptyList())
	}
	if errors.Is(err, context.Canceled) || strings.Contains(strings.ToLower(err.Error()), "interrupt") {
		return types.Ok(types.NewStr("interrupt"))
	}
	return types.Ok(types.NewStr(err.Error()))
}

func sqliteScanRows(rows *sql.Rows, includeHeaders bool) types.Result {
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return sqliteErrorResult(err)
	}

	resultRows := make([]types.Value, 0)
	for rows.Next() {
		rawValues := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range rawValues {
			scanTargets[i] = &rawValues[i]
		}

		if err := rows.Scan(scanTargets...); err != nil {
			return sqliteErrorResult(err)
		}

		rowValues := make([]types.Value, 0, len(columns))
		for i, raw := range rawValues {
			converted := sqliteRowValue(raw)
			if includeHeaders {
				rowValues = append(rowValues, types.NewList([]types.Value{types.NewStr(columns[i]), converted}))
				continue
			}
			rowValues = append(rowValues, converted)
		}
		resultRows = append(resultRows, types.NewList(rowValues))
	}

	if err := rows.Err(); err != nil {
		return sqliteErrorResult(err)
	}
	return types.Ok(types.NewList(resultRows))
}

func sqliteExecOrQuery(handle *sqliteHandle, sqlText string, params []any, includeHeaders bool) types.Result {
	opCtx, ok := beginSQLiteOperation(handle)
	if !ok {
		return types.Err(types.E_INVARG)
	}
	defer endSQLiteOperation(handle)

	if sqliteReturnsRows(sqlText) {
		rows, err := handle.conn.QueryContext(opCtx, sqlText, params...)
		if err != nil {
			return sqliteErrorResult(err)
		}
		return sqliteScanRows(rows, includeHeaders)
	}

	_, err := handle.conn.ExecContext(opCtx, sqlText, params...)
	if err != nil {
		return sqliteErrorResult(err)
	}
	return types.Ok(types.NewEmptyList())
}

func sqliteExecOrQueryAsync(ctx *Execution, handle *sqliteHandle, sqlText string, params []any, includeHeaders bool) types.Result {
	return runSQLiteAsync(ctx, func() types.Result {
		return sqliteExecOrQuery(handle, sqlText, params, includeHeaders)
	})
}

// runSQLiteAsync keeps waits on a handle's serialized operation queue off the
// scheduler's task goroutines. Every completion, including an error, resumes
// the suspended task exactly once.
func runSQLiteAsync(ctx *Execution, operation func() types.Result) types.Result {
	t := ctx.Task
	if t == nil {
		return operation()
	}

	mgr := taskManagerOf(ctx)
	if mgr == nil {
		return types.Err(types.E_INVARG)
	}
	mgr.SuspendTask(t, -1)
	go func() {
		result := operation()
		if result.IsError() {
			_ = t.Resume(types.NewErr(result.Error))
			return
		}
		_ = t.Resume(result.Val)
	}()
	return types.Suspend(-1)
}

func sqliteLimitCategory(v types.Value) (int64, types.ErrorCode) {
	switch v.Type() {
	case types.TYPE_INT:
		if _, ok := defaultSQLiteLimits()[v.Int()]; !ok {
			return 0, types.E_INVARG
		}
		return v.Int(), types.E_NONE
	case types.TYPE_STR:
		category, ok := sqliteLimitNames[v.Str()]
		if !ok {
			return 0, types.E_INVARG
		}
		return category, types.E_NONE
	default:
		return 0, types.E_TYPE
	}
}

// sqliteOpenError mirrors Toast's make_raise_pack(E_NONE, err, zero) on sqlite3_open failure.
func sqliteOpenError(msg string) types.Result {
	exceptionList := types.NewList([]types.Value{
		types.NewErr(types.E_NONE),
		types.NewStr(msg),
		types.NewInt(0),
	})
	return types.Result{Flow: types.FlowException, Error: types.E_NONE, Val: exceptionList}
}

func builtinSqliteOpen(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	path := args[0].Str()
	if len(args) == 2 {
		if args[1].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
	}

	if path != ":memory:" {
		// Confine the database file to the files/ sandbox the same way every
		// fileio builtin does. Toast resolves the sqlite path through
		// file_resolve_path (toaststunt/src/sqlite.cc:241), which both verifies
		// the path (file_verify_path) and prepends file_subdir
		// (toaststunt/src/fileio.cc:318-335). sanitizeFilePath is the verify
		// step; resolveFilePath is the file_subdir-prefix step. Calling only
		// sanitizeFilePath left the DB at a CWD-relative path (sandbox escape).
		sanitized, err := sanitizeFilePath(path)
		if err != nil {
			return types.Err(types.E_INVARG)
		}
		if err := ensureFilesRoot(); err != nil {
			return types.Err(types.E_FILE)
		}
		path = resolveFilePath(sanitized)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return sqliteOpenError(err.Error())
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	conn, err := db.Conn(context.Background())
	if err != nil {
		_ = db.Close()
		return sqliteOpenError(err.Error())
	}

	ctx.Registry.runtime.sqlite.mu.Lock()
	id := ctx.Registry.runtime.sqlite.nextID
	ctx.Registry.runtime.sqlite.nextID++
	ctx.Registry.runtime.sqlite.handles[id] = newSQLiteHandle(id, path, db, conn)
	ctx.Registry.runtime.sqlite.mu.Unlock()
	return types.Ok(types.NewInt(id))
}

func builtinSqliteClose(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	handle, code := getSQLiteHandle(ctx, args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}

	handle.mu.Lock()
	handle.closed = true
	handle.mu.Unlock()

	ctx.Registry.runtime.sqlite.mu.Lock()
	delete(ctx.Registry.runtime.sqlite.handles, handle.id)
	ctx.Registry.runtime.sqlite.mu.Unlock()

	return runSQLiteAsync(ctx, func() types.Result {
		handle.mu.Lock()
		for handle.activeOps > 0 {
			handle.cond.Wait()
		}
		handle.mu.Unlock()

		if handle.conn != nil {
			_ = handle.conn.Close()
		}
		if handle.db != nil {
			_ = handle.db.Close()
		}
		return types.Ok(types.NewInt(0))
	})
}

func builtinSqliteHandles(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}

	ctx.Registry.runtime.sqlite.mu.Lock()
	ids := make([]int64, 0, len(ctx.Registry.runtime.sqlite.handles))
	for id := range ctx.Registry.runtime.sqlite.handles {
		ids = append(ids, id)
	}
	ctx.Registry.runtime.sqlite.mu.Unlock()

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]types.Value, 0, len(ids))
	for _, id := range ids {
		out = append(out, types.NewInt(id))
	}
	return types.Ok(types.NewList(out))
}

func builtinSqliteInfo(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	handle, code := getSQLiteHandle(ctx, args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}

	handle.mu.Lock()
	locks := handle.activeOps
	handle.mu.Unlock()

	return types.Ok(types.NewMap([][2]types.Value{
		{types.NewStr("path"), types.NewStr(handle.path)},
		{types.NewStr("parse_types"), types.NewInt(1)},
		{types.NewStr("parse_objects"), types.NewInt(1)},
		{types.NewStr("sanitize_strings"), types.NewInt(0)},
		{types.NewStr("locks"), types.NewInt(int64(locks))},
	}))
}

func builtinSqliteQuery(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	handle, code := getSQLiteHandle(ctx, args[0])
	if code == types.E_INVARG {
		// An invalid handle yields the error value, not a raise (Toast).
		return types.Ok(types.NewErr(types.E_INVARG))
	}
	if code != types.E_NONE {
		return types.Err(code)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}

	includeHeaders := false
	if len(args) == 3 {
		includeHeaders = args[2].Truthy()
	}
	return sqliteExecOrQueryAsync(ctx, handle, args[1].Str(), nil, includeHeaders)
}

func builtinSqliteExecute(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	handle, code := getSQLiteHandle(ctx, args[0])
	if code == types.E_INVARG {
		// An invalid handle yields the error value, not a raise (Toast).
		return types.Ok(types.NewErr(types.E_INVARG))
	}
	if code != types.E_NONE {
		return types.Err(code)
	}
	if args[1].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	if args[2].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}

	params := make([]any, 0, args[2].Len())
	for _, value := range args[2].Elements() {
		params = append(params, sqliteParamValue(value))
	}
	return sqliteExecOrQueryAsync(ctx, handle, args[1].Str(), params, false)
}

func builtinSqliteLastInsertRowID(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	handle, code := getSQLiteHandle(ctx, args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}

	return runSQLiteAsync(ctx, func() types.Result {
		opCtx, ok := beginSQLiteOperation(handle)
		if !ok {
			return types.Err(types.E_INVARG)
		}
		defer endSQLiteOperation(handle)

		var lastID int64
		if err := handle.conn.QueryRowContext(opCtx, "SELECT last_insert_rowid()").Scan(&lastID); err != nil {
			return sqliteErrorResult(err)
		}
		return types.Ok(types.NewInt(lastID))
	})
}

func builtinSqliteLimit(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 3 {
		return types.Err(types.E_ARGS)
	}

	handle, code := getSQLiteHandle(ctx, args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}
	category, code := sqliteLimitCategory(args[1])
	if code != types.E_NONE {
		return types.Err(code)
	}
	if args[2].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}

	handle.mu.Lock()
	prior := handle.limits[category]
	if args[2].Int() >= 0 {
		handle.limits[category] = args[2].Int()
	}
	handle.mu.Unlock()
	return types.Ok(types.NewInt(prior))
}

func builtinSqliteInterrupt(ctx *Execution, args []types.Value) types.Result {
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	handle, code := getSQLiteHandle(ctx, args[0])
	if code != types.E_NONE {
		return types.Err(code)
	}

	handle.mu.Lock()
	if handle.currentCancel != nil {
		handle.currentCancel()
	} else {
		handle.pendingInterrupt = true
	}
	handle.mu.Unlock()
	return types.Ok(types.NewInt(0))
}
