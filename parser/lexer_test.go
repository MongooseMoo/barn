package parser

import "testing"

func TestLexerIntegerTokens(t *testing.T) {
	tests := []struct {
		input string
		want  []Token
	}{
		{
			"42",
			[]Token{
				{Type: TOKEN_INT, Value: "42"},
				{Type: TOKEN_EOF, Value: ""},
			},
		},
		{
			"-5",
			[]Token{
				{Type: TOKEN_INT, Value: "-5"},
				{Type: TOKEN_EOF, Value: ""},
			},
		},
		{
			"0",
			[]Token{
				{Type: TOKEN_INT, Value: "0"},
				{Type: TOKEN_EOF, Value: ""},
			},
		},
		{
			"42 -17 0",
			[]Token{
				{Type: TOKEN_INT, Value: "42"},
				{Type: TOKEN_INT, Value: "-17"},
				{Type: TOKEN_INT, Value: "0"},
				{Type: TOKEN_EOF, Value: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := NewLexer(tt.input)
			for i, want := range tt.want {
				tok := l.NextToken()
				if tok.Type != want.Type {
					t.Errorf("token[%d] type = %s, want %s", i, tok.Type, want.Type)
				}
				if tok.Value != want.Value {
					t.Errorf("token[%d] value = %s, want %s", i, tok.Value, want.Value)
				}
			}
		})
	}
}

func TestLexerKeywords(t *testing.T) {
	tests := []struct {
		input string
		want  TokenType
	}{
		{"if", TOKEN_IF},
		{"else", TOKEN_ELSE},
		{"endif", TOKEN_ENDIF},
		{"while", TOKEN_WHILE},
		{"for", TOKEN_FOR},
		{"return", TOKEN_RETURN},
		{"true", TOKEN_TRUE},
		{"false", TOKEN_FALSE},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := NewLexer(tt.input)
			tok := l.NextToken()
			if tok.Type != tt.want {
				t.Errorf("Lexer(%s) = %s, want %s", tt.input, tok.Type, tt.want)
			}
		})
	}
}
