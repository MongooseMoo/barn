package vm

import (
	"math"
	"testing"
)

// Integer division, modulus and power edges as ToastStunt 2.8 numbers.cc
// defines them: do_divide negates with wrap for any divisor of -1, do_modulus
// returns 0 for -1 and never overflows, and do_power gives (-1) ^ n by the
// exact parity of n for negative n.
func TestIntDivisionEdgesMatchToast28(t *testing.T) {
	cases := []struct {
		code string
		want int64
	}{
		{"return -9223372036854775807 / -1;", math.MaxInt64},
		{"x = -9223372036854775808; return x / -1;", math.MinInt64},
		{"return 9223372036854775807 / -1;", -math.MaxInt64},
		{"x = -9223372036854775808; return x % -1;", 0},
		{"return -9223372036854775807 % -1;", 0},
		{"return 9223372036854775806 % 9223372036854775807;", 9223372036854775806},
		{"return -9223372036854775806 % -9223372036854775807;", -9223372036854775806},
		{"d = -9223372036854775808; return 5 % d;", -9223372036854775803},
		{"x = -9223372036854775808; return -x;", math.MinInt64},
		{"return (-1) ^ -3;", -1},
		{"return (-1) ^ -4;", 1},
		{"return (-1) ^ -9223372036854775807;", -1},
		{"m = -9223372036854775808; return (-1) ^ m;", 1},
		{"return (-1) ^ -9007199254740993;", -1},
		{"return 1 ^ -9223372036854775807;", 1},
		{"return (-2) ^ -1;", 0},
	}
	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			requireInt(t, runBytecodeProgram(t, c.code, nil, strictCtx()), c.want)
		})
	}
}
