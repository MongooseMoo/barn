package vm

import (
	"testing"

	"barn/compiler"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
)

func runBytecodeProgram(t *testing.T, code string, store *dbstore.Store, ctx *kernel.TaskContext) types.Result {
	t.Helper()
	if store == nil {
		store = dbstore.NewStore()
	}
	if ctx == nil {
		ctx = kernel.NewTaskContext()
	}
	if ctx.Task == nil {
		ctx.Task = task.NewTask(1, types.ObjID(0), ctx.TicksRemaining, 1)
	}

	registry := BuildVMRegistry()
	ctx.Store = store
	ctx.Registry = registry

	prog, diagnostics := compiler.CompileMOO([]string{code}, registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	machine := NewVM(store, registry)
	machine.Context = ctx
	result := machine.Run(prog)
	for result.Flow == types.FlowSuspend || result.Flow == types.FlowFork {
		if result.Flow == types.FlowFork && result.ForkInfo != nil && result.ForkInfo.VarName != "" {
			machine.SetForkResult(0)
		}
		result = machine.Resume()
	}
	return result
}

func runBytecodeExpr(t *testing.T, expr string) types.Result {
	t.Helper()
	return runBytecodeProgram(t, "return "+expr+";", nil, nil)
}

func requireInt(t *testing.T, result types.Result, want int64) {
	t.Helper()
	if result.Flow != types.FlowReturn && result.Flow != types.FlowNormal {
		t.Fatalf("flow = %v, want value %d (error %s, val %v)", result.Flow, want, result.Error, result.Val)
	}
	if result.Val.Type() != types.TYPE_INT {
		t.Fatalf("value = %v, want int %d", result.Val, want)
	}
	if result.Val.Int() != want {
		t.Fatalf("value = %d, want %d", result.Val.Int(), want)
	}
}

func requireString(t *testing.T, result types.Result, want string) {
	t.Helper()
	if result.Flow != types.FlowReturn && result.Flow != types.FlowNormal {
		t.Fatalf("flow = %v, want string %q (error %s, val %v)", result.Flow, want, result.Error, result.Val)
	}
	if result.Val.Type() != types.TYPE_STR {
		t.Fatalf("value = %v, want string %q", result.Val, want)
	}
	if result.Val.Str() != want {
		t.Fatalf("value = %q, want %q", result.Val.Str(), want)
	}
}

func requireError(t *testing.T, result types.Result, want types.ErrorCode) {
	t.Helper()
	if result.Flow != types.FlowException {
		t.Fatalf("flow = %v, want exception %s with val %v", result.Flow, want, result.Val)
	}
	if result.Error != want {
		t.Fatalf("error = %s, want %s", result.Error, want)
	}
}

func requireList(t *testing.T, result types.Result, want ...types.Value) {
	t.Helper()
	if result.Flow != types.FlowReturn && result.Flow != types.FlowNormal {
		t.Fatalf("flow = %v, want list (error %s, val %v)", result.Flow, result.Error, result.Val)
	}
	if result.Val.Type() != types.TYPE_LIST {
		t.Fatalf("value = %v, want list", result.Val)
	}
	got := result.Val
	if got.Len() != len(want) {
		t.Fatalf("list length = %d, want %d: %v", got.Len(), len(want), got)
	}
	for i, wantVal := range want {
		gotVal := got.Get(i + 1)
		if gotVal.Type() != wantVal.Type() || !gotVal.Equal(wantVal) {
			t.Fatalf("list[%d] = %v (%T), want %v (%T)", i+1, gotVal, gotVal, wantVal, wantVal)
		}
	}
}

func TestBytecodeExpressionBasics(t *testing.T) {
	cases := map[string]int64{
		"1 + 2 * 3":         7,
		"{10, 20}[2]":       20,
		"length(\"hello\")": 5,
		"#123 == #123":      1,
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			requireInt(t, runBytecodeExpr(t, expr), want)
		})
	}
}

func TestBytecodeScatterAssignment(t *testing.T) {
	cases := map[string][]types.Value{
		"optional_default": {
			types.NewInt(5),
		},
		"optional_keeps_existing": {types.NewInt(42)},
		"optional_and_rest":       {types.NewStr("hello"), types.NewList([]types.Value{})},
	}
	programs := map[string]string{
		"optional_default":        `{a, ?b = 5} = {1}; return {b};`,
		"optional_keeps_existing": `b = 42; {a, ?b} = {1}; return {b};`,
		"optional_and_rest":       `b = "hello"; {a, ?b, @rest} = {1}; return {b, rest};`,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			requireList(t, runBytecodeProgram(t, programs[name], nil, nil), want...)
		})
	}

	t.Run("optional_without_existing_value", func(t *testing.T) {
		requireError(t, runBytecodeProgram(t, `{a, ?b} = {1}; return b;`, nil, nil), types.E_VARNF)
	})
}

