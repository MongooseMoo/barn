package builtins

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/types"

	"modernc.org/libc"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
	sqlite3 "modernc.org/sqlite/lib"
)

type sqliteHandle struct {
	id      int64
	path    string
	options int64 // sqlite_open's option bits (sqlite_raw.go)
	db      *sql.DB
	conn    *sql.Conn
	rawDB   uintptr // the connection's sqlite3*, for interrupt and limits
	// limits are the values Toast's library would hold (sqlite_raw.go); the
	// connection holds them too, bounded by modernc's compiled limits.
	limits    map[int64]int64
	mu        sync.Mutex
	cond      *sync.Cond
	activeOps int
	closed    bool
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
	limits := make(map[int64]int64, len(sqliteToastLimitDefaults))
	for category, value := range sqliteToastLimitDefaults {
		limits[int64(category)] = value
	}
	return limits
}

func newSQLiteHandle(id int64, path string, db *sql.DB, conn *sql.Conn) *sqliteHandle {
	handle := &sqliteHandle{
		id:      id,
		path:    path,
		options: sqliteDefaultOptions,
		db:      db,
		conn:    conn,
		limits:  defaultSQLiteLimits(),
	}
	handle.cond = sync.NewCond(&handle.mu)
	return handle
}

func getSQLiteHandle(ctx *Execution, v types.Value) (*sqliteHandle, types.ErrorCode) {
	if v.Type() != types.TYPE_INT {
		return nil, types.E_TYPE
	}

	ctx.Session.runtime.sqlite.mu.Lock()
	handle := ctx.Session.runtime.sqlite.handles[v.Int()]
	ctx.Session.runtime.sqlite.mu.Unlock()
	if handle == nil {
		return nil, types.E_INVARG
	}
	return handle, types.E_NONE
}

func beginSQLiteOperation(handle *sqliteHandle) bool {
	handle.mu.Lock()
	defer handle.mu.Unlock()

	for handle.activeOps > 0 && !handle.closed {
		handle.cond.Wait()
	}
	if handle.closed {
		return false
	}
	handle.activeOps = 1
	return true
}

func endSQLiteOperation(handle *sqliteHandle) {
	handle.mu.Lock()
	handle.activeOps = 0
	handle.cond.Broadcast()
	handle.mu.Unlock()
}

// sqliteExecOrQuery runs sqlText on handle. With readOnly set it runs it only
// if it is shareable (sqlite_readonly.go) and the handle is open, and otherwise
// returns ran false having run nothing; epoch is the store's external write
// epoch loaded before the statement ran. Without readOnly every statement
// runs, and one that is not shareable moves the epoch once it has run, before
// the handle is released. A nil store skips both. A nil params runs the text
// as sqlite_query does; a non-nil one, even empty, as sqlite_execute does.
func sqliteExecOrQuery(handle *sqliteHandle, sqlText string, params []types.Value, includeHeaders bool, id metrics.Identity, store *dbstore.Store, readOnly bool) (result types.Result, ran bool, epoch uint64) {
	waitStart := time.Now()
	ok := beginSQLiteOperation(handle)
	handleWait := time.Since(waitStart)
	if !ok {
		metrics.ObserveRuntime(metrics.SQLiteHandleWait, id, handleWait, 0)
		return types.Err(types.E_INVARG), !readOnly, 0
	}
	workStart := time.Now()
	defer func() {
		workElapsed := time.Since(workStart)
		// Release the slot before diagnostic logging can perform disk I/O.
		endSQLiteOperation(handle)
		metrics.ObserveRuntime(metrics.SQLiteHandleWait, id, handleWait, 0)
		metrics.ObserveRuntime(metrics.SQLiteWork, id, workElapsed, 0)
	}()

	if store != nil {
		shareable := sqliteStatementShareable(handle.conn, sqlText)
		if readOnly && !shareable {
			return types.Result{}, false, 0
		}
		epoch = store.ExternalWrites()
		if !shareable {
			defer store.BumpExternalWrites() // runs before the slot is released
		}
	}
	return sqliteRunStatement(handle, sqlText, params, includeHeaders), true, epoch
}

var errSQLiteDriverUnreachable = errors.New("SQLite connection is unavailable")

// sqliteWithConnection runs f on the handle's sqlite3* and libc thread state.
// The caller holds the handle's operation slot.
func sqliteWithConnection(handle *sqliteHandle, f func(tls *libc.TLS, db uintptr)) error {
	if handle.conn == nil {
		return errSQLiteDriverUnreachable
	}
	return handle.conn.Raw(func(driverConn any) error {
		tls, db, ok := sqliteDriverHandles(driverConn)
		if !ok {
			return errSQLiteDriverUnreachable
		}
		f(tls, db)
		return nil
	})
}

func sqliteRunStatement(handle *sqliteHandle, sqlText string, params []types.Value, includeHeaders bool) types.Result {
	var result types.Result
	err := sqliteWithConnection(handle, func(tls *libc.TLS, db uintptr) {
		if params == nil {
			result = sqliteRawQuery(tls, db, sqlText, handle.options, includeHeaders)
		} else {
			result = sqliteRawExecute(tls, db, sqlText, params, handle.options)
		}
	})
	if err != nil {
		return types.Ok(types.NewStr(err.Error()))
	}
	return result
}

