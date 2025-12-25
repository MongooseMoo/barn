package parser

import (
	"barn/types"
	"testing"
)

func TestParseObjectLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected types.ObjID
	}{
		{"#0", 0},
		{"#1", 1},
		{"#42", 42},
		{"#123", 123},
		{"#-1", -1},  // NOTHING
		{"#-2", -2},  // AMBIGUOUS
		{"#-3", -3},  // FAILED_MATCH
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			obj, ok := val.(types.ObjValue)
			if !ok {
				t.Fatalf("expected ObjValue, got %T", val)
			}

			if obj.ID() != tt.expected {
				t.Errorf("expected ID %v, got %v", tt.expected, obj.ID())
			}

			// Check type
			if obj.Type() != types.TYPE_OBJ {
				t.Errorf("expected type TYPE_OBJ, got %v", obj.Type())
			}

			// Check string representation
			if obj.String() != tt.input {
				t.Errorf("expected String() %q, got %q", tt.input, obj.String())
			}
		})
	}
}

func TestObjectTruthy(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"#0", true},
		{"#1", true},
		{"#42", true},
		{"#-1", false}, // NOTHING is falsy
		{"#-2", true},  // AMBIGUOUS is truthy
		{"#-3", true},  // FAILED_MATCH is truthy
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

func TestObjectEqual(t *testing.T) {
	o1 := types.NewObj(42)
	o2 := types.NewObj(42)
	o3 := types.NewObj(43)

	if !o1.Equal(o2) {
		t.Error("same object IDs should be equal")
	}

	if o1.Equal(o3) {
		t.Error("different object IDs should not be equal")
	}

	// Test cross-type equality
	i := types.NewInt(42)
	if o1.Equal(i) {
		t.Error("object should not equal int")
	}
}

func TestSpecialObjectConstants(t *testing.T) {
	nothing := types.NewObj(types.NOTHING)
	ambiguous := types.NewObj(types.AMBIGUOUS)
	failedMatch := types.NewObj(types.FAILED_MATCH)

	if nothing.ID() != -1 {
		t.Errorf("NOTHING should be -1, got %v", nothing.ID())
	}

	if ambiguous.ID() != -2 {
		t.Errorf("AMBIGUOUS should be -2, got %v", ambiguous.ID())
	}

	if failedMatch.ID() != -3 {
		t.Errorf("FAILED_MATCH should be -3, got %v", failedMatch.ID())
	}
}
