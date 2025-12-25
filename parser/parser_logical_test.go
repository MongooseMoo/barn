package parser

import "testing"

func TestLogicalOperators(t *testing.T) {
	tests := []struct {
		input    string
		operator TokenType
	}{
		{"a && b", TOKEN_AND},
		{"x || y", TOKEN_OR},
		{"true && false", TOKEN_AND},
		{"1 || 0", TOKEN_OR},
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

func TestLogicalPrecedence(t *testing.T) {
	tests := []struct {
		input    string
		rootOp   TokenType
		desc     string
	}{
		{"a || b && c", TOKEN_OR, "should parse as a || (b && c)"},
		{"a && b || c && d", TOKEN_OR, "should parse as (a && b) || (c && d)"},
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

func TestLogicalVsComparison(t *testing.T) {
	// Logical operators have lower precedence than comparison
	input := "a < b && c > d"
	p := NewParser(input)
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	binary, ok := expr.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr at root, got %T", expr)
	}

	if binary.Operator != TOKEN_AND {
		t.Errorf("expected AND at root, got %s", binary.Operator)
	}

	// Both sides should be comparison operators
	leftBinary, ok := binary.Left.(*BinaryExpr)
	if !ok {
		t.Errorf("expected left to be BinaryExpr, got %T", binary.Left)
	} else if leftBinary.Operator != TOKEN_LT {
		t.Errorf("expected left to be <, got %s", leftBinary.Operator)
	}

	rightBinary, ok := binary.Right.(*BinaryExpr)
	if !ok {
		t.Errorf("expected right to be BinaryExpr, got %T", binary.Right)
	} else if rightBinary.Operator != TOKEN_GT {
		t.Errorf("expected right to be >, got %s", rightBinary.Operator)
	}
}
