package parser

import (
	"github.com/MongooseMoo/barn/ir"
	"testing"
)

func TestLogicalOperators(t *testing.T) {
	tests := []struct {
		input    string
		operator ir.BinaryOperator
	}{
		{"a && b", ir.BinaryAnd},
		{"x || y", ir.BinaryOr},
		{"true && false", ir.BinaryAnd},
		{"1 || 0", ir.BinaryOr},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*ir.BinaryExpr)
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
		rootOp ir.BinaryOperator
		desc   string
	}{
		// Toast's grammar declares `%left tOR tAND`: one left-associative level.
		{"a || b && c", ir.BinaryAnd, "should parse as (a || b) && c"},
		{"a && b || c && d", ir.BinaryAnd, "should parse as ((a && b) || c) && d"},
		{"a && b || c", ir.BinaryOr, "should parse as (a && b) || c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*ir.BinaryExpr)
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

	binary, ok := expr.(*ir.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr at root, got %T", expr)
	}

	if binary.Operator != ir.BinaryAnd {
		t.Errorf("expected AND at root, got %s", binary.Operator)
	}

	// Both sides should be comparison operators
	leftBinary, ok := binary.Left.(*ir.BinaryExpr)
	if !ok {
		t.Errorf("expected left to be BinaryExpr, got %T", binary.Left)
	} else if leftBinary.Operator != ir.BinaryLess {
		t.Errorf("expected left to be <, got %s", leftBinary.Operator)
	}

	rightBinary, ok := binary.Right.(*ir.BinaryExpr)
	if !ok {
		t.Errorf("expected right to be BinaryExpr, got %T", binary.Right)
	} else if rightBinary.Operator != ir.BinaryGreater {
		t.Errorf("expected right to be >, got %s", rightBinary.Operator)
	}
}
