package vm

import (
	"strings"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Toast builds call arguments as a list (code_gen.cc generate_arg_list) and
// holds each prefix after the first argument to max_list_value_bytes as it is
// built (execute.cc OP_LIST_ADD_TAIL, PUSH_ERROR_UNLESS_QUOTA). These tests
// pin Barn's stack-held arguments to the same results.

type argListFixture struct {
	listLimit int64 // 0 leaves max_list_value_bytes at its default
	catchable bool
	props     map[string]types.Value // properties defined on #0
}

func (f argListFixture) run(t *testing.T, debug bool, code string) types.Result {
	t.Helper()
	store := dbstore.NewStore()
	for _, id := range []types.ObjID{0, 1} {
		if err := store.Add(dbstore.NewObject(id, 0)); err != nil {
			t.Fatalf("store.Add(%d): %v", id, err)
		}
	}
	txn := store.DirectTxn()
	define := func(obj types.ObjID, name string, value types.Value) {
		if err := txn.DefineProperty(obj, name, dbstore.NewProperty(value, 0, dbstore.PropRead|dbstore.PropWrite, false, true)); err != types.E_NONE {
			t.Fatalf("DefineProperty(%s): %v", name, err)
		}
	}
	define(0, "server_options", types.NewObj(1))
	if f.listLimit != 0 {
		define(1, "max_list_value_bytes", types.NewInt(f.listLimit))
	}
	define(1, "max_concat_catchable", types.NewBool(f.catchable))
	define(0, "mark", types.NewInt(0))
	for name, value := range f.props {
		define(0, name, value)
	}
	registry := BuildVMRegistry()
	session := newTestSession(registry)
	session.LoadServerOptionsFromStore(store)
	program, diagnostics := registry.Compiler().CompileMOO([]string{code})
	if len(diagnostics) != 0 {
		t.Fatalf("CompileMOO: %v", diagnostics)
	}
	machine := NewVM(store, session)
	frame := machine.PrepareVerbFrame(program, 0, 0, 0, "test", 0, nil)
	frame.VerbDebug = debug
	return machine.ExecuteLoop()
}

func requireReturn(t *testing.T, result types.Result, want types.Value) {
	t.Helper()
	if result.Flow != types.FlowReturn || !result.Val.Equal(want) {
		t.Fatalf("result = flow %v error %v value %v, want return %v", result.Flow, result.Error, result.Val, want)
	}
}

func errs(codes ...any) types.Value {
	values := make([]types.Value, len(codes))
	for i, code := range codes {
		switch c := code.(type) {
		case types.ErrorCode:
			values[i] = types.NewErr(c)
		case int:
			values[i] = types.NewInt(int64(c))
		case types.Value:
			values[i] = c
		}
	}
	return types.NewList(values)
}

const x600 = `x = ""; for i in [1..600] x = x + "x"; endfor `

func TestArgListPrefixRefusedBeforeLaterArguments(t *testing.T) {
	fixture := argListFixture{listLimit: 1100, catchable: true}
	result := fixture.run(t, true, x600+"return {`equal(x, x, y = 1) ! ANY', `y ! ANY', `equal(x, x, 1 / 0) ! ANY', `equal(x, 1, x) ! ANY', `#0:none(x, x, z = 1) ! ANY', `z ! ANY'};")
	requireReturn(t, result, errs(types.E_QUOTA, types.E_VARNF, types.E_QUOTA, types.E_QUOTA, types.E_QUOTA, types.E_VARNF))
}

func TestArgListRefusedInVerbWithoutDebugFlag(t *testing.T) {
	fixture := argListFixture{listLimit: 1100, catchable: true}
	result := fixture.run(t, false, x600+"return {equal(x, x, y = 1), y, equal(x, x, 1 / 0), equal(x, 1, x), equal(x, x), #0:none(x, x)};")
	requireReturn(t, result, errs(types.E_TYPE, 1, types.E_TYPE, types.E_TYPE, types.E_TYPE, types.E_TYPE))
}

// A refusal recorded by OP_CHECK_ARGS stands even when a later argument
// raises the limit: Toast's list was already replaced by E_QUOTA.
func TestArgListRefusalSurvivesLaterLimitChange(t *testing.T) {
	fixture := argListFixture{listLimit: 1100, catchable: true}
	result := fixture.run(t, false, x600+"return equal(x, x, $server_options.max_list_value_bytes = 1000000, load_server_options());")
	requireReturn(t, result, types.NewErr(types.E_TYPE))
}

func TestArgListRefusalAbortsWhenNotCatchable(t *testing.T) {
	for _, debug := range []bool{true, false} {
		fixture := argListFixture{listLimit: 1100}
		result := fixture.run(t, debug, x600+"try equal(x, x, #0.mark = 1); except (ANY) return 2; endtry return 3;")
		requireExceptionReason(t, result, types.ExceptionReasonSecondsBudget)
	}
}

func TestArgListAtDefaultLimit(t *testing.T) {
	// {s, t} takes 32 + (16 + 32268897 + 1) + (16 + 32268898 + 1) = 64537861
	// bytes, exactly the default max_list_value_bytes; {t, t} one more.
	s := strings.Repeat("x", 32268897)
	fixture := argListFixture{catchable: true, props: map[string]types.Value{
		"s": types.NewStr(s),
		"t": types.NewStr(s + "x"),
	}}
	for _, debug := range []bool{true, false} {
		want := types.E_QUOTA
		if !debug {
			want = types.E_TYPE
		}
		result := fixture.run(t, debug, "s = #0.s; t = #0.t; return {equal(s, t), `equal(t, t) ! ANY', `equal(t, t, 1) ! ANY', `#0:none(t, t) ! ANY'};")
		requireReturn(t, result, errs(0, want, want, want))
	}
}

func TestSplicedArgListCheckedOnlyAfterFirstArgument(t *testing.T) {
	x := types.NewStr(strings.Repeat("x", 600))
	fixture := argListFixture{listLimit: 1100, catchable: true, props: map[string]types.Value{
		"big": types.NewList([]types.Value{x, x}),
		"x":   x,
	}}
	result := fixture.run(t, true, "big = #0.big; x = #0.x; return {equal(@big), length({@big}), length({x}), `equal(1, @big) ! ANY', `equal(x, @{x}) ! ANY', `{x, x} ! ANY'};")
	requireReturn(t, result, errs(1, 2, 1, types.E_QUOTA, types.E_QUOTA, types.E_QUOTA))
}

func TestCheckArgsEmittedBeforeNonLiteralLaterArguments(t *testing.T) {
	registry := BuildVMRegistry()
	program, diagnostics := registry.Compiler().CompileMOO([]string{"a = 1;", "if (1) b = 2; endif", "return {equal(a, b), #0:v(a, b, c, 1, \"s\", d), equal(a, b, 1), equal(b, c, a), equal(a, c, b)};"})
	if len(diagnostics) != 0 {
		t.Fatalf("CompileMOO: %v", diagnostics)
	}
	var counts []string
	for _, row := range bytecode.Disassemble(program) {
		// "ip: 0NN 0CC   CHECK_ARGS": the opcode byte, then the count.
		if fields := strings.Fields(row); len(fields) == 4 && fields[3] == "CHECK_ARGS" {
			counts = append(counts, fields[2])
		}
	}
	// Not before a literal, nor before a read of a, which a finished
	// top-level assignment bound; b was assigned only inside an if.
	if strings.Join(counts, ",") != "002,005,002" {
		t.Fatalf("OP_CHECK_ARGS counts = %v, want [002 005 002]", counts)
	}
}

func TestListBytesCountsRefusedMarkAboveEveryLimit(t *testing.T) {
	if size := types.ListBytes([]types.Value{types.None, types.NewInt(1)}); size <= 2147483647-1021 {
		t.Fatalf("ListBytes with the refused mark = %d", size)
	}
}

// A read of a surely bound variable is not preceded by OP_CHECK_ARGS; the
// call's own check gives the same result.
func TestArgListBoundVariableAfterRefusedPrefix(t *testing.T) {
	fixture := argListFixture{listLimit: 1100, catchable: true}
	for _, debug := range []bool{true, false} {
		want := types.E_QUOTA
		if !debug {
			want = types.E_TYPE
		}
		result := fixture.run(t, debug, "y = 1; "+x600+"return `equal(x, x, y) ! ANY';")
		requireReturn(t, result, types.NewErr(want))
	}
}
