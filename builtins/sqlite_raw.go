package builtins

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/MongooseMoo/barn/types"

	"modernc.org/libc"
	sqlite3 "modernc.org/sqlite/lib"
)

// Toast runs SQL through the C API, not a Go driver, and every visible result
// follows from that: sqlite_query is sqlite3_exec (every statement, a row
// callback per result row, sqlite.cc:485-508), sqlite_execute is one
// sqlite3_prepare_v2 + bind + step loop (only the first statement,
// sqlite.cc:369-452), and every column arrives as text and is converted by
// string_to_moo_type (sqlite.cc:48-81). Barn makes the same calls on the
// modernc.org/sqlite connection (sqliteDriverHandles, sqlite_readonly.go).

// Open option bits (toaststunt src/include/sqlite.h:13-15).
const (
	sqliteParseTypes      = 2
	sqliteParseObjects    = 4
	sqliteSanitizeStrings = 8
	sqliteDefaultOptions  = sqliteParseTypes | sqliteParseObjects // sqlite.cc:114
)

// cIsSpace is C isspace() in the C locale.
func cIsSpace(c byte) bool {
	return c == ' ' || (c >= '\t' && c <= '\r')
}

// toastParseNumber is Toast's parse_number(str, &n, 0) (numbers.cc:81-97):
// strtoimax base 10 (leading isspace, sign, digits, clamped on overflow),
// followed only by ' '.
func toastParseNumber(s string) (int64, bool) {
	i := 0
	for i < len(s) && cIsSpace(s[i]) {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	start := i
	var mag uint64
	overflow := false
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		d := uint64(s[i] - '0')
		if mag > (math.MaxUint64-d)/10 {
			overflow = true
		} else {
			mag = mag*10 + d
		}
		i++
	}
	if i == start {
		return 0, false
	}
	for ; i < len(s); i++ {
		if s[i] != ' ' {
			return 0, false
		}
	}
	switch {
	case neg && (overflow || mag > 1<<63):
		return math.MinInt64, true
	case neg:
		return -int64(mag), true
	case overflow || mag > math.MaxInt64:
		return math.MaxInt64, true
	}
	return int64(mag), true
}

// toastParseFloat is Toast's parse_float (numbers.cc:116-135): skip ' ', take
// one '-', strtod the rest, allow only ' ' after it, then negate.
func toastParseFloat(s string) (float64, bool) {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	neg := false
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	f, n := cStrtod(s[i:])
	if n == 0 {
		return 0, false
	}
	for _, c := range []byte(s[i+n:]) {
		if c != ' ' {
			return 0, false
		}
	}
	if neg {
		f = -f
	}
	return f, true
}

func hasFoldPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// cStrtod is glibc strtod in the C locale: it returns the value and the number
// of bytes consumed, 0 when no conversion is performed. It accepts leading
// isspace, a sign, decimal and hexadecimal floats, "inf"/"infinity" and
// "nan"/"nan(chars)", case-insensitively. Overflow gives an infinity.
func cStrtod(s string) (float64, int) {
	i := 0
	for i < len(s) && cIsSpace(s[i]) {
		i++
	}
	sign := 1.0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		if s[i] == '-' {
			sign = -1
		}
		i++
	}
	rest := s[i:]
	switch {
	case hasFoldPrefix(rest, "infinity"):
		return math.Inf(int(sign)), i + 8
	case hasFoldPrefix(rest, "inf"):
		return math.Inf(int(sign)), i + 3
	case hasFoldPrefix(rest, "nan"):
		n := i + 3
		if n < len(s) && s[n] == '(' {
			j := n + 1
			for j < len(s) && (s[j] == '_' || (s[j] >= '0' && s[j] <= '9') || (s[j]|0x20 >= 'a' && s[j]|0x20 <= 'z')) {
				j++
			}
			if j < len(s) && s[j] == ')' {
				n = j + 1
			}
		}
		return math.Copysign(math.NaN(), sign), n
	}
	if len(rest) > 2 && rest[0] == '0' && (rest[1] == 'x' || rest[1] == 'X') &&
		(isHexDigit(rest[2]) || (rest[2] == '.' && len(rest) > 3 && isHexDigit(rest[3]))) {
		j := 2
		for j < len(rest) && isHexDigit(rest[j]) {
			j++
		}
		if j < len(rest) && rest[j] == '.' {
			j++
			for j < len(rest) && isHexDigit(rest[j]) {
				j++
			}
		}
		mantissa := rest[:j]
		exponent := "p0"
		if j < len(rest) && (rest[j] == 'p' || rest[j] == 'P') {
			k := j + 1
			if k < len(rest) && (rest[k] == '+' || rest[k] == '-') {
				k++
			}
			if k < len(rest) && rest[k] >= '0' && rest[k] <= '9' {
				for k < len(rest) && rest[k] >= '0' && rest[k] <= '9' {
					k++
				}
				exponent = rest[j:k]
				j = k
			}
		}
		f, _ := strconv.ParseFloat(mantissa+exponent, 64)
		return sign * f, i + j
	}
	j := 0
	digits := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
		digits++
	}
	if j < len(rest) && rest[j] == '.' {
		j++
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
			digits++
		}
	}
	if digits == 0 {
		return 0, 0
	}
	if j < len(rest) && (rest[j] == 'e' || rest[j] == 'E') {
		k := j + 1
		if k < len(rest) && (rest[k] == '+' || rest[k] == '-') {
			k++
		}
		if k < len(rest) && rest[k] >= '0' && rest[k] <= '9' {
			for k < len(rest) && rest[k] >= '0' && rest[k] <= '9' {
				k++
			}
			j = k
		}
	}
	// ParseFloat returns the overflowed infinity or underflowed value with a
	// range error, which is what strtod returns.
	f, _ := strconv.ParseFloat(rest[:j], 64)
	return sign * f, i + j
}

