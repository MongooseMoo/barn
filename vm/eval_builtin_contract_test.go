package vm

import (
	"barn/db"
	"barn/parser"
	"barn/types"
	"strings"
	"testing"
)

func addProgrammer(t *testing.T, store *db.Store, id types.ObjID) {
	t.Helper()
	obj := db.NewObject(id, id)
	obj.Flags = obj.Flags.Set(db.FlagProgrammer)
	store.Add(obj)
}

func asEvalErrorTuple(t *testing.T, value types.Value) (types.IntValue, types.ListValue) {
	t.Helper()
	resultList, ok := value.(types.ListValue)
	if !ok {
		t.Fatalf("expected eval result list, got %T", value)
	}
	if resultList.Len() != 2 {
		t.Fatalf("expected 2-tuple from eval(), got length %d", resultList.Len())
	}
	okFlag, ok := resultList.Get(1).(types.IntValue)
	if !ok {
		t.Fatalf("expected eval tuple[1] to be INT, got %T", resultList.Get(1))
	}
	lines, ok := resultList.Get(2).(types.ListValue)
	if !ok {
		t.Fatalf("expected eval tuple[2] to be LIST, got %T", resultList.Get(2))
	}
	return okFlag, lines
}

func TestEvalBuiltinRuntimeErrorReturnsLineList_TreeWalker(t *testing.T) {
	store := db.NewStore()
	addProgrammer(t, store, 2)

	eval := NewEvaluatorWithStore(store)
	ctx := types.NewTaskContext()
	ctx.Player = 2
	ctx.Programmer = 2
	ctx.ThisObj = 2

	p := parser.NewParser(`eval("1 / 0;")`)
	expr, err := p.ParseExpression(0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	result := eval.Eval(expr, ctx)
	if result.Flow != types.FlowNormal {
		t.Fatalf("expected normal flow, got %v (%v)", result.Flow, result.Error)
	}

	okFlag, lines := asEvalErrorTuple(t, result.Val)
	if okFlag.Val != 0 {
		t.Fatalf("expected eval failure tuple {0, ...}, got {%d, ...}", okFlag.Val)
	}
	if lines.Len() < 1 {
		t.Fatal("expected at least one error line in eval failure payload")
	}

	first, ok := lines.Get(1).(types.StrValue)
	if !ok {
		t.Fatalf("expected first error line string, got %T", lines.Get(1))
	}
	if !strings.Contains(first.Value(), "Division by zero") {
		t.Fatalf("expected runtime error line to mention division by zero, got %q", first.Value())
	}
}

func TestEvalBuiltinRuntimeErrorReturnsTracebackLines_VM(t *testing.T) {
	store := db.NewStore()
	addProgrammer(t, store, 2)

	registry := BuildVMRegistry(store)

	p := parser.NewParser(`return eval("1 / 0;");`)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	compiler := NewCompilerWithRegistry(registry)
	prog, err := compiler.CompileStatements(stmts)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	vm := NewVM(store, registry)
	ctx := types.NewTaskContext()
	ctx.Player = 2
	ctx.Programmer = 2
	ctx.ThisObj = 2
	vm.Context = ctx

	vm.PrepareVerbFrame(prog, 2, 2, 2, "test_eval", 2, []types.Value{})
	result := vm.ExecuteLoop()
	if result.Flow != types.FlowReturn {
		t.Fatalf("expected FlowReturn, got %v (%v)", result.Flow, result.Error)
	}

	okFlag, lines := asEvalErrorTuple(t, result.Val)
	if okFlag.Val != 0 {
		t.Fatalf("expected eval failure tuple {0, ...}, got {%d, ...}", okFlag.Val)
	}
	if lines.Len() < 2 {
		t.Fatalf("expected traceback-style error lines, got %d line(s)", lines.Len())
	}

	first, ok := lines.Get(1).(types.StrValue)
	if !ok {
		t.Fatalf("expected first error line string, got %T", lines.Get(1))
	}
	if !strings.Contains(first.Value(), "Division by zero") {
		t.Fatalf("expected runtime error line to mention division by zero, got %q", first.Value())
	}

	foundBuiltinLine := false
	for i := 1; i <= lines.Len(); i++ {
		line, ok := lines.Get(i).(types.StrValue)
		if !ok {
			continue
		}
		if line.Value() == "... called from built-in function eval()" {
			foundBuiltinLine = true
			break
		}
	}
	if !foundBuiltinLine {
		t.Fatal("expected eval traceback payload to include built-in eval() call line")
	}

	last, ok := lines.Get(lines.Len()).(types.StrValue)
	if !ok {
		t.Fatalf("expected final traceback line string, got %T", lines.Get(lines.Len()))
	}
	if last.Value() != "(End of traceback)" {
		t.Fatalf("expected traceback terminator, got %q", last.Value())
	}
}
