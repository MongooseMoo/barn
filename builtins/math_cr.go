package builtins

import (
	"math"
	"math/big"
	"sync"
)

// Correctly rounded elementary functions.
//
// Toast's math builtins call the host C library. On the WSL oracle (glibc
// 2.36) exp, log, sin, cos, tan, asin and acos return the correctly rounded
// float64 on all but a handful of the arguments swept in
// agent-notes/fix-exp-displayed-digits.md, while Go's math differs on up to 14%
// of them in the 15 displayed digits. The functions here are written from the
// mathematics (no C library code is used or ported): each computes a
// double-double approximation (hi + lo, about 2^-70 or better) with an error
// bound, and accepts the float64 rounding of hi + lo when it is the same at
// both ends of the error interval (Ziv's rounding test). Otherwise the result
// is decided with math/big at increasing precision. For a float64 argument
// other than the trivial ones (exp(0), log(1), ...), these functions are
// transcendental numbers (Lindemann-Weierstrass), never a float64 or a
// midpoint between two, so that loop ends.

// twoSum returns s = fl(a+b) and the exact error e = a + b - s (Knuth).
func twoSum(a, b float64) (s, e float64) {
	s = a + b
	bb := s - a
	e = (a - (s - bb)) + (b - bb)
	return s, e
}

// fastTwoSum is twoSum for |a| >= |b| (Dekker).
func fastTwoSum(a, b float64) (s, e float64) {
	s = a + b
	e = b - (s - a)
	return s, e
}

// twoProd returns p = fl(a*b) and the exact error e = a*b - p.
func twoProd(a, b float64) (p, e float64) {
	p = a * b
	return p, math.FMA(a, b, -p)
}

// zivRound returns fl(hi+lo) and whether every value within err of hi + lo
// rounds to it. lo must be at most about one ulp of hi.
func zivRound(hi, lo, err float64) (float64, bool) {
	r := hi + lo
	return r, hi+(lo+err) == r && hi+(lo-err) == r
}

// zivBig returns the float64 nearest the value f computes. f(prec) must return
// the value with relative error below 2^-prec.
func zivBig(f func(prec uint) *big.Float) float64 {
	for prec := uint(128); ; prec *= 2 {
		v := f(prec)
		if v.Sign() == 0 {
			r, _ := v.Float64()
			return r
		}
		d := new(big.Float).SetMantExp(v, -int(prec))
		d.Abs(d)
		below, _ := new(big.Float).Sub(v, d).Float64()
		above, _ := new(big.Float).Add(v, d).Float64()
		if below == above || prec >= 1<<14 {
			r, _ := v.Float64()
			return r
		}
	}
}

// Double-double constants and tables, computed once with math/big from series
// whose terms are exact rationals (see initMathTables).
var (
	ln2DD     [2]float64 // ln 2
	invLn10DD [2]float64 // 1/ln 10
	halfPiDD  [4]float64 // pi/2 as four float64 terms (about 2^-210)

	// logTable[j-logTableMin] is ln(j/128) for j in [90, 181].
	logTable [logTableMax - logTableMin + 1][2]float64
	// sinTable[i], cosTable[i] are sin(i/64) and cos(i/64) for i in [0, 51].
	sinTable, cosTable [sinCosTableSize][2]float64
)

const (
	logTableMin     = 90
	logTableMax     = 181
	sinCosTableSize = 52
	tablePrec       = 256
)

func init() { initMathTables() }

func newTableFloat() *big.Float { return new(big.Float).SetPrec(tablePrec) }

func splitDD(v *big.Float) [2]float64 {
	hi, _ := v.Float64()
	lo, _ := newTableFloat().Sub(v, newTableFloat().SetFloat64(hi)).Float64()
	return [2]float64{hi, lo}
}

// atanhInv returns atanh(1/n) = sum over k >= 0 of 1/((2k+1) n^(2k+1)) at
// precision prec, for an integer n >= 2.
func atanhInv(n int64, prec uint) *big.Float {
	return inverseSeries(n, prec, false)
}

// atanInv returns atan(1/n) = sum over k >= 0 of (-1)^k/((2k+1) n^(2k+1)).
func atanInv(n int64, prec uint) *big.Float {
	return inverseSeries(n, prec, true)
}

