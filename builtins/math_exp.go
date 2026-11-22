package builtins

import (
	"math"
	"math/big"
)

// mathExp returns e^x correctly rounded to the nearest float64 (ties to even),
// including subnormal results, overflow to +Inf and underflow to 0.
//
// Toast's exp is the host C library's. A correctly rounded exp agrees with the
// WSL oracle's (glibc 2.36) in the 15 displayed digits on all but 2 of a
// 27,732-argument sweep (agent-notes/fix-exp-displayed-digits.md); the
// remaining differences are rounding errors of that library's exp.
//
// The method is written from the mathematics, not from any C library:
//
//   - Argument reduction: x = (64m + j) ln2/64 + r with k = 64m + j the nearest
//     integer to 64x/ln2, so e^x = 2^m * 2^(j/64) * e^r with |r| <= ln2/128.
//     ln2/64 is split into a float64 hi part and a float64 lo part; x - k*hi is
//     exact in one fused multiply-add (the true difference is a multiple of
//     2^-60 below 2^-7 in magnitude, so it fits in 53 bits), and k*lo is
//     carried as a double-double.
//   - 2^(j/64) is a double-double table computed once with math/big
//     (initMathTables, math_cr.go).
//   - e^r - 1 is its Taylor series to degree 8 (truncation below 2^-86), with
//     the r and r^2/2 terms kept exactly by error-free transformations
//     (two-sum, and a two-product by fused multiply-add) and the r^3.. terms
//     in float64.
//   - The result's relative error is below 2^-75. When the float64 rounding of
//     the result is the same at both ends of a +-2^-70 interval it is the
//     correctly rounded value (Ziv's rounding test). Otherwise, and for every
//     subnormal result, expSlow decides the rounding with math/big.
func mathExp(x float64) float64 {
	switch {
	case x != x:
		return x
	case x > 709.79: // e^709.79 is past math.MaxFloat64
		return math.Inf(1)
	case x < -745.2: // e^-745.2 is below 2^-1075, half the smallest subnormal
		return 0
	}
	const invLn2N = expTableSize / math.Ln2
	kf := math.RoundToEven(x * invLn2N)
	k := int(kf)
	j := k & (expTableSize - 1)
	m := k >> 6 // floor(k/64)

	// r = x - k*ln2/64 as rh + rl.
	r0 := math.FMA(-kf, expLn2NHi, x)
	p := kf * expLn2NLo
	pe := math.FMA(kf, expLn2NLo, -p)
	rh, rl := twoSum(r0, -p)
	rh, rl = fastTwoSum(rh, rl-pe)

	// e^r - 1 = q as qh + ql.
	sq := rh * rh
	sqe := math.FMA(rh, rh, -sq)
	tail := float64(rh*sq) * (1.0/6 + rh*(1.0/24+rh*(1.0/120+rh*(1.0/720+rh*(1.0/5040+rh*(1.0/40320))))))
	s, se := twoSum(0.5*sq, tail)
	qh, qe := twoSum(rh, s)
	ql := qe + se + 0.5*sqe + rl + float64(rl*rh)

	// 2^(j/64) * (1 + q) as hi + lo.
	th, tl := expTable[j][0], expTable[j][1]
	a := th * qh
	ae := math.FMA(th, qh, -a)
	hi, e1 := twoSum(th, a)
	lo := e1 + ae + float64(th*ql) + tl + float64(tl*qh)

	if m < -1021 {
		// The result may be subnormal, where rounding is at a coarser
		// quantum than hi + lo's.
		return expSlow(x)
	}
	res := hi + lo
	eps := 0x1p-70 * hi
	if hi+(lo+eps) != res || hi+(lo-eps) != res {
		return expSlow(x)
	}
	if m > 1023 {
		return res * 2 * math.Float64frombits(uint64(m-1+1023)<<52)
	}
	return res * math.Float64frombits(uint64(m+1023)<<52)
}

const expTableSize = 64

var (
	// expTable[j] is 2^(j/64) as a double-double: the nearest float64 and the
	// nearest float64 to the remainder (initMathTables).
	expTable [expTableSize][2]float64
	// expLn2NHi + expLn2NLo is ln2/64 to about 2^-113 relative.
	expLn2NHi, expLn2NLo float64
)

// expSlow returns e^x correctly rounded with math/big (zivBig).
func expSlow(x float64) float64 {
	if x == 0 {
		return 1
	}
	return zivBig(func(prec uint) *big.Float { return expBig(new(big.Float).SetFloat64(x), prec) })
}
