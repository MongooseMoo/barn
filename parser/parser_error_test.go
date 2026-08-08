package parser

import (
	"github.com/MongooseMoo/barn/verb"
	"testing"
)

func TestParseErrorLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"E_NONE", "E_NONE"},
		{"E_TYPE", "E_TYPE"},
		{"E_DIV", "E_DIV"},
		{"E_PERM", "E_PERM"},
		{"E_PROPNF", "E_PROPNF"},
		{"E_VERBNF", "E_VERBNF"},
		{"E_VARNF", "E_VARNF"},
		{"E_INVIND", "E_INVIND"},
		{"E_RECMOVE", "E_RECMOVE"},
		{"E_MAXREC", "E_MAXREC"},
		{"E_RANGE", "E_RANGE"},
		{"E_ARGS", "E_ARGS"},
		{"E_NACC", "E_NACC"},
		{"E_INVARG", "E_INVARG"},
		{"E_QUOTA", "E_QUOTA"},
		{"E_FLOAT", "E_FLOAT"},
		{"E_FILE", "E_FILE"},
		{"E_EXEC", "E_EXEC"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lit := parseLiteralForTest(t, tt.input)
			if lit.Kind != verb.LiteralErr {
				t.Fatalf("literal kind = %v, want LiteralErr", lit.Kind)
			}
			if lit.ErrorName != tt.expected {
				t.Errorf("ErrorName = %q, want %q", lit.ErrorName, tt.expected)
			}
		})
	}
}
