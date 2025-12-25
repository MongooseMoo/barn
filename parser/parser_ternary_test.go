package parser

import "testing"

func TestParseTernary(t *testing.T) {
	tests := []string{
		"x ? 1 | 2",
		"true ? a | b",
		"1 > 0 ? 10 | 20",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			ternary, ok := expr.(*TernaryExpr)
			if !ok {
				t.Fatalf("expected TernaryExpr, got %T", expr)
			}

			if ternary.Condition == nil {
				t.Error("Condition should not be nil")
			}
			if ternary.ThenExpr == nil {
				t.Error("ThenExpr should not be nil")
			}
			if ternary.ElseExpr == nil {
				t.Error("ElseExpr should not be nil")
			}
		})
	}
}

func TestTernaryRightAssociative(t *testing.T) {
	// a ? b ? 1 | 2 | 3 should parse as a ? (b ? 1 | 2) | 3
	input := "a ? b ? 1 | 2 | 3"
	p := NewParser(input)
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	outer, ok := expr.(*TernaryExpr)
	if !ok {
		t.Fatalf("expected TernaryExpr at root, got %T", expr)
	}

	// The then branch should be another ternary
	inner, ok := outer.ThenExpr.(*TernaryExpr)
	if !ok {
		t.Errorf("expected ThenExpr to be TernaryExpr for right-associativity, got %T", outer.ThenExpr)
	} else {
		// Verify the inner ternary structure
		if inner.Condition == nil || inner.ThenExpr == nil || inner.ElseExpr == nil {
			t.Error("Inner ternary has nil components")
		}
	}
}

func TestTernaryPrecedence(t *testing.T) {
	// Ternary has very low precedence
	tests := []struct {
		input string
		desc  string
	}{
		{"1 + 1 ? 10 | 20", "arithmetic before ternary"},
		{"a && b ? x | y", "logical before ternary"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			ternary, ok := expr.(*TernaryExpr)
			if !ok {
				t.Fatalf("%s - expected TernaryExpr at root, got %T", tt.desc, expr)
			}

			// Condition should be a binary expression
			_, ok = ternary.Condition.(*BinaryExpr)
			if !ok {
				t.Errorf("%s - expected condition to be BinaryExpr, got %T", tt.desc, ternary.Condition)
			}
		})
	}
}

func TestParenthesesOverridePrecedence(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"(1 + 2) * 3", "parentheses force addition first"},
		{"2 * (3 + 4)", "parentheses on right"},
		{"(a && b) || c", "parentheses with logical"},
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
				t.Fatalf("%s - expected BinaryExpr at root, got %T", tt.desc, expr)
			}

			// One side should have parentheses
			foundParen := false
			if _, ok := binary.Left.(*ParenExpr); ok {
				foundParen = true
			}
			if _, ok := binary.Right.(*ParenExpr); ok {
				foundParen = true
			}

			if !foundParen {
				t.Errorf("%s - expected ParenExpr on one side", tt.desc)
			}
		})
	}
}

func TestNestedParentheses(t *testing.T) {
	input := "((5))"
	p := NewParser(input)
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	outer, ok := expr.(*ParenExpr)
	if !ok {
		t.Fatalf("expected outer ParenExpr, got %T", expr)
	}

	inner, ok := outer.Expr.(*ParenExpr)
	if !ok {
		t.Fatalf("expected inner ParenExpr, got %T", outer.Expr)
	}

	_, ok = inner.Expr.(*LiteralExpr)
	if !ok {
		t.Errorf("expected innermost to be LiteralExpr, got %T", inner.Expr)
	}
}
