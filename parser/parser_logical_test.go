package parser

import (
	"barn/verb"
	"testing"
)

func TestLogicalOperators(t *testing.T) {
	tests := []struct {
		input    string
		operator verb.BinaryOperator
	}{
		{"a && b", verb.BinaryAnd},
		{"x || y", verb.BinaryOr},
		{"true && false", verb.BinaryAnd},
		{"1 || 0", verb.BinaryOr},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if binary.Operator != tt.operator {
				t.Errorf("expected operator %s, got %s", tt.operator, binary.Operator)
			}
		})
	}
}

func TestLogicalPrecedence(t *testing.T) {
	tests := []struct {
		input  string
		rootOp verb.BinaryOperator
		desc   string
	}{
		{"a || b && c", verb.BinaryOr, "should parse as a || (b && c)"},
		{"a && b || c && d", verb.BinaryOr, "should parse as (a && b) || (c && d)"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr at root, got %T", expr)
			}

			if binary.Operator != tt.rootOp {
				t.Errorf("%s - expected root %s, got %s", tt.desc, tt.rootOp, binary.Operator)
			}
		})
	}
}

func TestLogicalVsComparison(t *testing.T) {
	// Logical operators have lower precedence than comparison
	input := "a < b && c > d"
	p := NewParser(input)
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	binary, ok := expr.(*verb.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr at root, got %T", expr)
	}

	if binary.Operator != verb.BinaryAnd {
		t.Errorf("expected AND at root, got %s", binary.Operator)
	}

	// Both sides should be comparison operators
	leftBinary, ok := binary.Left.(*verb.BinaryExpr)
	if !ok {
		t.Errorf("expected left to be BinaryExpr, got %T", binary.Left)
	} else if leftBinary.Operator != verb.BinaryLess {
		t.Errorf("expected left to be <, got %s", leftBinary.Operator)
	}

	rightBinary, ok := binary.Right.(*verb.BinaryExpr)
	if !ok {
		t.Errorf("expected right to be BinaryExpr, got %T", binary.Right)
	} else if rightBinary.Operator != verb.BinaryGreater {
		t.Errorf("expected right to be >, got %s", rightBinary.Operator)
	}
}