func newBytecodeVerbStore() *dbstore.Store {
	store := dbstore.NewStore()
	rootBuilder := dbstore.NewObjectBuilder(0)
	rootBuilder.SetOwner(0)
	rootBuilder.SetName("Root")
	rootBuilder.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagProgrammer)
	store.Add(rootBuilder.Build())

	execPerms := dbstore.VerbRead | dbstore.VerbWrite | dbstore.VerbExecute
	debugPerms := execPerms | dbstore.VerbDebug
	store.AddVerb(0, dbstore.NewVerb("test_return", []string{"test_return"}, 0,
		execPerms, dbstore.VerbArgs{}, []string{"return 42;"}))
	store.AddVerb(0, dbstore.NewVerb("test_throw", []string{"test_throw"}, 0,
		debugPerms, dbstore.VerbArgs{}, []string{"return 1 / 0;"}))
	store.AddVerb(0, dbstore.NewVerb("test_chain_b", []string{"test_chain_b"}, 0,
		debugPerms, dbstore.VerbArgs{}, []string{"return this:test_chain_c();"}))
	store.AddVerb(0, dbstore.NewVerb("test_chain_c", []string{"test_chain_c"}, 0,
		debugPerms, dbstore.VerbArgs{}, []string{"return 1 / 0;"}))
	store.AddVerb(0, dbstore.NewVerb("test_no_exec", []string{"test_no_exec"}, 0,
		dbstore.VerbRead|dbstore.VerbWrite, dbstore.VerbArgs{}, []string{"return 1;"}))
	return store
}

func TestBytecodeVerbCallExceptions(t *testing.T) {
	store := newBytecodeVerbStore()
	requireError(t, runBytecodeProgram(t, `return #0:test_throw();`, store, nil), types.E_DIV)

	caught := `
		try
			return #0:test_throw();
		except (E_DIV)
			return "caught";
		endtry`
	requireString(t, runBytecodeProgram(t, caught, store, nil), "caught")

	deepCaught := `
		try
			return #0:test_chain_b();
		except (E_DIV)
			return "caught from C";
		endtry`
	requireString(t, runBytecodeProgram(t, deepCaught, store, nil), "caught from C")

	noExecCaught := `
		try
			return #0:test_no_exec();
		except (E_VERBNF)
			return "no such verb";
		endtry`
	requireString(t, runBytecodeProgram(t, noExecCaught, store, nil), "no such verb")
}

func TestPassPreservesOriginalCaller(t *testing.T) {
	store := newBytecodeVerbStore()
	for id, parent := range map[types.ObjID]types.ObjID{1: 0, 2: 1, 3: 2} {
		builder := dbstore.NewObjectBuilder(id)
		builder.SetOwner(0)
		builder.SetName("pass test object")
		builder.SetFlags(dbstore.FlagRead | dbstore.FlagWrite)
		builder.SetParents([]types.ObjID{parent})
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", id, err)
		}
	}

	execPerms := dbstore.VerbRead | dbstore.VerbWrite | dbstore.VerbExecute | dbstore.VerbDebug
	if _, errCode := store.AddVerb(1, dbstore.NewVerb("pass_caller", []string{"pass_caller"}, 0,
		execPerms, dbstore.VerbArgs{}, []string{"return caller;"})); errCode != types.E_NONE {
		t.Fatalf("add pass target: %s", errCode)
	}
	if _, errCode := store.AddVerb(2, dbstore.NewVerb("pass_caller", []string{"pass_caller"}, 0,
		execPerms, dbstore.VerbArgs{}, []string{"return pass();"})); errCode != types.E_NONE {
		t.Fatalf("add pass source: %s", errCode)
	}

	verb, defObjID, err := store.FindVerb(3, "pass_caller")
	if err != nil {
		t.Fatalf("find inherited verb: %v", err)
	}
	registry := BuildVMRegistry()
	prog, diagnostics := compiler.CompileMOO(verb.Code, registry)
	if len(diagnostics) != 0 {
		t.Fatalf("compile inherited verb: %v", diagnostics)
	}
	machine := NewVM(store, registry)
	machine.Context = kernel.NewTaskContext()
	result := machine.RunWithVerbContext(prog, 3, 3, 3, "pass_caller", defObjID, nil)
	if result.Flow != types.FlowReturn || result.Val.Type() != types.TYPE_OBJ || result.Val.Obj() != 3 {
		t.Fatalf("pass target caller = %v, want original caller #3", result.Val)
	}
}

