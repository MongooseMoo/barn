package parser

import (
	"barn/types"
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
		{`"line\nbreak"`, "line\nbreak"},
		{`"tab\there"`, "tab\there"},
		{`"backslash\\"`, `backslash\`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			str, ok := val.(types.StrValue)
			if !ok {
				t.Fatalf("expected StrValue, got %T", val)
			}

			if str.Value() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, str.Value())
			}

			// Check type
			if str.Type() != types.TYPE_STR {
				t.Errorf("expected type TYPE_STR, got %v", str.Type())
			}
		})
	}
}

func TestStringTruthy(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`"hello"`, true},
		{`""`, false},
		{`" "`, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			if val.Truthy() != tt.expected {
				t.Errorf("expected truthy=%v, got %v", tt.expected, val.Truthy())
			}
		})
	}
}

func TestStringEqual(t *testing.T) {
	s1 := types.NewStr("hello")
	s2 := types.NewStr("hello")
	s3 := types.NewStr("world")

	if !s1.Equal(s2) {
		t.Error("identical strings should be equal")
	}

	if s1.Equal(s3) {
		t.Error("different strings should not be equal")
	}

	// Test cross-type equality
	i := types.NewInt(42)
	if s1.Equal(i) {
		t.Error("string should not equal int")
	}
}
