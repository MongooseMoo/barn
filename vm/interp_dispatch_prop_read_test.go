package vm

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// The dispatch fast path keeps the instruction pointer in a local while it
// runs; every exit from it must leave cur.IP where execution really is. These
// tests raise, catch, suspend and call right after long runs of fast-path
// instructions (int arithmetic, local loads/stores, range loops, jumps).

func TestErrorLineAfterFastPathRun(t *testing.T) {
	code := "x = 0;\n" +
		"for i in [1..50]\n" +
		"x = x + i * 2 - 1;\n" +
		"endfor\n" +
		"y = x + \"a\";\n" +
		"return y;"
	result := runBytecodeProgram(t, code, nil, nil)
	requireError(t, result, types.E_TYPE)
	if len(result.CallStack) == 0 {
		t.Fatalf("uncaught error carries no call stack")
	}
	if got := result.CallStack[0].LineNumber; got != 5 {
		t.Fatalf("error line = %d, want 5", got)
	}
}

func TestTracebackLinesAfterFastPathRunInEveryFrame(t *testing.T) {
	store := newBytecodeVerbStore()
	store.AddVerb(0, dbstore.NewVerb([]string{"probe"}, 0,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, dbstore.VerbArgs{}, []string{
			"n = 0;",
			"for i in [1..40] n = n + i; endfor",
			"while (n > 7) n = n - 7; endwhile",
			"return n / 0;",
		}))
	code := "s = 0;\n" +
		"for i in [1..30] s = s + i % 4; endfor\n" +
		"while (s > 3) s = s - 3; endwhile\n" +
		"return #0:probe();"
	result := runBytecodeProgram(t, code, store, nil)
	requireError(t, result, types.E_DIV)
	var lines []int
	for _, frame := range result.CallStack {
		lines = append(lines, frame.LineNumber)
	}
	if len(lines) != 2 || lines[0] != 4 || lines[1] != 4 {
		t.Fatalf("call stack lines = %v, want [4 4] (probe's division, then the caller's call)", lines)
	}
}

func TestSuspendResumeBetweenFastPathRuns(t *testing.T) {
	code := "x = 0;\n" +
		"for i in [1..20] x = x + i; endfor\n" +
		"suspend(0);\n" +
		"for i in [1..20] x = x + i; endfor\n" +
		"suspend(0);\n" +
		"y = x - 400;\n" +
		"return {x, y, 1 / (y - y)};"
	result := runBytecodeProgram(t, code, nil, nil)
	requireError(t, result, types.E_DIV)
	if len(result.CallStack) == 0 || result.CallStack[0].LineNumber != 7 {
		t.Fatalf("call stack = %+v, want the error on line 7", result.CallStack)
	}
	code = "x = 0;\n" +
		"for i in [1..20] x = x + i; endfor\n" +
		"suspend(0);\n" +
		"for i in [1..20] x = x + i; endfor\n" +
		"return x;"
	requireInt(t, runBytecodeProgram(t, code, nil, nil), 420)
}

// Property reads: a static name is classified once at compile time, a dynamic
// name at run time, and either way the object is checked before the name.

func propertyReadTestStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetName("Root")
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite)
	root.SetProperty("foo", dbstore.NewProperty(types.NewInt(42), 0, dbstore.PropRead, false, true))
	root.SetProperty("secret", dbstore.NewProperty(types.NewInt(7), 0, 0, false, true))
	root.SetProperty("m", dbstore.NewProperty(types.NewMap([][2]types.Value{{types.NewStr("k"), types.NewInt(9)}}), 0, dbstore.PropRead, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	child := dbstore.NewObjectBuilder(1)
	child.SetOwner(0)
	child.SetName("Child")
	child.SetParents([]types.ObjID{0})
	if err := store.Add(child.Build()); err != nil {
		t.Fatalf("Add #1: %v", err)
	}
	gone := dbstore.NewObjectBuilder(2)
	gone.SetOwner(0)
	gone.SetName("Gone")
	gone.SetParents([]types.ObjID{0})
	if err := store.Add(gone.Build()); err != nil {
		t.Fatalf("Add #2: %v", err)
	}
	if err := store.Recycle(2); err != nil {
		t.Fatalf("Recycle #2: %v", err)
	}
	return store
}

func TestCompilerClassifiesStaticPropertyNames(t *testing.T) {
	registry := BuildVMRegistry()
	prog, diagnostics := registry.Compiler().CompileMOO([]string{
		`x = {#1.name, #1.Owner, #1.foo, "r", "w" + "x", 5};`,
		`fork (0) y = #1.LAST_MOVE; endfork`,
	})
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO: %v", diagnostics[0])
	}
	if len(prog.BuiltinPropertyConstants) != len(prog.Constants) {
		t.Fatalf("BuiltinPropertyConstants has %d entries for %d constants", len(prog.BuiltinPropertyConstants), len(prog.Constants))
	}
	want := map[string]bool{"name": true, "Owner": true, "foo": false, "r": true, "w": true, "x": false, "LAST_MOVE": true}
	seen := 0
	for i, constant := range prog.Constants {
		if constant.Type() != types.TYPE_STR {
			if prog.BuiltinPropertyConstants[i] {
				t.Fatalf("constant %d (%v) is not a string but is marked built-in", i, constant)
			}
			continue
		}
		if expected, ok := want[constant.Str()]; ok {
			seen++
			if prog.BuiltinPropertyConstants[i] != expected {
				t.Fatalf("constant %q marked built-in = %v, want %v", constant.Str(), prog.BuiltinPropertyConstants[i], expected)
			}
		}
	}
	if seen != len(want) {
		t.Fatalf("found %d of the %d expected string constants in %v", seen, len(want), prog.Constants)
	}
}