func sqliteExecOrQueryAsync(ctx *Execution, builtin string, handle *sqliteHandle, sqlText string, params []types.Value, includeHeaders bool) types.Result {
	id := runtimeIdentity(ctx)
	store := ctx.Store
	if ctx.Task != nil && !threadedCall(ctx) {
		// A read-only statement runs at once and is validated at commit
		// instead (sqlite_readonly.go).
		if sqliteMayReadOptimistically(ctx) {
			if result, ran, epoch := sqliteExecOrQuery(handle, sqlText, params, includeHeaders, id, store, true); ran {
				readTxn(ctx).NoteExternalRead(epoch)
				return result
			}
		}
		// Row-returning statements can write (WITH, PRAGMA), so protect every
		// other inline statement before entering SQLite or waiting on its handle.
		// A stale attempt must stop before executing any external operation.
		// Threaded operations instead start only after the slice commits.
		if !beginIrreversible(ctx, builtin) {
			return abortedAttempt()
		}
		ctx.IrreversibleSideEffect = true
	}
	return runInBackground(ctx, func() types.Result {
		result, _, _ := sqliteExecOrQuery(handle, sqlText, params, includeHeaders, id, store, false)
		return result
	}, types.NewList(params))
}

// sqliteMayReadOptimistically reports whether an inline read-only statement
// may skip the irreversible-effect boundary: the attempt can still be re-run
// and does not hold the exclusive commit gate. Otherwise the statement takes
// the path every statement took before: across the boundary, or, where the
// boundary is already behind the attempt or the context has none, straight in.
func sqliteMayReadOptimistically(ctx *Execution) bool {
	tc := ctx.Context
	if ctx.Store == nil || tc == nil || tc.BeforeIrreversibleEffect == nil ||
		tc.ConflictRetryRequested || tc.IrreversibleSideEffect || tc.LiveStoreMutated {
		return false
	}
	tx := readTxn(ctx)
	return !tx.IsDirect() && !tx.IsCommitGateExempt()
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
		// Toast leaves the category at -1 for any other type (sqlite.cc:600-617).
		return 0, types.E_INVARG
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
	options := int64(sqliteDefaultOptions)
	if len(args) == 2 {
		if args[1].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		// Toast stores the argument as the option bits (sqlite.cc:261-262).
		options = args[1].Int()
	}

	// osPath is what the driver opens; path stays the MOO-visible string that
	// sqlite_info() reports and duplicate detection compares.
	osPath := path
	if path != ":memory:" && path != "" {
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
		osPath = resolveFilePath(sanitized)
		// The reported path is Toast's file_resolve_path output verbatim:
		// file_subdir plus the caller's spelling with one leading "/" removed,
		// forward slashes on every platform. Database code compares this string
		// (Mongoose #3882::is_open checks info["path"] == "files/" + path).
		path = "files/" + strings.TrimPrefix(path, "/")
	}

	// Toast refuses to open a database that is already open at the same
	// resolved path (sqlite.cc database_already_open): E_INVARG carrying the
	// existing handle. ":memory:" databases are never duplicates of each other.
	if path != ":memory:" && path != "" {
		ctx.Session.runtime.sqlite.mu.Lock()
		var existing *sqliteHandle
		for _, h := range ctx.Session.runtime.sqlite.handles {
			if h.path == path && (existing == nil || h.id < existing.id) {
				existing = h
			}
		}
		ctx.Session.runtime.sqlite.mu.Unlock()
		if existing != nil {
			return types.Result{
				Flow:  types.FlowException,
				Error: types.E_INVARG,
				Val: types.NewList([]types.Value{
					types.NewErr(types.E_INVARG),
					types.NewStr(fmt.Sprintf("Database already open with handle: %d", existing.id)),
					types.NewInt(existing.id),
				}),
			}
		}
	}

	// Toast allocates the handle before sqlite3_open and deallocates it when
	// the open fails (sqlite.cc:258-273), so a failed open still uses a number.
	handle := newSQLiteHandle(0, path, nil, nil)
	handle.options = options
	rt := &ctx.Session.runtime.sqlite
	rt.mu.Lock()
	handle.id = rt.nextID
	rt.nextID++
	rt.handles[handle.id] = handle
	rt.order.insert(handle.id)
	rt.mu.Unlock()
	fail := func(msg string) types.Result {
		handle.mu.Lock()
		handle.closed = true
		handle.mu.Unlock()
		removeSQLiteHandle(ctx, handle.id)
		if handle.conn != nil {
			_ = handle.conn.Close()
		}
		if handle.db != nil {
			_ = handle.db.Close()
		}
		return sqliteOpenError(msg)
	}

	db, err := sql.Open("sqlite", osPath)
	if err != nil {
		return fail(err.Error())
	}
	handle.db = db
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	conn, err := db.Conn(context.Background())
	if err != nil {
		return fail(err.Error())
	}
	handle.conn = conn
	// Toast opens with SQLite's 2 MiB default. A larger cache changes no result;
	// it stops each scan of a big database from rereading it through the OS.
	if _, err := conn.ExecContext(context.Background(), fmt.Sprintf("PRAGMA cache_size = -%d", ctx.RuntimeOptions.SQLiteCacheSizeKiB())); err != nil {
		return fail(err.Error())
	}
	if err := sqliteWithConnection(handle, func(tls *libc.TLS, db uintptr) {
		handle.mu.Lock()
		handle.rawDB = db
		sqliteSetUpConnection(tls, db, handle.limits)
		handle.mu.Unlock()
	}); err != nil {
		return fail(err.Error())
	}
	return types.Ok(types.NewInt(handle.id))
}

