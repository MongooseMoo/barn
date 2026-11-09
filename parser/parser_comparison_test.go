package parser

import (
	"github.com/MongooseMoo/barn/ir"
	"testing"
)

func TestComparisonOperators(t *testing.T) {
	tests := []struct {
		input    string
		operator ir.BinaryOperator
	}{
		{"1 == 2", ir.BinaryEqual},
		{"x != y", ir.BinaryNotEqual},
		{"a < b", ir.BinaryLess},
		{"c > d", ir.BinaryGreater},
		{"e <= f", ir.BinaryLessEqual},
		{"g >= h", ir.BinaryGreaterEqual},
		{"x in {1, 2, 3}", ir.BinaryIn},
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

func TestComparisonPrecedence(t *testing.T) {
	// Comparison has lower precedence than arithmetic
	tests := []string{
		"1 + 1 == 2",     // should parse as (1 + 1) == 2
		"x * 2 < 10",     // should parse as (x * 2) < 10
		"a - b >= c + d", // should parse as (a - b) >= (c + d)
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*ir.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr at root, got %T", expr)
			}

			// Root should be comparison operator
			switch binary.Operator {
			case ir.BinaryEqual, ir.BinaryNotEqual, ir.BinaryLess, ir.BinaryLessEqual,
				ir.BinaryGreater, ir.BinaryGreaterEqual, ir.BinaryIn:
				// Good
			default:
				t.Errorf("expected comparison operator at root, got %s", binary.Operator)
			}

		})
	}
}
