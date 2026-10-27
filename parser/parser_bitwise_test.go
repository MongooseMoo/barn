package parser

import (
	"github.com/MongooseMoo/barn/ir"
	"testing"
)

func TestBitwiseOperators(t *testing.T) {
	tests := []struct {
		input    string
		operator ir.BinaryOperator
	}{
		{"5 &. 3", ir.BinaryBitAnd},
		{"7 |. 1", ir.BinaryBitOr},
		{"9 ^. 2", ir.BinaryBitXor},
		{"1 << 4", ir.BinaryShiftLeft},
		{"16 >> 2", ir.BinaryShiftRight},
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

func TestBitwisePrecedence(t *testing.T) {
	// Bitwise operators bind tighter than comparison and looser than shift.
	// Toast declares |., &., and ^. at one left-associative level.
	tests := []struct {
		input  string
		rootOp ir.BinaryOperator
		desc   string
	}{
		{"a |. b &. c", ir.BinaryBitAnd, "should parse as (a |. b) &. c"},
		{"a ^. b &. c", ir.BinaryBitAnd, "should parse as (a ^. b) &. c"},
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

func TestBitwiseBindsMoreTightlyThanComparison(t *testing.T) {
	p := NewParser("a < b |. c")
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	comparison, ok := expr.(*ir.BinaryExpr)
	if !ok || comparison.Operator != ir.BinaryLess {
		t.Fatalf("root = %T (%v), want less-than", expr, expr)
	}
	bitwise, ok := comparison.Right.(*ir.BinaryExpr)
	if !ok || bitwise.Operator != ir.BinaryBitOr {
		t.Fatalf("right = %T (%v), want bitwise-or", comparison.Right, comparison.Right)
	}
}

func TestShiftPrecedence(t *testing.T) {
	// Shift has lower precedence than additive
	input := "1 + 2 << 3"
	p := NewParser(input)
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	binary, ok := expr.(*ir.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr at root, got %T", expr)
	}

	if binary.Operator != ir.BinaryShiftLeft {
		t.Errorf("expected LSHIFT at root, got %s", binary.Operator)
	}

	// Left should be addition
	leftBinary, ok := binary.Left.(*ir.BinaryExpr)
	if !ok {
		t.Errorf("expected left to be BinaryExpr, got %T", binary.Left)
	} else if leftBinary.Operator != ir.BinaryAdd {
		t.Errorf("expected left to be +, got %s", leftBinary.Operator)
	}
}
