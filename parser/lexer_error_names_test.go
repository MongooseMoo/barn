package parser

import "testing"

// Toast matches the whole identifier against its keyword table in any letter
// case (issue #893). An identifier that only begins with an error name, such
// as E_IN3ARG or E_TYPEx, is an ordinary variable; e_type is E_TYPE. Expected
// values were checked against the stock Toast 2.8.0_7 oracle.
func TestLexerErrorNamesUseWholeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  TokenType
		value string
	}{
		{"E_TYPE", TOKEN_ERROR_LIT, "E_TYPE"},
		{"e_type", TOKEN_ERROR_LIT, "E_TYPE"},
		{"E_Type", TOKEN_ERROR_LIT, "E_TYPE"},
		{"e_INTRPT", TOKEN_ERROR_LIT, "E_INTRPT"},
		{"E_IN3ARG", TOKEN_IDENTIFIER, "E_IN3ARG"},
		{"E_TYPEx", TOKEN_IDENTIFIER, "E_TYPEx"},
		{"E_TYPE3", TOKEN_IDENTIFIER, "E_TYPE3"},
		{"E_TYPE_", TOKEN_IDENTIFIER, "E_TYPE_"},
		{"E_A_B", TOKEN_IDENTIFIER, "E_A_B"},
		{"E_", TOKEN_IDENTIFIER, "E_"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			l := NewLexer(tt.input + ";")
			tok := l.NextToken()
			if tok.Type != tt.want || tok.Value != tt.value {
				t.Fatalf("Lexer(%s) = %s %q, want %s %q", tt.input, tok.Type, tok.Value, tt.want, tt.value)
			}
			if next := l.NextToken(); next.Type != TOKEN_SEMICOLON {
				t.Fatalf("Lexer(%s) left %s %q before the semicolon", tt.input, next.Type, next.Value)
			}
		})
	}
}

func TestParseIssue893UnknownErrorPrefixIsVariable(t *testing.T) {
	src := "typeof(this) == OBJ || raise(E_IN3ARG);\n#13986.keep = this;\n"
	if _, err := NewParser(src).ParseProgram(); err != nil {
		t.Fatalf("ParseProgram(%q) = %v, want success (Toast compiles it and raises E_INVIND)", src, err)
	}
}
