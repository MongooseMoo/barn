package parser

import (
	"github.com/MongooseMoo/barn/verb"
	"testing"
)

func TestParseStringLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello"`, "hello"},
		{`""`, ""},
		{`"with \"quotes\""`, `with "quotes"`},
		{`"line\nbreak"`, "linenbreak"},
		{`"tab\there"`, "tabthere"},
		{`"backslash\\"`, `backslash\`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lit := parseLiteralForTest(t, tt.input)
			if lit.Kind != verb.LiteralString {
				t.Fatalf("literal kind = %v, want LiteralString", lit.Kind)
			}
			if lit.StringValue != tt.expected {
				t.Errorf("StringValue = %q, want %q", lit.StringValue, tt.expected)
			}
		})
	}
}
