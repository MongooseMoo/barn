package builtins

import (
	"math"
	"math/big"
)

// First-level paths. Each is a lean version of the double-double path in
// math_exp.go / math_cr.go, accurate to about 2^-65 instead of 2^-75: the
// leading products and sums are still exact (two-product, two-sum), but the
// smaller terms are summed in float64 without renormalising. Each returns the
// float64 and whether Ziv's test proved it correctly rounded under its error
// bound; on false (about 1 call in 3,000 on the sweeps) the caller runs the
// full double-double path, and that one falls back to math/big.

// roundBits returns v rounded to the nearest integer, as a float64 and as an
// int, for |v| < 2^51: v + 1.5*2^52 has the integer in its low mantissa bits.
func roundBits(v float64) (float64, int) {
	u := v + roundToInt
	return u - roundToInt, int(int64(math.Float64bits(u)) - int64(math.Float64bits(roundToInt)))
}

// expFast is e^x for -707.7 < x < 709.79 (normal results), else ok = false.
// x = k ln2/64 + r as in mathExp; e^r = 1 + rh + q2 + rl(1 + rh) with
// q2 = rh^2/2 + ... + rh^7/5040 (the rest is below 2^-75) in float64.
func expFast(x float64) (float64, bool) {
	if !(x > -707.7 && x < 709.79) {
		return 0, false
	}
	const invLn2N = expTableSize / math.Ln2
	kf, k := roundBits(x * invLn2N)
	j := k & (expTableSize - 1)
	m := k >> 6
	rh, rl := twoSum(math.FMA(-kf, expLn2NHi, x), -kf*expLn2NLo)
	r2 := rh * rh
	q2 := r2 * (0.5 + rh*(1.0/6+rh*(1.0/24+rh*(1.0/120+rh*(1.0/720+rh*(1.0/5040))))))
	T := expTable[j]
	a, ae := twoProd(T[0], rh)
	h, e := twoSum(T[0], a)
	lo := (e + ae) + (T[0]*(q2+rl*(1+rh)) + T[1]*(1+rh))
	r, ok := zivRound(h, lo, 0x1p-49*math.Abs(lo)+0x1p-100*h)
	if !ok {
		return 0, false
	}
	if m > 1023 {
		return r * 2 * math.Float64frombits(uint64(m-1+1023)<<52), true
	}
	return r * math.Float64frombits(uint64(m+1023)<<52), true
}

// logFast returns ln x as hi + lo with an error bound for a finite x > 0,
// x != 1, by the reduction of logDD: ln x = e ln2 - ln r + ln(1 + z), with
// e ln2 - ln r + zh summed exactly and ln(1+z) - zh = -z^2/2 + ... - z^10/10
// (the rest is below 2^-77 |z|) in float64.
func logFast(x float64) (hi, lo, err float64) {
	e := 0
	if x < minNormal {
		x *= 0x1p54
		e = -54
	}
	bits := math.Float64bits(x)
	e += int(bits>>52) - 1023
	m := math.Float64frombits(bits&(1<<52-1) | 1023<<52)
	if m > math.Sqrt2 {
		m *= 0.5
		e++
	}
	_, j := roundBits(m * 128)
	j -= logTableMin
	p, zl := twoProd(m, logInv[j])
	zh := p - 1 // exact: p is within 2^-7 of 1
	z2 := zh * zh
	z4 := z2 * z2
	// Estrin's scheme for -z^2/2 + z^3/3 - ... - z^10/10.
	poly := z2*(-0.5+zh*(1.0/3)) + z4*((-1.0/4+zh*(1.0/5))+z2*(-1.0/6+zh*(1.0/7))) +
		z4*z4*((-1.0/8+zh*(1.0/9))+z2*(-1.0/10))
	lt := logTable[j]
	ef := float64(e)
	a, ae := twoProd(ef, ln2DD[0])
	s1, e1 := twoSum(a, lt[0])
	hi, e2 := twoSum(s1, zh)
	lo = (e1 + e2) + (ae + ef*ln2DD[1] + lt[1]) + (zl*(1-zh+z2) + poly)
	err = 0x1p-49*math.Abs(poly) + math.Abs(zl*zh)*z2 + 0x1p-90*math.Abs(hi) + 0x1p-77*math.Abs(zh)
	return hi, lo, err
}