// sqliteStringToMOO is string_to_moo_type (sqlite.cc:48-81) for non-NULL text.
func sqliteStringToMOO(s string, parseObjects, sanitize bool) types.Value {
	if parseObjects && len(s) > 0 && s[0] == '#' {
		if n, ok := toastParseNumber(s[1:]); ok {
			return types.NewObj(types.ObjID(n))
		}
	}
	if n, ok := toastParseNumber(s); ok {
		return types.NewInt(n)
	}
	if f, ok := toastParseFloat(s); ok {
		return types.NewFloat(f)
	}
	if sanitize {
		s = strings.ReplaceAll(s, "\n", "\t")
	}
	return types.NewStr(s)
}

// sqliteColumnValue converts one column's sqlite3_column_text / exec argv
// pointer the way both Toast paths do (sqlite.cc:163-173, 428-441). Without
// PARSE_TYPES the text is returned as is and NULL becomes str_dup(NULL), the
// empty string; with it NULL is "NULL". Sanitizing replaces newlines with tabs;
// doing it before parsing, as the execute path does, gives the same result
// because strtoimax and strtod treat both as space.
func sqliteColumnValue(p uintptr, options int64) types.Value {
	sanitize := options&sqliteSanitizeStrings != 0
	if options&sqliteParseTypes == 0 {
		if p == 0 {
			return types.NewStr("")
		}
		s := libc.GoString(p)
		if sanitize {
			s = strings.ReplaceAll(s, "\n", "\t")
		}
		return types.NewStr(s)
	}
	if p == 0 {
		return types.NewStr("NULL")
	}
	return sqliteStringToMOO(libc.GoString(p), options&sqliteParseObjects != 0, sanitize)
}

// sqliteCString copies s into SQLite-allocated memory with a terminating NUL;
// the caller frees it with sqlite3_free.
func sqliteCString(tls *libc.TLS, s string) uintptr {
	p := sqlite3.Xsqlite3_malloc64(tls, uint64(len(s))+1)
	if p == 0 {
		return 0
	}
	b := sqliteCBytes(p, len(s)+1)
	copy(b, s)
	b[len(s)] = 0
	return p
}

func sqliteErrmsg(tls *libc.TLS, db uintptr) types.Result {
	return types.Ok(types.NewStr(libc.GoString(sqlite3.Xsqlite3_errmsg(tls, db))))
}

var errSQLiteOutOfMemory = types.Ok(types.NewStr("out of memory"))

// sqliteExecState collects one sqlite3_exec call's rows; the callback's user
// argument is its key in sqliteExecStates, not a pointer.
type sqliteExecState struct {
	options int64
	headers bool
	rows    []types.Value
}

var (
	sqliteExecStates sync.Map // uintptr -> *sqliteExecState
	sqliteExecKey    atomic.Uintptr
)