func inverseSeries(n int64, prec uint, alternate bool) *big.Float {
	w := prec + 32
	sum := new(big.Float).SetPrec(w)
	pow := new(big.Float).SetPrec(w).Quo(new(big.Float).SetPrec(w).SetInt64(1), new(big.Float).SetPrec(w).SetInt64(n))
	n2 := new(big.Float).SetPrec(w).SetInt64(n * n)
	term := new(big.Float).SetPrec(w)
	div := new(big.Float).SetPrec(w)
	for k := int64(0); pow.MantExp(nil) > -int(w); k++ {
		term.Quo(pow, div.SetInt64(2*k+1))
		if alternate && k%2 == 1 {
			sum.Sub(sum, term)
		} else {
			sum.Add(sum, term)
		}
		pow.Quo(pow, n2)
	}
	return sum
}

func initMathTables() {
	// ln 2 = 2 atanh(1/3); ln 10 = 3 ln 2 + ln(5/4) = 3 ln 2 + 2 atanh(1/9).
	ln2 := newTableFloat().Mul(atanhInv(3, tablePrec), big.NewFloat(2))
	ln2DD = splitDD(ln2)
	ln10 := newTableFloat().Mul(atanhInv(9, tablePrec), big.NewFloat(2))
	ln10.Add(ln10, newTableFloat().Mul(ln2, big.NewFloat(3)))
	invLn10DD = splitDD(newTableFloat().Quo(newTableFloat().SetInt64(1), ln10))
	expLn2N := splitDD(newTableFloat().Quo(ln2, big.NewFloat(expTableSize)))
	expLn2NHi, expLn2NLo = expLn2N[0], expLn2N[1]

	// 2^(j/64): 2^(1/64) by six square roots of 2, then repeated products.
	root := newTableFloat().SetInt64(2)
	for range 6 {
		root.Sqrt(root)
	}
	pow := newTableFloat().SetInt64(1)
	for j := range expTableSize {
		expTable[j] = splitDD(pow)
		pow.Mul(pow, root)
	}

	// pi/2 = 2(4 atan(1/5) - atan(1/239)) (Machin).
	halfPi := newTableFloat().Mul(atanInv(5, tablePrec), big.NewFloat(4))
	halfPi.Sub(halfPi, atanInv(239, tablePrec))
	halfPi.Mul(halfPi, big.NewFloat(2))
	rest := newTableFloat().Set(halfPi)
	for i := range halfPiDD {
		halfPiDD[i], _ = rest.Float64()
		rest.Sub(rest, newTableFloat().SetFloat64(halfPiDD[i]))
	}

	// ln(j/128) by steps from ln(128/128) = 0: ln(j/(j-1)) = 2 atanh(1/(2j-1))
	// upward and ln(j/(j+1)) = -2 atanh(1/(2j+1)) downward.
	logs := make(map[int]*big.Float)
	cur := newTableFloat()
	logs[128] = newTableFloat().Set(cur)
	for j := 129; j <= logTableMax; j++ {
		cur.Add(cur, newTableFloat().Mul(atanhInv(int64(2*j-1), tablePrec), big.NewFloat(2)))
		logs[j] = newTableFloat().Set(cur)
	}
	cur.SetInt64(0)
	for j := 127; j >= logTableMin; j-- {
		// ln(j/(j+1)) = -2 atanh(1/(2j+1))
		cur.Sub(cur, newTableFloat().Mul(atanhInv(int64(2*j+1), tablePrec), big.NewFloat(2)))
		logs[j] = newTableFloat().Set(cur)
	}
	for j := logTableMin; j <= logTableMax; j++ {
		logTable[j-logTableMin] = splitDD(logs[j])
	}

	// sin and cos of i/64 by rotating (cos h, sin h), h = 1/64, from Taylor
	// series for h.
	h := newTableFloat().SetMantExp(big.NewFloat(1), -6)
	sinH, cosH := sinCosTaylor(h, tablePrec)
	s, c := newTableFloat(), newTableFloat().SetInt64(1)
	for i := range sinCosTableSize {
		sinTable[i], cosTable[i] = splitDD(s), splitDD(c)
		ns := newTableFloat().Mul(s, cosH)
		ns.Add(ns, newTableFloat().Mul(c, sinH))
		nc := newTableFloat().Mul(c, cosH)
		nc.Sub(nc, newTableFloat().Mul(s, sinH))
		s, c = ns, nc
	}
}

