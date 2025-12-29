package vm

import (
	"barn/db"
	"barn/task"
	"barn/types"
	"testing"
)

// TestCallStackPreservedOnError verifies that when verbs call other verbs
// and an error occurs deep in the call chain, all frames are preserved
// for the traceback (not just the top frame).
func TestCallStackPreservedOnError(t *testing.T) {
	store := db.NewStore()
	evaluator := NewEvaluatorWithStore(store)

	// Create test objects
	obj1 := &db.Object{
		ID:         1,
		Parents:    []types.ObjID{},
		Properties: make(map[string]*db.Property),
		Verbs:      make(map[string]*db.Verb),
		VerbList:   []*db.Verb{},
		Flags:      0,
	}
	store.Add(obj1)

	// Create verb A that calls B
	verbA := &db.Verb{
		Name:  "test_a",
		Names: []string{"test_a"},
		Owner: 1,
		Perms: db.VerbExecute | db.VerbRead,
		ArgSpec: db.VerbArgs{
			This: "any",
			Prep: "any",
			That: "any",
		},
	}
	// Compile: return this:test_b();
	verbACode := []string{"return this:test_b();"}
	verbAProgram, errors := db.CompileVerb(verbACode)
	if len(errors) > 0 {
		t.Fatalf("Failed to compile verb A: %v", errors)
	}
	verbA.Code = verbACode
	verbA.Program = verbAProgram
	obj1.Verbs["test_a"] = verbA
	obj1.VerbList = append(obj1.VerbList, verbA)

	// Create verb B that calls C
	verbB := &db.Verb{
		Name:  "test_b",
		Names: []string{"test_b"},
		Owner: 1,
		Perms: db.VerbExecute | db.VerbRead,
		ArgSpec: db.VerbArgs{
			This: "any",
			Prep: "any",
			That: "any",
		},
	}
	// Compile: return this:test_c();
	verbBCode := []string{"return this:test_c();"}
	verbBProgram, errors := db.CompileVerb(verbBCode)
	if len(errors) > 0 {
		t.Fatalf("Failed to compile verb B: %v", errors)
	}
	verbB.Code = verbBCode
	verbB.Program = verbBProgram
	obj1.Verbs["test_b"] = verbB
	obj1.VerbList = append(obj1.VerbList, verbB)

	// Create verb C that causes an error (index out of range)
	verbC := &db.Verb{
		Name:  "test_c",
		Names: []string{"test_c"},
		Owner: 1,
		Perms: db.VerbExecute | db.VerbRead,
		ArgSpec: db.VerbArgs{
			This: "any",
			Prep: "any",
			That: "any",
		},
	}
	// Compile: return args[999];
	verbCCode := []string{"return args[999];"}
	verbCProgram, errors := db.CompileVerb(verbCCode)
	if len(errors) > 0 {
		t.Fatalf("Failed to compile verb C: %v", errors)
	}
	verbC.Code = verbCCode
	verbC.Program = verbCProgram
	obj1.Verbs["test_c"] = verbC
	obj1.VerbList = append(obj1.VerbList, verbC)

	// Create a task for call stack tracking
	testTask := task.NewTask(1, 1, 100000, 5.0)

	// Set up execution context
	ctx := types.NewTaskContext()
	ctx.Player = 1
	ctx.Programmer = 1
	ctx.ThisObj = 1
	ctx.Task = testTask

	// Call verb A (which will call B, which will call C, which will error)
	result := evaluator.CallVerb(1, "test_a", []types.Value{}, ctx)

	// Verify we got an error
	if result.Flow != types.FlowException {
		t.Fatalf("Expected FlowException, got %v", result.Flow)
	}

	// The critical test: verify that all 3 frames are preserved
	stack := testTask.GetCallStack()
	if len(stack) != 3 {
		t.Errorf("Expected 3 frames in call stack, got %d", len(stack))
		t.Logf("Call stack:")
		for i, frame := range stack {
			t.Logf("  Frame %d: #%d:%s", i, frame.VerbLoc, frame.Verb)
		}
		t.Fatalf("Call stack should contain: test_a -> test_b -> test_c")
	}

	// Verify the frames are in the correct order (bottom to top)
	if stack[0].Verb != "test_a" {
		t.Errorf("Frame 0 should be test_a, got %s", stack[0].Verb)
	}
	if stack[1].Verb != "test_b" {
		t.Errorf("Frame 1 should be test_b, got %s", stack[1].Verb)
	}
	if stack[2].Verb != "test_c" {
		t.Errorf("Frame 2 should be test_c, got %s", stack[2].Verb)
	}

	// Verify all frames have the correct object
	for i, frame := range stack {
		if frame.This != 1 {
			t.Errorf("Frame %d: expected This=#1, got #%d", i, frame.This)
		}
		if frame.VerbLoc != 1 {
			t.Errorf("Frame %d: expected VerbLoc=#1, got #%d", i, frame.VerbLoc)
		}
	}
}
