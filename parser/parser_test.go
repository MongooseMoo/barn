package parser

import (
	"barn/types"
	"testing"
)

func TestParseIntegerLiterals(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"42", 42},
		{"-5", -5},
		{"0", 0},
		{"9223372036854775807", 9223372036854775807},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p := NewParser(tt.input)
			val, err := p.ParseLiteral()
			if err != nil {
				t.Fatalf("ParseLiteral() error = %v", err)
			}

			intVal, ok := val.(types.IntValue)
			if !ok {
				t.Fatalf("ParseLiteral() returned %T, want IntValue", val)
			}

			if intVal.Val != tt.want {
				t.Errorf("ParseLiteral() = %d, want %d", intVal.Val, tt.want)
			}
		})
	}
}

func TestIntValueTruthiness(t *testing.T) {
	tests := []struct {
		val   int64
		truthy bool
	}{
		{0, false},
		{1, true},
		{-1, true},
		{42, true},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.val)), func(t *testing.T) {
			intVal := types.NewInt(tt.val)
			if intVal.Truthy() != tt.truthy {
				t.Errorf("IntValue(%d).Truthy() = %v, want %v", tt.val, intVal.Truthy(), tt.truthy)
			}
		})
	}
}

func TestIntValueString(t *testing.T) {
	tests := []struct {
		val  int64
		want string
	}{
		{42, "42"},
		{-5, "-5"},
		{0, "0"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			intVal := types.NewInt(tt.val)
			if intVal.String() != tt.want {
				t.Errorf("IntValue(%d).String() = %s, want %s", tt.val, intVal.String(), tt.want)
			}
		})
	}
}

func TestIntValueEqual(t *testing.T) {
	tests := []struct {
		a     int64
		b     int64
		equal bool
	}{
		{42, 42, true},
		{42, 43, false},
		{0, 0, true},
		{-5, -5, true},
	}

	for _, tt := range tests {
		intA := types.NewInt(tt.a)
		intB := types.NewInt(tt.b)
		if intA.Equal(intB) != tt.equal {
			t.Errorf("IntValue(%d).Equal(IntValue(%d)) = %v, want %v", tt.a, tt.b, intA.Equal(intB), tt.equal)
		}
	}
}
