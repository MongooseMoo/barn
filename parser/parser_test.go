package parser

import (
	"barn/verb"
	"testing"
)

func TestParseIntegerLiterals(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"42", 42},
		{"5", 5}, // MOO has no negative literals; "-5" is unary minus (see TestParseUnaryMinus)
		{"0", 0},
		{"9223372036854775807", 9223372036854775807},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lit := parseLiteralForTest(t, tt.input)
			if lit.Kind != verb.LiteralInt {
				t.Fatalf("literal kind = %v, want LiteralInt", lit.Kind)
			}
			if lit.IntValue != tt.want {
				t.Errorf("IntValue = %d, want %d", lit.IntValue, tt.want)
			}
		})
	}
}

func TestParseFloatLiterals(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{".5", 0.5},
		{"1.", 1.0},
		{"3.14", 3.14},
		{"0.5", 0.5}, // "-0.5" is unary minus on 0.5; not a literal (B1)
		{"1e10", 1e10},
		{"1E-5", 1e-5},
		{"3.14e+2", 3.14e+2},
		{"1.5e-3", 1.5e-3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lit := parseLiteralForTest(t, tt.input)
			if lit.Kind != verb.LiteralFloat {
				t.Fatalf("literal kind = %v, want LiteralFloat", lit.Kind)
			}
			if lit.FloatValue != tt.want {
				t.Errorf("FloatValue = %f, want %f", lit.FloatValue, tt.want)
			}
		})
	}
}
