package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/parser"
	"barn/task"
	"barn/types"
	"fmt"
	"strings"
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
	prog, err := c.CompileStatements(stmts)
	if err != nil {
		return nil, fmt.Errorf("compile error: %w", err)
	}

	vm := NewVM(nil, registry)
	vm.Context = types.NewTaskContext()
	return vm.Run(prog)
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

func TestParity_BreakWithExprValue(t *testing.T) {
	cases := map[string]string{
		"while_break_42": `while (1) break 42; endwhile`,
		"while_break_expr": `
			x = 10;
			while (1)
				x = x + 5;
				break x * 2;
			endwhile`,
		"while_break_string": `while (1) break "done"; endwhile`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_BreakWithoutExprValue(t *testing.T) {
	cases := map[string]string{
		"while_break_no_val": `while (1) break; endwhile`,
		"while_break_no_val_with_body": `
			x = 0;
			while (1)
				x = x + 1;
				if (x == 3)
					break;
				endif
			endwhile`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_ForLoopBreakExpr(t *testing.T) {
	cases := map[string]string{
		"for_list_break_expr": `for x in ({1, 2, 3}) break x * 10; endfor`,
		"for_range_break_expr": `for x in [1..5] break x * 100; endfor`,
		"for_list_break_conditional": `
			for x in ({10, 20, 30, 40})
				if (x == 30)
					break x + 5;
				endif
			endfor`,
		"for_range_break_conditional": `
			for x in [1..10]
				if (x == 7)
					break x * x;
				endif
			endfor`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_NestedLoopBreakExpr(t *testing.T) {
	cases := map[string]string{
		// Inner loop break value becomes inner loop's result.
		// Since loops are statements (not expressions), inner loop result
		// is only observable as the last statement's implicit return.
		// Here the inner loop is the LAST statement, so its break value
		// is the program's implicit return.
		"inner_break_expr_is_last": `
			for x in [1..3]
				for y in [1..3]
					if (y == 2)
						break 99;
					endif
				endfor
			endfor`,
		// Outer loop with break expr, inner loop runs to completion
		"outer_break_after_inner": `
			for x in [1..3]
				for y in [1..2]
				endfor
				break x * 100;
			endfor`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_BreakExprSideEffects(t *testing.T) {
	// Verify that break expr evaluates the expression (observable via side effects)
	// even though the value itself is only accessible via implicit return.
	cases := map[string]string{
		"break_expr_side_effect": `
			x = 0;
			while (1)
				x = 10;
				break x + 5;
			endwhile`,
		// Loop not at end, break value is discarded by subsequent return
		"break_value_discarded_by_return": `
			while (1)
				break 42;
			endwhile
			return 99;`,
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
	root.Properties["foo"] = &db.Property{Name: "foo", Value: types.NewInt(42), Perms: db.PropRead | db.PropWrite, Defined: true}
	root.Properties["bar"] = &db.Property{Name: "bar", Value: types.NewStr("hello"), Perms: db.PropRead | db.PropWrite, Defined: true}
	store.Add(root)

	child := db.NewObject(1, 0)
	child.Name = "Child"
	child.Parents = []types.ObjID{0}
	child.Properties["baz"] = &db.Property{Name: "baz", Value: types.NewInt(99), Perms: db.PropRead | db.PropWrite, Defined: true}
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
	prog, err := c.CompileStatements(stmts)
	if err != nil {
		return nil, fmt.Errorf("compile error: %w", err)
	}

	v := NewVM(store, registry)
	v.Context = types.NewTaskContext()
	return v.Run(prog)
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

	// Verb: test_this - returns `this` variable
	root.Verbs["test_this"] = &db.Verb{
		Name:  "test_this",
		Names: []string{"test_this"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return this;"},
	}

	// Verb: test_caller - returns `caller` variable
	root.Verbs["test_caller"] = &db.Verb{
		Name:  "test_caller",
		Names: []string{"test_caller"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return caller;"},
	}

	// Verb: test_verb - returns `verb` variable
	root.Verbs["test_verb"] = &db.Verb{
		Name:  "test_verb",
		Names: []string{"test_verb"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return verb;"},
	}

	// Verb: test_nested - calls test_return on the same object, returns result + 1
	root.Verbs["test_nested"] = &db.Verb{
		Name:  "test_nested",
		Names: []string{"test_nested"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return this:test_return() + 1;"},
	}

	// Verb: test_recursive - recursive countdown: if args[1] <= 0, return 0; else return 1 + this:test_recursive(args[1] - 1)
	root.Verbs["test_recursive"] = &db.Verb{
		Name:  "test_recursive",
		Names: []string{"test_recursive"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code: []string{
			"if (args[1] <= 0)",
			"  return 0;",
			"endif",
			"return 1 + this:test_recursive(args[1] - 1);",
		},
	}

	// Verb: test_throw - raises E_DIV by dividing by zero
	root.Verbs["test_throw"] = &db.Verb{
		Name:  "test_throw",
		Names: []string{"test_throw"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return 1 / 0;"},
	}

	// Verb: test_args_access - returns args[2] (second argument)
	root.Verbs["test_args_access"] = &db.Verb{
		Name:  "test_args_access",
		Names: []string{"test_args_access"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return args[2];"},
	}

	// Verb: test_return_string - returns a string
	root.Verbs["test_return_string"] = &db.Verb{
		Name:  "test_return_string",
		Names: []string{"test_return_string"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{`return "hello world";`},
	}

	// Verb: test_return_list - returns a list
	root.Verbs["test_return_list"] = &db.Verb{
		Name:  "test_return_list",
		Names: []string{"test_return_list"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return {1, 2, 3};"},
	}

	// Verb: test_return_float - returns a float
	root.Verbs["test_return_float"] = &db.Verb{
		Name:  "test_return_float",
		Names: []string{"test_return_float"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return 3.14;"},
	}

	// Verb: test_return_map - returns a map
	root.Verbs["test_return_map"] = &db.Verb{
		Name:  "test_return_map",
		Names: []string{"test_return_map"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{`return ["x" -> 1, "y" -> 2];`},
	}

	// Verb: test_player - returns `player` variable
	root.Verbs["test_player"] = &db.Verb{
		Name:  "test_player",
		Names: []string{"test_player"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return player;"},
	}

	// Verb: test_no_exec - verb WITHOUT execute permission (for E_PERM tests)
	root.Verbs["test_no_exec"] = &db.Verb{
		Name:  "test_no_exec",
		Names: []string{"test_no_exec"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite, // no VerbExecute
		Code:  []string{"return 1;"},
	}

	// Verb: test_chain_c - throws an error (for deep unwinding tests)
	root.Verbs["test_chain_c"] = &db.Verb{
		Name:  "test_chain_c",
		Names: []string{"test_chain_c"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return 1 / 0;"},
	}

	// Verb: test_chain_b - calls test_chain_c (no handler, for deep unwinding)
	root.Verbs["test_chain_b"] = &db.Verb{
		Name:  "test_chain_b",
		Names: []string{"test_chain_b"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return this:test_chain_c();"},
	}

	// Verb: test_finally_normal - returns normally (for finally tests)
	root.Verbs["test_finally_normal"] = &db.Verb{
		Name:  "test_finally_normal",
		Names: []string{"test_finally_normal"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return 99;"},
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

// --- Map indexing parity tests (B2) ---

func TestParity_MapIndexRead(t *testing.T) {
	cases := []string{
		`["a" -> 1, "b" -> 2]["a"]`,    // string key -> int value
		`["a" -> 1, "b" -> 2]["b"]`,    // second key
		`[1 -> "x", 2 -> "y"][1]`,      // int key -> string value
		`[1 -> "x", 2 -> "y"][2]`,      // second int key
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) { comparePaths(t, c) })
	}
}

func TestParity_MapIndexReadProgram(t *testing.T) {
	cases := map[string]string{
		"map_read_string_key": `m = ["a" -> 1, "b" -> 2]; return m["a"];`,
		"map_read_int_key":    `m = [1 -> "x", 2 -> "y"]; return m[1];`,
		"map_read_missing":    `m = ["a" -> 1, "b" -> 2]; return m["c"];`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- Map index assignment parity tests (B3) ---

func TestParity_MapIndexAssign(t *testing.T) {
	cases := map[string]string{
		"map_assign_existing": `m = ["a" -> 1]; m["a"] = 99; return m["a"];`,
		"map_assign_new_key":  `m = ["a" -> 1]; m["b"] = 2; return m;`,
		"map_assign_int_key":  `m = [1 -> 10]; m[1] = 20; return m[1];`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- Map range assignment parity tests (B4) ---
// The tree-walker DOES support map range assignment (key-position-based slicing).
// This is complex behavior, so for now we just test that both paths agree on errors
// when map range assignment is attempted with integer indices.

func TestParity_MapRangeAssign(t *testing.T) {
	cases := map[string]string{
		"map_range_assign": `m = ["a" -> 1, "b" -> 2]; m[1..2] = ["c" -> 3]; return m;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- Dollar marker in read expressions parity tests (B1) ---

func TestParity_DollarIndexRead(t *testing.T) {
	cases := map[string]string{
		"list_dollar_last":    `l = {1, 2, 3}; return l[$];`,
		"string_dollar_last":  `s = "hello"; return s[$];`,
		"list_dollar_arith":   `l = {10, 20, 30}; return l[$ - 1];`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_DollarRangeRead(t *testing.T) {
	cases := map[string]string{
		"list_range_dollar":    `l = {1, 2, 3, 4, 5}; return l[2..$];`,
		"string_range_dollar":  `s = "hello"; return s[2..$];`,
		"list_range_full":      `l = {1, 2, 3}; return l[1..$];`,
		"string_range_full":    `s = "hello"; return s[1..$];`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_CaretIndexRead(t *testing.T) {
	cases := map[string]string{
		"list_caret_first":  `l = {10, 20, 30}; return l[^];`,
		"string_caret_first": `s = "hello"; return s[^];`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_SpliceInBuiltinArgs(t *testing.T) {
	cases := map[string]string{
		"splice_all_args":    `return tostr(@{1, 2, 3});`,
		"splice_single_list": `return length(@{{1, 2, 3}});`,
		"mixed_regular_splice": `l = {1, 2}; return tostr("a", @l, "b");`,
		"splice_var":         `args = {1, 2, 3}; return tostr(@args);`,
		"splice_empty":       `return tostr(@{});`,
		"splice_nested_list": `l = {4, 5}; return tostr(1, 2, 3, @l);`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_SpliceInVerbArgs(t *testing.T) {
	// Verb calls currently delegate to tree-walker, so once the compiler
	// stops rejecting splice, these should work. Test anyway for when
	// native verb calls land.
	cases := map[string]string{
		// Use builtins wrapped in a return to test the splice-in-args path.
		// Since verb calls delegate to tree-walker, we test that the compiler
		// at least accepts and correctly builds the args list.
		"splice_in_tostr": `args = {1, 2, 3}; return tostr(@args);`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- Tick counting parity tests (E2) ---

func TestParity_TickCountingInfiniteWhile(t *testing.T) {
	// Test that infinite loops in the VM are caught by tick limit.
	// The VM uses OP_LOOP which now counts ticks via CountsTick().
	// We create a VM with a small tick limit to make the test fast.
	code := `i = 0; while (1) i = i + 1; endwhile return i;`

	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	registry := newTestRegistry()
	c := NewCompilerWithRegistry(registry)
	c.beginScope()
	for _, stmt := range stmts {
		if err := c.compileNode(stmt); err != nil {
			t.Fatalf("compile error: %v", err)
		}
	}
	c.emit(OP_RETURN_NONE)
	c.endScope()

	vm := NewVM(nil, registry)
	vm.TickLimit = 100 // Small limit for fast test
	vm.Context = types.NewTaskContext()
	_, vmErr := vm.Run(c.program)

	if vmErr == nil {
		t.Errorf("expected VM to hit tick limit on infinite while loop, but it succeeded")
	}
	if vmErr != nil && !containsAny(vmErr.Error(), "E_MAXREC", "tick limit") {
		t.Errorf("expected tick limit error, got: %v", vmErr)
	}
}

func TestParity_TickCountingInfiniteFor(t *testing.T) {
	// Verify that a for loop with many iterations is also bounded.
	// This uses OP_LOOP for the back-edge, so it should count ticks.
	code := `s = 0; for x in [1..999999] s = s + x; endfor return s;`

	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	registry := newTestRegistry()
	c := NewCompilerWithRegistry(registry)
	c.beginScope()
	for _, stmt := range stmts {
		if err := c.compileNode(stmt); err != nil {
			t.Fatalf("compile error: %v", err)
		}
	}
	c.emit(OP_RETURN_NONE)
	c.endScope()

	vm := NewVM(nil, registry)
	vm.TickLimit = 50 // Very small limit
	vm.Context = types.NewTaskContext()
	_, vmErr := vm.Run(c.program)

	if vmErr == nil {
		t.Errorf("expected VM to hit tick limit on long for loop, but it succeeded")
	}
}

// helper for tick tests
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// --- Nested index assignment parity tests (A3) ---

func TestParity_NestedIndexAssign(t *testing.T) {
	cases := map[string]string{
		"nested_2d_first": `l = {{1, 2}, {3, 4}}; l[1][2] = 99; return l;`,
		"nested_2d_second": `l = {{1, 2}, {3, 4}}; l[2][1] = 99; return l;`,
		"nested_3elem_middle": `l = {{"a", "b"}, {"c", "d"}, {"e", "f"}}; l[2][2] = "Z"; return l;`,
		"nested_3d": `l = {{{1}}}; l[1][1][1] = 42; return l;`,
		"nested_string_char": `l = {"hello", "world"}; l[1][2] = "a"; return l;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_NestedIndexAssignExprResult(t *testing.T) {
	// Verify the assignment expression returns the assigned value
	cases := map[string]string{
		"assign_returns_value": `l = {{1, 2}, {3, 4}}; x = (l[1][2] = 99); return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

func TestParity_NestedIndexAssignErrors(t *testing.T) {
	// Out of range errors should match between tree-walker and VM
	cases := map[string]string{
		"out_of_range_outer": `l = {{1, 2}}; l[3][1] = 99; return l;`,
		"out_of_range_inner": `l = {{1, 2}}; l[1][3] = 99; return l;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}

// --- CompileStatements unit tests ---

func TestCompileStatements(t *testing.T) {
	t.Run("simple_program", func(t *testing.T) {
		// Parse a simple MOO program
		p := parser.NewParser("x = 1 + 2; return x;")
		stmts, err := p.ParseProgram()
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}

		// Compile using CompileStatements
		registry := newTestRegistry()
		c := NewCompilerWithRegistry(registry)
		prog, err := c.CompileStatements(stmts)
		if err != nil {
			t.Fatalf("CompileStatements error: %v", err)
		}

		// Verify the program is not nil and has bytecode
		if prog == nil {
			t.Fatal("CompileStatements returned nil program")
		}
		if len(prog.Code) == 0 {
			t.Fatal("CompileStatements produced empty bytecode")
		}

		// Run the program in a VM
		vm := NewVM(nil, registry)
		vm.Context = types.NewTaskContext()
		result, err := vm.Run(prog)
		if err != nil {
			t.Fatalf("VM.Run error: %v", err)
		}

		// Verify result is 3
		expected := types.IntValue{Val: 3}
		if !valuesEqual(result, expected) {
			t.Errorf("expected %v, got %v", expected, result)
		}

		// Verify VarNames contains "x"
		foundX := false
		for _, name := range prog.VarNames {
			if name == "x" {
				foundX = true
				break
			}
		}
		if !foundX {
			t.Errorf("VarNames %v does not contain 'x'", prog.VarNames)
		}
	})

	t.Run("implicit_return_zero", func(t *testing.T) {
		// A program with no explicit return should return 0
		p := parser.NewParser("x = 42;")
		stmts, err := p.ParseProgram()
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}

		registry := newTestRegistry()
		c := NewCompilerWithRegistry(registry)
		prog, err := c.CompileStatements(stmts)
		if err != nil {
			t.Fatalf("CompileStatements error: %v", err)
		}

		vm := NewVM(nil, registry)
		vm.Context = types.NewTaskContext()
		result, err := vm.Run(prog)
		if err != nil {
			t.Fatalf("VM.Run error: %v", err)
		}

		expected := types.IntValue{Val: 0}
		if !valuesEqual(result, expected) {
			t.Errorf("expected %v (implicit return 0), got %v", expected, result)
		}
	})

	t.Run("multiple_variables", func(t *testing.T) {
		// Test that VarNames captures all variables
		p := parser.NewParser("a = 1; b = 2; c = a + b; return c;")
		stmts, err := p.ParseProgram()
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}

		registry := newTestRegistry()
		c := NewCompilerWithRegistry(registry)
		prog, err := c.CompileStatements(stmts)
		if err != nil {
			t.Fatalf("CompileStatements error: %v", err)
		}

		// Verify VarNames contains a, b, c
		varSet := make(map[string]bool)
		for _, name := range prog.VarNames {
			varSet[name] = true
		}
		for _, expected := range []string{"a", "b", "c"} {
			if !varSet[expected] {
				t.Errorf("VarNames %v does not contain '%s'", prog.VarNames, expected)
			}
		}

		// Verify result
		vm := NewVM(nil, registry)
		vm.Context = types.NewTaskContext()
		result, err := vm.Run(prog)
		if err != nil {
			t.Fatalf("VM.Run error: %v", err)
		}
		if !valuesEqual(result, types.IntValue{Val: 3}) {
			t.Errorf("expected 3, got %v", result)
		}
	})

	t.Run("with_loops_and_conditionals", func(t *testing.T) {
		// More complex program to stress-test CompileStatements
		code := `
			s = 0;
			for x in [1..5]
				if (x % 2 == 1)
					s = s + x;
				endif
			endfor
			return s;`
		p := parser.NewParser(code)
		stmts, err := p.ParseProgram()
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}

		registry := newTestRegistry()
		c := NewCompilerWithRegistry(registry)
		prog, err := c.CompileStatements(stmts)
		if err != nil {
			t.Fatalf("CompileStatements error: %v", err)
		}

		vm := NewVM(nil, registry)
		vm.Context = types.NewTaskContext()
		result, err := vm.Run(prog)
		if err != nil {
			t.Fatalf("VM.Run error: %v", err)
		}

		// 1 + 3 + 5 = 9
		if !valuesEqual(result, types.IntValue{Val: 9}) {
			t.Errorf("expected 9, got %v", result)
		}
	})
}

// --- CompileVerbBytecode unit tests ---

func TestCompileVerbBytecode(t *testing.T) {
	t.Run("basic_verb", func(t *testing.T) {
		verb := &db.Verb{
			Name:  "test",
			Names: []string{"test"},
			Code:  []string{"return 42;"},
		}

		registry := newTestRegistry()
		prog, err := CompileVerbBytecode(verb, registry)
		if err != nil {
			t.Fatalf("CompileVerbBytecode error: %v", err)
		}

		// Should have cached the bytecode
		if verb.BytecodeCache == nil {
			t.Error("BytecodeCache should be set after compilation")
		}

		// Run the program
		vm := NewVM(nil, registry)
		vm.Context = types.NewTaskContext()
		result, err := vm.Run(prog)
		if err != nil {
			t.Fatalf("VM.Run error: %v", err)
		}
		if !valuesEqual(result, types.IntValue{Val: 42}) {
			t.Errorf("expected 42, got %v", result)
		}
	})

	t.Run("cache_hit", func(t *testing.T) {
		verb := &db.Verb{
			Name:  "test",
			Names: []string{"test"},
			Code:  []string{"return 99;"},
		}

		registry := newTestRegistry()
		prog1, err := CompileVerbBytecode(verb, registry)
		if err != nil {
			t.Fatalf("first compile error: %v", err)
		}

		// Second call should return cached result
		prog2, err := CompileVerbBytecode(verb, registry)
		if err != nil {
			t.Fatalf("second compile error: %v", err)
		}

		if prog1 != prog2 {
			t.Error("second call should return same *Program pointer (cache hit)")
		}
	})

	t.Run("pre_parsed_verb", func(t *testing.T) {
		// Verb already has parsed AST
		verb := &db.Verb{
			Name:  "test",
			Names: []string{"test"},
			Code:  []string{"return 10 + 20;"},
		}
		// Pre-parse
		vp, errs := db.CompileVerb(verb.Code)
		if errs != nil {
			t.Fatalf("pre-parse error: %v", errs)
		}
		verb.Program = vp

		registry := newTestRegistry()
		prog, err := CompileVerbBytecode(verb, registry)
		if err != nil {
			t.Fatalf("CompileVerbBytecode error: %v", err)
		}

		vm := NewVM(nil, registry)
		vm.Context = types.NewTaskContext()
		result, err := vm.Run(prog)
		if err != nil {
			t.Fatalf("VM.Run error: %v", err)
		}
		if !valuesEqual(result, types.IntValue{Val: 30}) {
			t.Errorf("expected 30, got %v", result)
		}
	})
}

// --- Cross-frame exception unwinding tests ---

func TestCrossFrameExceptionUnwinding(t *testing.T) {
	t.Run("error_caught_by_outer_frame", func(t *testing.T) {
		// Simulate: outer frame has try/except, inner frame raises an error.
		// Outer: try { <call inner> } except (E_DIV) { return 99; }
		// Inner: 1 / 0 (raises E_DIV)
		//
		// We manually create two programs and push frames.

		registry := newTestRegistry()

		// Compile the "inner" program: raises E_DIV
		innerParser := parser.NewParser("return 1 / 0;")
		innerStmts, err := innerParser.ParseProgram()
		if err != nil {
			t.Fatalf("inner parse error: %v", err)
		}
		innerCompiler := NewCompilerWithRegistry(registry)
		innerProg, err := innerCompiler.CompileStatements(innerStmts)
		if err != nil {
			t.Fatalf("inner compile error: %v", err)
		}

		// Compile the "outer" program: try { <placeholder> } except (E_DIV) { x = 99; }
		// Since we can't easily compile a "call inner" instruction, we test HandleError
		// directly by setting up frames manually.
		outerParser := parser.NewParser(`
			try
				x = 0;
			except e (E_DIV)
				x = 99;
			endtry
			return x;`)
		outerStmts, err := outerParser.ParseProgram()
		if err != nil {
			t.Fatalf("outer parse error: %v", err)
		}
		outerCompiler := NewCompilerWithRegistry(registry)
		outerProg, err := outerCompiler.CompileStatements(outerStmts)
		if err != nil {
			t.Fatalf("outer compile error: %v", err)
		}

		// Create VM with outer frame, step through try setup
		vm := NewVM(nil, registry)
		vm.Context = types.NewTaskContext()

		// Push outer frame
		outerFrame := &StackFrame{
			Program:     outerProg,
			IP:          0,
			BasePointer: vm.SP,
			Locals:      make([]types.Value, outerProg.NumLocals),
			LoopStack:   make([]LoopState, 0),
			ExceptStack: make([]Handler, 0),
		}
		for i := range outerFrame.Locals {
			outerFrame.Locals[i] = types.IntValue{Val: 0}
		}
		vm.Frames = append(vm.Frames, outerFrame)

		// Step through the outer program until the try/except handler is registered
		// (we need to execute OP_TRY_EXCEPT to get the handler onto ExceptStack)
		for outerFrame.IP < len(outerFrame.Program.Code) {
			op := OpCode(outerFrame.Program.Code[outerFrame.IP])
			outerFrame.IP++
			if CountsTick(op) {
				vm.Ticks++
			}
			if err := vm.Execute(op); err != nil {
				t.Fatalf("unexpected error during outer setup: %v", err)
			}
			// Once we've set up the try handler, stop
			if len(outerFrame.ExceptStack) > 0 {
				break
			}
		}

		if len(outerFrame.ExceptStack) == 0 {
			t.Fatal("outer frame should have an except handler after OP_TRY_EXCEPT")
		}

		// Now push the inner frame (simulating a verb call)
		innerFrame := &StackFrame{
			Program:     innerProg,
			IP:          0,
			BasePointer: vm.SP,
			Locals:      make([]types.Value, innerProg.NumLocals),
			LoopStack:   make([]LoopState, 0),
			ExceptStack: make([]Handler, 0),
		}
		for i := range innerFrame.Locals {
			innerFrame.Locals[i] = types.IntValue{Val: 0}
		}
		vm.Frames = append(vm.Frames, innerFrame)

		// Step the inner frame — it should raise E_DIV
		var divErr error
		for innerFrame.IP < len(innerFrame.Program.Code) && len(vm.Frames) > 1 {
			frame := vm.CurrentFrame()
			if frame.IP >= len(frame.Program.Code) {
				break
			}
			op := OpCode(frame.Program.Code[frame.IP])
			frame.IP++
			if CountsTick(op) {
				vm.Ticks++
			}
			divErr = vm.Execute(op)
			if divErr != nil {
				break
			}
		}

		if divErr == nil {
			t.Fatal("inner frame should have raised E_DIV")
		}

		// HandleError should unwind the inner frame and find the outer's handler
		handled := vm.HandleError(divErr)
		if !handled {
			t.Fatal("HandleError should have found the outer frame's E_DIV handler")
		}

		// The inner frame should have been popped
		if len(vm.Frames) != 1 {
			t.Errorf("expected 1 frame after unwinding, got %d", len(vm.Frames))
		}

		// Continue executing the outer frame (the handler body)
		for len(vm.Frames) > 0 {
			if err := vm.Step(); err != nil {
				if !vm.HandleError(err) {
					t.Fatalf("unhandled error during outer execution: %v", err)
				}
			}
		}

		// The result should be on the stack: 99
		if vm.SP == 0 {
			t.Fatal("expected result on stack")
		}
		result := vm.Stack[vm.SP-1]
		if !valuesEqual(result, types.IntValue{Val: 99}) {
			t.Errorf("expected 99, got %v", result)
		}
	})

	t.Run("error_no_handler_anywhere", func(t *testing.T) {
		// Two frames, neither has a handler. HandleError should return false.
		registry := newTestRegistry()

		prog1Parser := parser.NewParser("x = 1;")
		prog1Stmts, _ := prog1Parser.ParseProgram()
		c1 := NewCompilerWithRegistry(registry)
		prog1, _ := c1.CompileStatements(prog1Stmts)

		prog2Parser := parser.NewParser("y = 2;")
		prog2Stmts, _ := prog2Parser.ParseProgram()
		c2 := NewCompilerWithRegistry(registry)
		prog2, _ := c2.CompileStatements(prog2Stmts)

		vm := NewVM(nil, registry)
		vm.Context = types.NewTaskContext()

		// Push two frames, neither with handlers
		frame1 := &StackFrame{
			Program:     prog1,
			IP:          0,
			BasePointer: 0,
			Locals:      make([]types.Value, prog1.NumLocals),
			LoopStack:   make([]LoopState, 0),
			ExceptStack: make([]Handler, 0),
		}
		frame2 := &StackFrame{
			Program:     prog2,
			IP:          0,
			BasePointer: 0,
			Locals:      make([]types.Value, prog2.NumLocals),
			LoopStack:   make([]LoopState, 0),
			ExceptStack: make([]Handler, 0),
		}
		vm.Frames = append(vm.Frames, frame1)
		vm.Frames = append(vm.Frames, frame2)

		testErr := MooError{Code: types.E_DIV}
		handled := vm.HandleError(testErr)
		if handled {
			t.Error("HandleError should return false when no frame has a handler")
		}
	})

	t.Run("single_frame_still_works", func(t *testing.T) {
		// Existing single-frame behavior should be preserved
		code := `
			try
				x = 1 / 0;
			except (E_DIV)
				x = 42;
			endtry
			return x;`
		val, err := vmEvalProgram(t, code)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !valuesEqual(val, types.IntValue{Val: 42}) {
			t.Errorf("expected 42, got %v", val)
		}
	})
}

// --- Native verb call parity tests (Task 5) ---

func TestParity_VerbCallBuiltinVars(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"this_is_obj":     `return #0:test_this();`,
		"verb_name":       `return #0:test_verb();`,
		"caller_default":  `return #0:test_caller();`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallNested(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"nested_call":     `return #0:test_nested();`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallRecursive(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"recursive_base":  `return #0:test_recursive(0);`,
		"recursive_one":   `return #0:test_recursive(1);`,
		"recursive_five":  `return #0:test_recursive(5);`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallThrows(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"verb_throws_div":  `return #0:test_throw();`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallThrowsCaught(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"caller_catches_verb_error": `
			try
				return #0:test_throw();
			except (E_DIV)
				return "caught";
			endtry`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallArgsAccess(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"args_second_element": `return #0:test_args_access(10, 20, 30);`,
		"args_full_list":      `return #0:test_args(100, 200);`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallReturnTypes(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"return_string": `return #0:test_return_string();`,
		"return_list":   `return #0:test_return_list();`,
		"return_float":  `return #0:test_return_float();`,
		"return_map":    `return #0:test_return_map();`,
		"return_int":    `return #0:test_return();`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallVerbnfCaught(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"catch_verbnf": `
			try
				return #0:nonexistent();
			except (E_VERBNF)
				return "no such verb";
			endtry`,
		"catch_invind": `
			try
				return #99:test_return();
			except (E_INVIND)
				return "bad object";
			endtry`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallPermission(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"no_exec_perm": `return #0:test_no_exec();`,
		"catch_perm": `
			try
				return #0:test_no_exec();
			except (E_PERM)
				return "no permission";
			endtry`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallPlayer(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		"player_propagated": `return #0:test_player();`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallUnhandledException(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		// B calls C, C throws, B has no handler -> error propagates out
		"unhandled_through_chain": `return #0:test_chain_b();`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallDeepUnwind(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		// A calls B, B calls C, C throws E_DIV.
		// B has no handler. A catches the error from C.
		"a_catches_c_error_through_b": `
			try
				return #0:test_chain_b();
			except (E_DIV)
				return "caught from C";
			endtry`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallCrossFrameFinally(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		// Called verb returns normally -> finally block executes
		"finally_normal_return": `
			x = 0;
			try
				x = #0:test_finally_normal();
			finally
				x = x + 1;
			endtry
			return x;`,
		// Called verb throws -> finally block executes, then error propagates.
		// Wrap in outer try/except to catch the propagated error.
		"finally_on_error": `
			x = 0;
			try
				try
					#0:test_throw();
				finally
					x = 1;
				endtry
			except (E_DIV)
				return x;
			endtry`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

func TestParity_VerbCallDeepFinallyUnwind(t *testing.T) {
	store := newVerbTestStore()
	cases := map[string]string{
		// A has try/finally, A calls B, B calls C, C throws.
		// A's finally must execute during unwinding.
		// Outer try/except catches the error so we can observe the finally side-effect.
		"finally_through_nested_verbs": `
			x = 0;
			try
				try
					#0:test_chain_b();
				finally
					x = 42;
				endtry
			except (E_DIV)
				return x;
			endtry`,
		// Same but with finally in try that also has except for a DIFFERENT error.
		// E_DIV should NOT match E_VERBNF, so it continues unwinding through finally.
		"finally_mismatched_except": `
			x = 0;
			try
				try
					#0:test_chain_b();
				except (E_VERBNF)
					x = -1;
				finally
					x = x + 100;
				endtry
			except (E_DIV)
				return x;
			endtry`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { compareProgramsWithStore(t, code, store) })
	}
}

// --- Waif property parity tests ---

// newWaifTestStore creates a store with an object suitable for waif testing.
// Object #0: name="WaifClass", properties: {data: "class_data"} (readable/writable)
// This object serves as the class for waifs created with new_waif().
func newWaifTestStore() *db.Store {
	store := db.NewStore()

	classObj := db.NewObject(0, 0)
	classObj.Name = "WaifClass"
	classObj.Properties["data"] = &db.Property{
		Name:    "data",
		Value:   types.NewStr("class_data"),
		Perms:   db.PropRead | db.PropWrite,
		Defined: true,
	}
	classObj.Properties["extra"] = &db.Property{
		Name:    "extra",
		Value:   types.NewInt(100),
		Perms:   db.PropRead | db.PropWrite,
		Defined: true,
	}
	store.Add(classObj)

	return store
}

// waifTestContext creates a task context suitable for new_waif() calls.
// Sets ThisObj=0 (the class object) and Programmer=0 (owner).
func waifTestContext() *types.TaskContext {
	ctx := types.NewTaskContext()
	ctx.ThisObj = 0
	ctx.Programmer = 0
	ctx.IsWizard = true // Wizard so property permissions don't interfere
	return ctx
}

// vmEvalProgramWithStoreAndCtx compiles and runs a MOO program through the bytecode VM,
// using a shared store and custom context.
func vmEvalProgramWithStoreAndCtx(t *testing.T, code string, store *db.Store, ctx *types.TaskContext) (types.Value, error) {
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
	prog, err := c.CompileStatements(stmts)
	if err != nil {
		return nil, fmt.Errorf("compile error: %w", err)
	}

	v := NewVM(store, registry)
	v.Context = ctx
	return v.Run(prog)
}

// treeEvalProgramWithStoreAndCtx evaluates a MOO program through the tree-walker,
// using a shared store and custom context.
func treeEvalProgramWithStoreAndCtx(t *testing.T, code string, store *db.Store, ctx *types.TaskContext) types.Result {
	t.Helper()
	evaluator := NewEvaluatorWithStore(store)
	return evaluator.EvalString(code, ctx)
}

// compareProgramsWithStoreAndCtx runs a MOO program through both paths with a shared store and context.
func compareProgramsWithStoreAndCtx(t *testing.T, code string, store *db.Store, ctx *types.TaskContext) {
	t.Helper()

	treeResult := treeEvalProgramWithStoreAndCtx(t, code, store, ctx)
	vmVal, vmErr := vmEvalProgramWithStoreAndCtx(t, code, store, ctx)

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

func TestParity_WaifPropertyRead(t *testing.T) {
	cases := map[string]string{
		// Read .owner special property
		"read_owner": `w = new_waif(); return w.owner;`,
		// Read .class special property
		"read_class": `w = new_waif(); return w.class;`,
		// Read property that falls back to class object
		"read_class_fallback": `w = new_waif(); return w.data;`,
		// Read another class fallback property
		"read_class_fallback_extra": `w = new_waif(); return w.extra;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			store := newWaifTestStore()
			ctx := waifTestContext()
			compareProgramsWithStoreAndCtx(t, code, store, ctx)
		})
	}
}

func TestParity_WaifPropertyWrite(t *testing.T) {
	cases := map[string]string{
		// Write to .owner should fail with E_PERM
		"write_owner_eperm": `w = new_waif(); w.owner = #1; return 0;`,
		// Write to .class should fail with E_PERM
		"write_class_eperm": `w = new_waif(); w.class = #1; return 0;`,
		// Write to .wizard should fail with E_PERM
		"write_wizard_eperm": `w = new_waif(); w.wizard = 1; return 0;`,
		// Write to .programmer should fail with E_PERM
		"write_programmer_eperm": `w = new_waif(); w.programmer = 1; return 0;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			store := newWaifTestStore()
			ctx := waifTestContext()
			compareProgramsWithStoreAndCtx(t, code, store, ctx)
		})
	}
}

func TestParity_PropertyPermissions(t *testing.T) {
	// Create a store with a property that has NO read permission
	store := db.NewStore()
	obj := db.NewObject(0, 0)
	obj.Name = "PermTest"
	obj.Properties["secret"] = &db.Property{
		Name:    "secret",
		Value:   types.NewStr("hidden"),
		Owner:   99, // Owned by #99, not the programmer
		Perms:   0,  // No read, no write
		Defined: true,
	}
	obj.Properties["readable"] = &db.Property{
		Name:    "readable",
		Value:   types.NewStr("visible"),
		Owner:   99,
		Perms:   db.PropRead, // Read only, no write
		Defined: true,
	}
	store.Add(obj)

	// Non-wizard, non-owner context
	ctx := types.NewTaskContext()
	ctx.Programmer = 1 // Not #99 (the owner)
	ctx.IsWizard = false

	t.Run("non_wizard_read_non_readable_eperm", func(t *testing.T) {
		// VM should return E_PERM for non-wizard reading a non-readable property
		_, vmErr := vmEvalProgramWithStoreAndCtx(t, `return #0.secret;`, store, ctx)
		if vmErr == nil {
			t.Errorf("expected E_PERM error, but VM succeeded")
		} else if !containsStr(vmErr.Error(), "E_PERM") {
			t.Errorf("expected E_PERM, got: %v", vmErr)
		}
	})

	t.Run("non_wizard_read_readable_ok", func(t *testing.T) {
		// VM should allow reading a readable property even as non-wizard non-owner
		vmVal, vmErr := vmEvalProgramWithStoreAndCtx(t, `return #0.readable;`, store, ctx)
		if vmErr != nil {
			t.Errorf("expected success, but VM errored: %v", vmErr)
		} else if vmVal == nil || vmVal.String() != `"visible"` {
			t.Errorf("expected \"visible\", got: %v", vmVal)
		}
	})

	t.Run("non_wizard_write_non_writable_eperm", func(t *testing.T) {
		// VM should return E_PERM for non-wizard writing a non-writable property
		_, vmErr := vmEvalProgramWithStoreAndCtx(t, `#0.readable = "new"; return 0;`, store, ctx)
		if vmErr == nil {
			t.Errorf("expected E_PERM error, but VM succeeded")
		} else if !containsStr(vmErr.Error(), "E_PERM") {
			t.Errorf("expected E_PERM, got: %v", vmErr)
		}
	})

	t.Run("wizard_reads_anything", func(t *testing.T) {
		// Wizard should be able to read non-readable properties
		wizCtx := types.NewTaskContext()
		wizCtx.IsWizard = true
		vmVal, vmErr := vmEvalProgramWithStoreAndCtx(t, `return #0.secret;`, store, wizCtx)
		if vmErr != nil {
			t.Errorf("expected success for wizard, but VM errored: %v", vmErr)
		} else if vmVal == nil || vmVal.String() != `"hidden"` {
			t.Errorf("expected \"hidden\", got: %v", vmVal)
		}
	})

	t.Run("owner_reads_own_property", func(t *testing.T) {
		// Property owner should be able to read their own non-readable property
		ownerCtx := types.NewTaskContext()
		ownerCtx.Programmer = 99 // Same as property owner
		ownerCtx.IsWizard = false
		vmVal, vmErr := vmEvalProgramWithStoreAndCtx(t, `return #0.secret;`, store, ownerCtx)
		if vmErr != nil {
			t.Errorf("expected success for owner, but VM errored: %v", vmErr)
		} else if vmVal == nil || vmVal.String() != `"hidden"` {
			t.Errorf("expected \"hidden\", got: %v", vmVal)
		}
	})
}

// containsStr checks if s contains substr (simple helper for error message checking)
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- Line number tracking tests ---

func TestLineInfoPopulated(t *testing.T) {
	// Multi-line program: 3 statements on separate lines
	code := "x = 1;\ny = 2;\nz = x + y;\nreturn z;"
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	registry := newTestRegistry()
	c := NewCompilerWithRegistry(registry)
	prog, err := c.CompileStatements(stmts)
	if err != nil {
		t.Fatalf("CompileStatements error: %v", err)
	}

	// LineInfo should have entries (at least one per line)
	if len(prog.LineInfo) == 0 {
		t.Fatal("LineInfo is empty; expected entries for multi-line program")
	}

	// Verify we have entries for lines 1 through 4
	linesPresent := make(map[int]bool)
	for _, entry := range prog.LineInfo {
		linesPresent[entry.Line] = true
	}
	for _, expectedLine := range []int{1, 2, 3, 4} {
		if !linesPresent[expectedLine] {
			t.Errorf("LineInfo missing entry for line %d; entries: %v", expectedLine, prog.LineInfo)
		}
	}

	// Verify entries are sorted by StartIP (ascending)
	for i := 1; i < len(prog.LineInfo); i++ {
		if prog.LineInfo[i].StartIP < prog.LineInfo[i-1].StartIP {
			t.Errorf("LineInfo not sorted by StartIP: entry[%d]=%v, entry[%d]=%v",
				i-1, prog.LineInfo[i-1], i, prog.LineInfo[i])
		}
	}

	// Verify the program still produces the correct result
	vm := NewVM(nil, registry)
	vm.Context = types.NewTaskContext()
	result, err := vm.Run(prog)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if intVal, ok := result.(types.IntValue); !ok || intVal.Val != 3 {
		t.Errorf("Expected 3, got %v", result)
	}
}

func TestErrorIncludesLineNumber(t *testing.T) {
	// Division by zero on line 3
	code := "x = 1;\ny = 0;\nz = x / y;\nreturn z;"
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	registry := newTestRegistry()
	c := NewCompilerWithRegistry(registry)
	prog, err := c.CompileStatements(stmts)
	if err != nil {
		t.Fatalf("CompileStatements error: %v", err)
	}

	vm := NewVM(nil, registry)
	vm.Context = types.NewTaskContext()
	_, err = vm.Run(prog)
	if err == nil {
		t.Fatal("Expected E_DIV error, got nil")
	}

	errMsg := err.Error()
	// Verify error mentions E_DIV
	if !strings.Contains(errMsg, "E_DIV") {
		t.Errorf("Expected error to contain E_DIV, got: %s", errMsg)
	}
	// Verify error includes line 3 (where the division happens)
	if !strings.Contains(errMsg, "line 3") {
		t.Errorf("Expected error to mention 'line 3', got: %s", errMsg)
	}
}

func TestLineForIP(t *testing.T) {
	// Test the Program.LineForIP lookup directly
	prog := &Program{
		LineInfo: []LineEntry{
			{StartIP: 0, Line: 1},
			{StartIP: 5, Line: 2},
			{StartIP: 12, Line: 3},
		},
	}

	tests := []struct {
		ip       int
		expected int
	}{
		{0, 1},  // Exactly at line 1 start
		{3, 1},  // Within line 1's range
		{5, 2},  // Exactly at line 2 start
		{10, 2}, // Within line 2's range
		{12, 3}, // Exactly at line 3 start
		{99, 3}, // Past all entries, should return last line
	}

	for _, tt := range tests {
		got := prog.LineForIP(tt.ip)
		if got != tt.expected {
			t.Errorf("LineForIP(%d) = %d, want %d", tt.ip, got, tt.expected)
		}
	}

	// Empty LineInfo should return 0
	emptyProg := &Program{}
	if got := emptyProg.LineForIP(5); got != 0 {
		t.Errorf("LineForIP on empty program = %d, want 0", got)
	}
}

// --- VerbLoc and Programmer parity tests (Phase 4 Task 1) ---

// newInheritedVerbStore creates a store with:
//   - #0 (parent): has verb "check_callers" that calls callers() and returns the result
//   - #0 also has "inner_check" that calls callers()
//   - #1 (child): inherits from #0, no verbs of its own
//
// When #1:check_callers() is called, VerbLoc should be #0 (where the verb is defined).
func newInheritedVerbStore() *db.Store {
	store := db.NewStore()

	parent := db.NewObject(0, 0)
	parent.Name = "Parent"
	parent.Flags = db.FlagRead | db.FlagWrite | db.FlagWizard

	// Verb: inner_check - calls callers() and returns the result.
	// callers() returns the stack EXCLUDING the current frame.
	// So when outer calls inner, inner's callers() shows outer's frame.
	parent.Verbs["inner_check"] = &db.Verb{
		Name:  "inner_check",
		Names: []string{"inner_check"},
		Owner: 7, // Deliberate non-player owner to test Programmer field
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return callers();"},
	}

	// Verb: outer_call - calls this:inner_check() and returns the result.
	// When called on #1, this will be #1, but VerbLoc should be #0.
	parent.Verbs["outer_call"] = &db.Verb{
		Name:  "outer_call",
		Names: []string{"outer_call"},
		Owner: 7,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return this:inner_check();"},
	}

	store.Add(parent)

	child := db.NewObject(1, 0)
	child.Name = "Child"
	child.Parents = []types.ObjID{0} // Inherits from parent #0
	child.Flags = db.FlagRead | db.FlagWrite

	store.Add(child)

	return store
}

// inheritedVerbTestCtx creates a task context with a task for callers() to work.
func inheritedVerbTestCtx() *types.TaskContext {
	ctx := types.NewTaskContext()
	ctx.Player = 2        // Some player, NOT the verb owner
	ctx.Programmer = 0    // Initial programmer (will be overridden per verb call)
	ctx.ThisObj = 0
	ctx.IsWizard = true
	ctx.Task = task.NewTask(1, 2, 100000, 5.0)
	return ctx
}

func TestParity_VerbLocInherited(t *testing.T) {
	// When #1:outer_call() is called, it calls this:inner_check() which calls callers().
	// callers() returns the stack excluding the current frame (inner_check).
	// The returned list should include outer_call's frame.
	// outer_call is DEFINED on #0, CALLED on #1.
	// VerbLoc (4th element, index [4]) should be #0, NOT #1.
	//
	// callers() format: {this, verb_name, programmer, verb_loc, player, line_number}

	store := newInheritedVerbStore()
	code := `return #1:outer_call();`

	// Tree-walker path
	treeCtx := inheritedVerbTestCtx()
	treeResult := treeEvalProgramWithStoreAndCtx(t, code, store, treeCtx)

	// VM path (fresh context to avoid call stack contamination)
	vmCtx := inheritedVerbTestCtx()
	vmVal, vmErr := vmEvalProgramWithStoreAndCtx(t, code, store, vmCtx)

	if treeResult.IsError() {
		t.Fatalf("tree-walker errored: %v", treeResult.Error)
	}
	if vmErr != nil {
		t.Fatalf("VM errored: %v", vmErr)
	}

	// Both should return a list (the callers() output)
	treeList, ok := treeResult.Val.(types.ListValue)
	if !ok {
		t.Fatalf("tree-walker did not return list, got %T: %v", treeResult.Val, treeResult.Val)
	}
	vmList, ok := vmVal.(types.ListValue)
	if !ok {
		t.Fatalf("VM did not return list, got %T: %v", vmVal, vmVal)
	}

	if treeList.Len() == 0 {
		t.Fatalf("tree-walker callers() returned empty list")
	}
	if vmList.Len() == 0 {
		t.Fatalf("VM callers() returned empty list")
	}

	// Get outer_call's frame (first element in callers output)
	treeFrame := treeList.Get(1).(types.ListValue) // 1-based indexing
	vmFrame := vmList.Get(1).(types.ListValue)

	// Element 4 is VerbLoc (1-based: {this, verb, programmer, verbloc, player, line})
	treeVerbLoc := treeFrame.Get(4)
	vmVerbLoc := vmFrame.Get(4)

	t.Logf("Tree-walker callers frame: %v", treeFrame)
	t.Logf("VM callers frame: %v", vmFrame)

	// VerbLoc should be #0 (where outer_call is defined), not #1
	expectedVerbLoc := types.NewObj(0)

	if !treeVerbLoc.Equal(expectedVerbLoc) {
		t.Errorf("tree-walker VerbLoc: got %v, want #0", treeVerbLoc)
	}
	if !vmVerbLoc.Equal(expectedVerbLoc) {
		t.Errorf("VM VerbLoc: got %v, want #0", vmVerbLoc)
	}
}

func TestParity_ProgrammerIsVerbOwner(t *testing.T) {
	// When a verb with Owner=#7 is called, the Programmer field in the activation
	// frame should be #7, not the player (#2).
	//
	// callers() format: {this, verb_name, programmer, verb_loc, player, line_number}
	// Programmer is element 3.

	store := newInheritedVerbStore()
	code := `return #0:outer_call();` // Call on #0 directly to isolate Programmer test

	// Tree-walker path
	treeCtx := inheritedVerbTestCtx()
	treeResult := treeEvalProgramWithStoreAndCtx(t, code, store, treeCtx)

	// VM path
	vmCtx := inheritedVerbTestCtx()
	vmVal, vmErr := vmEvalProgramWithStoreAndCtx(t, code, store, vmCtx)

	if treeResult.IsError() {
		t.Fatalf("tree-walker errored: %v", treeResult.Error)
	}
	if vmErr != nil {
		t.Fatalf("VM errored: %v", vmErr)
	}

	treeList, ok := treeResult.Val.(types.ListValue)
	if !ok {
		t.Fatalf("tree-walker did not return list, got %T: %v", treeResult.Val, treeResult.Val)
	}
	vmList, ok := vmVal.(types.ListValue)
	if !ok {
		t.Fatalf("VM did not return list, got %T: %v", vmVal, vmVal)
	}

	if treeList.Len() == 0 {
		t.Fatalf("tree-walker callers() returned empty list")
	}
	if vmList.Len() == 0 {
		t.Fatalf("VM callers() returned empty list")
	}

	// Get outer_call's frame
	treeFrame := treeList.Get(1).(types.ListValue)
	vmFrame := vmList.Get(1).(types.ListValue)

	// Element 3 is Programmer
	treeProgrammer := treeFrame.Get(3)
	vmProgrammer := vmFrame.Get(3)

	t.Logf("Tree-walker callers frame: %v", treeFrame)
	t.Logf("VM callers frame: %v", vmFrame)

	// Programmer should be the verb owner - but what does the tree-walker use?
	// The tree-walker sets Programmer from ctx.Programmer which was set at task start.
	// For this test, ctx.Programmer starts as #0 (set in inheritedVerbTestCtx).
	// The scheduler normally sets ctx.Programmer = verb.Owner, but in tests
	// the ctx.Programmer is set manually.
	// What we're really testing is that the bytecode VM activation frame records
	// verb.Owner, not player.
	//
	// Since the tree-walker uses ctx.Programmer for the activation frame, and the
	// bytecode VM should use verb.Owner, we need to check that the VM activation
	// frame's Programmer field matches verb.Owner (#7).
	t.Logf("Tree-walker Programmer: %v", treeProgrammer)
	t.Logf("VM Programmer: %v", vmProgrammer)

	// The verb owner is #7
	expectedProgrammer := types.NewObj(7)

	// Note: tree-walker sets Programmer from ctx.Programmer (which is #0 in our test),
	// NOT from verb.Owner. The scheduler normally sets ctx.Programmer = verb.Owner
	// before calling the tree-walker. So in unit tests, the tree-walker's Programmer
	// will be whatever ctx.Programmer was, not verb.Owner.
	//
	// The bytecode VM should set Programmer to verb.Owner in the activation frame.
	// After the fix, the VM's programmer should be #7 (verb.Owner).
	// The tree-walker's programmer will be #0 (ctx.Programmer from our test setup).
	//
	// This means for a TRUE parity test, we need to set ctx.Programmer = verb.Owner
	// to match what the scheduler would do. Let's just verify the VM gets verb.Owner right.
	if !vmProgrammer.Equal(expectedProgrammer) {
		t.Errorf("VM Programmer: got %v, want #7 (verb owner)", vmProgrammer)
	}
}

// TestParity_ThisValueRestoredAfterVerbCall verifies that ctx.ThisValue is
// saved before a verb call and restored after it returns. This is essential
// for waif/primitive verb calls where ThisValue != NewObj(ThisObj).
func TestParity_ThisValueRestoredAfterVerbCall(t *testing.T) {
	store := newVerbTestStore()

	// We'll run a program that calls a verb on #0, which internally calls
	// another verb. After the call returns, we verify ctx.ThisValue was restored.
	code := `x = #0:test_nested(); return x;`

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
			t.Fatalf("compile error: %v", err)
		}
	}
	c.emit(OP_RETURN_NONE)
	c.endScope()

	// Set up VM with a specific ThisValue to track save/restore
	sentinel := types.NewStr("sentinel_this_value")
	v := NewVM(store, registry)
	v.Context = types.NewTaskContext()
	v.Context.ThisValue = sentinel

	result, vmErr := v.Run(c.program)
	if vmErr != nil {
		t.Fatalf("VM error: %v", vmErr)
	}

	// test_nested calls test_return which returns 42, then adds 1 -> 43
	if !valuesEqual(result, types.NewInt(43)) {
		t.Errorf("expected 43, got %v", result)
	}

	// After execution, ctx.ThisValue should be restored to our sentinel
	if v.Context.ThisValue == nil {
		t.Errorf("ctx.ThisValue is nil after verb call, expected sentinel")
	} else if !v.Context.ThisValue.Equal(sentinel) {
		t.Errorf("ctx.ThisValue not restored: got %v, want %v", v.Context.ThisValue, sentinel)
	}
}

// TestParity_CommandEnvVarsInVerbFrame verifies that command environment
// variables (argstr, dobj, dobjstr, iobj, iobjstr, prepstr) are propagated
// from the task to verb frames in the bytecode VM.
func TestParity_CommandEnvVarsInVerbFrame(t *testing.T) {
	store := db.NewStore()

	root := db.NewObject(0, 0)
	root.Name = "Root"
	root.Flags = db.FlagRead | db.FlagWrite

	// Verb that reads argstr
	root.Verbs["test_argstr"] = &db.Verb{
		Name:  "test_argstr",
		Names: []string{"test_argstr"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return argstr;"},
	}

	// Verb that reads dobjstr
	root.Verbs["test_dobjstr"] = &db.Verb{
		Name:  "test_dobjstr",
		Names: []string{"test_dobjstr"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return dobjstr;"},
	}

	// Verb that reads prepstr
	root.Verbs["test_prepstr"] = &db.Verb{
		Name:  "test_prepstr",
		Names: []string{"test_prepstr"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return prepstr;"},
	}

	// Verb that reads iobjstr
	root.Verbs["test_iobjstr"] = &db.Verb{
		Name:  "test_iobjstr",
		Names: []string{"test_iobjstr"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return iobjstr;"},
	}

	// Verb that reads dobj
	root.Verbs["test_dobj"] = &db.Verb{
		Name:  "test_dobj",
		Names: []string{"test_dobj"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return dobj;"},
	}

	// Verb that reads iobj
	root.Verbs["test_iobj"] = &db.Verb{
		Name:  "test_iobj",
		Names: []string{"test_iobj"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return iobj;"},
	}

	store.Add(root)

	// Create a task with command context fields populated
	tsk := task.NewTask(1, 0, 30000, 5.0)
	tsk.Argstr = "apple to the basket"
	tsk.Dobjstr = "apple"
	tsk.Dobj = 5
	tsk.Prepstr = "to"
	tsk.Iobjstr = "the basket"
	tsk.Iobj = 10

	ctx := types.NewTaskContext()
	ctx.Task = tsk

	// Also set up the tree-walker's env context so parity works
	// The tree-walker reads these from its Environment, which is set by SetVerbContext.
	// For parity, we set the context on both paths.

	tests := []struct {
		name     string
		code     string
		expected types.Value
	}{
		{"argstr", `return #0:test_argstr();`, types.NewStr("apple to the basket")},
		{"dobjstr", `return #0:test_dobjstr();`, types.NewStr("apple")},
		{"prepstr", `return #0:test_prepstr();`, types.NewStr("to")},
		{"iobjstr", `return #0:test_iobjstr();`, types.NewStr("the basket")},
		{"dobj", `return #0:test_dobj();`, types.NewObj(5)},
		{"iobj", `return #0:test_iobj();`, types.NewObj(10)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parser.NewParser(tt.code)
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
					t.Fatalf("compile error: %v", err)
				}
			}
			c.emit(OP_RETURN_NONE)
			c.endScope()

			v := NewVM(store, registry)
			v.Context = ctx

			result, vmErr := v.Run(c.program)
			if vmErr != nil {
				t.Fatalf("VM error: %v", vmErr)
			}

			if !valuesEqual(result, tt.expected) {
				t.Errorf("got %v (%T), want %v (%T)", result, result, tt.expected, tt.expected)
			}
		})
	}
}

// --- Waif, Primitive, and Anonymous verb target tests ---

// newWaifVerbTestStore creates a store for testing waif verb calls.
// #0 has verb "get_this" (returns this) and property "waif_ref" holding a waif with class=#0.
func newWaifVerbTestStore() *db.Store {
	store := db.NewStore()

	root := db.NewObject(0, 0)
	root.Name = "WaifClass"
	root.Flags = db.FlagRead | db.FlagWrite

	root.Verbs["get_this"] = &db.Verb{
		Name:  "get_this",
		Names: []string{"get_this"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return this;"},
	}

	// Store a waif value as a property so MOO code can retrieve it
	waif := types.NewWaif(0, 0)
	root.Properties["waif_ref"] = &db.Property{
		Name:    "waif_ref",
		Value:   waif,
		Owner:   0,
		Perms:   db.PropRead | db.PropWrite,
		Defined: true,
	}

	store.Add(root)
	return store
}

func TestParity_WaifVerbCall(t *testing.T) {
	store := newWaifVerbTestStore()
	// Call get_this on the waif. The waif's class is #0 which has verb get_this.
	// Inside the verb, "this" should be the waif, not #0.
	code := `return #0.waif_ref:get_this();`
	compareProgramsWithStore(t, code, store)
}

// newPrimitiveProtoTestStore creates a store for testing primitive prototype verb calls.
// #0 has str_proto property pointing to #1. #1 has verb "get_this" that returns this.
func newPrimitiveProtoTestStore() *db.Store {
	store := db.NewStore()

	root := db.NewObject(0, 0)
	root.Name = "Root"
	root.Flags = db.FlagRead | db.FlagWrite

	proto := db.NewObject(1, 0)
	proto.Name = "StrProto"
	proto.Flags = db.FlagRead | db.FlagWrite

	proto.Verbs["get_this"] = &db.Verb{
		Name:  "get_this",
		Names: []string{"get_this"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return this;"},
	}

	// Set #0.str_proto = #1
	root.Properties["str_proto"] = &db.Property{
		Name:    "str_proto",
		Value:   types.NewObj(1),
		Owner:   0,
		Perms:   db.PropRead | db.PropWrite,
		Defined: true,
	}

	store.Add(root)
	store.Add(proto)
	return store
}

func TestParity_PrimitivePrototypeVerbCall(t *testing.T) {
	store := newPrimitiveProtoTestStore()
	// Call get_this on a string literal. The str_proto (#1) has verb get_this.
	// Inside the verb, "this" should be the string "hello", not #1.
	code := `return "hello":get_this();`
	compareProgramsWithStore(t, code, store)
}

// newAnonVerbTestStore creates a store for testing anonymous object verb calls.
// #0 is root (has anon_ref property pointing to *#1). #1 is anonymous with verb get_this.
func newAnonVerbTestStore() *db.Store {
	store := db.NewStore()

	root := db.NewObject(0, 0)
	root.Name = "Root"
	root.Flags = db.FlagRead | db.FlagWrite

	anon := db.NewObject(1, 0)
	anon.Name = "AnonObj"
	anon.Flags = db.FlagRead | db.FlagWrite
	anon.Anonymous = true

	anon.Verbs["get_this"] = &db.Verb{
		Name:  "get_this",
		Names: []string{"get_this"},
		Owner: 0,
		Perms: db.VerbRead | db.VerbWrite | db.VerbExecute,
		Code:  []string{"return this;"},
	}

	// Store an anonymous ObjValue so MOO code can reference it
	root.Properties["anon_ref"] = &db.Property{
		Name:    "anon_ref",
		Value:   types.NewAnon(1),
		Owner:   0,
		Perms:   db.PropRead | db.PropWrite,
		Defined: true,
	}

	store.Add(root)
	store.Add(anon)
	return store
}

func TestParity_AnonymousObjectVerbCall(t *testing.T) {
	store := newAnonVerbTestStore()
	// Call get_this on an anonymous object. Inside the verb, "this" should be
	// the anonymous ObjValue (*#1), not a regular ObjValue (#1).
	code := `return #0.anon_ref:get_this();`
	compareProgramsWithStore(t, code, store)
}

// --- FlowSuspend parity tests ---
// These verify that the bytecode VM handles FlowSuspend from suspend() correctly.
// suspend(0) should: sleep 0 seconds, push 0 onto the stack, and continue execution.

// suspendTestCtx creates a task context with a real task so suspend() doesn't return E_INVARG.
func suspendTestCtx() *types.TaskContext {
	ctx := types.NewTaskContext()
	ctx.Task = task.NewTask(1, 0, 100000, 5.0)
	return ctx
}

func TestParity_SuspendDoesNotCorruptStack(t *testing.T) {
	// suspend(0) should not push a float onto the stack; execution should continue normally.
	// Before the fix, suspend(0) would push a float value, corrupting the stack.
	code := `suspend(0); return 42;`

	store := db.NewStore()
	treeCtx := suspendTestCtx()
	treeResult := treeEvalProgramWithStoreAndCtx(t, code, store, treeCtx)

	vmCtx := suspendTestCtx()
	vmVal, vmErr := vmEvalProgramWithStoreAndCtx(t, code, store, vmCtx)

	if treeResult.IsError() {
		t.Fatalf("tree-walker errored: %v", treeResult.Error)
	}
	if vmErr != nil {
		t.Fatalf("VM errored: %v (tree-walker returned %v)", vmErr, treeResult.Val)
	}
	if !valuesEqual(treeResult.Val, vmVal) {
		t.Errorf("MISMATCH: tree-walker=%v (%T), VM=%v (%T)",
			treeResult.Val, treeResult.Val, vmVal, vmVal)
	}
}

func TestParity_SuspendReturnsZero(t *testing.T) {
	// In the bytecode VM, suspend(0) should push integer 0 (not a float) as its return value.
	// The tree-walker propagates FlowSuspend at the statement level so assignment never happens,
	// but in the bytecode VM the value is pushed onto the stack and assignment proceeds.
	// This test verifies the VM pushes int 0 (not float 0.0) for suspend().
	code := `x = suspend(0); return x;`

	store := db.NewStore()
	vmCtx := suspendTestCtx()
	vmVal, vmErr := vmEvalProgramWithStoreAndCtx(t, code, store, vmCtx)

	if vmErr != nil {
		t.Fatalf("VM errored: %v", vmErr)
	}
	// suspend() should return integer 0
	expected := types.NewInt(0)
	if !valuesEqual(expected, vmVal) {
		t.Errorf("expected %v (%T), got %v (%T)", expected, expected, vmVal, vmVal)
	}
}

func TestParity_SuspendContinuesExecution(t *testing.T) {
	// After suspend(0), execution should continue to the next statement.
	code := `suspend(0); return "after";`

	store := db.NewStore()
	treeCtx := suspendTestCtx()
	treeResult := treeEvalProgramWithStoreAndCtx(t, code, store, treeCtx)

	vmCtx := suspendTestCtx()
	vmVal, vmErr := vmEvalProgramWithStoreAndCtx(t, code, store, vmCtx)

	if treeResult.IsError() {
		t.Fatalf("tree-walker errored: %v", treeResult.Error)
	}
	if vmErr != nil {
		t.Fatalf("VM errored: %v (tree-walker returned %v)", vmErr, treeResult.Val)
	}
	if !valuesEqual(treeResult.Val, vmVal) {
		t.Errorf("MISMATCH: tree-walker=%v (%T), VM=%v (%T)",
			treeResult.Val, treeResult.Val, vmVal, vmVal)
	}
}

// --- Property-indexed assignment parity tests ---

func TestParity_PropertyIndexAssign(t *testing.T) {
	cases := map[string]string{
		"list_single_index": `#0.foo = {10, 20, 30}; #0.foo[2] = 99; return #0.foo;`,
		"list_first_index":  `#0.foo = {10, 20, 30}; #0.foo[1] = 99; return #0.foo;`,
		"list_last_index":   `#0.foo = {10, 20, 30}; #0.foo[3] = 99; return #0.foo;`,
		"returns_value":     `#0.foo = {10, 20, 30}; x = (#0.foo[2] = 99); return x;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			store := newPropertyTestStore()
			compareProgramsWithStore(t, code, store)
		})
	}
}

func TestParity_PropertyNestedIndexAssign(t *testing.T) {
	cases := map[string]string{
		"nested_2d": `#0.foo = {{1, 2}, {3, 4}}; #0.foo[1][2] = 99; return #0.foo;`,
		"nested_2d_second": `#0.foo = {{1, 2}, {3, 4}}; #0.foo[2][1] = 99; return #0.foo;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			store := newPropertyTestStore()
			compareProgramsWithStore(t, code, store)
		})
	}
}

func TestParity_PropertyMapIndexAssign(t *testing.T) {
	cases := map[string]string{
		"map_assign_existing": `#0.foo = ["a" -> 1, "b" -> 2]; #0.foo["a"] = 99; return #0.foo["a"];`,
		"map_assign_new_key":  `#0.foo = ["a" -> 1]; #0.foo["b"] = 2; return #0.foo;`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			store := newPropertyTestStore()
			compareProgramsWithStore(t, code, store)
		})
	}
}

func TestParitySpliceExpression(t *testing.T) {
	cases := map[string]string{
		// Standalone @list evaluates to the list itself
		"splice_list_passthrough": `return @{1, 2, 3};`,
		// Standalone @string raises E_TYPE
		"splice_non_list_error": `return @"not a list";`,
		// Splice in list context (regression — already works via OP_LIST_EXTEND)
		"splice_in_list_context": `return {1, @{2, 3}, 4};`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) { comparePrograms(t, code) })
	}
}
