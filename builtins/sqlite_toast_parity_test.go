package builtins

import (
	"math"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Stock Toast 2.8.0_7 on a fresh managed server, fifteen sqlite_open calls
// and no close, then sqlite_handles(): {15, 14, 1, 2, ..., 13} (run record in
// agent-notes/fix-sqlite-toast-parity.md). Three handles give {3, 2, 1}; with
// a, b, c open, closing b and opening d gives {4, 3, 1}.
func TestSqliteHandleOrderMatchesToastUnorderedMap(t *testing.T) {
	var o sqliteHandleOrder
	for h := int64(1); h <= 3; h++ {
		o.insert(h)
	}
	assertOrder(t, o.list, []int64{3, 2, 1})
	o.erase(2)
	o.insert(4)
	assertOrder(t, o.list, []int64{4, 3, 1})

	var fresh sqliteHandleOrder
	for h := int64(1); h <= 15; h++ {
		fresh.insert(h)
	}
	assertOrder(t, fresh.list, []int64{15, 14, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13})
	if fresh.buckets != 29 {
		t.Fatalf("buckets = %d, want 29", fresh.buckets)
	}
}

func assertOrder(t *testing.T, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestSqliteHandlesRestartAtOneAndListNewestFirst(t *testing.T) {
	ctx := sqliteWizardCtx()
	defer sqliteCloseAllHandles(ctx)
	open := func() int64 {
		return sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})))
	}
	closeHandle := func(h int64) {
		sqliteMustResult(t, builtinSqliteClose(ctx, []types.Value{types.NewInt(h)}))
	}
	a := open()
	closeHandle(a)
	b := open()
	c := open()
	closeHandle(b)
	d := open()
	if got := types.NewList([]types.Value{types.NewInt(a), types.NewInt(b), types.NewInt(c), types.NewInt(d)}); got.String() != "{1, 1, 2, 3}" {
		t.Fatalf("handles = %s, want {1, 1, 2, 3}", got)
	}
	if got := sqliteMustResult(t, builtinSqliteHandles(ctx, nil)); got.String() != "{3, 2}" {
		t.Fatalf("sqlite_handles() = %s, want {3, 2}", got)
	}
}

func TestToastParseNumberAndFloat(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want types.Value
	}{
		{"12", types.NewInt(12)},
		{" 12 ", types.NewInt(12)},
		{"\t5", types.NewInt(5)},
		{"12\t", types.NewStr("12\t")},
		{"+7", types.NewInt(7)},
		{"99999999999999999999", types.NewInt(math.MaxInt64)},
		{"-99999999999999999999", types.NewInt(math.MinInt64)},
		{"1.5", types.NewFloat(1.5)},
		{"1e3", types.NewFloat(1000)},
		{"0x10", types.NewFloat(16)},
		{"0x1p3", types.NewFloat(8)},
		{"0x", types.NewStr("0x")},
		{"1e", types.NewStr("1e")},
		{".5", types.NewFloat(0.5)},
		{"-Infinity", types.NewFloat(math.Inf(-1))},
		{"1e999", types.NewFloat(math.Inf(1))},
		{"#5", types.NewObj(5)},
		{"#-1", types.NewObj(-1)},
		{"#", types.NewStr("#")},
		{"abc", types.NewStr("abc")},
		{"", types.NewStr("")},
	} {
		got := sqliteStringToMOO(tc.in, true, false)
		if got.Type() != tc.want.Type() || got.String() != tc.want.String() {
			t.Errorf("%q => %s (type %d), want %s (type %d)", tc.in, got, got.Type(), tc.want, tc.want.Type())
		}
	}
	if got := sqliteStringToMOO("nan", true, false); got.Type() != types.TYPE_FLOAT || !math.IsNaN(got.Float()) {
		t.Errorf("nan => %v, want NaN float", got)
	}
	if got := types.NewFloat(math.Inf(1)).String(); got != "inf.0" {
		t.Errorf("inf prints %q, want inf.0", got)
	}
}

