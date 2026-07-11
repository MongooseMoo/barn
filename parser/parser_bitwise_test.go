package parser

import (
	"barn/verb"
	"testing"
)

func TestBitwiseOperators(t *testing.T) {
	tests := []struct {
		input    string
		operator verb.BinaryOperator
	}{
		{"5 &. 3", verb.BinaryBitAnd},
		{"7 |. 1", verb.BinaryBitOr},
		{"9 ^. 2", verb.BinaryBitXor},
		{"1 << 4", verb.BinaryShiftLeft},
		{"16 >> 2", verb.BinaryShiftRight},
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

func TestBitwisePrecedence(t *testing.T) {
	// Bitwise operators are between comparison and shift
	// &. has higher precedence than |., ^. is in between
	tests := []struct {
		input  string
		rootOp verb.BinaryOperator
		desc   string
	}{
		{"a |. b &. c", verb.BinaryBitOr, "should parse as a |. (b &. c)"},
		{"a ^. b &. c", verb.BinaryBitXor, "should parse as a ^. (b &. c)"},
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

func TestShiftPrecedence(t *testing.T) {
	// Shift has lower precedence than additive
	input := "1 + 2 << 3"
	p := NewParser(input)
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	binary, ok := expr.(*verb.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr at root, got %T", expr)
	}

	if binary.Operator != verb.BinaryShiftLeft {
		t.Errorf("expected LSHIFT at root, got %s", binary.Operator)
	}

	// Left should be addition
	leftBinary, ok := binary.Left.(*verb.BinaryExpr)
	if !ok {
		t.Errorf("expected left to be BinaryExpr, got %T", binary.Left)
	} else if leftBinary.Operator != verb.BinaryAdd {
		t.Errorf("expected left to be +, got %s", leftBinary.Operator)
	}
}
