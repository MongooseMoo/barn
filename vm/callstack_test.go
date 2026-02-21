package vm

import (
	"barn/builtins"
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

// TestLineNumbersInElseIf verifies that line numbers are updated correctly
// when executing elseif branches. The line number should reflect the current
// elseif being evaluated, not the original if statement line.
func TestLineNumbersInElseIf(t *testing.T) {
	store := db.NewStore()
	evaluator := NewEvaluatorWithStore(store)

	// Create test object
	obj1 := &db.Object{
		ID:         1,
		Parents:    []types.ObjID{},
		Properties: make(map[string]*db.Property),
		Verbs:      make(map[string]*db.Verb),
		VerbList:   []*db.Verb{},
		Flags:      0,
	}
	store.Add(obj1)

	// Create a verb with if/elseif that has an error in elseif condition
	// Line 1: if (0)
	// Line 2:   return 1;
	// Line 3: elseif (1/0)     <- error here, should report line 3
	// Line 4:   return 2;
	// Line 5: endif
	verb := &db.Verb{
		Name:  "test_elseif_line",
		Names: []string{"test_elseif_line"},
		Owner: 1,
		Perms: db.VerbExecute | db.VerbRead,
		ArgSpec: db.VerbArgs{
			This: "any",
			Prep: "any",
			That: "any",
		},
	}
	verbCode := []string{
		"if (0)",
		"  return 1;",
		"elseif (1/0)",
		"  return 2;",
		"endif",
	}
	verbProgram, errors := db.CompileVerb(verbCode)
	if len(errors) > 0 {
		t.Fatalf("Failed to compile verb: %v", errors)
	}
	verb.Code = verbCode
	verb.Program = verbProgram
	obj1.Verbs["test_elseif_line"] = verb
	obj1.VerbList = append(obj1.VerbList, verb)

	// Create a task for call stack tracking
	testTask := task.NewTask(1, 1, 100000, 5.0)

	// Set up execution context
	ctx := types.NewTaskContext()
	ctx.Player = 1
	ctx.Programmer = 1
	ctx.ThisObj = 1
	ctx.Task = testTask

	// Call the verb - should error with E_DIV at line 3
	result := evaluator.CallVerb(1, "test_elseif_line", []types.Value{}, ctx)

	// Verify we got a division by zero error
	if result.Flow != types.FlowException {
		t.Fatalf("Expected FlowException, got %v", result.Flow)
	}
	if result.Error != types.E_DIV {
		t.Errorf("Expected E_DIV error, got %v", result.Error)
	}

	// The critical test: verify the line number is 3 (the elseif line), not 1 (the if line)
	stack := testTask.GetCallStack()
	if len(stack) != 1 {
		t.Fatalf("Expected 1 frame in call stack, got %d", len(stack))
	}

	frame := stack[0]
	if frame.LineNumber != 3 {
		t.Errorf("Expected line number 3 (elseif line), got %d", frame.LineNumber)
	}
}

// TestLineNumbersInWhileLoop verifies that line numbers are reset to the while
// line at the start of each iteration, so errors in the condition are correctly
// attributed to the while line.
func TestLineNumbersInWhileLoop(t *testing.T) {
	store := db.NewStore()
	evaluator := NewEvaluatorWithStore(store)

	// Create test object
	obj1 := &db.Object{
		ID:         1,
		Parents:    []types.ObjID{},
		Properties: make(map[string]*db.Property),
		Verbs:      make(map[string]*db.Verb),
		VerbList:   []*db.Verb{},
		Flags:      0,
	}
	store.Add(obj1)

	// Create a verb where while condition fails on 2nd iteration
	// Line 1: x = 0;
	// Line 2: y = 1;
	// Line 3: while ((y / x) > 0)   <- error on 2nd iteration when x=0
	// Line 4:   x = x - 1;
	// Line 5: endwhile
	verb := &db.Verb{
		Name:  "test_while_line",
		Names: []string{"test_while_line"},
		Owner: 1,
		Perms: db.VerbExecute | db.VerbRead,
		ArgSpec: db.VerbArgs{
			This: "any",
			Prep: "any",
			That: "any",
		},
	}
	verbCode := []string{
		"x = 1;",
		"y = 1;",
		"while ((y / x) > 0)",
		"  x = x - 1;",
		"endwhile",
	}
	verbProgram, errors := db.CompileVerb(verbCode)
	if len(errors) > 0 {
		t.Fatalf("Failed to compile verb: %v", errors)
	}
	verb.Code = verbCode
	verb.Program = verbProgram
	obj1.Verbs["test_while_line"] = verb
	obj1.VerbList = append(obj1.VerbList, verb)

	// Create a task for call stack tracking
	testTask := task.NewTask(1, 1, 100000, 5.0)

	// Set up execution context
	ctx := types.NewTaskContext()
	ctx.Player = 1
	ctx.Programmer = 1
	ctx.ThisObj = 1
	ctx.Task = testTask

	// Call the verb - should error with E_DIV at line 3
	result := evaluator.CallVerb(1, "test_while_line", []types.Value{}, ctx)

	// Verify we got a division by zero error
	if result.Flow != types.FlowException {
		t.Fatalf("Expected FlowException, got %v", result.Flow)
	}
	if result.Error != types.E_DIV {
		t.Errorf("Expected E_DIV error, got %v", result.Error)
	}

	// The critical test: verify the line number is 3 (the while line), not 4 (last body line)
	stack := testTask.GetCallStack()
	if len(stack) != 1 {
		t.Fatalf("Expected 1 frame in call stack, got %d", len(stack))
	}

	frame := stack[0]
	if frame.LineNumber != 3 {
		t.Errorf("Expected line number 3 (while line), got %d", frame.LineNumber)
	}
}

// TestBuiltinExceptionTracebackLines verifies that nested bytecode verb calls
// preserve full call stacks and line numbers when an exception originates from
// a builtin (raise()).
func TestBuiltinExceptionTracebackLines(t *testing.T) {
	store := db.NewStore()
	reg := builtins.NewRegistry()

	obj := &db.Object{
		ID:         1,
		Parents:    []types.ObjID{},
		Properties: map[string]*db.Property{},
		Verbs:      map[string]*db.Verb{},
		VerbList:   []*db.Verb{},
	}
	store.Add(obj)

	addVerb := func(name string, code []string) {
		prog, errs := db.CompileVerb(code)
		if len(errs) > 0 {
			t.Fatalf("compile %s failed: %v", name, errs)
		}
		v := &db.Verb{
			Name:  name,
			Names: []string{name},
			Owner: 1,
			Perms: db.VerbExecute | db.VerbRead,
			ArgSpec: db.VerbArgs{
				This: "any",
				Prep: "any",
				That: "any",
			},
			Code:    code,
			Program: prog,
		}
		obj.Verbs[name] = v
		obj.VerbList = append(obj.VerbList, v)
	}

	addVerb("a", []string{
		"x = 1;",
		"return this:b();",
	})
	addVerb("b", []string{
		"y = 1;",
		"return this:c();",
	})
	addVerb("c", []string{
		"z = 1;",
		"raise(E_INVARG);",
		"return 0;",
	})

	topVerb := obj.Verbs["a"]
	topProg, err := CompileVerbBytecode(topVerb, reg)
	if err != nil {
		t.Fatalf("bytecode compile failed: %v", err)
	}

	testTask := task.NewTask(1, 1, 100000, 5.0)
	ctx := types.NewTaskContext()
	ctx.Player = 1
	ctx.Programmer = 1
	ctx.ThisObj = 1
	ctx.Task = testTask

	machine := NewVM(store, reg)
	machine.Context = ctx
	result := machine.RunWithVerbContext(topProg, 1, 1, 1, "a", 1, nil)
	if result.Flow != types.FlowException {
		t.Fatalf("expected FlowException, got %v", result.Flow)
	}
	if result.Error != types.E_INVARG {
		t.Fatalf("expected E_INVARG, got %v", result.Error)
	}

	stack, ok := result.CallStack.([]task.ActivationFrame)
	if !ok {
		t.Fatalf("expected []ActivationFrame callstack, got %T", result.CallStack)
	}
	if len(stack) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(stack))
	}

	if stack[0].Verb != "a" || stack[0].LineNumber != 2 {
		t.Fatalf("frame 0 mismatch: verb=%s line=%d (want a line 2)", stack[0].Verb, stack[0].LineNumber)
	}
	if stack[1].Verb != "b" || stack[1].LineNumber != 2 {
		t.Fatalf("frame 1 mismatch: verb=%s line=%d (want b line 2)", stack[1].Verb, stack[1].LineNumber)
	}
	if stack[2].Verb != "c" || stack[2].LineNumber != 2 {
		t.Fatalf("frame 2 mismatch: verb=%s line=%d (want c line 2)", stack[2].Verb, stack[2].LineNumber)
	}
}