// sqliteExecCallback is Toast's callback() (sqlite.cc:153-188).
func sqliteExecCallback(_ *libc.TLS, key uintptr, argc int32, argv, colNames uintptr) int32 {
	v, ok := sqliteExecStates.Load(key)
	if !ok {
		return 0
	}
	st := v.(*sqliteExecState)
	values := sqliteCWords(argv, int(argc))
	var names []uintptr
	if st.headers {
		names = sqliteCWords(colNames, int(argc))
	}
	row := make([]types.Value, 0, argc)
	for i := range values {
		value := sqliteColumnValue(values[i], st.options)
		if st.headers {
			value = types.NewList([]types.Value{types.NewStr(libc.GoString(names[i])), value})
		}
		row = append(row, value)
	}
	st.rows = append(st.rows, types.NewList(row))
	return 0
}

var sqliteExecCallbackFn = *(*uintptr)(unsafe.Pointer(&struct {
	f func(*libc.TLS, uintptr, int32, uintptr, uintptr) int32
}{sqliteExecCallback}))

// sqliteRawQuery is sqlite_query_thread_callback (sqlite.cc:485-508).
func sqliteRawQuery(tls *libc.TLS, db uintptr, sqlText string, options int64, headers bool) types.Result {
	text := sqliteCString(tls, sqlText)
	if text == 0 {
		return errSQLiteOutOfMemory
	}
	defer sqlite3.Xsqlite3_free(tls, text)
	errOut := sqlite3.Xsqlite3_malloc64(tls, uint64(unsafe.Sizeof(uintptr(0))))
	if errOut == 0 {
		return errSQLiteOutOfMemory
	}
	defer sqlite3.Xsqlite3_free(tls, errOut)
	sqliteCWords(errOut, 1)[0] = 0

	key := sqliteExecKey.Add(1)
	st := &sqliteExecState{options: options, headers: headers, rows: make([]types.Value, 0)}
	sqliteExecStates.Store(key, st)
	defer sqliteExecStates.Delete(key)

	if rc := sqlite3.Xsqlite3_exec(tls, db, text, sqliteExecCallbackFn, key, errOut); rc != sqlite3.SQLITE_OK {
		msg := ""
		if p := sqliteCWords(errOut, 1)[0]; p != 0 {
			msg = libc.GoString(p)
			sqlite3.Xsqlite3_free(tls, p)
		}
		return types.Ok(types.NewStr(msg))
	}
	return types.Ok(types.NewList(st.rows))
}

// sqliteBindParameter binds one sqlite_execute parameter as
// sqlite_execute_thread_callback does (sqlite.cc:386-407): STR, INT, FLOAT
// and OBJ ("#N" text) are bound, every other type is left unbound, which is
// NULL. An INT keeps its low 32 bits. Bind failures are ignored, as in Toast.
func sqliteBindParameter(tls *libc.TLS, stmt uintptr, index int32, value types.Value) {
	bindText := func(s string) {
		text := sqliteCString(tls, s)
		if text == 0 {
			return
		}
		sqlite3.Xsqlite3_bind_text(tls, stmt, index, text, -1, sqlite3.SQLITE_TRANSIENT)
		sqlite3.Xsqlite3_free(tls, text)
	}
	switch value.Type() {
	case types.TYPE_STR:
		bindText(value.Str())
	case types.TYPE_INT:
		// sqlite3_bind_int takes a C int: Toast keeps only the low 32 bits
		// of the MOO integer (sqlite.cc:394), so 2147483648 binds as
		// -2147483648.
		sqlite3.Xsqlite3_bind_int(tls, stmt, index, int32(value.Int()))
	case types.TYPE_FLOAT:
		sqlite3.Xsqlite3_bind_double(tls, stmt, index, value.Float())
	case types.TYPE_OBJ:
		bindText(fmt.Sprintf("#%d", value.ID()))
	}
}