// removeSQLiteHandle forgets a handle as Toast's deallocate_handle does: it
// leaves the connection map, and the numbering restarts at 1 once the map is
// empty (sqlite.cc:123-137).
func removeSQLiteHandle(ctx *Execution, id int64) {
	rt := &ctx.Session.runtime.sqlite
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.handles, id)
	rt.order.erase(id)
	if len(rt.handles) == 0 {
		rt.nextID = 1
	}
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

	removeSQLiteHandle(ctx, handle.id)

	return runInBackground(ctx, func() types.Result {
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

	// Toast's unordered_map order (sqlite_handle_order.go).
	rt := &ctx.Session.runtime.sqlite
	rt.mu.Lock()
	out := make([]types.Value, 0, len(rt.handles))
	for _, id := range rt.order.list {
		if _, ok := rt.handles[id]; ok {
			out = append(out, types.NewInt(id))
		}
	}
	rt.mu.Unlock()
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

	flag := func(bit int64) types.Value {
		if handle.options&bit != 0 {
			return types.NewInt(1)
		}
		return types.NewInt(0)
	}
	return types.Ok(types.NewMap([][2]types.Value{
		{types.NewStr("path"), types.NewStr(handle.path)},
		{types.NewStr("parse_types"), flag(sqliteParseTypes)},
		{types.NewStr("parse_objects"), flag(sqliteParseObjects)},
		{types.NewStr("sanitize_strings"), flag(sqliteSanitizeStrings)},
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
	return sqliteExecOrQueryAsync(ctx, "sqlite_query", handle, args[1].Str(), nil, includeHeaders)
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

	params := append(make([]types.Value, 0, args[2].Len()), args[2].Elements()...)
	return sqliteExecOrQueryAsync(ctx, "sqlite_execute", handle, args[1].Str(), params, false)
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

	return runInBackground(ctx, func() types.Result {
		if !beginSQLiteOperation(handle) {
			return types.Err(types.E_INVARG)
		}
		defer endSQLiteOperation(handle)

		var lastID int64
		if err := sqliteWithConnection(handle, func(tls *libc.TLS, db uintptr) {
			lastID = sqlite3.Xsqlite3_last_insert_rowid(tls, db)
		}); err != nil {
			return types.Ok(types.NewStr(err.Error()))
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
	if args[2].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	category, code := sqliteLimitCategory(args[1])
	if code != types.E_NONE {
		return types.Err(code)
	}

	handle.mu.Lock()
	prior := handle.limits[category]
	if newLimit := args[2].Int(); newLimit >= 0 {
		value := sqliteToastLimitValue(category, newLimit)
		handle.limits[category] = value
		if !handle.closed {
			sqliteApplyLimit(handle.rawDB, category, value)
		}
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

	// sqlite3_interrupt (sqlite.cc:647): with no statement running, SQLite
	// clears it when the next statement starts.
	handle.mu.Lock()
	sqliteInterruptLocked(handle)
	handle.mu.Unlock()
	return types.Ok(types.NewInt(0))
}

func sqliteDescriptors() []Descriptor {
	return []Descriptor{
		function("sqlite_open", builtinSqliteOpen, 1, 2, 2, 0).withEffect(Irreversible).withCapability(config.SQLite),
		function("sqlite_close", builtinSqliteClose, 1, 1, 0).withEffect(Irreversible).withCapability(config.SQLite),
		function("sqlite_handles", builtinSqliteHandles, 0, 0).withCapability(config.SQLite),
		function("sqlite_info", builtinSqliteInfo, 1, 1, 0).withCapability(config.SQLite),
		function("sqlite_query", builtinSqliteQuery, 2, 3, 0, 2, -1).withEffect(ImplementationOwned).withCapability(config.SQLite),
		function("sqlite_execute", builtinSqliteExecute, 3, 3, 0, 2, 4).withEffect(ImplementationOwned).withCapability(config.SQLite),
		function("sqlite_last_insert_row_id", builtinSqliteLastInsertRowID, 1, 1, 0).withCapability(config.SQLite),
		function("sqlite_limit", builtinSqliteLimit, 3, 3, 0, -1, 0).withEffect(Irreversible).withCapability(config.SQLite),
		function("sqlite_interrupt", builtinSqliteInterrupt, 1, 1, 0).withEffect(Irreversible).withCapability(config.SQLite),
	}
}
