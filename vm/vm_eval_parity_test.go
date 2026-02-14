package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/parser"
	"barn/types"
	"fmt"
	"testing"
)

// newTestRegistry creates a builtins registry suitable for parity tests.
// Both the VM and tree-walker paths should use the same registry so builtins
// resolve identically.
func newTestRegistry() *builtins.Registry {
	store := db.NewStore()
	registry := builtins.NewRegistry()
	registry.RegisterObjectBuiltins(store)
	registry.RegisterPropertyBuiltins(store)
	registry.RegisterVerbBuiltins(store)
	registry.RegisterCryptoBuiltins(store)
	registry.RegisterSystemBuiltins(store)
	return registry
}

// vmEvalExpr compiles and runs an expression through the bytecode VM.
// Returns (value, error). The value is nil if execution fails.
func vmEvalExpr(t *testing.T, input string) (types.Value, error) {
	t.Helper()
	p := parser.NewParser(input)
	expr, err := p.ParseExpression(0)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Wrap expression in a return statement so the VM yields the value
	retStmt := &parser.ReturnStmt{Value: expr}

	registry := newTestRegistry()
	c := NewCompilerWithRegistry(registry)
	prog, err := c.Compile(retStmt)
	if err != nil {
		return nil, fmt.Errorf("compile error: %w", err)
	}

	vm := NewVM(nil, registry)
	vm.Context = types.NewTaskContext()
	return vm.Run(prog)
}

// treeEvalExpr evaluates an expression through the tree-walking Evaluator.
func treeEvalExpr(t *testing.T, input string) types.Result {
	t.Helper()
	p := parser.NewParser(input)
	expr, err := p.ParseExpression(0)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	evaluator := NewEvaluator()
	ctx := types.NewTaskContext()
	return evaluator.Eval(expr, ctx)
}

// comparePaths runs an expression through both the tree-walker and the bytecode
// VM and asserts they produce the same result.
func comparePaths(t *testing.T, input string) {
	t.Helper()

	treeResult := treeEvalExpr(t, input)
	vmVal, vmErr := vmEvalExpr(t, input)

	if treeResult.IsError() {
		// Tree-walker returned an error — VM should also error
		if vmErr == nil {
			t.Errorf("tree-walker returned error %v, but VM succeeded with %v", treeResult.Error, vmVal)
			return
		}
		// Both errored — that's parity (we could check error codes match, but
		// the VM uses Go errors while the evaluator uses ErrorCode, so for now
		// just confirming both error is sufficient)
		return
	}

	// Tree-walker succeeded
	if vmErr != nil {
		t.Errorf("tree-walker returned %v, but VM errored: %v", treeResult.Val, vmErr)
		return
	}

	// Both succeeded — compare values
	if !valuesEqual(treeResult.Val, vmVal) {
		t.Errorf("MISMATCH: tree-walker=%v (%T), VM=%v (%T)",
			treeResult.Val, treeResult.Val, vmVal, vmVal)
	}
}

// valuesEqual compares two MOO values, treating IntValue(1) and BoolValue(true)
// as NOT equal (they are different types — this is intentional to surface bugs).
func valuesEqual(a, b types.Value) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Strict type + value check
	if a.Type() != b.Type() {
		return false
	}
	return a.Equal(b)
}

// --- Parity Tests ---