// sinCosTaylor returns sin r and cos r for 0 < |r| < 2 from their Taylor
// series: the sum stops once a term is below 2^-(w+4) min(|r|, 1), so sin r
// (at least |r|/2 there) has relative error about 2^-w and cos r absolute
// error about 2^-w (relative for |r| <= pi/4, where cos r > 0.7).
func sinCosTaylor(r *big.Float, w uint) (sin, cos *big.Float) {
	sin = new(big.Float).SetPrec(w).Set(r)
	cos = new(big.Float).SetPrec(w).SetInt64(1)
	term := new(big.Float).SetPrec(w).Set(r) // r^n/n!
	div := new(big.Float).SetPrec(w)
	stop := min(r.MantExp(nil), 1) - int(w) - 4
	for n := int64(2); term.Sign() != 0 && term.MantExp(nil) > stop; n += 2 {
		// cos term r^n/n!, then sin term r^(n+1)/(n+1)!
		cterm := new(big.Float).SetPrec(w).Quo(new(big.Float).SetPrec(w).Mul(term, r), div.SetInt64(n))
		term.Quo(new(big.Float).SetPrec(w).Mul(cterm, r), div.SetInt64(n+1))
		if (n/2)%2 == 1 {
			cos.Sub(cos, cterm)
			sin.Sub(sin, term)
		} else {
			cos.Add(cos, cterm)
			sin.Add(sin, term)
		}
	}
	return sin, cos
}

// ---- big slow paths -------------------------------------------------------

// expBig returns e^y with relative error below 2^-prec, as
// (e^(y/2^s))^(2^s) with |y/2^s| < 2^-8 so that the Taylor series converges
// quickly. Squaring s times multiplies the relative error by about 2^s, so the
// working precision carries s + 16 guard bits.
func expBig(y *big.Float, prec uint) *big.Float {
	ey := y.MantExp(nil) // |y| < 2^ey
	if y.Sign() == 0 {
		ey = 0
	}
	s := max(ey+8, 0)
	w := prec + uint(s) + 16
	z := new(big.Float).SetPrec(w).SetMantExp(y, -s)
	sum := new(big.Float).SetPrec(w).SetInt64(1)
	term := new(big.Float).SetPrec(w).SetInt64(1)
	n := new(big.Float).SetPrec(w)
	for i := int64(1); term.Sign() != 0 && term.MantExp(nil) >= -int(w); i++ {
		term.Mul(term, z)
		term.Quo(term, n.SetInt64(i))
		sum.Add(sum, term)
	}
	for range s {
		sum.Mul(sum, sum)
	}
	return sum
}

// logBig returns ln x for a finite x > 0, x != 1, with relative error below
// 2^-prec, by Newton's method on e^y = x (y <- y + x e^-y - 1, which squares
// the error each step) from Go's log. Each step's absolute error is about
// 2^-w; |ln x| >= 2^-54 for a float64 x != 1, so w = prec + 64 suffices.
func logBig(x float64, prec uint) *big.Float {
	w := prec + 64
	xb := new(big.Float).SetPrec(w).SetFloat64(x)
	m, e := math.Frexp(x)
	y := new(big.Float).SetPrec(w).SetFloat64(math.Log(m) + float64(e)*math.Ln2)
	one := new(big.Float).SetPrec(w).SetInt64(1)
	for bits := uint(20); bits < 2*w; bits *= 2 {
		t := new(big.Float).SetPrec(w).Mul(xb, expBig(new(big.Float).Neg(y), w))
		t.Sub(t, one)
		y.Add(y, t)
	}
	return y
}

var piCache struct {
	sync.Mutex
	halfPi *big.Float
}

