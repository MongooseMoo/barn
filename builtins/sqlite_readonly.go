package builtins

import (
	"database/sql"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"

	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

// An inline SQLite statement that SQLite itself reports as read-only, and that
// changes no connection state, runs without the irreversible-effect boundary:
// re-running it on a retried attempt has no effect, so it needs neither the
// exclusive commit gate nor the end of the attempt's retryability. It is made
// serializable with SQLite writers by the store's external write epoch
// (db/store/store_external_reads.go): the reader records the epoch before it
// runs, and every statement that is not read-only moves the epoch after it
// runs, while still holding the handle.
//
// The epoch is one counter for every handle, not one per database file:
// ATTACH, and two spellings of one path, let different handles reach the same
// file. Writes made by processes outside Barn move no epoch and are out of
// scope, as they are for the exclusive gate.

// sqliteStatementShareable reports whether every statement in sqlText may run
// without the irreversible-effect boundary. The caller holds the handle's
// operation slot, so nothing else runs on the connection between this check and
// the statement. Any doubt (a statement that does not prepare, a driver whose
// connection cannot be reached) answers false, which keeps today's protection.
//
// sqlite3_stmt_readonly alone is not enough: it is true for BEGIN, COMMIT,
// ROLLBACK, SAVEPOINT, RELEASE, ATTACH and DETACH
// (https://www.sqlite.org/c3ref/stmt_readonly.html), which change connection
// state another task sees, and a read inside an open transaction can see rows
// a later ROLLBACK removes. So the connection must be in autocommit mode, and
// the authorizer must report no transaction, savepoint, attach, detach or
// pragma action while the text is prepared.
func sqliteStatementShareable(conn *sql.Conn, sqlText string) bool {
	shareable := false
	err := conn.Raw(func(driverConn any) error {
		tls, db, ok := sqliteDriverHandles(driverConn)
		if ok {
			shareable = sqliteClassifyReadOnly(tls, db, sqlText)
		}
		return nil
	})
	return err == nil && shareable
}

var sqliteTLSType = reflect.TypeOf((*libc.TLS)(nil))

// sqliteDriverHandles returns the sqlite3* and the libc thread state of a
// modernc.org/sqlite connection. The driver exports neither, so they are read
// from its unexported fields; a driver that renames them makes every statement
// take the boundary (TestSQLiteStatementShareable fails).
func sqliteDriverHandles(driverConn any) (*libc.TLS, uintptr, bool) {
	v := reflect.ValueOf(driverConn)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil, 0, false
	}
	db := v.Elem().FieldByName("db")
	tls := v.Elem().FieldByName("tls")
	if !db.IsValid() || db.Kind() != reflect.Uintptr || !tls.IsValid() || tls.Type() != sqliteTLSType || tls.IsNil() {
		return nil, 0, false
	}
	return (*libc.TLS)(tls.UnsafePointer()), uintptr(db.Uint()), db.Uint() != 0
}

// sqliteAuthorizerFlags holds, per classification in progress, whether the
// authorizer saw an action that makes the text unshareable. The authorizer's
// user argument is the key, not a pointer.
var (
	sqliteAuthorizerFlags sync.Map // uintptr -> *atomic.Bool
	sqliteAuthorizerKey   atomic.Uintptr
)

func sqliteClassifyAuthorizer(_ *libc.TLS, key uintptr, action int32, _, _, _, _ uintptr) int32 {
	switch action {
	case sqlite3.SQLITE_TRANSACTION, sqlite3.SQLITE_SAVEPOINT, sqlite3.SQLITE_ATTACH, sqlite3.SQLITE_DETACH, sqlite3.SQLITE_PRAGMA:
		if flag, ok := sqliteAuthorizerFlags.Load(key); ok {
			flag.(*atomic.Bool).Store(true)
		}
	}
	return sqlite3.SQLITE_OK
}

// sqliteClassifyAuthorizerFn is sqliteClassifyAuthorizer as the C function
// pointer the transpiled SQLite calls (the driver passes its own callbacks the
// same way).
var sqliteClassifyAuthorizerFn = *(*uintptr)(unsafe.Pointer(&struct {
	f func(*libc.TLS, uintptr, int32, uintptr, uintptr, uintptr, uintptr) int32
}{sqliteClassifyAuthorizer}))

func sqliteClassifyReadOnly(tls *libc.TLS, db uintptr, sqlText string) bool {
	if sqlite3.Xsqlite3_get_autocommit(tls, db) == 0 {
		return false
	}
	key := sqliteAuthorizerKey.Add(1)
	flag := new(atomic.Bool)
	sqliteAuthorizerFlags.Store(key, flag)
	defer sqliteAuthorizerFlags.Delete(key)
	sqlite3.Xsqlite3_set_authorizer(tls, db, sqliteClassifyAuthorizerFn, key)
	defer sqlite3.Xsqlite3_set_authorizer(tls, db, 0, 0)

	// The text and the two out-parameters live in SQLite's own allocations,
	// which never move, as the transpiled C code requires.
	base := sqlite3.Xsqlite3_malloc64(tls, uint64(len(sqlText))+1)
	if base == 0 {
		return false
	}
	defer sqlite3.Xsqlite3_free(tls, base)
	text := sqliteCBytes(base, len(sqlText)+1)
	copy(text, sqlText)
	text[len(sqlText)] = 0
	outPtr := sqlite3.Xsqlite3_malloc64(tls, 2*uint64(unsafe.Sizeof(uintptr(0))))
	if outPtr == 0 {
		return false
	}
	defer sqlite3.Xsqlite3_free(tls, outPtr)
	out := sqliteCWords(outPtr, 2) // *ppStmt, *pzTail
	ppStmt, pzTail := outPtr, outPtr+unsafe.Sizeof(uintptr(0))
	offset := uintptr(0)
	for text[offset] != 0 {
		out[0], out[1] = 0, 0
		if rc := sqlite3.Xsqlite3_prepare_v2(tls, db, base+offset, -1, ppStmt, pzTail); rc != sqlite3.SQLITE_OK {
			return false
		}
		stmt := out[0]
		if stmt != 0 {
			readOnly := sqlite3.Xsqlite3_stmt_readonly(tls, stmt) != 0
			sqlite3.Xsqlite3_finalize(tls, stmt)
			if !readOnly || flag.Load() {
				return false
			}
		}
		next := out[1] - base
		if out[1] < base || next <= offset || next >= uintptr(len(text)) {
			break
		}
		offset = next
	}
	return true
}

// sqliteCBytes and sqliteCWords view n bytes or words of SQLite-allocated
// memory at p.
func sqliteCBytes(p uintptr, n int) []byte {
	return unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), n)
}

func sqliteCWords(p uintptr, n int) []uintptr {
	return unsafe.Slice(*(**uintptr)(unsafe.Pointer(&p)), n)
}