func TestParity_IntLiterals(t *testing.T) {
	cases := []string{
		"0", "1", "42", "-1", "-10", "143", "144", "1000",
		"9223372036854775807", // max int64
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_FloatLiterals(t *testing.T) {
	cases := []string{"0.0", "1.5", "3.14", "-2.5"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_StringLiterals(t *testing.T) {
	cases := []string{`""`, `"hello"`, `"hello world"`, `"tab\there"`}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_Arithmetic(t *testing.T) {
	cases := []string{
		// Basic ops
		"1 + 2", "10 - 3", "4 * 5", "20 / 4", "17 % 5", "2 ^ 3",
		// Negation
		"-5", "-0", "--5",
		// Precedence
		"1 + 2 * 3", "(1 + 2) * 3", "2 ^ 3 + 1",
		// Float arithmetic
		"1.5 + 2.5", "3.0 - 1.5", "2.0 * 3.0", "7.0 / 2.0",
		// Mixed int/float
		"1 + 2.0", "3.0 - 1", "2 * 3.0", "7 / 2.0",
		// String concatenation
		`"hello" + " " + "world"`,
		// Negative modulo (floored division semantics)
		"-7 % 3", "7 % -3",
		// Power edge cases
		"2 ^ 0", "0 ^ 0", "2 ^ -1",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_ArithmeticErrors(t *testing.T) {
	cases := []string{
		"1 / 0",        // E_DIV
		"1 % 0",        // E_DIV
		`"a" + 1`,      // E_TYPE
		`"a" - "b"`,    // E_TYPE
		`"a" * 2`,      // E_TYPE
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_Comparison(t *testing.T) {
	cases := []string{
		"1 == 1", "1 == 2",
		"1 != 2", "1 != 1",
		"1 < 2", "2 < 1", "1 < 1",
		"1 <= 1", "1 <= 2", "2 <= 1",
		"2 > 1", "1 > 2", "1 > 1",
		"1 >= 1", "2 >= 1", "1 >= 2",
		// Float comparison
		"1.0 == 1.0", "1.0 < 2.0",
		// Mixed int/float
		"1 == 1.0", "1 < 2.0",
		// String comparison
		`"abc" == "abc"`, `"abc" < "def"`,
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_Logical(t *testing.T) {
	cases := []string{
		"1 && 2", "0 && 2",
		"1 || 2", "0 || 2",
		"!0", "!1", "!42",
		"1 && 2 && 3", "0 || 0 || 5",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_Bitwise(t *testing.T) {
	cases := []string{
		"5 & 3",   // AND → 1
		"5 | 3",   // OR → 7
		"5 ^ 3",   // XOR → 6 -- NOTE: ^ is power in MOO, not XOR
		"~0",      // NOT → -1
		"1 << 4",  // left shift → 16
		"16 >> 4", // right shift → 1
		// Edge cases
		"~-1",       // NOT of -1 → 0
		"-1 >> 1",   // logical right shift of -1
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_Ternary(t *testing.T) {
	cases := []string{
		"1 ? 100 | 200",
		"0 ? 100 | 200",
		"1 ? 2 ? 10 | 20 | 30",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_ListConstruction(t *testing.T) {
	cases := []string{
		"{}",
		"{1, 2, 3}",
		`{1, "two", 3.0}`,
		"{{1, 2}, {3, 4}}",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_MapConstruction(t *testing.T) {
	cases := []string{
		"[]",
		`["a" -> 1, "b" -> 2]`,
		"[1 -> 100, 2 -> 200]",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_In(t *testing.T) {
	cases := []string{
		"2 in {1, 2, 3}",    // found at index 2
		"5 in {1, 2, 3}",    // not found → 0
		`"ll" in "hello"`,   // substring → 1
		`"xyz" in "hello"`,  // not found → 0
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_DivisionEdgeCases(t *testing.T) {
	cases := []string{
		"0 / 1",           // zero divided by something
		"1.0 / 0.0",       // float division by zero
		"0.0 / 0.0",       // zero/zero float
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

// --- Statement-level parity ---

// vmEvalProgram compiles and runs a MOO program (multiple statements) through
// the bytecode VM. The program must use `return expr;` to produce a value.
func vmEvalProgram(t *testing.T, code string) (types.Value, error) {
	t.Helper()
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	registry := newTestRegistry()
	c := NewCompilerWithRegistry(registry)
	// Compile statements one by one into the same program
	c.beginScope()
	for _, stmt := range stmts {
		if err := c.compileNode(stmt); err != nil {
			return nil, fmt.Errorf("compile error: %w", err)
		}
	}
	c.emit(OP_RETURN_NONE)
	c.endScope()

	vm := NewVM(nil, registry)
	vm.Context = types.NewTaskContext()
	return vm.Run(c.program)
}

// treeEvalProgram evaluates a MOO program through the tree-walking Evaluator.
func treeEvalProgram(t *testing.T, code string) types.Result {
	t.Helper()
	evaluator := NewEvaluator()
	ctx := types.NewTaskContext()
	return evaluator.EvalString(code, ctx)
}

// comparePrograms runs a MOO program through both paths and compares results.
func comparePrograms(t *testing.T, code string) {
	t.Helper()

	treeResult := treeEvalProgram(t, code)
	vmVal, vmErr := vmEvalProgram(t, code)

	if treeResult.IsError() {
		if vmErr == nil {
			t.Errorf("tree-walker returned error %v, but VM succeeded with %v", treeResult.Error, vmVal)
		}
		return
	}

	if vmErr != nil {
		t.Errorf("tree-walker returned %v, but VM errored: %v", treeResult.Val, vmErr)
		return
	}

	if !valuesEqual(treeResult.Val, vmVal) {
		t.Errorf("MISMATCH: tree-walker=%v (%T), VM=%v (%T)",
			treeResult.Val, treeResult.Val, vmVal, vmVal)
	}
}

func TestParity_ReturnStatements(t *testing.T) {
	cases := map[string]string{
		"return_int":    "return 42;",
		"return_float":  "return 3.14;",
		"return_string": `return "hello";`,
		"return_expr":   "return 1 + 2 * 3;",
		"return_list":   "return {1, 2, 3};",
		"return_nested": "return {{1}, {2, 3}};",
		"return_none":   "return;",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_Variables(t *testing.T) {
	cases := map[string]string{
		"assign_return":     "x = 42; return x;",
		"assign_expr":       "x = 1 + 2; return x;",
		"multi_assign":      "x = 10; y = 20; return x + y;",
		"reassign":          "x = 1; x = x + 1; return x;",
		"assign_is_expr":    "return x = 42;",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_IfStatements(t *testing.T) {
	cases := map[string]string{
		"if_true":         "if (1) return 10; endif return 20;",
		"if_false":        "if (0) return 10; endif return 20;",
		"if_else_true":    "if (1) return 10; else return 20; endif",
		"if_else_false":   "if (0) return 10; else return 20; endif",
		"if_elseif":       "if (0) return 1; elseif (1) return 2; else return 3; endif",
		"if_elseif_chain": "if (0) return 1; elseif (0) return 2; elseif (1) return 3; else return 4; endif",
		"if_no_return":    "x = 0; if (1) x = 42; endif return x;",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_WhileLoops(t *testing.T) {
	cases := map[string]string{
		"count_to_5":    "x = 0; while (x < 5) x = x + 1; endwhile return x;",
		"sum_1_to_10":   "s = 0; i = 1; while (i <= 10) s = s + i; i = i + 1; endwhile return s;",
		"while_false":   "x = 99; while (0) x = 0; endwhile return x;",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ComplexExpressions(t *testing.T) {
	cases := map[string]string{
		"nested_ternary":     "return 1 ? (2 ? 10 | 20) | 30;",
		"list_in_expr":       "return 2 in {1, 2, 3};",
		"comparison_chain":   "return (1 < 2) && (3 > 2);",
		"mixed_arithmetic":   "return (10 + 5) * 2 - 3;",
		"string_concat":      `return "a" + "b" + "c";`,
		"negative_power":     "return 2 ^ 10;",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ListIndexing(t *testing.T) {
	cases := []string{
		"{1, 2, 3}[1]",     // first element (1-based)
		"{1, 2, 3}[2]",     // middle
		"{1, 2, 3}[3]",     // last
		`{"a", "b", "c"}[2]`, // string elements
		"{{1, 2}, {3, 4}}[1]", // nested list
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_StringIndexing(t *testing.T) {
	cases := []string{
		`"hello"[1]`,   // first char
		`"hello"[3]`,   // middle
		`"hello"[5]`,   // last
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_ListSlicing(t *testing.T) {
	cases := []string{
		"{1, 2, 3, 4, 5}[2..4]",   // middle slice
		"{1, 2, 3}[1..3]",         // whole list
		"{1, 2, 3}[1..1]",         // single element
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_StringSlicing(t *testing.T) {
	cases := []string{
		`"hello"[2..4]`,   // "ell"
		`"hello"[1..5]`,   // whole string
		`"hello"[3..3]`,   // single char
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_NestedWhileLoops(t *testing.T) {
	cases := map[string]string{
		"nested_count": `
			s = 0;
			i = 1;
			while (i <= 3)
				j = 1;
				while (j <= 3)
					s = s + 1;
					j = j + 1;
				endwhile
				i = i + 1;
			endwhile
			return s;`,
		"multiplication_table": `
			result = 0;
			i = 1;
			while (i <= 4)
				j = 1;
				while (j <= 4)
					result = result + i * j;
					j = j + 1;
				endwhile
				i = i + 1;
			endwhile
			return result;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ComplexPrograms(t *testing.T) {
	cases := map[string]string{
		"fibonacci": `
			a = 0;
			b = 1;
			i = 0;
			while (i < 10)
				c = a + b;
				a = b;
				b = c;
				i = i + 1;
			endwhile
			return a;`,
		"factorial_5": `
			n = 5;
			result = 1;
			while (n > 0)
				result = result * n;
				n = n - 1;
			endwhile
			return result;`,
		"nested_if_while": `
			s = 0;
			i = 1;
			while (i <= 20)
				if (i % 2 == 0)
					s = s + i;
				endif
				i = i + 1;
			endwhile
			return s;`,
		"build_list": `
			return {1 + 1, 2 * 3, 10 - 4};`,
		"index_expr_result": `
			x = {10, 20, 30};
			return x[2];`,
		"computed_index": `
			x = {10, 20, 30};
			i = 1;
			return x[i + 1];`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForRangeLoops(t *testing.T) {
	cases := map[string]string{
		"sum_1_to_5": `
			s = 0;
			for x in [1..5]
				s = s + x;
			endfor
			return s;`,
		"count_down": `
			s = 0;
			for x in [1..3]
				s = s + x * 10;
			endfor
			return s;`,
		"empty_range": `
			s = 99;
			for x in [5..1]
				s = 0;
			endfor
			return s;`,
		"single_iteration": `
			for x in [42..42]
				return x;
			endfor
			return 0;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForListLoops(t *testing.T) {
	cases := map[string]string{
		"sum_list": `
			s = 0;
			for x in ({10, 20, 30})
				s = s + x;
			endfor
			return s;`,
		"concat_strings": `
			s = "";
			for x in ({"a", "b", "c"})
				s = s + x;
			endfor
			return s;`,
		"empty_list": `
			s = 99;
			for x in ({})
				s = 0;
			endfor
			return s;`,
		"nested_for": `
			s = 0;
			for x in ({1, 2, 3})
				for y in ({10, 20})
					s = s + x * y;
				endfor
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForMixedLoops(t *testing.T) {
	cases := map[string]string{
		"for_range_with_if": `
			evens = 0;
			for x in [1..10]
				if (x % 2 == 0)
					evens = evens + 1;
				endif
			endfor
			return evens;`,
		"for_in_while": `
			total = 0;
			rounds = 0;
			while (rounds < 3)
				for x in [1..3]
					total = total + x;
				endfor
				rounds = rounds + 1;
			endwhile
			return total;`,
		"for_with_list_build": `
			s = 0;
			nums = {5, 10, 15, 20};
			for x in (nums)
				s = s + x;
			endfor
			return s;`,
		"for_range_computed_bounds": `
			a = 3;
			b = 7;
			s = 0;
			for x in [a..b]
				s = s + x;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_AssignmentExpressions(t *testing.T) {
	cases := map[string]string{
		"assign_in_if": `
			if (x = 42)
				return x;
			endif
			return 0;`,
		"chain_assign": `
			a = b = c = 10;
			return a + b + c;`,
		"assign_in_while": `
			x = 5;
			while (x = x - 1)
			endwhile
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_EdgeCases(t *testing.T) {
	cases := map[string]string{
		"empty_while": `
			x = 0;
			while (x < 3)
				x = x + 1;
			endwhile
			return x;`,
		"deep_nesting": `
			s = 0;
			for x in [1..3]
				for y in [1..3]
					if (x == y)
						s = s + x;
					endif
				endfor
			endfor
			return s;`,
		"list_of_results": `
			return {1 + 1, 2 * 2, 3 ^ 2};`,
		"multiple_returns": `
			if (1)
				return 42;
			endif
			return 0;`,
		"ternary_in_assign": `
			x = 1 ? 100 | 200;
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_IndexErrors(t *testing.T) {
	cases := []string{
		"{1, 2, 3}[0]",   // out of range (MOO is 1-based)
		"{1, 2, 3}[4]",   // out of range
		`"hello"[0]`,      // out of range
		`"hello"[6]`,      // out of range
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_BreakWhile(t *testing.T) {
	cases := map[string]string{
		"break_exits_early": `
			x = 0;
			while (1)
				x = x + 1;
				if (x == 3)
					break;
				endif
			endwhile
			return x;`,
		"break_in_condition_loop": `
			x = 0;
			while (x < 100)
				x = x + 1;
				if (x == 5)
					break;
				endif
			endwhile
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ContinueWhile(t *testing.T) {
	cases := map[string]string{
		"continue_skips_body": `
			s = 0;
			x = 0;
			while (x < 10)
				x = x + 1;
				if (x % 2 == 0)
					continue;
				endif
				s = s + x;
			endwhile
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_BreakForRange(t *testing.T) {
	cases := map[string]string{
		"break_range_early": `
			s = 0;
			for x in [1..10]
				if (x == 4)
					break;
				endif
				s = s + x;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ContinueForRange(t *testing.T) {
	cases := map[string]string{
		"continue_range_skip_evens": `
			s = 0;
			for x in [1..10]
				if (x % 2 == 0)
					continue;
				endif
				s = s + x;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_BreakForList(t *testing.T) {
	cases := map[string]string{
		"break_list_early": `
			s = 0;
			for x in ({10, 20, 30, 40, 50})
				if (x == 30)
					break;
				endif
				s = s + x;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ContinueForList(t *testing.T) {
	cases := map[string]string{
		"continue_list_skip": `
			s = 0;
			for x in ({1, 2, 3, 4, 5})
				if (x == 3)
					continue;
				endif
				s = s + x;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_NestedBreak(t *testing.T) {
	cases := map[string]string{
		"inner_break_doesnt_exit_outer": `
			s = 0;
			for x in [1..3]
				for y in [1..3]
					if (y == 2)
						break;
					endif
					s = s + 1;
				endfor
				s = s + 10;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_TryExceptBasic(t *testing.T) {
	cases := map[string]string{
		"catch_E_DIV": `
			try
				x = 1 / 0;
			except (E_DIV)
				x = 99;
			endtry
			return x;`,
		"catch_E_TYPE": `
			try
				x = "hello" + 1;
			except (E_TYPE)
				x = 42;
			endtry
			return x;`,
		"no_error_skip_except": `
			try
				x = 10 + 5;
			except (E_DIV)
				x = 99;
			endtry
			return x;`,
		"catch_ANY": `
			try
				x = 1 / 0;
			except (ANY)
				x = 100;
			endtry
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_TryExceptNoMatch(t *testing.T) {
	cases := map[string]string{
		"wrong_error_code_propagates": `
			try
				x = 1 / 0;
			except (E_TYPE)
				x = 99;
			endtry
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_TryExceptMultipleClauses(t *testing.T) {
	cases := map[string]string{
		"second_clause_matches": `
			try
				x = "hello" + 1;
			except (E_DIV)
				x = 1;
			except (E_TYPE)
				x = 2;
			except (E_RANGE)
				x = 3;
			endtry
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_TryFinallyNormalPath(t *testing.T) {
	cases := map[string]string{
		"finally_runs_on_success": `
			x = 0;
			try
				x = 10;
			finally
				x = x + 1;
			endtry
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_TryFinallyErrorPath(t *testing.T) {
	cases := map[string]string{
		"finally_runs_on_error": `
			x = 0;
			try
				try
					y = 1 / 0;
				finally
					x = 99;
				endtry
			except (E_DIV)
				x = x;
			endtry
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_TryExceptFinallyCombined(t *testing.T) {
	cases := map[string]string{
		"except_and_finally": `
			x = 0;
			try
				y = 1 / 0;
			except (E_DIV)
				x = 10;
			finally
				x = x + 1;
			endtry
			return x;`,
		"finally_runs_no_error": `
			x = 0;
			try
				x = 5;
			except (E_DIV)
				x = 99;
			finally
				x = x + 1;
			endtry
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_TryExceptNested(t *testing.T) {
	cases := map[string]string{
		"nested_try_inner_catches": `
			x = 0;
			try
				try
					y = 1 / 0;
				except (E_DIV)
					x = 42;
				endtry
			except (E_DIV)
				x = 99;
			endtry
			return x;`,
		"nested_try_outer_catches": `
			x = 0;
			try
				try
					y = 1 / 0;
				except (E_TYPE)
					x = 42;
				endtry
			except (E_DIV)
				x = 99;
			endtry
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_CatchExpressions(t *testing.T) {
	cases := []string{
		// Catch with default: error matches, return default
		"`1/0 ! E_DIV => 99'",
		// Catch with default: error doesn't match, propagates
		"`1/0 ! E_TYPE => 99'",
		// Catch with default: no error, return expression value
		"`1 + 2 ! E_DIV => 99'",
		// Catch ANY with default
		"`1/0 ! ANY => -1'",
		// Catch without default: error matches, return error value
		"`1/0 ! E_DIV'",
		// Multiple error codes with default
		"`1/0 ! E_DIV, E_TYPE => 0'",
		// Catch E_TYPE with default
		"`\"hello\" + 1 ! E_TYPE => 42'",
		// No error with ANY, return expression value
		"`5 + 3 ! ANY => -1'",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_CatchExprInProgram(t *testing.T) {
	cases := map[string]string{
		"catch_in_assign": "x = `1/0 ! E_DIV => 99'; return x;",
		"catch_no_error":  "x = `10 + 5 ! E_DIV => 0'; return x;",
		"catch_in_expr":   "return `1/0 ! E_DIV => 99' + 1;",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ScatterBasic(t *testing.T) {
	cases := map[string]string{
		"scatter_first":  `{a, b} = {1, 2}; return a;`,
		"scatter_second": `{a, b} = {1, 2}; return b;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ScatterOptional(t *testing.T) {
	cases := map[string]string{
		"optional_with_value":     `{a, ?b} = {1, 2}; return b;`,
		"optional_without_value":  `{a, ?b} = {1}; return b;`,
		"optional_with_default":   `{a, ?b = 5} = {1}; return b;`,
		"optional_default_unused": `{a, ?b = 5} = {1, 2}; return b;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ScatterRest(t *testing.T) {
	cases := map[string]string{
		"rest_basic":       `{a, @rest} = {1, 2, 3}; return rest;`,
		"rest_single":      `{a, @rest} = {1, 2}; return rest;`,
		"rest_empty":       `{a, @rest} = {1}; return rest;`,
		"rest_first_elem":  `{a, @rest} = {1, 2, 3}; return a;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ScatterErrors(t *testing.T) {
	cases := map[string]string{
		"too_few_elements":  `{a, b, c} = {1, 2}; return a;`,
		"too_many_elements": `{a, b} = {1, 2, 3}; return a;`,
		"not_a_list":        `{a, b} = 42; return a;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ScatterComplex(t *testing.T) {
	cases := map[string]string{
		"three_required": `{a, b, c} = {10, 20, 30}; return a + b + c;`,
		"mixed_optional_required": `{a, ?b, ?c} = {1}; return {a, b, c};`,
		"mixed_optional_required_2": `{a, ?b, ?c} = {1, 2}; return {a, b, c};`,
		"mixed_optional_required_3": `{a, ?b, ?c} = {1, 2, 3}; return {a, b, c};`,
		"rest_with_multiple_required": `{a, b, @rest} = {1, 2, 3, 4, 5}; return rest;`,
		"rest_with_optional": `{a, ?b, @rest} = {1}; return {b, rest};`,
		"optional_default_expr": `{a, ?b = 10 + 5} = {1}; return b;`,
		"scatter_in_loop": `
			s = 0;
			for x in ({{1, 2}, {3, 4}, {5, 6}})
				{a, b} = x;
				s = s + a * b;
			endfor
			return s;`,
		"scatter_single": `{a} = {42}; return a;`,
		"rest_only": `{@rest} = {1, 2, 3}; return rest;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_BuiltinCalls(t *testing.T) {
	cases := []string{
		// length - list
		`length({1, 2, 3})`,
		// length - string
		`length("hello")`,
		// length - empty list
		`length({})`,
		// length - empty string
		`length("")`,
		// typeof - integer returns 0 (TYPE_INT)
		`typeof(1)`,
		// typeof - string returns 2 (TYPE_STR)
		`typeof("hello")`,
		// typeof - float returns 9 (TYPE_FLOAT)
		`typeof(1.5)`,
		// typeof - list returns 4 (TYPE_LIST)
		`typeof({1, 2})`,
		// tostr - integer to string
		`tostr(42)`,
		// tostr - multiple args
		`tostr("hello", " ", "world")`,
		// toint - string to int
		`toint("42")`,
		// tofloat - int to float
		`tofloat(42)`,
		// tofloat - string to float
		`tofloat("3.14")`,
		// nested builtin calls
		`length({1, 2, length({3, 4, 5})})`,
		// builtin result in arithmetic
		`length({1, 2, 3}) + 10`,
		// builtin in conditional
		`typeof(1) == 0`,
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_BuiltinCallsInPrograms(t *testing.T) {
	cases := map[string]string{
		"length_in_assign": `x = length({1, 2, 3}); return x;`,
		"tostr_in_concat":  `return tostr(1) + tostr(2);`,
		"builtin_in_loop": `
			s = 0;
			for x in ({"a", "bb", "ccc"})
				s = s + length(x);
			endfor
			return s;`,
		"builtin_in_if": `
			x = {1, 2, 3};
			if (length(x) == 3)
				return 1;
			endif
			return 0;`,
		"nested_builtins": `return tostr(typeof(42));`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_IndexAssignList(t *testing.T) {
	cases := map[string]string{
		"set_middle_element": `l = {1, 2, 3}; l[2] = 99; return l;`,
		"set_first_element":  `l = {1, 2, 3}; l[1] = 10; return l[1];`,
		"set_last_element":   `l = {1, 2, 3}; l[3] = 30; return l;`,
		"assign_returns_value": `l = {1, 2, 3}; x = (l[2] = 99); return x;`,
		"multiple_assigns":   `l = {1, 2, 3}; l[1] = 10; l[2] = 20; l[3] = 30; return l;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_IndexAssignString(t *testing.T) {
	cases := map[string]string{
		"set_first_char":     `s = "hello"; s[1] = "H"; return s;`,
		"set_last_char":      `s = "hello"; s[5] = "O"; return s;`,
		"set_middle_char":    `s = "hello"; s[3] = "L"; return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_IndexAssignErrors(t *testing.T) {
	cases := map[string]string{
		"list_out_of_range_high": `l = {1, 2, 3}; l[4] = 99; return l;`,
		"list_out_of_range_zero": `l = {1, 2, 3}; l[0] = 99; return l;`,
		"string_out_of_range":    `s = "hi"; s[3] = "x"; return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- Property access parity tests ---
// Property tests need a shared store with test objects so both paths
// (tree-walker and bytecode VM) resolve properties identically.

// newPropertyTestStore creates a store with test objects for property parity tests.
// Object #0: name="Root", properties: {foo: 42, bar: "hello"}
// Object #1: parent=#0, name="Child", properties: {baz: 99} (inherits foo, bar from #0)
func newPropertyTestStore() *db.Store {
	store := db.NewStore()

	root := db.NewObject(0, 0)
	root.Name = "Root"
	root.Properties["foo"] = &db.Property{Name: "foo", Value: types.NewInt(42), Defined: true}
	root.Properties["bar"] = &db.Property{Name: "bar", Value: types.NewStr("hello"), Defined: true}
	store.Add(root)

	child := db.NewObject(1, 0)
	child.Name = "Child"
	child.Parents = []types.ObjID{0}
	child.Properties["baz"] = &db.Property{Name: "baz", Value: types.NewInt(99), Defined: true}
	store.Add(child)

	return store
}

// vmEvalProgramWithStore compiles and runs a MOO program through the bytecode VM,
// using a shared store for property access.
func vmEvalProgramWithStore(t *testing.T, code string, store *db.Store) (types.Value, error) {
	t.Helper()
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	registry := builtins.NewRegistry()
	registry.RegisterObjectBuiltins(store)
	registry.RegisterPropertyBuiltins(store)
	registry.RegisterVerbBuiltins(store)
	registry.RegisterCryptoBuiltins(store)
	registry.RegisterSystemBuiltins(store)

	c := NewCompilerWithRegistry(registry)
	c.beginScope()
	for _, stmt := range stmts {
		if err := c.compileNode(stmt); err != nil {
			return nil, fmt.Errorf("compile error: %w", err)
		}
	}
	c.emit(OP_RETURN_NONE)
	c.endScope()

	v := NewVM(store, registry)
	v.Context = types.NewTaskContext()
	return v.Run(c.program)
}

// treeEvalProgramWithStore evaluates a MOO program through the tree-walker,
// using a shared store for property access.
func treeEvalProgramWithStore(t *testing.T, code string, store *db.Store) types.Result {
	t.Helper()
	evaluator := NewEvaluatorWithStore(store)
	ctx := types.NewTaskContext()
	return evaluator.EvalString(code, ctx)
}

// compareProgramsWithStore runs a MOO program through both paths with a shared store.
func compareProgramsWithStore(t *testing.T, code string, store *db.Store) {
	t.Helper()

	treeResult := treeEvalProgramWithStore(t, code, store)
	vmVal, vmErr := vmEvalProgramWithStore(t, code, store)

	if treeResult.IsError() {
		if vmErr == nil {
			t.Errorf("tree-walker returned error %v, but VM succeeded with %v", treeResult.Error, vmVal)
		}
		return
	}

	if vmErr != nil {
		t.Errorf("tree-walker returned %v, but VM errored: %v", treeResult.Val, vmErr)
		return
	}

	if !valuesEqual(treeResult.Val, vmVal) {
		t.Errorf("MISMATCH: tree-walker=%v (%T), VM=%v (%T)",
			treeResult.Val, treeResult.Val, vmVal, vmVal)
	}
}

func TestParity_PropertyRead(t *testing.T) {
	store := newPropertyTestStore()
	cases := map[string]string{
		"read_defined_int":    `return #0.foo;`,
		"read_defined_str":    `return #0.bar;`,
		"read_builtin_name":   `return #0.name;`,
		"read_builtin_owner":  `return #0.owner;`,
		"read_child_own_prop": `return #1.baz;`,
		"read_inherited_prop": `return #1.foo;`,
		"read_inherited_str":  `return #1.bar;`,
		"read_child_name":     `return #1.name;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_PropertyWrite(t *testing.T) {
	cases := map[string]string{
		"write_defined_prop": `#0.foo = 100; return #0.foo;`,
		"write_builtin_name": `#0.name = "NewName"; return #0.name;`,
		"write_inherited_creates_local": `#1.foo = 999; return #1.foo;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			// Each write test gets a fresh store to avoid cross-test mutation
			store := newPropertyTestStore()
			compareProgramsWithStore(t, code, store)
		})
	}
}

func TestParity_PropertyErrors(t *testing.T) {
	store := newPropertyTestStore()
	cases := map[string]string{
		"prop_not_found":     `return #0.nonexistent;`,
		"invalid_object":     `return #99.foo;`,
		"type_error":         `return 42.foo;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

// --- Verb call parity tests ---
// Verb tests need a shared store with test objects that have verbs defined,
// so both paths (tree-walker and bytecode VM) can find and execute them.

// newVerbTestStore creates a store with test objects that have verbs.
// Object #0: name="Root", verbs:
//
//	test_return: "return 42;"
//	test_args:   "return args;"
//	test_add:    "return args[1] + args[2];"
//	test_builtin: "return length(args);"
func newVerbTestStore() *db.Store {
	store := db.NewStore()

	root := db.NewObject(0, 0)
	root.Name = "Root"
	root.Flags = db.FlagRead | db.FlagWrite

	// Verb: test_return - returns a constant
	root.Verbs["test_return"] = &db.Verb{
		Name:  "test_return",
		Names: []string{"test_return"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return 42;"},
	}

	// Verb: test_args - returns args list
	root.Verbs["test_args"] = &db.Verb{
		Name:  "test_args",
		Names: []string{"test_args"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return args;"},
	}

	// Verb: test_add - adds first two args
	root.Verbs["test_add"] = &db.Verb{
		Name:  "test_add",
		Names: []string{"test_add"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return args[1] + args[2];"},
	}

	// Verb: test_builtin - calls length() on args
	root.Verbs["test_builtin"] = &db.Verb{
		Name:  "test_builtin",
		Names: []string{"test_builtin"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return length(args);"},
	}

	store.Add(root)

	return store
}

func TestParity_VerbCallSimple(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"call_return_constant": `return #0:test_return();`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallWithArgs(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"call_with_args":   `return #0:test_args(1, 2, 3);`,
		"call_add_two":     `return #0:test_add(10, 20);`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallWithBuiltin(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"call_verb_uses_builtin": `return #0:test_builtin(1, 2, 3);`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallErrors(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"verb_not_found":   `return #0:nonexistent();`,
		"invalid_object":   `return #99:test_return();`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

// --- For-string and for-map parity tests ---

func TestParity_ForString(t *testing.T) {
	cases := map[string]string{
		"iterate_chars": `
			s = "";
			for c in ("abc")
				s = s + c + ",";
			endfor
			return s;`,
		"empty_string": `
			s = "";
			for c in ("")
				s = s + "x";
			endfor
			return s;`,
		"single_char": `
			s = "";
			for c in ("X")
				s = s + c;
			endfor
			return s;`,
		"build_reversed": `
			s = "";
			for c in ("hello")
				s = c + s;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForStringWithIndex(t *testing.T) {
	cases := map[string]string{
		"index_and_char": `
			s = "";
			for c, i in ("ab")
				s = s + tostr(i) + c;
			endfor
			return s;`,
		"index_starts_at_1": `
			s = "";
			for c, i in ("xyz")
				s = s + tostr(i) + ",";
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForMap(t *testing.T) {
	cases := map[string]string{
		"iterate_values": `
			s = "";
			for v in (["x" -> 1])
				s = s + tostr(v);
			endfor
			return s;`,
		"empty_map": `
			s = "ok";
			for v in ([])
				s = "bad";
			endfor
			return s;`,
		"multiple_values": `
			s = 0;
			for v in (["a" -> 10, "b" -> 20])
				s = s + v;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForMapWithKey(t *testing.T) {
	cases := map[string]string{
		"key_and_value": `
			s = "";
			for v, k in (["x" -> 1])
				s = k + "=" + tostr(v);
			endfor
			return s;`,
		"multiple_pairs": `
			s = "";
			for v, k in (["a" -> 1, "b" -> 2])
				s = s + k + tostr(v);
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForStringBreakContinue(t *testing.T) {
	cases := map[string]string{
		"break_in_string": `
			s = "";
			for c in ("abcde")
				if (c == "c")
					break;
				endif
				s = s + c;
			endfor
			return s;`,
		"continue_in_string": `
			s = "";
			for c in ("abcde")
				if (c == "c")
					continue;
				endif
				s = s + c;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForMapBreakContinue(t *testing.T) {
	cases := map[string]string{
		"break_in_map": `
			s = 0;
			for v in ([1 -> 10, 2 -> 20, 3 -> 30])
				if (v == 20)
					break;
				endif
				s = s + v;
			endfor
			return s;`,
		"continue_in_map": `
			s = 0;
			for v in ([1 -> 10, 2 -> 20, 3 -> 30])
				if (v == 20)
					continue;
				endif
				s = s + v;
			endfor
			return s;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- List range expression parity tests ---

func TestParity_ListRangeExpr(t *testing.T) {
	cases := []string{
		"{1..5}",       // ascending: {1, 2, 3, 4, 5}
		"{3..1}",       // descending: {3, 2, 1}
		"{1..1}",       // single element: {1}
		"{0..0}",       // single zero: {0}
		"{-2..2}",      // negative to positive: {-2, -1, 0, 1, 2}
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_ListRangeExprProgram(t *testing.T) {
	cases := map[string]string{
		"variable_endpoint": `x = 3; return {1..x};`,
		"variable_both":     `a = 2; b = 5; return {a..b};`,
		"range_in_for":      `s = 0; for x in ({1..3}) s = s + x; endfor return s;`,
		"descending_range":  `return {5..1};`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- Splice in list construction parity tests ---

func TestParity_SpliceInList(t *testing.T) {
	cases := map[string]string{
		"splice_at_end":      `l = {1, 2}; return {@l, 3};`,
		"splice_at_start":    `l = {2, 3}; return {1, @l};`,
		"splice_in_middle":   `l = {1, 2}; return {0, @l, 3};`,
		"splice_two_lists":   `l = {1, 2}; m = {3, 4}; return {@l, @m};`,
		"splice_inline":      `return {@{1, 2}, 3};`,
		"splice_empty_list":  `l = {}; return {1, @l, 2};`,
		"splice_only":        `l = {1, 2, 3}; return {@l};`,
		"no_splice_unchanged": `return {1, 2, 3};`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- Range assignment parity tests ---

func TestParity_RangeAssignString(t *testing.T) {
	cases := map[string]string{
		"replace_prefix":       `s = "hello"; s[1..3] = "HEL"; return s;`,
		"replace_middle":       `s = "abcdef"; s[2..4] = "XY"; return s;`,
		"replace_suffix":       `s = "hello"; s[4..5] = "LO"; return s;`,
		"replace_single_char":  `s = "hello"; s[3..3] = "X"; return s;`,
		"replace_all":          `s = "abc"; s[1..3] = "XYZ"; return s;`,
		"longer_replacement":   `s = "abc"; s[2..2] = "XYZ"; return s;`,
		"shorter_replacement":  `s = "abcde"; s[2..4] = "X"; return s;`,
		"assign_returns_value": `s = "hello"; x = (s[1..3] = "HEL"); return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_RangeAssignList(t *testing.T) {
	cases := map[string]string{
		"replace_middle":       `l = {1, 2, 3, 4, 5}; l[2..4] = {20, 30, 40}; return l;`,
		"replace_start":        `l = {1, 2, 3}; l[1..2] = {10, 20}; return l;`,
		"replace_end":          `l = {1, 2, 3}; l[2..3] = {20, 30}; return l;`,
		"replace_single":       `l = {1, 2, 3}; l[2..2] = {99}; return l;`,
		"replace_all":          `l = {1, 2, 3}; l[1..3] = {10, 20, 30}; return l;`,
		"longer_replacement":   `l = {1, 2, 3}; l[2..2] = {20, 30, 40}; return l;`,
		"shorter_replacement":  `l = {1, 2, 3, 4, 5}; l[2..4] = {99}; return l;`,
		"empty_replacement":    `l = {1, 2, 3, 4, 5}; l[2..4] = {}; return l;`,
		"assign_returns_value": `l = {1, 2, 3}; x = (l[2..3] = {20, 30}); return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_RangeAssignDollar(t *testing.T) {
	cases := map[string]string{
		"string_dollar_end":   `s = "hello"; s[2..$] = "ELLO"; return s;`,
		"list_dollar_end":     `l = {1, 2, 3, 4}; l[2..$] = {20, 30, 40}; return l;`,
		"string_full_range":   `s = "hello"; s[1..$] = "WORLD"; return s;`,
		"list_full_range":     `l = {1, 2, 3}; l[1..$] = {10, 20, 30}; return l;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- Fork statement parity tests ---

func TestParity_ForkBasic(t *testing.T) {
	cases := map[string]string{
		"fork_body_not_executed": `
			x = 1;
			fork (0)
				x = 2;
			endfork
			return x;`,
		"fork_code_after_executes": `
			x = 1;
			fork (0)
			endfork
			x = x + 1;
			return x;`,
		"fork_delay_evaluated": `
			d = 5;
			fork (d)
			endfork
			return d;`,
		"fork_empty_body": `
			fork (0)
			endfork
			return 42;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForkWithVariable(t *testing.T) {
	cases := map[string]string{
		"fork_var_anonymous": `
			fork (0)
			endfork
			return 1;`,
		"fork_var_named": `
			fork id (0)
			endfork
			return 1;`,
		"fork_var_code_after": `
			x = 10;
			fork id (0)
				x = 99;
			endfork
			return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForkDelayErrors(t *testing.T) {
	cases := map[string]string{
		"fork_negative_delay": `
			fork (-1)
			endfork
			return 1;`,
		"fork_string_delay": `
			fork ("hello")
			endfork
			return 1;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_RangeAssignErrors(t *testing.T) {
	cases := map[string]string{
		"string_type_mismatch": `s = "hello"; s[1..3] = 42; return s;`,
		"list_type_mismatch":   `l = {1, 2, 3}; l[1..2] = "ab"; return l;`,
		"int_range_assign":     `x = 42; x[1..2] = 3; return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}