// halfPiBig returns pi/2 to at least prec bits (Machin's formula), cached.
func halfPiBig(prec uint) *big.Float {
	piCache.Lock()
	defer piCache.Unlock()
	if piCache.halfPi == nil || piCache.halfPi.Prec() < prec {
		p := max(2*prec, 1024)
		v := new(big.Float).SetPrec(p).Mul(atanInv(5, p), big.NewFloat(4))
		v.Sub(v, atanInv(239, p))
		v.Mul(v, big.NewFloat(2))
		piCache.halfPi = v
	}
	return piCache.halfPi
}

// sinCosBig returns sin x and cos x for a finite x, each with relative error
// below 2^-prec. x = k pi/2 + r; the reduction's absolute error is about
// 2^(ex-w) for |x| < 2^ex, so r is accepted once it is larger than that by
// prec + 32 bits, raising the precision otherwise.
func sinCosBig(x float64, prec uint) (sin, cos *big.Float) {
	_, ex := math.Frexp(x)
	ex = max(ex, 0)
	for extra := uint(64); ; extra *= 2 {
		w := prec + extra + uint(ex)
		hp := halfPiBig(w + 8)
		xb := new(big.Float).SetPrec(w).SetFloat64(x)
		// k = floor(x/(pi/2) + 1/2); Int truncates toward zero.
		q := new(big.Float).SetPrec(w).Quo(xb, hp)
		k, acc := q.Add(q, big.NewFloat(0.5)).Int(nil)
		if acc == big.Above {
			k.Sub(k, bigOne)
		}
		r := new(big.Float).SetPrec(w).Mul(new(big.Float).SetPrec(w).SetInt(k), hp)
		r.Sub(xb, r)
		if r.Sign() != 0 && r.MantExp(nil) < ex+1+int(prec)+32-int(w) && extra < 1<<14 {
			continue
		}
		s, c := sinCosTaylor(r, w)
		switch new(big.Int).And(k, bigThree).Int64() {
		case 1:
			s, c = c, s.Neg(s)
		case 2:
			s, c = s.Neg(s), c.Neg(c)
		case 3:
			s, c = c.Neg(c), s
		}
		return s, c
	}
}

var (
	bigOne   = big.NewInt(1)
	bigThree = big.NewInt(3)
)

// asinBig returns asin x for |x| <= 1, x != 0, with relative error below
// 2^-prec, by Newton's method on sin y = x from Go's asin. Near |x| = 1,
// cos y is as small as 2^-27 for a float64 x, which costs that many bits; the
// 64 guard bits cover it.
func asinBig(x float64, prec uint) *big.Float {
	w := prec + 64
	if math.Abs(x) == 1 {
		v := new(big.Float).SetPrec(w).Set(halfPiBig(w))
		if x < 0 {
			v.Neg(v)
		}
		return v
	}
	xb := new(big.Float).SetPrec(w).SetFloat64(x)
	y := new(big.Float).SetPrec(w).SetFloat64(math.Asin(x))
	for bits := uint(20); bits < 2*w; bits *= 2 {
		s, c := sinCosTaylor(y, w)
		s.Sub(s, xb)
		s.Quo(s, c)
		y.Sub(y, s)
	}
	return y
}

// ---- log and log10 ----------------------------------------------------------

// logDD returns ln x for a finite x > 0, x != 1, as hi + lo with absolute
// error below err.
//
// x = 2^e m with m in [sqrt(1/2), sqrt(2)); F = j/128 is the nearest such
// fraction to m and u = (m - F)/F, |u| < 1/180, so
// ln x = e ln2 + ln F + ln(1 + u), with ln(1+u) = u - u^2/2 + u^3/3 - ...
// (truncated after u^10: the rest is below 2^-77 |u|). m - F is exact, u is
// carried as uh + ul, and u, u^2/2 are summed exactly; u^3.. are in float64.
func logDD(x float64) (hi, lo, err float64) {
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
	j := int(m*128 + 0.5)
	F := float64(j) * (1.0 / 128)
	f := m - F
	uh := f / F
	ul := math.FMA(-uh, F, f) / F
	sq, sqe := twoProd(uh, uh)
	poly := float64(uh*sq) * (1.0/3 + uh*(-1.0/4+uh*(1.0/5+uh*(-1.0/6+uh*(1.0/7+uh*(-1.0/8+uh*(1.0/9+uh*(-1.0/10))))))))
	lf := logTable[j-logTableMin]
	ef := float64(e)
	a, ae := twoProd(ef, ln2DD[0])
	s1, e1 := twoSum(a, lf[0])
	s2, e2 := twoSum(s1, uh)
	s3, e3 := twoSum(s2, -0.5*sq)
	lo = e1 + e2 + e3 + ae + ef*ln2DD[1] + lf[1] + ul/(1+uh) - 0.5*sqe + poly
	hi, lo = fastTwoSum(s3, lo)
	err = 0x1p-49*math.Abs(poly) + 0x1p-76*math.Abs(uh) + 0x1p-85*math.Abs(hi)
	return hi, lo, err
}

