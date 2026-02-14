package vm

import (
	"barn/db"
	"barn/parser"
	"barn/types"
	"testing"
)

func TestForkSourceLineExtraction(t *testing.T) {
	// Create a store and add a test object
	store := db.NewStore()
	obj := db.NewObject(1, 0)
	store.Add(obj)

	// Create a test verb with fork statement
	verbCode := []string{
		"x = 1;",
		"fork (5)",
		"  y = 2;",
		"  return y;",
		"endfork",
		"return x;",
	}

	verb := &db.Verb{
		Name:  "test_fork",
		Names: []string{"test_fork"},
		Owner: 0,
		Code:  verbCode,
	}

	// Parse the verb code
	source := ""
	for _, line := range verbCode {
		source += line + "\n"
	}

	p := parser.NewParser(source)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Compile the verb
	verb.Program = &db.VerbProgram{Statements: stmts}
	obj.Verbs = map[string]*db.Verb{"test_fork": verb}
	obj.VerbList = []*db.Verb{verb}

	// Create evaluator with this store
	env := NewEnvironment()
	e := &Evaluator{
		env:   env,
		store: store,
	}

	// Find the fork statement (should be statement 2)
	if len(stmts) < 2 {
		t.Fatalf("Expected at least 2 statements, got %d", len(stmts))
	}

	forkStmt, ok := stmts[1].(*parser.ForkStmt)
	if !ok {
		t.Fatalf("Second statement is not a fork statement: %T", stmts[1])
	}

	// Create a task context
	ctx := &types.TaskContext{
		ThisObj: 1,
		Verb:    "test_fork",
		Player:  0,
	}

	// Extract source lines
	sourceLines := e.extractForkSourceLines(forkStmt, ctx)

	// Verify we got the right lines
	if sourceLines == nil {
		t.Fatal("extractForkSourceLines returned nil")
	}

	// The fork body should be lines 3-4 (0-indexed: "  y = 2;" and "  return y;")
	expectedLines := []string{
		"  y = 2;",
		"  return y;",
	}

	if len(sourceLines) != len(expectedLines) {
		t.Fatalf("Expected %d source lines, got %d: %v", len(expectedLines), len(sourceLines), sourceLines)
	}

	for i, expected := range expectedLines {
		if sourceLines[i] != expected {
			t.Errorf("Line %d mismatch: expected %q, got %q", i, expected, sourceLines[i])
		}
	}
}

func TestForkStatementPopulatesSourceLines(t *testing.T) {
	// Create a store and add a test object
	store := db.NewStore()
	obj := db.NewObject(1, 0)
	store.Add(obj)

	// Create a test verb with fork statement
	verbCode := []string{
		"fork (0)",
		"  return 42;",
		"endfork",
	}

	verb := &db.Verb{
		Name:  "test",
		Names: []string{"test"},
		Owner: 0,
		Code:  verbCode,
	}

	// Parse and compile
	source := ""
	for _, line := range verbCode {
		source += line + "\n"
	}

	p := parser.NewParser(source)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	verb.Program = &db.VerbProgram{Statements: stmts}
	obj.Verbs = map[string]*db.Verb{"test": verb}
	obj.VerbList = []*db.Verb{verb}

	// Create evaluator
	env := NewEnvironment()
	e := &Evaluator{
		env:   env,
		store: store,
	}

	// Execute the fork statement
	ctx := &types.TaskContext{
		ThisObj:        1,
		Verb:           "test",
		Player:         0,
		TicksRemaining: 1000,
	}

	// Get the fork statement
	if len(stmts) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(stmts))
	}

	forkStmt, ok := stmts[0].(*parser.ForkStmt)
	if !ok {
		t.Fatalf("Expected ForkStmt, got %T", stmts[0])
	}

	// Execute just the fork statement
	result := e.EvalStmt(forkStmt, ctx)

	// Verify we got a fork result
	if result.Flow != types.FlowFork {
		t.Fatalf("Expected FlowFork, got %v (error: %v, val: %v)", result.Flow, result.Error, result.Val)
	}

	// Verify ForkInfo has source lines
	if result.ForkInfo == nil {
		t.Fatal("ForkInfo is nil")
	}

	if result.ForkInfo.SourceLines == nil {
		t.Fatal("ForkInfo.SourceLines is nil")
	}

	if len(result.ForkInfo.SourceLines) == 0 {
		t.Fatal("ForkInfo.SourceLines is empty")
	}

	// Should have the return statement
	if len(result.ForkInfo.SourceLines) != 1 {
		t.Errorf("Expected 1 source line, got %d: %v", len(result.ForkInfo.SourceLines), result.ForkInfo.SourceLines)
	}

	expected := "  return 42;"
	if result.ForkInfo.SourceLines[0] != expected {
		t.Errorf("Expected source line %q, got %q", expected, result.ForkInfo.SourceLines[0])
	}
}
