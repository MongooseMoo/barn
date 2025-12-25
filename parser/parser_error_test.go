package parser

import (
	"barn/types"
	"testing"
)

func TestParseErrorLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected types.ErrorCode
	}{
		{"E_NONE", types.E_NONE},
		{"E_TYPE", types.E_TYPE},
		{"E_DIV", types.E_DIV},
		{"E_PERM", types.E_PERM},
		{"E_PROPNF", types.E_PROPNF},
		{"E_VERBNF", types.E_VERBNF},
		{"E_VARNF", types.E_VARNF},
		{"E_INVIND", types.E_INVIND},
		{"E_RECMOVE", types.E_RECMOVE},
		{"E_MAXREC", types.E_MAXREC},
		{"E_RANGE", types.E_RANGE},
		{"E_ARGS", types.E_ARGS},
		{"E_NACC", types.E_NACC},
		{"E_INVARG", types.E_INVARG},
		{"E_QUOTA", types.E_QUOTA},
		{"E_FLOAT", types.E_FLOAT},
		{"E_FILE", types.E_FILE},
		{"E_EXEC", types.E_EXEC},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			errVal, ok := val.(types.ErrValue)
			if !ok {
				t.Fatalf("expected ErrValue, got %T", val)
			}

			if errVal.Code() != tt.expected {
				t.Errorf("expected code %v, got %v", tt.expected, errVal.Code())
			}

			// Check type
			if errVal.Type() != types.TYPE_ERR {
				t.Errorf("expected type TYPE_ERR, got %v", errVal.Type())
			}

			// Check string representation
			if errVal.String() != tt.input {
				t.Errorf("expected String() %q, got %q", tt.input, errVal.String())
			}
		})
	}
}

func TestErrorTruthy(t *testing.T) {
	// All errors are truthy
	tests := []string{
		"E_NONE",
		"E_TYPE",
		"E_DIV",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			p := NewParser(input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			if !val.Truthy() {
				t.Error("errors should always be truthy")
			}
		})
	}
}

func TestErrorEqual(t *testing.T) {
	e1 := types.NewErr(types.E_TYPE)
	e2 := types.NewErr(types.E_TYPE)
	e3 := types.NewErr(types.E_DIV)

	if !e1.Equal(e2) {
		t.Error("same error codes should be equal")
	}

	if e1.Equal(e3) {
		t.Error("different error codes should not be equal")
	}

	// Test cross-type equality
	i := types.NewInt(1)
	if e1.Equal(i) {
		t.Error("error should not equal int")
	}
}
