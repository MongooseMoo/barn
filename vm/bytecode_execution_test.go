package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/compiler"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
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
	registry.SetTaskManager(task.NewManager())
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

func TestComputedPropertyAssignmentEvaluatesTargetBeforeValue(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagProgrammer)
	root.SetProperty("first", dbstore.NewProperty(types.NewStr("initial"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	root.SetProperty("second", dbstore.NewProperty(types.NewStr("unchanged"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	result := runBytecodeProgram(t, `name = "first"; #0.(name) = (name = "second"); return {#0.first, #0.second, name};`, store, nil)
	if result.Flow != types.FlowReturn {
		t.Fatalf("assignment flow = %v error = %v value = %v, want return", result.Flow, result.Error, result.Val)
	}
	if got := result.Val.String(); got != `{"second", "unchanged", "second"}` {
		t.Fatalf("assignment result = %s, want target evaluated before value", got)
	}
}

func TestComputedVerbCallEvaluatesNameBeforeArguments(t *testing.T) {
	store := newBytecodeVerbStore()
	if _, errCode := store.AddVerb(0, dbstore.NewVerb("target", []string{"target"}, 0,
		dbstore.VerbRead|dbstore.VerbWrite|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"return args[1];"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	result := runBytecodeProgram(t, `name = "target"; result = #0:(name)((name = "missing")); return {result, name};`, store, nil)
	if result.Flow != types.FlowReturn {
		t.Fatalf("call flow = %v error = %v value = %v, want return", result.Flow, result.Error, result.Val)
	}
	if got := result.Val.String(); got != `{"missing", "missing"}` {
		t.Fatalf("call result = %s, want name evaluated before arguments", got)
	}
}

func TestWaifIndexOperationsDispatchToClassHandlers(t *testing.T) {
	store := newBytecodeVerbStore()
	if errCode := store.SetObjectFlag(0, dbstore.FlagWizard, true); errCode != types.E_NONE {
		t.Fatalf("SetObjectFlag wizard failed: %v", errCode)
	}
	class := dbstore.NewObjectBuilder(1)
	class.SetOwner(0)
	class.SetParents([]types.ObjID{0})
	class.SetFlags(dbstore.FlagRead | dbstore.FlagWrite)
	if err := store.Add(class.Build()); err != nil {
		t.Fatalf("store.Add class failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "waif", dbstore.NewProperty(types.NewWaif(1, 0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty waif failed: %v", errCode)
	}
	if errCode := store.DefineProperty(1, ":last_key", dbstore.NewProperty(types.NewStr(""), 0, 0, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty last_key failed: %v", errCode)
	}
	if errCode := store.DefineProperty(1, ":last_value", dbstore.NewProperty(types.NewInt(0), 0, 0, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty last_value failed: %v", errCode)
	}
	execPerms := dbstore.VerbRead | dbstore.VerbWrite | dbstore.VerbExecute
	if _, errCode := store.AddVerb(1, dbstore.NewVerb(":_set_index", []string{":_set_index"}, 0,
		execPerms, dbstore.VerbArgs{}, []string{
			"this.last_key = args[1];",
			"this.last_value = args[2];",
			"return this;",
		})); errCode != types.E_NONE {
		t.Fatalf("AddVerb _set_index failed: %v", errCode)
	}
	if _, errCode := store.AddVerb(1, dbstore.NewVerb(":_index", []string{":_index"}, 0,
		execPerms, dbstore.VerbArgs{}, []string{
			"return {args[1], this.last_key, this.last_value, typeof(this) == WAIF};",
		})); errCode != types.E_NONE {
		t.Fatalf("AddVerb _index failed: %v", errCode)
	}

	result := runBytecodeProgram(t, `w = #0.waif; w["answer"] = 42; return w["answer"];`, store, nil)
	requireList(t, result,
		types.NewStr("answer"),
		types.NewStr("answer"),
		types.NewInt(42),
		types.NewInt(1),
	)
}

func TestWaifIndexWithoutHandlerReturnsTypeError(t *testing.T) {
	store := newBytecodeVerbStore()
	if errCode := store.SetObjectFlag(0, dbstore.FlagWizard, true); errCode != types.E_NONE {
		t.Fatalf("SetObjectFlag wizard failed: %v", errCode)
	}
	class := dbstore.NewObjectBuilder(1)
	class.SetOwner(0)
	class.SetParents([]types.ObjID{0})
	class.SetFlags(dbstore.FlagRead | dbstore.FlagWrite)
	if err := store.Add(class.Build()); err != nil {
		t.Fatalf("store.Add class failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "waif", dbstore.NewProperty(types.NewWaif(1, 0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty waif failed: %v", errCode)
	}

	result := runBytecodeProgram(t, `return #0.waif["missing"];`, store, nil)
	requireError(t, result, types.E_TYPE)
}

func TestNonwizardOwnedWaifClassCannotDispatchIndex(t *testing.T) {
	store := newBytecodeVerbStore()
	class := dbstore.NewObjectBuilder(1)
	class.SetOwner(0)
	class.SetParents([]types.ObjID{0})
	class.SetFlags(dbstore.FlagRead | dbstore.FlagWrite)
	if err := store.Add(class.Build()); err != nil {
		t.Fatalf("store.Add class failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "waif", dbstore.NewProperty(types.NewWaif(1, 0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty waif failed: %v", errCode)
	}
	execPerms := dbstore.VerbRead | dbstore.VerbWrite | dbstore.VerbExecute
	if _, errCode := store.AddVerb(1, dbstore.NewVerb(":_index", []string{":_index"}, 0,
		execPerms, dbstore.VerbArgs{}, []string{"return args[1];"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb _index failed: %v", errCode)
	}

	result := runBytecodeProgram(t, `return #0.waif["key"];`, store, nil)
	requireError(t, result, types.E_TYPE)
}

func TestListAppendOpcodeUsesPendingListValueByteLimit(t *testing.T) {
	ctx := kernel.NewTaskContext()
	resultList := types.NewList([]types.Value{types.NewInt(1), types.NewInt(2)})
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind: kernel.PendingEffectServerOptions,
		ServerOptions: kernel.PendingServerOptions{
			MaxListValueBytes: types.ValueBytes(resultList),
		},
	}}

	result := runBytecodeProgram(t, `x = {1}; return {@x, 2};`, nil, ctx)
	requireError(t, result, types.E_QUOTA)
}

func TestMapIndexAssignmentUsesPendingListValueByteLimit(t *testing.T) {
	ctx := kernel.NewTaskContext()
	resultMap := types.NewMap([][2]types.Value{{types.NewInt(1), types.NewInt(1)}})
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind: kernel.PendingEffectServerOptions,
		ServerOptions: kernel.PendingServerOptions{
			MaxListValueBytes: types.ValueBytes(resultMap),
			MaxMapValueBytes:  types.ValueBytes(resultMap) + 1,
		},
	}}

	result := runBytecodeProgram(t, `x = [1 -> 0]; x[1] = 1; return x;`, nil, ctx)
	requireError(t, result, types.E_QUOTA)
}

func TestMapRangeAssignmentUsesPendingListValueByteLimit(t *testing.T) {
	ctx := kernel.NewTaskContext()
	initialMap := types.NewMap([][2]types.Value{{types.NewInt(1), types.NewInt(0)}})
	resultMap := types.NewMap([][2]types.Value{{types.NewInt(1), types.NewInt(1)}})
	ctx.PendingEffects = []kernel.PendingEffect{{
		Kind: kernel.PendingEffectServerOptions,
		ServerOptions: kernel.PendingServerOptions{
			MaxListValueBytes: types.ValueBytes(resultMap),
			MaxMapValueBytes:  types.ValueBytes(resultMap) + 1,
		},
	}}

	machine := NewVM(nil, nil)
	machine.Context = ctx
	machine.pushFrame(&StackFrame{
		Program: &bytecode.Program{Code: []byte{0}},
		Locals:  []types.Value{initialMap},
	})
	machine.Push(resultMap)
	machine.Push(types.NewInt(1))
	machine.Push(types.NewInt(1))

	if err := machine.executeRangeSet(); err == nil || err.Error() != "E_QUOTA: map too large" {
		t.Fatalf("map range assignment error = %v, want E_QUOTA", err)
	}
}

func TestCaughtErrorDiscardsPartialExpressionOperands(t *testing.T) {
	code := `return {` +
		"`max(1, @5) ! E_TYPE => 11'," +
		"`#0.(max(@5)) ! E_TYPE => 22'," +
		"{1, @`max(@5) ! E_TYPE => {2, 3}', 4}" +
		`};`
	result := runBytecodeProgram(t, code, nil, nil)
	if result.Flow != types.FlowReturn {
		t.Fatalf("guarded expression flow = %v error = %v value = %v, want return", result.Flow, result.Error, result.Val)
	}
	if got := result.Val.String(); got != `{11, 22, {1, 2, 3, 4}}` {
		t.Fatalf("guarded expression result = %s, want partial operands discarded", got)
	}
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

func TestBytecodeErrorOrderingComparisons(t *testing.T) {
	cases := map[string]int64{
		"E_NONE < E_TYPE":   1,
		"E_TYPE < E_NONE":   0,
		"E_NONE <= E_NONE":  1,
		"E_PERM > E_TYPE":   1,
		"E_PERM >= E_PERM":  1,
		"E_DIV < E_PERM":    1,
		"E_RANGE < E_PERM":  0,
		"E_QUOTA > E_RANGE": 1,
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			requireInt(t, runBytecodeExpr(t, expr), want)
		})
	}

	requireError(t, runBytecodeExpr(t, "E_NONE < 1"), types.E_TYPE)
}

func TestBytecodeStringOrderingIsCaseInsensitive(t *testing.T) {
	cases := map[string]int64{
		`"a" < "B"`:             1,
		`"B" < "a"`:             0,
		`"a" <= "A"`:            1,
		`"a" > "B"`:             0,
		`"abc" < "ABD"`:         1,
		`"Zebra" < "apple"`:     0,
		`"hello" <= "HELLO"`:    1,
		`"hello" > "HELLO"`:     0,
		`"a" == "A"`:            1,
		`equal("a", "A")`:       0,
		`strcmp("a", "A") != 0`: 1,
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			requireInt(t, runBytecodeExpr(t, expr), want)
		})
	}
}

func TestBytecodeFloatOverflowRaisesEFloat(t *testing.T) {
	exprs := []string{
		"1.0e308 * 10.0",
		"1.0e200 * 1.0e200",
		"1.0e308 + 1.0e308",
		"-1.0e308 - 1.0e308",
		"floor(1.0e308 * 10.0)",
	}
	for _, expr := range exprs {
		t.Run(expr, func(t *testing.T) {
			requireError(t, runBytecodeExpr(t, expr), types.E_FLOAT)
		})
	}
}

func TestBytecodeIntegerLiteralOverflowWraps(t *testing.T) {
	cases := map[string]int64{
		"9223372036854775808":  -9223372036854775807 - 1,
		"18446744073709551616": 0,
		"18446744073709551615": -1,
		"10000000000000000000": -8446744073709551616,
		"99999999999999999999": 7766279631452241919,
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			requireInt(t, runBytecodeExpr(t, expr), want)
		})
	}

	requireInt(t, runBytecodeExpr(t, "-9223372036854775808 == 9223372036854775807 + 1"), 1)
}

func TestBytecodeFloatFormattingUsesToastPrecision(t *testing.T) {
	cases := map[string]string{
		"tostr(sqrt(2.0))":          "1.4142135623731",
		"tostr(1.0 / 3.0)":          "0.333333333333333",
		"tostr(10.0 / 3.0)":         "3.33333333333333",
		"tostr(0.1 + 0.2)":          "0.3",
		"tostr(exp(1.0))":           "2.71828182845905",
		"tostr(123456789012345.0)":  "123456789012345.0",
		"tostr(1234567890123456.0)": "1.23456789012346e+15",
		"tostr(1.0e10)":             "10000000000.0",
		"toliteral(1.0 / 3.0)":      "0.333333333333333",
	}
	for expr, want := range cases {
		t.Run(expr, func(t *testing.T) {
			requireString(t, runBytecodeExpr(t, expr), want)
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

func TestRejectedVerbCallDoesNotLeakTaskActivationFrame(t *testing.T) {
	store := newBytecodeVerbStore()
	registry := BuildVMRegistry()
	program, diagnostics := compiler.CompileMOO([]string{`
		try
			#0:test_return();
		except (E_MAXREC)
			return "caught";
		endtry
	`}, registry)
	if len(diagnostics) != 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	taskValue := task.NewTask(1, 0, 30_000, 1)
	machine := NewVM(store, registry)
	machine.MaxStackDepth = 1
	machine.Context = kernel.NewTaskContext()
	machine.Context.Task = taskValue

	result := machine.Run(program)
	if result.Flow != types.FlowReturn || result.Val.String() != `"caught"` {
		t.Fatalf("result = flow %v, value %v, error %v; want caught E_MAXREC", result.Flow, result.Val, result.Error)
	}
	if got := len(taskValue.CallStack); got != 0 {
		t.Fatalf("task activation frames after rejected call = %d, want 0", got)
	}
}

func TestPassWithNoParentRaisesInvind(t *testing.T) {
	store := newBytecodeVerbStore()
	obj, errCode := store.CreateObject(nil, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	if _, errCode := store.AddVerb(obj, dbstore.NewVerb("foo", []string{"foo"}, 0,
		dbstore.VerbRead|dbstore.VerbWrite|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"return pass();"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	result := runBytecodeProgram(t, "return `#1:foo() ! ANY';", store, nil)
	if result.Flow != types.FlowReturn && result.Flow != types.FlowNormal {
		t.Fatalf("flow = %v error = %v val = %v, want caught error value", result.Flow, result.Error, result.Val)
	}
	if result.Val.Type() != types.TYPE_ERR {
		t.Fatalf("value = %T %v, want ErrValue", result.Val, result.Val)
	}
	if result.Val.Code() != types.E_INVIND {
		t.Fatalf("caught error = %v, want E_INVIND", result.Val.Code())
	}
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

func TestBytecodeNestedFinallyUnwindsEveryFinalizer(t *testing.T) {
	tests := []struct {
		name    string
		program string
		want    []types.Value
	}{
		{
			name: "two levels",
			program: `
				result = {};
				try
					try
						try
							raise(E_INVARG);
						finally
							result = {@result, "inner"};
						endtry
					finally
						result = {@result, "outer"};
					endtry
				except (E_INVARG)
					return result;
				endtry`,
			want: []types.Value{types.NewStr("inner"), types.NewStr("outer")},
		},
		{
			name: "three levels",
			program: `
				result = {};
				try
					try
						try
							try
								raise(E_INVARG);
							finally
								result = {@result, "inner"};
							endtry
						finally
							result = {@result, "middle"};
						endtry
					finally
						result = {@result, "outer"};
					endtry
				except (E_INVARG)
					return result;
				endtry`,
			want: []types.Value{
				types.NewStr("inner"),
				types.NewStr("middle"),
				types.NewStr("outer"),
			},
		},
		{
			name: "finally inside except",
			program: `
				result = {};
				try
					try
						raise(E_INVARG);
					except (E_INVARG)
						result = {@result, "except"};
						try
							raise(E_TYPE);
						finally
							result = {@result, "finally"};
						endtry
					endtry
				except (E_TYPE)
					result = {@result, "outer-except"};
				endtry
				return result;`,
			want: []types.Value{
				types.NewStr("except"),
				types.NewStr("finally"),
				types.NewStr("outer-except"),
			},
		},
		{
			name: "except inside finally",
			program: `
				result = {};
				try
					try
						raise(E_INVARG);
					finally
						try
							raise(E_TYPE);
						except (E_TYPE)
							result = {@result, "inner-except"};
						endtry
						result = {@result, "finally"};
					endtry
				except (E_INVARG)
					result = {@result, "outer-except"};
				endtry
				return result;`,
			want: []types.Value{
				types.NewStr("inner-except"),
				types.NewStr("finally"),
				types.NewStr("outer-except"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireList(t, runBytecodeProgram(t, test.program, nil, nil), test.want...)
		})
	}
}

func TestBytecodeForkAndSuspendResume(t *testing.T) {
	requireInt(t, runBytecodeProgram(t, `fork (0) x = 1; endfork return 7;`, nil, nil), 7)
	requireInt(t, runBytecodeProgram(t, `suspend(0); return 9;`, nil, nil), 9)
}

func TestBytecodeErrorResumeRaisesIntoSavedExcept(t *testing.T) {
	store := dbstore.NewStore()
	registry := BuildVMRegistry()
	registry.SetTaskManager(task.NewManager())
	ctx := kernel.NewTaskContext()
	ctx.Task = task.NewTask(1, 0, ctx.TicksRemaining, 1)
	ctx.Store = store
	ctx.Registry = registry
	program, diagnostics := compiler.CompileMOO([]string{`
		try
			suspend();
			return "returned";
		except error (E_INTRPT)
			return error[1];
		endtry
	`}, registry)
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	machine := NewVM(store, registry)
	machine.Context = ctx
	if result := machine.Run(program); result.Flow != types.FlowSuspend {
		t.Fatalf("initial result = %#v, want suspend", result)
	}
	machine.SetResumeValue(types.NewErr(types.E_INTRPT), false)
	result := machine.Resume()
	if result.Flow != types.FlowReturn || !result.Val.Equal(types.NewErr(types.E_INTRPT)) {
		t.Fatalf("error resume result = %#v, want caught E_INTRPT", result)
	}
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

func TestBytecodeMapRangeAcceptsAnonymousKeyEndpoints(t *testing.T) {
	anon := types.NewAnon(123)
	mapping := types.NewMap([][2]types.Value{
		{anon, types.NewStr("value")},
	})
	machine := NewVM(nil, nil)
	machine.Push(mapping)
	machine.Push(anon)
	machine.Push(anon)

	if err := machine.executeRange(); err != nil {
		t.Fatalf("anonymous map range: %v", err)
	}
	if got := machine.Pop(); !got.Equal(mapping) {
		t.Fatalf("anonymous map range = %v, want %v", got, mapping)
	}
}