// mathLog returns ln x correctly rounded. Other than for finite x > 0 it is
// Go's math.Log (NaN, -Inf at 0, +Inf).
func mathLog(x float64) float64 {
	if !(x > 0) || x > math.MaxFloat64 {
		return math.Log(x)
	}
	if x == 1 {
		return 0
	}
	if r, ok := zivRound(logDD(x)); ok {
		return r
	}
	return zivBig(func(prec uint) *big.Float { return logBig(x, prec) })
}

// mathLog10 returns log10 x = ln x / ln 10 correctly rounded.
func mathLog10(x float64) float64 {
	if !(x > 0) || x > math.MaxFloat64 {
		return math.Log10(x)
	}
	if x == 1 {
		return 0
	}
	h, l, err := logDD(x)
	p, pe := twoProd(h, invLn10DD[0])
	hi, lo := fastTwoSum(p, pe+h*invLn10DD[1]+l*invLn10DD[0])
	if r, ok := zivRound(hi, lo, err*0.5+0x1p-100*math.Abs(hi)); ok {
		return r
	}
	return zivBig(func(prec uint) *big.Float {
		v := logBig(x, prec+8)
		return v.Quo(v, logBig(10, prec+8))
	})
}

// ---- sin, cos, tan ------------------------------------------------------------

// reduceHalfPi writes x = k pi/2 + (rh + rl) for |x| < 2^30, with |r| <= pi/4
// plus rounding, and returns the reduction's absolute error bound. x - k P0
// (P0 the leading term of pi/2) is exact in one fused multiply-add: for
// |x| >= 1 both are multiples of 2^-52 and the difference is below 1.
func reduceHalfPi(x float64) (k int, rh, rl, err float64) {
	kf := math.RoundToEven(x * (2 / math.Pi))
	r0 := math.FMA(-kf, halfPiDD[0], x)
	p1, p1e := twoProd(kf, halfPiDD[1])
	p2, p2e := twoProd(kf, halfPiDD[2])
	rh, rl = twoSum(r0, -p1)
	rl += -p1e - p2 - p2e - kf*halfPiDD[3]
	rh, rl = fastTwoSum(rh, rl)
	if kf == 0 {
		return 0, rh, rl, 0 // r = x exactly
	}
	// Rounding of the tail sum is below 2^-104 |r| + 2^-128; pi/2's
	// truncation times |k| < 2^30 is below 2^-180.
	return int(kf), rh, rl, 0x1p-103*math.Abs(rh) + 0x1p-127
}

