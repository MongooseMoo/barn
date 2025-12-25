package eval

import (
	"barn/parser"
	"barn/types"
	"testing"
)

// Helper to parse and evaluate an expression
func evalExpr(t *testing.T, input string) types.Result {
	p := parser.NewParser(input)
	expr, err := p.ParseExpression(0) // 0 = lowest precedence
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	evaluator := NewEvaluator()
	ctx := types.NewTaskContext()
	return evaluator.Eval(expr, ctx)
}

// Test literal evaluation
func TestEvalLiterals(t *testing.T) {
	tests := []struct {
		input    string
		expected types.Value
	}{
		{"42", types.IntValue{Val: 42}},
		{"3.14", types.FloatValue{Val: 3.14}},
		{`"hello"`, types.NewStr("hello")},
		{"true", types.BoolValue{Val: true}},
		{"false", types.BoolValue{Val: false}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := evalExpr(t, tt.input)
			if !result.IsNormal() {
				t.Fatalf("Expected normal result, got flow %v", result.Flow)
			}
			if !result.Val.Equal(tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result.Val)
			}
		})
	}
}

// Test arithmetic operations
func TestEvalArithmetic(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1 + 2", 3},
		{"10 - 3", 7},
		{"4 * 5", 20},
		{"20 / 4", 5},
		{"17 % 5", 2},
		{"2 ^ 3", 8},
		{"-5", -5},
		{"1 + 2 * 3", 7},  // Precedence test
		{"(1 + 2) * 3", 9}, // Parentheses test
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := evalExpr(t, tt.input)
			if !result.IsNormal() {
				t.Fatalf("Expected normal result, got flow %v", result.Flow)
			}
			intVal, ok := result.Val.(types.IntValue)
			if !ok {
				t.Fatalf("Expected IntValue, got %T", result.Val)
			}
			if intVal.Val != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, intVal.Val)
			}
		})
	}
}

// Test comparison operations
func TestEvalComparison(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"1 == 1", true},
		{"1 == 2", false},
		{"1 != 2", true},
		{"1 < 2", true},
		{"2 < 1", false},
		{"1 <= 1", true},
		{"2 > 1", true},
		{"1 >= 1", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := evalExpr(t, tt.input)
			if !result.IsNormal() {
				t.Fatalf("Expected normal result, got flow %v", result.Flow)
			}
			intVal, ok := result.Val.(types.IntValue)
			if !ok {
				t.Fatalf("Expected IntValue, got %T", result.Val)
			}
			expectedInt := int64(0)
			if tt.expected {
				expectedInt = 1
			}
			if intVal.Val != expectedInt {
				t.Errorf("Expected %d, got %d", expectedInt, intVal.Val)
			}
		})
	}
}

// Test logical operations
func TestEvalLogical(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1 && 2", 2},        // Returns right if left is truthy
		{"0 && 2", 0},        // Returns left if left is falsy
		{"1 || 2", 1},        // Returns left if left is truthy
		{"0 || 2", 2},        // Returns right if left is falsy
		{"!0", 1},            // Not false is true
		{"!1", 0},            // Not true is false
		{"1 && 2 && 3", 3},   // Chained AND
		{"0 || 0 || 5", 5},   // Chained OR
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := evalExpr(t, tt.input)
			if !result.IsNormal() {
				t.Fatalf("Expected normal result, got flow %v", result.Flow)
			}
			intVal, ok := result.Val.(types.IntValue)
			if !ok {
				t.Fatalf("Expected IntValue, got %T", result.Val)
			}
			if intVal.Val != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, intVal.Val)
			}
		})
	}
}

// Test ternary operator
func TestEvalTernary(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1 ? 100 | 200", 100},
		{"0 ? 100 | 200", 200},
		{"1 ? 2 ? 10 | 20 | 30", 10}, // Nested ternary
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := evalExpr(t, tt.input)
			if !result.IsNormal() {
				t.Fatalf("Expected normal result, got flow %v", result.Flow)
			}
			intVal, ok := result.Val.(types.IntValue)
			if !ok {
				t.Fatalf("Expected IntValue, got %T", result.Val)
			}
			if intVal.Val != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, intVal.Val)
			}
		})
	}
}

// Test variable assignment and lookup
func TestEvalVariables(t *testing.T) {
	evaluator := NewEvaluator()
	ctx := types.NewTaskContext()

	// Parse and evaluate assignment
	p := parser.NewParser("x = 42")
	expr, err := p.ParseExpression(0)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	result := evaluator.Eval(expr, ctx)
	if !result.IsNormal() {
		t.Fatalf("Assignment failed with flow %v", result.Flow)
	}

	// Verify assignment returns the value
	intVal, ok := result.Val.(types.IntValue)
	if !ok || intVal.Val != 42 {
		t.Fatalf("Assignment should return 42, got %v", result.Val)
	}

	// Parse and evaluate variable lookup
	p = parser.NewParser("x")
	expr, err = p.ParseExpression(0)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	result = evaluator.Eval(expr, ctx)
	if !result.IsNormal() {
		t.Fatalf("Variable lookup failed with flow %v", result.Flow)
	}

	intVal, ok = result.Val.(types.IntValue)
	if !ok || intVal.Val != 42 {
		t.Fatalf("Expected x to be 42, got %v", result.Val)
	}
}

// Test builtin functions
func TestEvalBuiltins(t *testing.T) {
	tests := []struct {
		input    string
		check    func(t *testing.T, result types.Result)
	}{
		{
			"typeof(42)",
			func(t *testing.T, result types.Result) {
				intVal, ok := result.Val.(types.IntValue)
				if !ok || intVal.Val != int64(types.TYPE_INT) {
					t.Errorf("typeof(42) should return TYPE_INT (0), got %v", result.Val)
				}
			},
		},
		{
			`tostr(42)`,
			func(t *testing.T, result types.Result) {
				strVal, ok := result.Val.(types.StrValue)
				if !ok || strVal.Value() != "42" {
					t.Errorf(`tostr(42) should return "42", got %v`, result.Val)
				}
			},
		},
		{
			`toint("123")`,
			func(t *testing.T, result types.Result) {
				intVal, ok := result.Val.(types.IntValue)
				if !ok || intVal.Val != 123 {
					t.Errorf(`toint("123") should return 123, got %v`, result.Val)
				}
			},
		},
		{
			"tofloat(42)",
			func(t *testing.T, result types.Result) {
				floatVal, ok := result.Val.(types.FloatValue)
				if !ok || floatVal.Val != 42.0 {
					t.Errorf("tofloat(42) should return 42.0, got %v", result.Val)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := evalExpr(t, tt.input)
			if !result.IsNormal() {
				t.Fatalf("Expected normal result, got flow %v with error %v", result.Flow, result.Error)
			}
			tt.check(t, result)
		})
	}
}

// Test error cases
func TestEvalErrors(t *testing.T) {
	tests := []struct {
		input         string
		expectedError types.ErrorCode
	}{
		{"1 / 0", types.E_DIV},       // Division by zero
		{`toint("abc")`, types.E_INVARG}, // Invalid string to int conversion
		{`"a" + 1`, types.E_TYPE},     // Type mismatch in addition
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := evalExpr(t, tt.input)
			if !result.IsError() {
				t.Fatalf("Expected error, got normal result: %v", result.Val)
			}
			if result.Error != tt.expectedError {
				t.Errorf("Expected error %v, got %v", tt.expectedError, result.Error)
			}
		})
	}
}
