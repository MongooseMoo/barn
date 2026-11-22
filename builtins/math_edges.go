package builtins

import "math"

// Go's amd64 math.Exp is assembly (src/math/exp_amd64.s) and returns +Inf once
// round(x*log2(e)) reaches 1024, from x of about 709.44, although e^x stays
// finite up to about 709.78. Go's math.Sinh and math.Cosh use it above
// |x| = 21. exp, log, log10, sin, cos, tan, asin and acos are correctly
// rounded in math_exp.go and math_cr.go.

// minNormal is the smallest positive normal float64, 2^-1022.
const minNormal = 0x1p-1022

// halfExp returns e^a / 2 for a >= 22 from the correctly rounded exp: e^a/2
// exactly halved while e^a is finite, and above that (e^(a/2)/2) * e^(a/2),
// where a/2 is exact and the product overflows only past the float range.
func halfExp(a float64) float64 {
	if e := mathExp(a); !math.IsInf(e, 0) {
		return 0.5 * e
	}
	w := mathExp(a / 2)
	return float64(0.5*w) * w
}

// mathSinh returns sinh(x). For |x| >= 22, sinh(x) = sign(x)*(e^|x| -
// e^-|x|)/2 and e^-|x| is below 2^-63 of e^|x|, so it is e^|x|/2 to double
// precision.
func mathSinh(x float64) float64 {
	if a := math.Abs(x); a >= 22 {
		return math.Copysign(halfExp(a), x)
	}
	return math.Sinh(x)
}

// mathCosh returns cosh(x); for |x| >= 22 it is e^|x|/2 to double precision,
// as for sinh.
func mathCosh(x float64) float64 {
	if a := math.Abs(x); a >= 22 {
		return halfExp(a)
	}
	return math.Cosh(x)
}