// sinCosDD returns sin r and cos r as double-doubles for |r| <= about pi/4,
// with absolute error bounds. r = s(c + t), s the sign, c = i/64 from the
// table, |t| <= 1/128, and sin(c+t) = S cos t + C sin t,
// cos(c+t) = C cos t - S sin t, with Taylor series for sin t (to t^9) and
// cos t - 1 (to t^8); t and t^2/2 are exact, the rest in float64.
func sinCosDD(rh, rl float64) (sh, sl, serr, ch, cl, cerr float64) {
	sign := 1.0
	th, tl := rh, rl
	if rh < 0 {
		sign, th, tl = -1, -rh, -rl
	}
	i := int(th*64 + 0.5)
	th -= float64(i) * (1.0 / 64)
	S, C := sinTable[i], cosTable[i]
	sq, sqe := twoProd(th, th)
	sp := float64(th*sq) * (-1.0/6 + sq*(1.0/120+sq*(-1.0/5040+sq*(1.0/362880))))
	cp := float64(sq*sq) * (1.0/24 + sq*(-1.0/720+sq*(1.0/40320)))
	// sin t = th + tl + sp; cos t - 1 = -sq/2 + cm
	cm := cp - 0.5*sqe - th*tl
	st := tl + sp
	polyErr := 0x1p-50 * (math.Abs(sp) + math.Abs(cp))
	if i > 0 {
		polyErr += 0x1p-104 // table rounding; sin 0 and cos 0 are exact
	}

	p1, p1e := twoProd(C[0], th)
	p2, p2e := twoProd(S[0], -0.5*sq)
	h, e1 := twoSum(S[0], p1)
	h, e2 := twoSum(h, p2)
	l := e1 + e2 + p1e + p2e + S[1] + C[1]*th + C[0]*st + C[1]*st + S[0]*cm - S[1]*0.5*sq
	sh, sl = fastTwoSum(h, l)
	sh, sl = sign*sh, sign*sl
	serr = polyErr + 0x1p-100*math.Abs(sh)

	q1, q1e := twoProd(S[0], -th)
	q2, q2e := twoProd(C[0], -0.5*sq)
	h, e1 = twoSum(C[0], q1)
	h, e2 = twoSum(h, q2)
	l = e1 + e2 + q1e + q2e + C[1] - S[1]*th - S[0]*st - S[1]*st + C[0]*cm - C[1]*0.5*sq
	ch, cl = fastTwoSum(h, l)
	cerr = polyErr + 0x1p-100*math.Abs(ch)
	return sh, sl, serr, ch, cl, cerr
}

// trigArg reports whether x takes the double-double path (finite, nonzero,
// |x| < 2^30).
func trigArg(x float64) bool { return x != 0 && math.Abs(x) < 0x1p30 }

// mathSin returns sin x correctly rounded.
func mathSin(x float64) float64 {
	if trigArg(x) {
		k, rh, rl, rerr := reduceHalfPi(x)
		sh, sl, serr, ch, cl, cerr := sinCosDD(rh, rl)
		hi, lo, err := quadrant(k, sh, sl, serr, ch, cl, cerr)
		if r, ok := zivRound(hi, lo, err+rerr); ok {
			return r
		}
	} else if !(math.Abs(x) <= math.MaxFloat64) || x == 0 {
		return math.Sin(x)
	}
	return zivBig(func(prec uint) *big.Float { s, _ := sinCosBig(x, prec); return s })
}

// mathCos returns cos x correctly rounded.
func mathCos(x float64) float64 {
	if x == 0 {
		return 1
	}
	if trigArg(x) {
		k, rh, rl, rerr := reduceHalfPi(x)
		sh, sl, serr, ch, cl, cerr := sinCosDD(rh, rl)
		hi, lo, err := quadrant(k+1, sh, sl, serr, ch, cl, cerr)
		if r, ok := zivRound(hi, lo, err+rerr); ok {
			return r
		}
	} else if !(math.Abs(x) <= math.MaxFloat64) {
		return math.Cos(x)
	}
	return zivBig(func(prec uint) *big.Float { _, c := sinCosBig(x, prec); return c })
}

// quadrant returns sin(k pi/2 + r) from sin r and cos r.
func quadrant(k int, sh, sl, serr, ch, cl, cerr float64) (hi, lo, err float64) {
	switch k & 3 {
	case 0:
		return sh, sl, serr
	case 1:
		return ch, cl, cerr
	case 2:
		return -sh, -sl, serr
	default:
		return -ch, -cl, cerr
	}
}

// mathTan returns tan x correctly rounded: sin r / cos r for even k, and
// -cos r / sin r for odd k, divided in double-double.
func mathTan(x float64) float64 {
	if trigArg(x) {
		k, rh, rl, rerr := reduceHalfPi(x)
		sh, sl, serr, ch, cl, cerr := sinCosDD(rh, rl)
		nh, nl, nerr, dh, dl, derr := sh, sl, serr+rerr, ch, cl, cerr+rerr
		if k&1 == 1 {
			nh, nl, nerr, dh, dl, derr = -ch, -cl, cerr+rerr, sh, sl, serr+rerr
		}
		if nh != 0 && dh != 0 {
			qh := nh / dh
			ql := (math.FMA(-qh, dh, nh) + nl - qh*dl) / dh
			hi, lo := fastTwoSum(qh, ql)
			err := math.Abs(hi) * (nerr/math.Abs(nh) + derr/math.Abs(dh) + 0x1p-100)
			if r, ok := zivRound(hi, lo, err); ok {
				return r
			}
		}
	} else if !(math.Abs(x) <= math.MaxFloat64) || x == 0 {
		return math.Tan(x)
	}
	return zivBig(func(prec uint) *big.Float {
		s, c := sinCosBig(x, prec+2)
		return s.Quo(s, c)
	})
}

