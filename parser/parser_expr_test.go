package parser

import (
	"barn/types"
	"testing"
)

func TestParseUnaryMinus(t *testing.T) {
	tests := []struct {
		input      string
		expectUnary bool
		description string
	}{
		// Note: The lexer treats "-5" as a negative literal (TOKEN_INT with value "-5")
		// This is a lexer ambiguity but matches MOO behavior for simple cases
		{"-5", false, "negative literal"},
		{"-42", false, "negative literal"},
		// Double negation forces unary operator
		{"--3", true, "double negation"},
		{"- -3", true, "unary minus with space"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			if tt.expectUnary {
				unary, ok := expr.(*UnaryExpr)
				if !ok {
					t.Fatalf("expected UnaryExpr for %s, got %T", tt.description, expr)
				}
				if unary.Operator != TOKEN_MINUS {
					t.Errorf("expected operator MINUS, got %s", unary.Operator)
				}
			} else {
				// Should be a literal with negative value
				lit, ok := expr.(*LiteralExpr)
				if !ok {
					t.Fatalf("expected LiteralExpr for %s, got %T", tt.description, expr)
				}
				_ = lit // Just verify it's a literal
			}
		})
	}
}

func TestParseLogicalNot(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"!true"},
		{"!false"},
		{"!x"},
		{"!!y"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			unary, ok := expr.(*UnaryExpr)
			if !ok {
				t.Fatalf("expected UnaryExpr, got %T", expr)
			}

			if unary.Operator != TOKEN_NOT {
				t.Errorf("expected operator NOT, got %s", unary.Operator)
			}
		})
	}
}

func TestParseBitwiseNot(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"~0"},
		{"~5"},
		{"~x"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			unary, ok := expr.(*UnaryExpr)
			if !ok {
				t.Fatalf("expected UnaryExpr, got %T", expr)
			}

			if unary.Operator != TOKEN_BITNOT {
				t.Errorf("expected operator BITNOT, got %s", unary.Operator)
			}
		})
	}
}

func TestUnaryOperatorPrecedence(t *testing.T) {
	// Test that unary operators have higher precedence than binary
	tests := []struct {
		input    string
		hasUnary bool
	}{
		// Note: "-5" is lexed as a negative literal, not unary minus
		// But "!true" and "~0" are unary operators
		{"!true && false", true},
		{"~5 &. 3", true},  // Use &. to ensure it's a binary operator, not |
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			// Should be a binary expression
			binary, ok := expr.(*BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if tt.hasUnary {
				_, ok = binary.Left.(*UnaryExpr)
				if !ok {
					t.Errorf("expected left operand to be UnaryExpr, got %T", binary.Left)
				}
			}
		})
	}
}

func TestParseIntegerLiteralExpr(t *testing.T) {
	p := NewParser("42")
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	lit, ok := expr.(*LiteralExpr)
	if !ok {
		t.Fatalf("expected LiteralExpr, got %T", expr)
	}

	intVal, ok := lit.Value.(types.IntValue)
	if !ok {
		t.Fatalf("expected IntValue, got %T", lit.Value)
	}

	if intVal.Val != 42 {
		t.Errorf("expected 42, got %d", intVal.Val)
	}
}

func TestParseIdentifierExpr(t *testing.T) {
	p := NewParser("myvar")
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	ident, ok := expr.(*IdentifierExpr)
	if !ok {
		t.Fatalf("expected IdentifierExpr, got %T", expr)
	}

	if ident.Name != "myvar" {
		t.Errorf("expected name 'myvar', got %q", ident.Name)
	}
}

func TestParseParenExpr(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"(42)"},
		{"(x)"},
		{"((5))"},
		{"(1 + 2)"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			paren, ok := expr.(*ParenExpr)
			if !ok {
				t.Fatalf("expected ParenExpr, got %T", expr)
			}

			if paren.Expr == nil {
				t.Error("ParenExpr.Expr should not be nil")
			}
		})
	}
}
