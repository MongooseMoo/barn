package vm

import (
	"barn/parser"
	"barn/types"
	"testing"
)

func TestDeepCopyValueWaifCopiesNestedProperties(t *testing.T) {
	child := types.NewWaif(2, 3).SetProperty("count", types.NewInt(1))
	parent := types.NewWaif(1, 2)
	parent = parent.SetProperty("child", child)
	parent = parent.SetProperty("items", types.NewList([]types.Value{child}))

	copiedVal := deepCopyValue(parent)
	copied, ok := copiedVal.(types.WaifValue)
	if !ok {
		t.Fatalf("expected waif copy, got %T", copiedVal)
	}

	// Mutate original nested waif after copy.
	child = child.SetProperty("count", types.NewInt(99))
	parent = parent.SetProperty("child", child)
	parent = parent.SetProperty("items", types.NewList([]types.Value{child}))
	_ = parent

	copiedChildVal, ok := copied.GetProperty("child")
	if !ok {
		t.Fatal("expected copied child property")
	}
	copiedChild := copiedChildVal.(types.WaifValue)
	countVal, ok := copiedChild.GetProperty("count")
	if !ok {
		t.Fatal("expected copied child count property")
	}
	if count := countVal.(types.IntValue).Val; count != 1 {
		t.Fatalf("expected copied child count=1, got %d", count)
	}

	itemsVal, ok := copied.GetProperty("items")
	if !ok {
		t.Fatal("expected copied items property")
	}
	items := itemsVal.(types.ListValue)
	itemWaif := items.Get(1).(types.WaifValue)
	itemCountVal, ok := itemWaif.GetProperty("count")
	if !ok {
		t.Fatal("expected copied list waif count property")
	}
	if itemCount := itemCountVal.(types.IntValue).Val; itemCount != 1 {
		t.Fatalf("expected copied list waif count=1, got %d", itemCount)
	}
}

func TestForkStatementDeepCopiesWaifVariables(t *testing.T) {
	p := parser.NewParser("fork (0)\n  return 1;\nendfork\n")
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	forkStmt, ok := stmts[0].(*parser.ForkStmt)
	if !ok {
		t.Fatalf("expected parser.ForkStmt, got %T", stmts[0])
	}

	e := &Evaluator{
		env: NewEnvironment(),
	}
	waif := types.NewWaif(10, 11).SetProperty("value", types.NewInt(1))
	e.env.Set("w", waif)

	ctx := &types.TaskContext{
		ThisObj:        -1, // Skip source-line lookup path.
		Player:         1,
		TicksRemaining: 1000,
	}

	result := e.forkStmt(forkStmt, ctx)
	if result.Flow != types.FlowFork {
		t.Fatalf("expected FlowFork, got flow=%v err=%v", result.Flow, result.Error)
	}
	if result.ForkInfo == nil {
		t.Fatal("expected fork info")
	}

	forkWaifVal, ok := result.ForkInfo.Variables["w"]
	if !ok {
		t.Fatal("expected fork variable snapshot for w")
	}
	forkWaif := forkWaifVal.(types.WaifValue)

	// Mutate original env waif after fork snapshot.
	waif = waif.SetProperty("value", types.NewInt(99))
	e.env.Set("w", waif)

	forkValue, ok := forkWaif.GetProperty("value")
	if !ok {
		t.Fatal("expected fork waif value property")
	}
	if got := forkValue.(types.IntValue).Val; got != 1 {
		t.Fatalf("expected fork snapshot to keep value=1, got %d", got)
	}
}