// trigReduce writes x = k pi/2 + r0 + rl for |x| < 2^30 (|r0| <= pi/4 plus
// rounding). r0 = x - k P0 is exact in one fused multiply-add; rl = -k P1 is
// not normalised against r0 while |k| <= 2^14 (|rl| < 2^-40, so its square
// is below 2^-80), and is folded in by a two-sum above that.
func trigReduce(x float64) (kf float64, k int, r0, rl float64) {
	kf, k = roundBits(x * (2 / math.Pi))
	r0 = math.FMA(-kf, halfPiDD[0], x)
	rl = -kf * halfPiDD[1]
	if math.Abs(kf) > 0x1p14 {
		r0, rl = twoSum(r0, rl)
	}
	return kf, k, r0, rl
}

// trigFast is sin(x + shift pi/2) for 0 < |x| < 2^30 (shift 0: sin, 1: cos).
// With x = k pi/2 + r, |r| = c + t + rl, c = i/64, |t| <= 1/128:
// sin(c+t+rl) = A + B t + A (cos t - 1) + B (sin t - t) + rl (B cos t - A t)
// with (A, B) = (sin c, cos c); cos(c+t+rl) is the same with
// (A, B) = (cos c, -sin c). B t and A + B t are exact; the rest is float64.
func trigFast(x float64, shift int) (float64, bool) {
	kf, k, r, rl := trigReduce(x)
	k += shift
	neg := false
	if r < 0 {
		r, rl, neg = -r, -rl, true
	}
	c, i := roundBits(r * 64)
	t := r - c*(1.0/64)
	t2 := t * t
	s3 := t * t2 * (-1.0/6 + t2*(1.0/120-t2*(1.0/5040)))
	c2 := t2 * (-0.5 + t2*(1.0/24-t2*(1.0/720)))
	row := &trigTable[i]
	a0, a1, b0, b1 := row[0], row[1], row[2], row[3]
	if k&1 != 0 {
		a0, a1, b0, b1 = b0, b1, -a0, -a1
		neg = false // cos is even
	}
	p, pe := twoProd(b0, t)
	h, e := twoSum(a0, p)
	lo := (e + pe) + (a0*c2 + b0*s3 + rl*(b0*(1+c2)-a0*t)) + (a1 + b1*t)
	// The table's rounding is below 2^-100 |h| (|h| > 2^-7.1 when i > 0);
	// pi/2's dropped terms times k are below |k| 2^-105.
	err := 0x1p-49*math.Abs(lo) + 0x1p-100*math.Abs(h) + 0x1p-105*math.Abs(kf)
	if neg != (k&2 != 0) {
		h, lo = -h, -lo
	}
	return zivRound(h, lo, err)
}

// tanFast is tan x for 0 < |x| < 2^30: sin r and cos r as in trigFast, then
// sin r / cos r (even k) or -cos r / sin r (odd k) with one division.
func tanFast(x float64) (float64, bool) {
	kf, k, rh, rl := trigReduce(x)
	neg := false
	if rh < 0 {
		rh, rl, neg = -rh, -rl, true
	}
	c, i := roundBits(rh * 64)
	t := rh - c*(1.0/64)
	t2 := t * t
	s3 := t * t2 * (-1.0/6 + t2*(1.0/120-t2*(1.0/5040)))
	c2 := t2 * (-0.5 + t2*(1.0/24-t2*(1.0/720)))
	row := &trigTable[i]
	S, C := [2]float64{row[0], row[1]}, [2]float64{row[2], row[3]}
	p, pe := twoProd(C[0], t)
	sh, e := twoSum(S[0], p)
	sl := (e + pe) + (S[0]*c2 + C[0]*s3 + rl*(C[0]*(1+c2)-S[0]*t)) + (S[1] + C[1]*t)
	q, qe := twoProd(-S[0], t)
	ch, f := twoSum(C[0], q)
	cl := (f + qe) + (C[0]*c2 - S[0]*s3 - rl*(S[0]*(1+c2)+C[0]*t)) + (C[1] - S[1]*t)
	serr := 0x1p-49*math.Abs(sl) + 0x1p-105*math.Abs(kf)
	if i > 0 {
		serr += 0x1p-100
	}
	cerr := 0x1p-49*math.Abs(cl) + 0x1p-100 + 0x1p-105*math.Abs(kf)
	// The quotient below divides by the leading part only, so both parts are
	// renormalised first.
	sh, sl = fastTwoSum(sh, sl)
	ch, cl = fastTwoSum(ch, cl)
	nh, nl, nerr, dh, dl, derr := sh, sl, serr, ch, cl, cerr
	if k&1 != 0 {
		nh, nl, nerr, dh, dl, derr = -ch, -cl, cerr, sh, sl, serr
	}
	if nh == 0 || dh == 0 {
		return 0, false
	}
	inv := 1 / dh
	qh := nh * inv
	ql := (math.FMA(-qh, dh, nh) + nl - qh*dl) * inv
	err := math.Abs(qh) * (nerr/math.Abs(nh) + derr/math.Abs(dh) + 0x1p-98)
	if neg {
		qh, ql = -qh, -ql
	}
	return zivRound(qh, ql, err)
}

