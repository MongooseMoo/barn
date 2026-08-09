package parser

import (
	"github.com/MongooseMoo/barn/verb"
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
			lit := parseLiteralForTest(t, tt.input)
			if lit.Kind != verb.LiteralBool {
				t.Fatalf("literal kind = %v, want LiteralBool", lit.Kind)
			}
			if lit.BoolValue != tt.expected {
				t.Errorf("BoolValue = %v, want %v", lit.BoolValue, tt.expected)
			}
		})
	}
}
