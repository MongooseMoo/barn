package builtins

import "math"

// Go's amd64 math.Exp and math.Log are assembly (src/math/exp_amd64.s,
// log_amd64.s). Exp returns +Inf once round(x*log2(e)) reaches 1024, from x of
// about 709.44, although e^x stays finite up to about 709.78; Log takes the
// exponent straight from the bits, so a subnormal argument is treated as if
// its exponent were -1022. The helpers below route around exactly those two
// ranges and use the Go result everywhere else.

// minNormal is the smallest positive normal float64, 2^-1022.
const minNormal = 0x1p-1022

// mathExp returns e^x.
func mathExp(x float64) float64 {
	// Toast delegates to the platform libm exp(), whose last-bit result
	// differs from Go's architecture-specific Exp for some inputs (for example
	// 1.5). Exp2(x*log2(e)) follows the same correctly rounded values for
	// Toast's observable 15-digit float formatting while remaining pure Go.
	result := math.Exp2(x * math.Log2E)
	if result < minNormal {
		// Exp2 reaches zero early (at x*log2(e) < -1075 after rounding the
		// product), so the subnormal range uses math.Exp, which is finite and
		// gradual there: e^-745 is the smallest subnormal, e^-746 is zero.
		return math.Exp(x)
	}
	return result
}

// halfExpSquared returns e^a / 2 for a > 709 as (e^(a/2)/2) * e^(a/2): a/2 is
// exact, e^(a/2) is far from Go's early overflow, and the product overflows
// only when the true value exceeds the float range.
func halfExpSquared(a float64) float64 {
	w := math.Exp(a / 2)
	return float64(0.5*w) * w
}

// mathSinh returns sinh(x). For |x| > 709, sinh(x) = sign(x)*(e^|x| -
// e^-|x|)/2, and e^-|x| is below 2^-2000 of e^|x|, so it is e^|x|/2 to
// double precision.
func mathSinh(x float64) float64 {
	if a := math.Abs(x); a > 709 {
		return math.Copysign(halfExpSquared(a), x)
	}
	return math.Sinh(x)
}

// mathCosh returns cosh(x); above |x| = 709 it is e^|x|/2 to double precision,
// as for sinh.
func mathCosh(x float64) float64 {
	if a := math.Abs(x); a > 709 {
		return halfExpSquared(a)
	}
	return math.Cosh(x)
}

// mathLog returns ln(x). A subnormal x is written x = m * 2^e with m in
// [0.5, 1) (math.Frexp normalises subnormals), so ln(x) = ln(m) + e*ln(2).
func mathLog(x float64) float64 {
	if x > 0 && x < minNormal {
		m, e := math.Frexp(x)
		return math.Log(m) + float64(float64(e)*math.Ln2)
	}
	return math.Log(x)
}

// mathLog10 returns log10(x); for a subnormal x = m * 2^e it is
// log10(m) + e*log10(2).
func mathLog10(x float64) float64 {
	if x > 0 && x < minNormal {
		const log10Of2 = math.Ln2 / math.Ln10
		m, e := math.Frexp(x)
		return math.Log10(m) + float64(float64(e)*log10Of2)
	}
	return math.Log10(x)
}