// asinTableMax is the last row of asinTable: sin(34/64) > 1/2 + 2^-11.
const asinTableMax = 34

// asinTable[i] holds, for y_i = i/64 and c = sin y_i, the Taylor coefficients
// of asin at c: asin(c + d) = y_i + a1 d + a2 d^2 + ... + a12 d^12, as
// {a1 hi, a1 lo, a2, ..., a12}. Row 1 is unused (row 0 covers s < 3/128).
var asinTable [asinTableMax + 1][13]float64

// initAsinRow fills asinTable[i] from sin y_i and cos y_i. a1 = 1/cos y_i,
// a2 = c a1 / (2 cos^2 y_i), and from (1 - x^2) f” = x f' differentiated n
// times, a(n+2) = ((2n+1)(n+1) c a(n+1) + n^2 a(n)) / ((n+2)(n+1) cos^2 y_i).
func initAsinRow(i int, sinY, cosY *big.Float) {
	a := make([]*big.Float, 13)
	a[0] = newTableFloat()
	a[1] = newTableFloat().Quo(newTableFloat().SetInt64(1), cosY)
	cos2 := newTableFloat().Mul(cosY, cosY)
	for n := int64(0); n+2 <= 12; n++ {
		v := newTableFloat().Mul(sinY, a[n+1])
		v.Mul(v, newTableFloat().SetInt64((2*n+1)*(n+1)))
		v.Add(v, newTableFloat().Mul(a[n], newTableFloat().SetInt64(n*n)))
		v.Quo(v, newTableFloat().Mul(cos2, newTableFloat().SetInt64((n+2)*(n+1))))
		a[n+2] = v
	}
	a1 := splitDD(a[1])
	asinTable[i][0], asinTable[i][1] = a1[0], a1[1]
	for n := 2; n <= 12; n++ {
		asinTable[i][n], _ = a[n].Float64()
	}
}

// asinFast returns asin s for 0 <= s <= 1/2 + 2^-50 given as sh + sl, as
// hi + lo with an error bound, without square roots or divisions:
// y_i = i/64 is the table point nearest asin s (from asin s ~ s + s^3/6 +
// 3s^5/40, within 2^-11), d = s - sin y_i (|d| < 2^-6.8; sh - sin y_i's
// leading part is exact for i >= 2 by Sterbenz's lemma, and row 0 is used
// for i = 1), and asin s = y_i + a1 d + d^2 (a2 + ... + a12 d^10), whose
// terms fall by about 2^-5.9 each, so the rest is below 2^-70.
func asinFast(sh, sl float64) (hi, lo, err float64) {
	s2 := sh * sh
	_, i := roundBits(sh * (1 + s2*(1.0/6+s2*(3.0/40))) * 64)
	if i == 1 {
		i = 0
	}
	row := &trigTable[i]
	dh := sh - row[0]
	dl := sl - row[1]
	c := &asinTable[i]
	d2 := dh * dh
	d4 := d2 * d2
	d8 := d4 * d4
	poly := d2 * ((c[2] + dh*c[3]) + d2*(c[4]+dh*c[5]) + d4*((c[6]+dh*c[7])+d2*(c[8]+dh*c[9])) +
		d8*((c[10]+dh*c[11])+d2*c[12]))
	p, pe := twoProd(c[0], dh)
	y := float64(i) * (1.0 / 64)
	h, e := twoSum(y, p)
	// d = dh + dl: the d^2 and d^3 terms' first-order part in dl is
	// dl dh (2 a2 + 3 a3 dh); the rest is below |dl| 2^-20.
	lo = (e + pe) + (c[0]*dl + c[1]*dh + dl*dh*(2*c[2]+3*c[3]*dh)) + poly
	err = 0x1p-49*math.Abs(lo) + 0x1p-100*math.Abs(h) + 0x1p-20*math.Abs(dl)
	return h, lo, err
}
