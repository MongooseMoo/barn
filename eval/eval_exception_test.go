package eval

import (
	"barn/parser"
	"barn/types"
	"testing"
)

// TestErrorPropagation verifies errors propagate correctly
func TestErrorPropagation(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected types.ErrorCode
	}{
		{
			name:     "division by zero",
			code:     "1 / 0",
			expected: types.E_DIV,
		},
		{
			name:     "type error",
			code:     `"hello" + 1`,
			expected: types.E_TYPE,
		},
		{
			name:     "undefined variable",
			code:     "undefined_var",
			expected: types.E_VARNF,
		},
		{
			name:     "list range error - zero index",
			code:     "{1, 2, 3}[0]",
			expected: types.E_RANGE,
		},
		{
			name:     "list range error - overflow",
			code:     "{1, 2, 3}[10]",
			expected: types.E_RANGE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parser.NewParser(tt.code)
			expr, err := p.ParseExpression(0)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			ev := NewEvaluator()
			ctx := types.NewTaskContext()
			result := ev.Eval(expr, ctx)

			if !result.IsError() {
				t.Errorf("expected error, got %v", result.Val)
			}
			if result.Error != tt.expected {
				t.Errorf("expected error %s, got %s", tt.expected, result.Error)
			}
		})
	}
}

// TestTryExcept verifies try/except error handling
func TestTryExcept(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		expected      types.Value
		expectError   bool // True if we expect an uncaught exception
		expectedError types.ErrorCode
	}{
		{
			name: "catch division by zero",
			code: `
				try
					x = 1 / 0;
				except (E_DIV)
					x = 99;
				endtry
				return x;
			`,
			expected: types.NewInt(99),
		},
		{
			name: "catch type error",
			code: `
				try
					x = "hello" + 1;
				except (E_TYPE)
					x = 42;
				endtry
				return x;
			`,
			expected: types.NewInt(42),
		},
		{
			name: "catch with error variable - returns error VALUE",
			code: `
				try
					x = 1 / 0;
				except e (E_DIV)
					return e;
				endtry
				return 0;
			`,
			expected: types.NewErr(types.E_DIV),
		},
		{
			name: "catch ANY",
			code: `
				try
					x = 1 / 0;
				except (ANY)
					x = 100;
				endtry
				return x;
			`,
			expected: types.NewInt(100),
		},
		{
			name: "multiple except clauses",
			code: `
				try
					x = "hello" + 1;
				except (E_DIV)
					x = 1;
				except (E_TYPE)
					x = 2;
				except (E_RANGE)
					x = 3;
				endtry
				return x;
			`,
			expected: types.NewInt(2),
		},
		{
			name: "no error - skip except",
			code: `
				try
					x = 10 + 5;
				except (E_DIV)
					x = 99;
				endtry
				return x;
			`,
			expected: types.NewInt(15),
		},
		{
			name: "unhandled error propagates",
			code: `
				try
					x = 1 / 0;
				except (E_TYPE)
					x = 99;
				endtry
				return x;
			`,
			expectError:   true,
			expectedError: types.E_DIV,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parser.NewParser(tt.code)
			program, err := p.ParseProgram()
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			ev := NewEvaluator()
			ctx := types.NewTaskContext()
			result := ev.EvalStatements(program, ctx)

			if tt.expectError {
				if !result.IsError() {
					t.Errorf("expected error, got value %v", result.Val)
				}
				if result.Error != tt.expectedError {
					t.Errorf("expected error %s, got %s", tt.expectedError, result.Error)
				}
				return
			}

			// Check normal value
			if result.IsError() {
				t.Errorf("unexpected error: %s", result.Error)
				return
			}

			if !result.IsReturn() {
				t.Errorf("expected return, got %v", result.Flow)
				return
			}

			if !result.Val.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result.Val)
			}
		})
	}
}

// TestTryFinally verifies finally blocks always execute
func TestTryFinally(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected types.Value
	}{
		{
			name: "finally executes on success",
			code: `
				x = 0;
				try
					x = 10;
				finally
					x = x + 1;
				endtry
				return x;
			`,
			expected: types.NewInt(11),
		},
		{
			name: "finally executes on error",
			code: `
				x = 0;
				try
					try
						y = 1 / 0;
					finally
						x = 99;
					endtry
				except (E_DIV)
					// Caught outer error
				endtry
				return x;
			`,
			expected: types.NewInt(99),
		},
		{
			name: "finally return overrides try return",
			code: `
				try
					return 10;
				finally
					return 20;
				endtry
			`,
			expected: types.NewInt(20),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parser.NewParser(tt.code)
			program, err := p.ParseProgram()
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			ev := NewEvaluator()
			ctx := types.NewTaskContext()
			result := ev.EvalStatements(program, ctx)

			if result.IsError() {
				t.Errorf("unexpected error: %s", result.Error)
				return
			}

			if !result.IsReturn() {
				t.Errorf("expected return, got %v", result.Flow)
				return
			}

			if !result.Val.Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result.Val)
			}
		})
	}
}