// ---- asin, acos -------------------------------------------------------------

// asinDD returns asin(sh + sl) for |s| <= 1/2 as hi + lo with an absolute
// error bound, by one Newton step on sin y = s from Go's asin y0:
// y = y0 + (s - sin y0)/cos y0, whose error is about tan(y0)/2 times the
// square of y0's (below 2^-100 relative here).
func asinDD(sh, sl float64) (hi, lo, err float64) {
	y0 := math.Asin(sh)
	s, s1, serr, c, _, _ := sinCosDD(y0, 0)
	d := (sh - s) - s1 + sl
	delta := d / c
	hi, lo = fastTwoSum(y0, delta)
	err = serr/c + 0x1p-50*math.Abs(delta) + 0x1p-100*math.Abs(hi)
	return hi, lo, err
}

// asinHalfComplement returns asin(sqrt((1-a)/2)) for a in [1/2, 1], used in
// asin a = pi/2 - 2 asin(sqrt((1-a)/2)) and acos a = 2 asin(sqrt((1-a)/2)).
// 1 - a and the halving are exact; the square root is carried as sh + sl.
func asinHalfComplement(a float64) (hi, lo, err float64) {
	z := (1 - a) * 0.5
	if z == 0 {
		return 0, 0, 0
	}
	sh := math.Sqrt(z)
	sl := math.FMA(-sh, sh, z) / (2 * sh)
	return asinDD(sh, sl)
}

// mathAsin returns asin x correctly rounded.
func mathAsin(x float64) float64 {
	a := math.Abs(x)
	if !(a <= 1) || x == 0 {
		return math.Asin(x)
	}
	var hi, lo, err float64
	if a <= 0.5 {
		hi, lo, err = asinDD(x, 0)
	} else {
		ah, al, aerr := asinHalfComplement(a)
		h, e := twoSum(halfPiDD[0], -2*ah)
		hi, lo = fastTwoSum(h, e+halfPiDD[1]-2*al)
		err = 2*aerr + 0x1p-100*hi
		if x < 0 {
			hi, lo = -hi, -lo
		}
	}
	if r, ok := zivRound(hi, lo, err); ok {
		return r
	}
	return zivBig(func(prec uint) *big.Float { return asinBig(x, prec) })
}

// mathAcos returns acos x correctly rounded: pi/2 - asin x for |x| <= 1/2,
// 2 asin(sqrt((1-x)/2)) above, pi - 2 asin(sqrt((1+x)/2)) below.
func mathAcos(x float64) float64 {
	a := math.Abs(x)
	if !(a <= 1) {
		return math.Acos(x)
	}
	if x == 1 {
		return 0
	}
	var hi, lo, err float64
	switch {
	case a <= 0.5:
		ah, al, aerr := asinDD(x, 0)
		h, e := twoSum(halfPiDD[0], -ah)
		hi, lo = fastTwoSum(h, e+halfPiDD[1]-al)
		err = aerr + 0x1p-100*hi
	case x > 0:
		ah, al, aerr := asinHalfComplement(a)
		hi, lo, err = 2*ah, 2*al, 2*aerr
	default:
		ah, al, aerr := asinHalfComplement(a)
		h, e := twoSum(2*halfPiDD[0], -2*ah)
		hi, lo = fastTwoSum(h, e+2*halfPiDD[1]-2*al)
		err = 2*aerr + 0x1p-100*hi
	}
	if r, ok := zivRound(hi, lo, err); ok {
		return r
	}
	return zivBig(func(prec uint) *big.Float {
		v := asinBig(x, prec+64)
		return v.Sub(new(big.Float).SetPrec(prec+128).Set(halfPiBig(prec+128)), v)
	})
}
