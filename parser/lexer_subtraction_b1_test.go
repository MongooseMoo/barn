package parser

import (
	"barn/verb"
	"testing"
)

// TestB1SubtractionWithoutSpaces is the regression test for bug B1: the lexer
// used to fold a '-' immediately after a digit into a negative number literal,
// so "1-2" lexed as INT(1) then INT(-2) with no operator between them, which
// failed to parse. MOO has no negative numeric literals; '-' is always the
// minus operator and the parser builds unary/binary minus.
//
// Toast oracle (toastcore.db, port 9451) confirms the correct behavior:
//
//	; return 3-4;     => -1
//	; return 1-2;     => -1
//	; return 1+2*3-4; => 3
func TestB1SubtractionLexesAsMinusOperator(t *testing.T) {
	tests := []struct {
		input string
		want  []TokenType
	}{
		{"1-2", []TokenType{TOKEN_INT, TOKEN_MINUS, TOKEN_INT, TOKEN_EOF}},
		{"3-4", []TokenType{TOKEN_INT, TOKEN_MINUS, TOKEN_INT, TOKEN_EOF}},
		{"1+2*3-4", []TokenType{
			TOKEN_INT, TOKEN_PLUS, TOKEN_INT, TOKEN_STAR, TOKEN_INT, TOKEN_MINUS, TOKEN_INT, TOKEN_EOF,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := NewLexer(tt.input)
			for i, want := range tt.want {
				tok := l.NextToken()
				if tok.Type != want {
					t.Errorf("token[%d] type = %s, want %s", i, tok.Type, want)
				}
			}
		})
	}
}

// TestB1SubtractionParsesAsBinaryMinus confirms "1-2"/"3-4" parse as a binary
// subtraction with two positive integer literals.
func TestB1SubtractionParsesAsBinaryMinus(t *testing.T) {
	for _, input := range []string{"1-2", "3-4"} {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			expr, err := p.ParseExpression(PREC_LOWEST)
			if err != nil {
				t.Fatalf("ParseExpression(%q) error: %v", input, err)
			}
			bin, ok := expr.(*verb.BinaryExpr)
			if !ok {
				t.Fatalf("ParseExpression(%q) = %T, want *BinaryExpr", input, expr)
			}
			if bin.Operator != verb.BinarySubtract {
				t.Errorf("operator = %s, want MINUS", bin.Operator)
			}
			left, ok := bin.Left.(*verb.LiteralExpr)
			if !ok || left.Kind != verb.LiteralInt {
				t.Fatalf("Left = %T, want positive int LiteralExpr", bin.Left)
			}
			right, ok := bin.Right.(*verb.LiteralExpr)
			if !ok || right.Kind != verb.LiteralInt {
				t.Fatalf("Right = %T, want positive int LiteralExpr", bin.Right)
			}
			if left.IntValue < 0 || right.IntValue < 0 {
				t.Errorf("operands must be positive literals, got %d and %d", left.IntValue, right.IntValue)
			}
		})
	}
}

// TestB1Precedence checks that "1+2*3-4" parses with correct precedence:
// (1 + (2 * 3)) - 4, the structure that evaluates to 3 (matches Toast).
func TestB1Precedence(t *testing.T) {
	p := NewParser("1+2*3-4")
	expr, err := p.ParseExpression(PREC_LOWEST)
	if err != nil {
		t.Fatalf("ParseExpression error: %v", err)
	}
	// Top level is the subtraction.
	sub, ok := expr.(*verb.BinaryExpr)
	if !ok || sub.Operator != verb.BinarySubtract {
		t.Fatalf("top = %T (%v), want BinaryExpr MINUS", expr, expr)
	}
	// Right of subtraction is literal 4.
	right, ok := sub.Right.(*verb.LiteralExpr)
	if !ok || right.IntValue != 4 {
		t.Fatalf("subtraction right = %v, want literal 4", sub.Right)
	}
	// Left of subtraction is the addition 1 + (2*3).
	add, ok := sub.Left.(*verb.BinaryExpr)
	if !ok || add.Operator != verb.BinaryAdd {
		t.Fatalf("subtraction left = %T, want BinaryExpr PLUS", sub.Left)
	}
	mul, ok := add.Right.(*verb.BinaryExpr)
	if !ok || mul.Operator != verb.BinaryMultiply {
		t.Fatalf("addition right = %T, want BinaryExpr STAR", add.Right)
	}
}

// TestB1UnaryMinusStillParses confirms the fix did not break unary minus.
// "-5" must now parse as unary minus on the positive literal 5, and "-y" as
// unary minus on an identifier.
func TestB1UnaryMinusStillParses(t *testing.T) {
	t.Run("-5", func(t *testing.T) {
		p := NewParser("-5")
		expr, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			t.Fatalf("ParseExpression error: %v", err)
		}
		un, ok := expr.(*verb.UnaryExpr)
		if !ok || un.Operator != verb.UnaryNegate {
			t.Fatalf("expr = %T, want UnaryExpr MINUS", expr)
		}
		lit, ok := un.Operand.(*verb.LiteralExpr)
		if !ok || lit.Kind != verb.LiteralInt || lit.IntValue != 5 {
			t.Fatalf("operand = %v, want literal int 5", un.Operand)
		}
	})

	t.Run("-y", func(t *testing.T) {
		p := NewParser("-y")
		expr, err := p.ParseExpression(PREC_LOWEST)
		if err != nil {
			t.Fatalf("ParseExpression error: %v", err)
		}
		un, ok := expr.(*verb.UnaryExpr)
		if !ok || un.Operator != verb.UnaryNegate {
			t.Fatalf("expr = %T, want UnaryExpr MINUS", expr)
		}
		id, ok := un.Operand.(*verb.IdentifierExpr)
		if !ok || id.Name != "y" {
			t.Fatalf("operand = %v, want identifier y", un.Operand)
		}
	})
}