// sqliteRawExecute is sqlite_execute_thread_callback (sqlite.cc:369-452).
func sqliteRawExecute(tls *libc.TLS, db uintptr, sqlText string, params []types.Value, options int64) types.Result {
	text := sqliteCString(tls, sqlText)
	if text == 0 {
		return errSQLiteOutOfMemory
	}
	defer sqlite3.Xsqlite3_free(tls, text)
	stmtOut := sqlite3.Xsqlite3_malloc64(tls, uint64(unsafe.Sizeof(uintptr(0))))
	if stmtOut == 0 {
		return errSQLiteOutOfMemory
	}
	defer sqlite3.Xsqlite3_free(tls, stmtOut)
	sqliteCWords(stmtOut, 1)[0] = 0

	if rc := sqlite3.Xsqlite3_prepare_v2(tls, db, text, -1, stmtOut, 0); rc != sqlite3.SQLITE_OK {
		return sqliteErrmsg(tls, db)
	}
	// Empty text prepares to no statement; binding and stepping it fail with
	// SQLITE_MISUSE without setting an error, so errmsg is "not an error".
	stmt := sqliteCWords(stmtOut, 1)[0]
	defer sqlite3.Xsqlite3_finalize(tls, stmt)
	for i, param := range params {
		sqliteBindParameter(tls, stmt, int32(i+1), param)
	}

	rc := sqlite3.Xsqlite3_step(tls, stmt)
	if rc != sqlite3.SQLITE_OK && rc != sqlite3.SQLITE_DONE && rc != sqlite3.SQLITE_ROW {
		return sqliteErrmsg(tls, db)
	}
	columns := sqlite3.Xsqlite3_column_count(tls, stmt)
	rows := make([]types.Value, 0)
	// A failure after the first step ends the rows silently, as in Toast.
	for rc == sqlite3.SQLITE_ROW {
		row := make([]types.Value, 0, columns)
		for x := int32(0); x < columns; x++ {
			row = append(row, sqliteColumnValue(sqlite3.Xsqlite3_column_text(tls, stmt, x), options))
		}
		rows = append(rows, types.NewList(row))
		rc = sqlite3.Xsqlite3_step(tls, stmt)
	}
	return types.Ok(types.NewList(rows))
}

// Toast's limits are those of the linked libsqlite3 (Debian 3.40.1-2+deb12u2
// on the WSL oracles): sqlite_limit returns sqlite3_limit (sqlite.cc:623).
// Debian builds it with SQLITE_MAX_VARIABLE_NUMBER=250000 and the default
// SQLITE_MAX_FUNCTION_ARG 127; modernc 3.50.4 is built with 32766 and 1000,
// and from 3.48 raises any LIMIT_LENGTH below 30 to 30 where 3.40.1 allows 1.
// Barn reports the oracle library's values and gives the connection the same
// limit wherever modernc's compiled bound allows it.
var (
	sqliteToastLimitDefaults = [12]int64{1000000000, 1000000000, 2000, 1000, 500, 250000000, 127, 10, 50000, 250000, 1000, 0}
	sqliteToastHardLimits    = [12]int64{1000000000, 1000000000, 2000, 1000, 500, 250000000, 127, 10, 50000, 250000, 1000, 8}
	// modernc's compile-time bounds (the SQLITE_MAX_* constants it was built
	// with, in sqlite_linux_amd64.go and its siblings). Expression nodes store a
	// variable number in a type sized by SQLITE_MAX_VARIABLE_NUMBER, so no limit
	// may exceed these.
	sqliteBarnHardLimits = [12]int64{
		sqlite3.SQLITE_MAX_LENGTH, sqlite3.SQLITE_MAX_SQL_LENGTH, sqlite3.SQLITE_MAX_COLUMN,
		sqlite3.SQLITE_MAX_EXPR_DEPTH, sqlite3.SQLITE_MAX_COMPOUND_SELECT, sqlite3.SQLITE_MAX_VDBE_OP,
		sqlite3.SQLITE_MAX_FUNCTION_ARG, sqlite3.SQLITE_MAX_ATTACHED, sqlite3.SQLITE_MAX_LIKE_PATTERN_LENGTH,
		sqlite3.SQLITE_MAX_VARIABLE_NUMBER, sqlite3.SQLITE_MAX_TRIGGER_DEPTH, sqlite3.SQLITE_MAX_WORKER_THREADS,
	}
)

// sqliteToastLimitValue is what SQLite 3.40.1's sqlite3_limit stores for a
// non-negative newLimit: clamped to the hard bound, and LIMIT_LENGTH at least 1.
func sqliteToastLimitValue(category int64, newLimit int64) int64 {
	switch {
	case newLimit > sqliteToastHardLimits[category]:
		return sqliteToastHardLimits[category]
	case newLimit < 1 && category == 0:
		return 1
	}
	return newLimit
}

