package parser

import (
	"barn/types"
	"testing"
)

func TestParseBoolLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			boolVal, ok := val.(types.BoolValue)
			if !ok {
				t.Fatalf("expected BoolValue, got %T", val)
			}

			if boolVal.Val != tt.expected {
				t.Errorf("expected value %v, got %v", tt.expected, boolVal.Val)
			}

			// Check type
			if boolVal.Type() != types.TYPE_BOOL {
				t.Errorf("expected type TYPE_BOOL, got %v", boolVal.Type())
			}

			// Check string representation
			if boolVal.String() != tt.input {
				t.Errorf("expected String() %q, got %q", tt.input, boolVal.String())
			}
		})
	}
}

func TestBoolTruthy(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"false", false},
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

func TestBoolEqual(t *testing.T) {
	b1 := types.NewBool(true)
	b2 := types.NewBool(true)
	b3 := types.NewBool(false)

	if !b1.Equal(b2) {
		t.Error("identical bools should be equal")
	}

	if b1.Equal(b3) {
		t.Error("different bools should not be equal")
	}

	// Test cross-type equality
	i := types.NewInt(1)
	if b1.Equal(i) {
		t.Error("bool should not equal int")
	}
}
