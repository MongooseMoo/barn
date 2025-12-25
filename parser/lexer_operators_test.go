package parser

import "testing"

func TestOperatorTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected TokenType
		value    string
	}{
		// Arithmetic
		{"+", TOKEN_PLUS, "+"},
		{"-", TOKEN_MINUS, "-"},
		{"*", TOKEN_STAR, "*"},
		{"/", TOKEN_SLASH, "/"},
		{"%", TOKEN_PERCENT, "%"},
		{"^", TOKEN_CARET, "^"},

		// Comparison
		{"==", TOKEN_EQ, "=="},
		{"!=", TOKEN_NE, "!="},
		{"<", TOKEN_LT, "<"},
		{">", TOKEN_GT, ">"},
		{"<=", TOKEN_LE, "<="},
		{">=", TOKEN_GE, ">="},

		// Logical
		{"&&", TOKEN_AND, "&&"},
		{"||", TOKEN_OR, "||"},
		{"!", TOKEN_NOT, "!"},

		// Bitwise
		{"&.", TOKEN_BITAND, "&."},
		{"|.", TOKEN_BITOR, "|."},
		{"^.", TOKEN_BITXOR, "^."},
		{"~", TOKEN_BITNOT, "~"},
		{"<<", TOKEN_LSHIFT, "<<"},
		{">>", TOKEN_RSHIFT, ">>"},

		// Other
		{"=", TOKEN_ASSIGN, "="},
		{"?", TOKEN_QUESTION, "?"},
		{"|", TOKEN_PIPE, "|"},
		{"->", TOKEN_ARROW, "->"},
		{"..", TOKEN_RANGE, ".."},
		{".", TOKEN_DOT, "."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()

			if tok.Type != tt.expected {
				t.Errorf("expected token type %s, got %s", tt.expected, tok.Type)
			}
			if tok.Value != tt.value {
				t.Errorf("expected value %q, got %q", tt.value, tok.Value)
			}
		})
	}
}

func TestOperatorSequences(t *testing.T) {
	tests := []struct {
		input    string
		expected []TokenType
	}{
		{"1+2", []TokenType{TOKEN_INT, TOKEN_PLUS, TOKEN_INT, TOKEN_EOF}},
		{"a&&b||c", []TokenType{TOKEN_IDENTIFIER, TOKEN_AND, TOKEN_IDENTIFIER, TOKEN_OR, TOKEN_IDENTIFIER, TOKEN_EOF}},
		{"x^.y", []TokenType{TOKEN_IDENTIFIER, TOKEN_BITXOR, TOKEN_IDENTIFIER, TOKEN_EOF}},
		{"a<<2", []TokenType{TOKEN_IDENTIFIER, TOKEN_LSHIFT, TOKEN_INT, TOKEN_EOF}},
		{"x..y", []TokenType{TOKEN_IDENTIFIER, TOKEN_RANGE, TOKEN_IDENTIFIER, TOKEN_EOF}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			for i, expectedType := range tt.expected {
				tok := lexer.NextToken()
				if tok.Type != expectedType {
					t.Errorf("token %d: expected type %s, got %s", i, expectedType, tok.Type)
				}
			}
		})
	}
}
