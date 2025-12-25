package parser

import "testing"

func TestBitwiseOperators(t *testing.T) {
	tests := []struct {
		input    string
		operator TokenType
	}{
		{"5 &. 3", TOKEN_BITAND},
		{"7 |. 1", TOKEN_BITOR},
		{"9 ^. 2", TOKEN_BITXOR},
		{"1 << 4", TOKEN_LSHIFT},
		{"16 >> 2", TOKEN_RSHIFT},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*BinaryExpr)
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
		rootOp TokenType
		desc   string
	}{
		{"a |. b &. c", TOKEN_BITOR, "should parse as a |. (b &. c)"},
		{"a ^. b &. c", TOKEN_BITXOR, "should parse as a ^. (b &. c)"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			binary, ok := expr.(*BinaryExpr)
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

	binary, ok := expr.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr at root, got %T", expr)
	}

	if binary.Operator != TOKEN_LSHIFT {
		t.Errorf("expected LSHIFT at root, got %s", binary.Operator)
	}

	// Left should be addition
	leftBinary, ok := binary.Left.(*BinaryExpr)
	if !ok {
		t.Errorf("expected left to be BinaryExpr, got %T", binary.Left)
	} else if leftBinary.Operator != TOKEN_PLUS {
		t.Errorf("expected left to be +, got %s", leftBinary.Operator)
	}
}