func TestBytecodeAnonymousNestedThisCallPreservesCallerIdentity(t *testing.T) {
	store := newBytecodeVerbStore()
	anon := dbstore.NewObjectBuilder(1)
	anon.SetOwner(0)
	anon.SetFlags(dbstore.FlagRead | dbstore.FlagAnonymous)
	anon.SetAnonymous(true)
	anon.SetParents([]types.ObjID{0})
	if err := store.Add(anon.Build()); err != nil {
		t.Fatalf("add anonymous object: %v", err)
	}
	if errCode := store.DefineProperty(0, "anon", dbstore.NewProperty(types.NewAnon(1), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define anonymous reference: %s", errCode)
	}

	execPerms := dbstore.VerbRead | dbstore.VerbWrite | dbstore.VerbExecute
	if _, errCode := store.AddVerb(0, dbstore.NewVerb("anon_inner", []string{"anon_inner"}, 0,
		execPerms, dbstore.VerbArgs{}, []string{"return {typeof(this) == ANON, typeof(caller) == ANON, caller == this};"})); errCode != types.E_NONE {
		t.Fatalf("add inner verb: %s", errCode)
	}
	if _, errCode := store.AddVerb(0, dbstore.NewVerb("anon_outer", []string{"anon_outer"}, 0,
		execPerms, dbstore.VerbArgs{}, []string{"return this:anon_inner();"})); errCode != types.E_NONE {
		t.Fatalf("add outer verb: %s", errCode)
	}
	if _, errCode := store.AddVerb(0, dbstore.NewVerb("caller_type", []string{"caller_type"}, 0,
		execPerms, dbstore.VerbArgs{}, []string{"return typeof(caller);"})); errCode != types.E_NONE {
		t.Fatalf("add caller type verb: %s", errCode)
	}

	requireList(t, runBytecodeProgram(t, "return #0.anon:anon_outer();", store, nil),
		types.NewInt(1), types.NewInt(1), types.NewInt(1))

	staleContext := kernel.NewTaskContext()
	staleContext.ThisValue = types.NewAnon(1)
	requireInt(t, runBytecodeProgram(t, "return #0:caller_type();", store, staleContext), int64(types.TYPE_OBJ))
}

// TestBytecodeNestedCatchInsideTryDoesNotPopOuterHandler covers a case where
// a backtick catch-expression (its own single-clause try/except) evaluates
// inside the body of an outer try/except, on an early loop iteration that
// never reaches the exception-raising call. OP_END_EXCEPT used to pop every
// trailing Except-type handler instead of exactly the count its matching
// OP_TRY_EXCEPT pushed, so closing the inner catch-expression's handler also
// discarded the outer loop's still-live handler — leaving a later iteration's
// real exception uncaught.
func TestBytecodeNestedCatchInsideTryDoesNotPopOuterHandler(t *testing.T) {
	store := newBytecodeVerbStore()
	program := "" +
		"results = {};\n" +
		"for i in ({1, 2})\n" +
		"try\n" +
		"x = `1 + 1 ! ANY => 0';\n" +
		"if (i == 1)\n" +
		"continue;\n" +
		"endif\n" +
		"#0:test_no_exec();\n" +
		"except (E_VERBNF)\n" +
		"results = {@results, \"caught\"};\n" +
		"continue i;\n" +
		"endtry\n" +
		"endfor\n" +
		"return results;"
	requireList(t, runBytecodeProgram(t, program, store, nil), types.NewStr("caught"))
}

func TestBytecodeFinallyDuringVerbUnwind(t *testing.T) {
	store := newBytecodeVerbStore()
	program := `
		x = 0;
		try
			try
				#0:test_chain_b();
			finally
				x = 42;
			endtry
		except (E_DIV)
			return x;
		endtry`
	requireInt(t, runBytecodeProgram(t, program, store, nil), 42)
}

func TestBytecodeForkAndSuspendResume(t *testing.T) {
	requireInt(t, runBytecodeProgram(t, `fork (0) x = 1; endfork return 7;`, nil, nil), 7)
	requireInt(t, runBytecodeProgram(t, `suspend(0); return 9;`, nil, nil), 9)
}

func TestBytecodeMapRangeBoundariesRemainPositional(t *testing.T) {
	read := runBytecodeExpr(t, `["b" -> 2, "a" -> 1][^..$]`)
	wantRead := types.NewMap([][2]types.Value{
		{types.NewStr("a"), types.NewInt(1)},
		{types.NewStr("b"), types.NewInt(2)},
	})
	if read.Flow != types.FlowReturn || !read.Val.Equal(wantRead) {
		t.Fatalf("map range read = %#v, want %v", read, wantRead)
	}

	assigned := runBytecodeProgram(t,
		`m = [10 -> "a", 20 -> "b"]; m[^..$] = [30 -> "c"]; return m;`,
		nil,
		nil,
	)
	wantAssigned := types.NewMap([][2]types.Value{
		{types.NewInt(30), types.NewStr("c")},
	})
	if assigned.Flow != types.FlowReturn || !assigned.Val.Equal(wantAssigned) {
		t.Fatalf("map range assignment = %#v, want %v", assigned, wantAssigned)
	}
}
