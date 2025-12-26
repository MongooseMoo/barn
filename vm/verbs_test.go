package vm

import (
	"barn/db"
	"barn/parser"
	"barn/types"
	"testing"
)

// Helper to evaluate expression for verb tests
func evalVerbExpr(t *testing.T, input string, eval *Evaluator, ctx *types.TaskContext) types.Result {
	p := parser.NewParser(input)
	expr, err := p.ParseExpression(0)
	if err != nil {
		t.Fatalf("Parse error for '%s': %v", input, err)
	}
	return eval.Eval(expr, ctx)
}

// TestVerbBuiltins tests basic verb management functions
func TestVerbBuiltins(t *testing.T) {
	store := db.NewStore()
	eval := NewEvaluatorWithStore(store)
	ctx := types.NewTaskContext()

	// Create an object directly with a verb
	obj := db.NewObject(0, 0)
	obj.Verbs = make(map[string]*db.Verb)
	obj.Verbs["test"] = &db.Verb{
		Name:  "test",
		Names: []string{"test"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		ArgSpec: db.VerbArgs{
			This: "this",
			Prep: "none",
			That: "none",
		},
		Code:    []string{"return 42;"},
		Program: nil,
	}
	store.Add(obj)
	objVal := types.NewObj(obj.ID)

	// Test verbs() - should return list with "test"
	result := evalVerbExpr(t, "verbs("+objVal.String()+")", eval, ctx)
	if result.IsError() {
		t.Fatalf("verbs() failed: %v", result.Error)
	}
	list := result.Val.(types.ListValue)
	if list.Len() != 1 {
		t.Fatalf("Expected 1 verb, got %d", list.Len())
	}
	// MOO uses 1-based indexing
	elem := list.Get(1)
	if elem == nil {
		t.Fatalf("Got nil element from verb list")
	}
	verbNameVal, ok := elem.(types.StrValue)
	if !ok {
		t.Fatalf("Expected StrValue, got %T", elem)
	}
	// Note: StrValue.String() includes quotes, use Value() for raw string
	verbName := verbNameVal.Value()
	if verbName != "test" {
		t.Errorf("Expected verb name 'test', got '%s'", verbName)
	}

	// Test verb_info()
	// Debug: check if verb is actually in the object
	objFromStore := store.Get(obj.ID)
	t.Logf("Object verbs map: %#v", objFromStore.Verbs)
	result = evalVerbExpr(t, "verb_info("+objVal.String()+", \"test\")", eval, ctx)
	if result.IsError() {
		t.Fatalf("verb_info() failed: %v", result.Error)
	}
	info := result.Val.(types.ListValue)
	if info.Len() != 3 {
		t.Errorf("Expected 3-element info list, got %d", info.Len())
	}

	// Test verb_args()
	result = evalVerbExpr(t, "verb_args("+objVal.String()+", \"test\")", eval, ctx)
	if result.IsError() {
		t.Fatalf("verb_args() failed: %v", result.Error)
	}
	args := result.Val.(types.ListValue)
	if args.Len() != 3 {
		t.Errorf("Expected 3-element args list, got %d", args.Len())
	}

	// Test verb_code()
	result = evalVerbExpr(t, "verb_code("+objVal.String()+", \"test\")", eval, ctx)
	if result.IsError() {
		t.Fatalf("verb_code() failed: %v", result.Error)
	}
	code := result.Val.(types.ListValue)
	if code.Len() != 1 {
		t.Errorf("Expected 1 line of code, got %d", code.Len())
	}
	// MOO uses 1-based indexing
	codeLine := code.Get(1).(types.StrValue).Value()
	if codeLine != "return 42;" {
		t.Errorf("Expected 'return 42;', got '%s'", codeLine)
	}
}

// TestVerbNotFound tests E_VERBNF error conditions
func TestVerbNotFound(t *testing.T) {
	store := db.NewStore()
	eval := NewEvaluatorWithStore(store)
	ctx := types.NewTaskContext()

	// Create an object directly
	obj := db.NewObject(0, 0)
	store.Add(obj)
	objVal := types.NewObj(obj.ID)

	// Test verb_info() on non-existent verb
	result := evalVerbExpr(t, "verb_info("+objVal.String()+", \"nonexistent\")", eval, ctx)
	if !result.IsError() {
		t.Errorf("Expected E_VERBNF error")
	}
	if result.Error != types.E_VERBNF {
		t.Errorf("Expected E_VERBNF, got %v", result.Error)
	}

	// Test verb_args() on non-existent verb
	result = evalVerbExpr(t, "verb_args("+objVal.String()+", \"nonexistent\")", eval, ctx)
	if !result.IsError() {
		t.Errorf("Expected E_VERBNF error")
	}
	if result.Error != types.E_VERBNF {
		t.Errorf("Expected E_VERBNF, got %v", result.Error)
	}

	// Test verb_code() on non-existent verb
	result = evalVerbExpr(t, "verb_code("+objVal.String()+", \"nonexistent\")", eval, ctx)
	if !result.IsError() {
		t.Errorf("Expected E_VERBNF error")
	}
	if result.Error != types.E_VERBNF {
		t.Errorf("Expected E_VERBNF, got %v", result.Error)
	}
}
