package parser

import (
	"github.com/MongooseMoo/barn/verb"
	"testing"
)

func TestParseAddition(t *testing.T) {
	tests := []string{
		"1 + 2",
		"x + y",
		"10 + 20",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if binary.Operator != verb.BinaryAdd {
				t.Errorf("expected operator PLUS, got %s", binary.Operator)
			}
		})
	}
}

func TestParseSubtraction(t *testing.T) {
	tests := []string{
		"5 - 3",
		"x - y",
		"100 - 50",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if binary.Operator != verb.BinarySubtract {
				t.Errorf("expected operator MINUS, got %s", binary.Operator)
			}
		})
	}
}

func TestParseMultiplication(t *testing.T) {
	tests := []string{
		"2 * 3",
		"x * y",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if binary.Operator != verb.BinaryMultiply {
				t.Errorf("expected operator STAR, got %s", binary.Operator)
			}
		})
	}
}

func TestParseDivision(t *testing.T) {
	tests := []string{
		"10 / 2",
		"x / y",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if binary.Operator != verb.BinaryDivide {
				t.Errorf("expected operator SLASH, got %s", binary.Operator)
			}
		})
	}
}

func TestParseModulo(t *testing.T) {
	tests := []string{
		"10 % 3",
		"x % y",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if binary.Operator != verb.BinaryModulo {
				t.Errorf("expected operator PERCENT, got %s", binary.Operator)
			}
		})
	}
}

func TestParsePower(t *testing.T) {
	tests := []string{
		"2 ^ 3",
		"x ^ y",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if binary.Operator != verb.BinaryPower {
				t.Errorf("expected operator CARET, got %s", binary.Operator)
			}
		})
	}
}

// Test precedence: multiplication/division before addition/subtraction
func TestArithmeticPrecedence(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"1 + 2 * 3", "should parse as 1 + (2 * 3)"},
		{"10 - 4 / 2", "should parse as 10 - (4 / 2)"},
		{"5 * 3 + 2", "should parse as (5 * 3) + 2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			// Should be addition/subtraction at the root
			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			// Root should be lower precedence operator
			if binary.Operator != verb.BinaryAdd && binary.Operator != verb.BinarySubtract {
				t.Errorf("expected root to be PLUS or MINUS, got %s", binary.Operator)
			}

			// One side should have multiplication/division
			leftBinary, leftOk := binary.Left.(*verb.BinaryExpr)
			rightBinary, rightOk := binary.Right.(*verb.BinaryExpr)

			if !leftOk && !rightOk {
				t.Errorf("%s - expected one side to be BinaryExpr with * or /", tt.desc)
			}

			if leftOk && (leftBinary.Operator == verb.BinaryMultiply || leftBinary.Operator == verb.BinaryDivide) {
				// Good - left side has higher precedence
			} else if rightOk && (rightBinary.Operator == verb.BinaryMultiply || rightBinary.Operator == verb.BinaryDivide) {
				// Good - right side has higher precedence
			} else {
				t.Errorf("%s - expected multiplication or division in tree", tt.desc)
			}
		})
	}
}

// Test right-associativity of power operator
func TestPowerRightAssociativity(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"2 ^ 3 ^ 2", "should parse as 2 ^ (3 ^ 2)"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			// Should be power at root
			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if binary.Operator != verb.BinaryPower {
				t.Errorf("expected root to be CARET, got %s", binary.Operator)
			}

			// Right side should be another power operation (right-associative)
			rightBinary, ok := binary.Right.(*verb.BinaryExpr)
			if !ok {
				t.Errorf("%s - expected right side to be BinaryExpr, got %T", tt.desc, binary.Right)
				return
			}

			if rightBinary.Operator != verb.BinaryPower {
				t.Errorf("%s - expected right side to be CARET, got %s", tt.desc, rightBinary.Operator)
			}
		})
	}
}

// Test left-associativity of division/modulo
func TestDivisionLeftAssociativity(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"10 / 3 % 2", "should parse as (10 / 3) % 2"},
		{"20 / 4 / 2", "should parse as (20 / 4) / 2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			// Should be division/modulo at root
			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			// Left side should be another binary operation (left-associative)
			_, ok = binary.Left.(*verb.BinaryExpr)
			if !ok {
				t.Errorf("%s - expected left side to be BinaryExpr, got %T", tt.desc, binary.Left)
			}
		})
	}
}
