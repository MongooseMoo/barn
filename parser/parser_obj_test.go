package parser

import (
	"barn/verb"
	"testing"
)

func TestParseObjectLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"#0", 0},
		{"#1", 1},
		{"#42", 42},
		{"#123", 123},
		{"#-1", -1}, // NOTHING
		{"#-2", -2}, // AMBIGUOUS
		{"#-3", -3}, // FAILED_MATCH
		{"#-9223372036854775807", -9223372036854775807},
		{"#9223372036854775807", 9223372036854775807},
		{"#-9223372036854775808", -9223372036854775808},
		{"#9223372036854775808", -9223372036854775808},
		{"#-9223372036854775809", 9223372036854775807},
		{"#9223372036854775809", -9223372036854775807},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lit := parseLiteralForTest(t, tt.input)
			if lit.Kind != verb.LiteralObj {
				t.Fatalf("literal kind = %v, want LiteralObj", lit.Kind)
			}
			if lit.ObjID != tt.expected {
				t.Errorf("ObjID = %d, want %d", lit.ObjID, tt.expected)
			}
		})
	}
}
