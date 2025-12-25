package eval

import (
	"barn/parser"
	"barn/types"
	"testing"
)

func TestIfStatement(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected types.Value
	}{
		{
			name:     "if true returns then",
			code:     "if (1) return 5; endif return 0;",
			expected: types.NewInt(5),
		},
		{
			name:     "if false skips then",
			code:     "if (0) return 5; endif return 0;",
			expected: types.NewInt(0),
		},
		{
			name:     "elseif executes when if false",
			code:     "if (0) return 1; elseif (1) return 2; endif return 0;",
			expected: types.NewInt(2),
		},
		{
			name:     "else executes when all false",
			code:     "if (0) return 1; elseif (0) return 2; else return 3; endif return 0;",
			expected: types.NewInt(3),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator()
			result, err := eval.EvalProgram(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestWhileLoop(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected types.Value
	}{
		{
			name:     "while loop counts to 5",
			code:     "x = 0; while (x < 5) x = x + 1; endwhile return x;",
			expected: types.NewInt(5),
		},
		{
			name:     "while loop with false condition never executes",
			code:     "x = 0; while (0) x = 10; endwhile return x;",
			expected: types.NewInt(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator()
			result, err := eval.EvalProgram(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestForRangeLoop(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected types.Value
	}{
		{
			name:     "for range 1 to 5",
			code:     "sum = 0; for x in [1..5] sum = sum + x; endfor return sum;",
			expected: types.NewInt(15), // 1+2+3+4+5 = 15
		},
		{
			name:     "for range with negative numbers",
			code:     "sum = 0; for x in [-2..2] sum = sum + x; endfor return sum;",
			expected: types.NewInt(0), // -2 + -1 + 0 + 1 + 2 = 0
		},
		{
			name:     "for range empty (start > end)",
			code:     "x = 10; for i in [5..1] x = 0; endfor return x;",
			expected: types.NewInt(10), // Loop never executes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator()
			result, err := eval.EvalProgram(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestForListLoop(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected types.Value
	}{
		{
			name:     "for list iteration",
			code:     "sum = 0; for x in ({1, 2, 3, 4}) sum = sum + x; endfor return sum;",
			expected: types.NewInt(10),
		},
		{
			name:     "for list with index",
			code:     "sum = 0; for x, i in ({10, 20, 30}) sum = sum + i; endfor return sum;",
			expected: types.NewInt(6), // indices: 1+2+3 = 6
		},
		{
			name:     "for empty list",
			code:     "x = 5; for item in ({}) x = 0; endfor return x;",
			expected: types.NewInt(5), // Loop never executes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator()
			result, err := eval.EvalProgram(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestBreakAndContinue(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected types.Value
	}{
		{
			name:     "break exits loop",
			code:     "sum = 0; for x in [1..10] if (x == 5) break; endif sum = sum + x; endfor return sum;",
			expected: types.NewInt(10), // 1+2+3+4 = 10, breaks before adding 5
		},
		{
			name:     "continue skips iteration",
			code:     "sum = 0; for x in [1..5] if (x == 3) continue; endif sum = sum + x; endfor return sum;",
			expected: types.NewInt(12), // 1+2+4+5 = 12, skips 3
		},
		{
			name:     "break in while loop",
			code:     "x = 0; while (x < 10) x = x + 1; if (x == 5) break; endif endwhile return x;",
			expected: types.NewInt(5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator()
			result, err := eval.EvalProgram(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestReturnStatement(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected types.Value
	}{
		{
			name:     "return with value",
			code:     "x = 5; return x;",
			expected: types.NewInt(5),
		},
		{
			name:     "return without value defaults to 0",
			code:     "x = 5; return;",
			expected: types.NewInt(0),
		},
		{
			name:     "return from if statement",
			code:     "if (1) return 42; endif return 0;",
			expected: types.NewInt(42),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator()
			result, err := eval.EvalProgram(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Test that parser.ParseProgram is available
func TestParserIntegration(t *testing.T) {
	p := parser.NewParser("if (1) x = 5; endif")
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(stmts) == 0 {
		t.Errorf("expected statements, got none")
	}
}