func TestPropertyReadNameClassification(t *testing.T) {
	cases := []struct {
		expr string
		want string
	}{
		{`#1.name`, `"Child"`},
		{`#1.NaMe`, `"Child"`},
		{`#1.("name")`, `"Child"`},
		{`#1.("OWNER")`, `#0`},
		{`#1.r`, `0`},
		{`#0.R`, `1`},
		{`#1.foo`, `42`},
		{`#1.FOO`, `42`},
		{`#1.("foo")`, `42`},
		{`#1.m["k"]`, `9`},
		{`#0.foo`, `42`},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			ctx := task.NewContext()
			ctx.IsWizard = true
			result := runBytecodeProgram(t, "return "+tc.expr+";", propertyReadTestStore(t), ctx)
			if result.Flow != types.FlowReturn || result.Val.String() != tc.want {
				t.Fatalf("%s = flow %v, error %s, val %v; want %s", tc.expr, result.Flow, result.Error, result.Val, tc.want)
			}
		})
		// A program the compiler did not classify (restored from a database)
		// classifies its static names at run time, with the same results.
		t.Run(tc.expr+"/unclassified", func(t *testing.T) {
			store := propertyReadTestStore(t)
			registry := BuildVMRegistry()
			compiled, diagnostics := registry.Compiler().CompileMOO([]string{"return " + tc.expr + ";"})
			if len(diagnostics) > 0 {
				t.Fatalf("CompileMOO: %v", diagnostics[0])
			}
			prog := *compiled
			prog.BuiltinPropertyConstants = nil
			ctx := task.NewContext()
			ctx.IsWizard = true
			ctx.Store = store
			machine := NewVM(store, newTestSession(registry))
			machine.Context = ctx
			machine.Task = task.NewState(1, 0, 30000, 1)
			result := machine.Run(&prog)
			if result.Flow != types.FlowReturn || result.Val.String() != tc.want {
				t.Fatalf("%s = flow %v, error %s, val %v; want %s", tc.expr, result.Flow, result.Error, result.Val, tc.want)
			}
		})
	}
}

func TestPropertyReadErrors(t *testing.T) {
	cases := []struct {
		expr string
		want types.ErrorCode
	}{
		// Invalid and recycled objects raise E_INVIND before the name is
		// considered, for built-in, user, missing and dynamic names alike.
		{`#99.foo`, types.E_INVIND},
		{`#99.name`, types.E_INVIND},
		{`#99.nope`, types.E_INVIND},
		{`#99.("foo")`, types.E_INVIND},
		{`#99.m["k"]`, types.E_INVIND},
		{`#2.foo`, types.E_INVIND},
		{`#2.name`, types.E_INVIND},
		{`#2.("name")`, types.E_INVIND},
		{`#-1.foo`, types.E_INVIND},
		{`#1.nope`, types.E_PROPNF},
		{`#1.("nope")`, types.E_PROPNF},
		{`#1.secret`, types.E_PERM},
		{`#1.(5)`, types.E_TYPE},
		{`{5}[1].foo`, types.E_TYPE},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			ctx := task.NewContext()
			ctx.Programmer = 1
			requireError(t, runBytecodeProgram(t, "return "+tc.expr+";", propertyReadTestStore(t), ctx), tc.want)
		})
	}
}

// A property read depends on its object staying valid: the existence read it
// records makes a concurrent change to the object's scalar state (here its
// name) fail the reading task's commit, even though the property itself was
// not touched.
func TestPropertyReadRecordsObjectExistence(t *testing.T) {
	for _, root := range []bool{true, false} {
		name := map[bool]string{true: "root", false: "nested"}[root]
		t.Run(name, func(t *testing.T) {
			store := propertyReadTestStore(t)
			registry := BuildVMRegistry()
			prog, diagnostics := registry.Compiler().CompileMOO([]string{"return #1.foo;"})
			if len(diagnostics) > 0 {
				t.Fatalf("CompileMOO: %v", diagnostics[0])
			}
			ctx := task.NewContext()
			ctx.IsWizard = true
			ctx.Store = store
			ctx.StoreTxn = store.BeginSnapshot(0)
			defer func() { ctx.StoreTxn.Release() }()

			machine := NewVM(store, newTestSession(registry))
			machine.Context = ctx
			machine.Task = task.NewState(1, 0, 30000, 1)
			machine.Resumable = root
			machine.PrepareVerbFrame(prog, types.ObjNothing, 0, 0, "", types.ObjNothing, []types.Value{})
			result := machine.ExecuteLoop()
			if result.Flow != types.FlowReturn || result.Val.String() != "42" {
				t.Fatalf("#1.foo = flow %v, error %s, val %v", result.Flow, result.Error, result.Val)
			}

			other := store.BeginSnapshot(0)
			if ec := other.SetObjectName(1, "Renamed"); ec != types.E_NONE {
				t.Fatalf("SetObjectName: %v", ec)
			}
			if ec := other.Commit(); ec != types.E_NONE {
				t.Fatalf("other Commit: %v", ec)
			}
			other.Release()

			if ec := ctx.StoreTxn.SetPropertyValue(0, "foo", types.NewInt(43)); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue: %v", ec)
			}
			if ec := ctx.StoreTxn.Commit(); ec == types.E_NONE {
				t.Fatalf("commit after #1 was renamed succeeded; the read of #1.foo lost its existence dependency")
			}
		})
	}
}
