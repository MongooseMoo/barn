package parser

import (
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/verb"
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
		{"9223372036854775808", -9223372036854775808},
		{"18446744073709551616", 0},
		{"99999999999999999999", 7766279631452241919},
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

func TestParseMillionDigitIntegerWithoutPathologicalCost(t *testing.T) {
	input := "1" + strings.Repeat("0", 1_000_000-1)

	start := time.Now()
	lit := parseLiteralForTest(t, input)
	elapsed := time.Since(start)
	t.Logf("parsed one-million-digit integer in %v", elapsed)

	if lit.Kind != verb.LiteralInt {
		t.Fatalf("literal kind = %v, want LiteralInt", lit.Kind)
	}
	if lit.IntValue != 0 {
		t.Errorf("IntValue = %d, want 0", lit.IntValue)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("parsing a one-million-digit integer took %v, want at most 100ms", elapsed)
	}
}

func TestWrapMillionDigitIntegerUnderOneMillisecond(t *testing.T) {
	input := "1" + strings.Repeat("0", 1_000_000-2) + "1"

	start := time.Now()
	got := wrapInt64Literal(input)
	elapsed := time.Since(start)
	t.Logf("wrapped one-million-digit integer in %v", elapsed)

	if got != 1 {
		t.Errorf("wrapInt64Literal() = %d, want 1", got)
	}
	if elapsed >= time.Millisecond {
		t.Errorf("wrapping a one-million-digit integer took %v, want under 1ms", elapsed)
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