func sqliteRunText(t *testing.T, ctx *Execution, h int64, query string) string {
	t.Helper()
	return sqliteMustResult(t, builtinSqliteQuery(ctx, []types.Value{types.NewInt(h), types.NewStr(query)})).String()
}

func sqliteExecText(t *testing.T, ctx *Execution, h int64, query string, params ...types.Value) string {
	t.Helper()
	return sqliteMustResult(t, builtinSqliteExecute(ctx, []types.Value{types.NewInt(h), types.NewStr(query), types.NewList(params)})).String()
}

// Expected values are stock Toast's, from the managed conformance run of
// sqlite_toast_parity.yaml.
func TestSqliteStatementsRunAsToastDoes(t *testing.T) {
	ctx := sqliteWizardCtx()
	defer sqliteCloseAllHandles(ctx)
	h := sqliteMustInt(t, sqliteMustResult(t, builtinSqliteOpen(ctx, []types.Value{types.NewStr(":memory:")})))

	for _, tc := range []struct{ got, want string }{
		{sqliteRunText(t, ctx, h, "SELECT 1; SELECT 2"), "{{1}, {2}}"},
		{sqliteRunText(t, ctx, h, "SELECT 1; SELEC 2"), `"near \"SELEC\": syntax error"`},
		{sqliteRunText(t, ctx, h, "CREATE TABLE t (a, b)"), "{}"},
		{sqliteExecText(t, ctx, h, "INSERT INTO t VALUES (1, 2); INSERT INTO t VALUES (3, 4)"), "{}"},
		{sqliteRunText(t, ctx, h, "INSERT INTO t VALUES (5, 6) RETURNING a"), "{{5}}"},
		{sqliteRunText(t, ctx, h, "SELECT count(*) FROM t"), "{{2}}"},
		{sqliteExecText(t, ctx, h, ""), `"not an error"`},
		{sqliteExecText(t, ctx, h, "SELECT typeof(?), typeof(?), ?", types.NewErr(types.E_PERM), types.NewList(nil), types.NewObj(5)), `{{"null", "null", #5}}`},
		{sqliteRunText(t, ctx, h, "SELECT 'x' REGEXP 'x', 'ABC' REGEXP 'b'"), "{{1, 0}}"},
		{sqliteRunText(t, ctx, h, "SELECT 'x' REGEXP NULL"), `"SQLite REGEXP called with invalid pattern."`},
		{sqliteRunText(t, ctx, h, "SELECT 0.1 + 0.2, 1e300 * 1e10"), "{{0.3, inf.0}}"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %s, want %s", tc.got, tc.want)
		}
	}

	sqliteMustResult(t, builtinSqliteInterrupt(ctx, []types.Value{types.NewInt(h)}))
	if got := sqliteRunText(t, ctx, h, "SELECT 1"); got != "{{1}}" {
		t.Errorf("query after idle interrupt = %s, want {{1}}", got)
	}

	limit := func(category, value int64) int64 {
		return sqliteMustInt(t, sqliteMustResult(t, builtinSqliteLimit(ctx, []types.Value{types.NewInt(h), types.NewInt(category), types.NewInt(value)})))
	}
	if got := limit(9, -1); got != 250000 {
		t.Errorf("LIMIT_VARIABLE_NUMBER = %d, want 250000", got)
	}
	limit(0, 3)
	for _, tc := range []struct{ got, want string }{
		{sqliteRunText(t, ctx, h, "SELECT 'abcdef'"), `"string or blob too big"`},
		{sqliteRunText(t, ctx, h, "SELECT 'ab' AS x"), `{{"ab"}}`},
		{sqliteExecText(t, ctx, h, "SELECT ?", types.NewStr("abcdef")), `{{"NULL"}}`},
	} {
		if tc.got != tc.want {
			t.Errorf("LIMIT_LENGTH 3: got %s, want %s", tc.got, tc.want)
		}
	}
}
