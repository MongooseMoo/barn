package parser

import (
	"github.com/MongooseMoo/barn/verb"
	"testing"
)

func TestParseUnaryMinus(t *testing.T) {
	tests := []struct {
		input       string
		expectUnary bool
		description string
	}{
		// MOO has no negative numeric literals: "-5" is unary minus applied to
		// the literal 5 (B1 fix). The lexer never folds a leading '-' into a
		// number, so every leading '-' produces a UnaryExpr.
		{"-5", true, "unary minus on literal"},
		{"-42", true, "unary minus on literal"},
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
				unary, ok := expr.(*verb.UnaryExpr)
				if !ok {
					t.Fatalf("expected UnaryExpr for %s, got %T", tt.description, expr)
				}
				if unary.Operator != verb.UnaryNegate {
					t.Errorf("expected operator MINUS, got %s", unary.Operator)
				}
			} else {
				// Should be a literal with negative value
				lit, ok := expr.(*verb.LiteralExpr)
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

			unary, ok := expr.(*verb.UnaryExpr)
			if !ok {
				t.Fatalf("expected UnaryExpr, got %T", expr)
			}

			if unary.Operator != verb.UnaryNot {
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

			unary, ok := expr.(*verb.UnaryExpr)
			if !ok {
				t.Fatalf("expected UnaryExpr, got %T", expr)
			}

			if unary.Operator != verb.UnaryBitwiseNot {
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
		// "-5", "!true", and "~0" are all unary operators (B1: no negative literals)
		{"!true && false", true},
		{"~5 &. 3", true}, // Use &. to ensure it's a binary operator, not |
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			// Should be a binary expression
			binary, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("expected BinaryExpr, got %T", expr)
			}

			if tt.hasUnary {
				_, ok = binary.Left.(*verb.UnaryExpr)
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

	lit, ok := expr.(*verb.LiteralExpr)
	if !ok {
		t.Fatalf("expected LiteralExpr, got %T", expr)
	}

	if lit.Kind != verb.LiteralInt {
		t.Fatalf("literal kind = %v, want LiteralInt", lit.Kind)
	}

	if lit.IntValue != 42 {
		t.Errorf("IntValue = %d, want 42", lit.IntValue)
	}
}

func TestParseIdentifierExpr(t *testing.T) {
	p := NewParser("myvar")
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	ident, ok := expr.(*verb.IdentifierExpr)
	if !ok {
		t.Fatalf("expected IdentifierExpr, got %T", expr)
	}

	if ident.Name != "myvar" {
		t.Errorf("expected name 'myvar', got %q", ident.Name)
	}
}

func TestParenthesesDoNotCreateSemanticNodes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"(42)", "literal"},
		{"(x)", "identifier"},
		{"((5))", "literal"},
		{"(1 + 2)", "binary"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			switch tt.want {
			case "literal":
				if _, ok := expr.(*verb.LiteralExpr); !ok {
					t.Fatalf("expected semantic literal, got %T", expr)
				}
			case "identifier":
				if _, ok := expr.(*verb.IdentifierExpr); !ok {
					t.Fatalf("expected semantic identifier, got %T", expr)
				}
			case "binary":
				if _, ok := expr.(*verb.BinaryExpr); !ok {
					t.Fatalf("expected semantic binary expression, got %T", expr)
				}
			}
		})
	}
}

func TestParenthesesPreserveContainedExpressionLocation(t *testing.T) {
	expr := parseExprForTest(t, "(42)")
	literal, ok := expr.(*verb.LiteralExpr)
	if !ok {
		t.Fatalf("expected semantic literal, got %T", expr)
	}
	if got, want := literal.Position().Offset, 1; got != want {
		t.Fatalf("literal offset = %d, want %d for the contained expression", got, want)
	}
}