// sqliteApplyLimit stores value in the connection's limit table, bounded by
// what modernc was compiled to support. It writes the table directly rather
// than through sqlite3_limit so that LIMIT_LENGTH below 30 stays what 3.40.1
// stores. The caller holds handle.mu with the handle open.
func sqliteApplyLimit(db uintptr, category int64, value int64) {
	if db == 0 {
		return
	}
	value = min(value, sqliteBarnHardLimits[category])
	// db is SQLite-allocated memory that never moves (see sqliteCBytes).
	(*(**sqlite3.Tsqlite3)(unsafe.Pointer(&db))).FaLimit[category] = int32(value)
}

// sqliteInterruptLocked is sqlite3_interrupt on the handle's connection. It is
// safe while a statement runs on another goroutine, and with none running
// SQLite clears it when the next statement starts. The caller holds h.mu.
func sqliteInterruptLocked(h *sqliteHandle) {
	if h.closed || h.rawDB == 0 {
		return
	}
	tls := libc.NewTLS()
	sqlite3.Xsqlite3_interrupt(tls, h.rawDB)
	tls.Close()
}

// sqliteRegexp is Toast's sqlite_regexp (pcre_moo.cc:454-489), registered as
// REGEXP(pattern, string) on every opened connection (sqlite.cc:276-278).
func sqliteRegexp(tls *libc.TLS, ctx uintptr, argc int32, argv uintptr) {
	args := sqliteCWords(argv, int(argc))
	resultError := func(msg string) {
		text := sqliteCString(tls, msg)
		if text == 0 {
			sqlite3.Xsqlite3_result_error_nomem(tls, ctx)
			return
		}
		sqlite3.Xsqlite3_result_error(tls, ctx, text, -1)
		sqlite3.Xsqlite3_free(tls, text)
	}
	pattern := sqlite3.Xsqlite3_value_text(tls, args[0])
	if pattern == 0 {
		resultError("SQLite REGEXP called with invalid pattern.")
		return
	}
	subject := sqlite3.Xsqlite3_value_text(tls, args[1])
	if subject == 0 {
		resultError("SQLite REGEXP called with invalid string.")
		return
	}
	// get_pcre(pattern, 0): case-sensitive, the compile pcre_match uses.
	re, err := cachedPCREPattern(libc.GoString(pattern), true)
	if err != nil {
		resultError(err.Error())
		return
	}
	matched := false
	if mapped, ok := pcreSubjectRunes(libc.GoString(subject)); ok {
		matched = re.MatchString(mapped)
	}
	var result int32
	if matched {
		result = 1
	}
	sqlite3.Xsqlite3_result_int(tls, ctx, result)
}

var (
	sqliteRegexpFn = *(*uintptr)(unsafe.Pointer(&struct {
		f func(*libc.TLS, uintptr, int32, uintptr)
	}{sqliteRegexp}))
	sqliteRegexpName = func() uintptr {
		p, err := libc.CString("REGEXP") // lives as long as the program
		if err != nil {
			panic(err)
		}
		return p
	}()
)

// sqliteSetUpConnection registers REGEXP and matches the SQLite limits and
// REAL-to-text precision of the Toast oracle.
func sqliteSetUpConnection(tls *libc.TLS, db uintptr, limits map[int64]int64) error {
	// SQLite 3.52 changed the default from 15 digits to round-trip precision.
	// Both MOO query builtins convert REAL results through SQLite's text API.
	const argsSize = 2 * 8 // ccgo uses 8-byte slots for variadic arguments.
	args := tls.Alloc(argsSize)
	defer tls.Free(argsSize)
	if rc := sqlite3.Xsqlite3_db_config(tls, db, sqlite3.SQLITE_DBCONFIG_FP_DIGITS, libc.VaList(args, 15, uintptr(0))); rc != sqlite3.SQLITE_OK {
		return fmt.Errorf("setting SQLite floating-point text precision: %s", libc.GoString(sqlite3.Xsqlite3_errstr(tls, rc)))
	}

	sqlite3.Xsqlite3_create_function(tls, db, sqliteRegexpName, 2,
		sqlite3.SQLITE_UTF8|sqlite3.SQLITE_DETERMINISTIC, 0, sqliteRegexpFn, 0, 0)
	for category, value := range limits {
		sqliteApplyLimit(db, category, value)
	}
	return nil
}
