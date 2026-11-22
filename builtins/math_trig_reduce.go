package builtins

import (
	"math"
	"math/big"
	"math/bits"
	"sync"
)

// Argument reduction for |x| >= 2^30, where pi/2 in four float64 terms is no
// longer enough (Payne and Hanek's method, written from its description):
// x = m 2^e with m a 53-bit integer, and x (2/pi) mod 4 needs only the bits
// of 2/pi from position e - 1 on, because earlier bits contribute multiples
// of 4. Multiplying m by a 192-bit window of those bits gives the quadrant
// and 192 fraction bits; the bits after the window change the result by less
// than 2^53 2^-192 = 2^-139.

// twoOverPiWords holds the bits of 2/pi after the binary point, most
// significant first, behind one zero word (so a window may start before the
// binary point for x < 2^54). Built on first use from math/big's pi/2.
var (
	twoOverPiOnce  sync.Once
	twoOverPiWords []uint64
)

const twoOverPiWordCount = 22 // bit 64 + 971 + 192 < 22 * 64

func buildTwoOverPi() {
	const prec = twoOverPiWordCount*64 + 64
	v := new(big.Float).SetPrec(prec).Quo(new(big.Float).SetPrec(prec).SetInt64(1), halfPiBig(prec))
	v.SetMantExp(v, (twoOverPiWordCount-1)*64)
	n, _ := v.Int(nil)
	words := make([]uint64, twoOverPiWordCount)
	for i := twoOverPiWordCount - 1; i >= 1; i-- {
		words[i] = n.Uint64()
		n.Rsh(n, 64)
	}
	twoOverPiWords = words
}

// twoOverPiWindow returns the 192 bits of the padded bit string from bit
// index idx (bit 0 is the most significant bit of word 0).
func twoOverPiWindow(idx int) (w0, w1, w2 uint64) {
	q, s := idx/64, uint(idx%64)
	w := twoOverPiWords
	get := func(j int) uint64 {
		v := w[q+j] << s
		if s > 0 {
			v |= w[q+j+1] >> (64 - s)
		}
		return v
	}
	return get(0), get(1), get(2)
}

// reduceLarge writes x = k pi/2 + (rh + rl) for 2^30 <= |x| < 2^1024 and
// returns the reduction's absolute error bound; ok is false when r is too
// close to 0 for the window's precision (|r| < 2^-37), and the caller then
// uses math/big.
func reduceLarge(x float64) (k int, rh, rl, err float64, ok bool) {
	twoOverPiOnce.Do(buildTwoOverPi)
	ax := math.Abs(x)
	fr, ex := math.Frexp(ax)
	m := uint64(fr * (1 << 53)) // exact
	e := ex - 53                // ax = m 2^e
	// Bit k of 2/pi (k >= 1) has weight 2^-k and is bit index k + 63 in the
	// padded string; the window starts at k = e - 1.
	w0, w1, w2 := twoOverPiWindow(e - 1 + 63)
	h2, l2 := bits.Mul64(m, w2)
	h1, l1 := bits.Mul64(m, w1)
	_, l0 := bits.Mul64(m, w0) // the high half weighs 2^192 and up: a multiple of 4
	z0 := l2
	z1, c := bits.Add64(h2, l1, 0)
	z2, _ := bits.Add64(h1, l0, c) // the carry out also weighs 2^192
	// m W 2^(e - (e-1) - 191) = (z2:z1:z0) 2^-190 mod 4: bits 190-191 are the
	// quadrant and the low 190 bits the fraction, shifted here to 192 bits.
	q := int(z2 >> 62)
	z2 = z2<<2 | z1>>62
	z1 = z1<<2 | z0>>62
	z0 <<= 2
	neg := false
	if z2>>63 != 0 { // fraction >= 1/2: take k + 1 and the fraction f - 1
		q++
		neg = true
		var b uint64
		_, b = bits.Sub64(0, z0, 0) // only the borrow is needed below 2^-128
		z1, b = bits.Sub64(0, z1, b)
		z2, _ = bits.Sub64(0, z2, b)
	}
	// The window's error is 2^-139 in x (2/pi); below |f| = 2^-38 that would
	// exceed 2^-100 of f, so such x go to math/big.
	if z2 == 0 {
		return 0, 0, 0, 0, false
	}
	shift := bits.LeadingZeros64(z2)
	if shift > 37 {
		return 0, 0, 0, 0, false
	}
	if shift > 0 {
		z2 = z2<<shift | z1>>(64-shift)
		z1 <<= shift
	}
	// f = z2 2^-(64+shift) + z1 2^-(128+shift); split z2 = hi53 2^11 + r11.
	fh := math.Ldexp(float64(z2>>11), 11-64-shift)
	fl := math.Ldexp(float64(z2&0x7ff), -64-shift) + math.Ldexp(float64(z1), -128-shift)
	fh, fl = fastTwoSum(fh, fl)
	// r = f pi/2.
	rh, e1 := twoProd(fh, halfPiDD[0])
	rl = e1 + (fh*halfPiDD[1] + fl*halfPiDD[0])
	rh, rl = fastTwoSum(rh, rl)
	if neg {
		rh, rl = -rh, -rl
	}
	if x < 0 {
		q, rh, rl = -q, -rh, -rl
	}
	// The window's truncation (2^-139 in x 2/pi) and the fraction's float64
	// split (2^-104 |f|) times pi/2.
	return q, rh, rl, 0x1p-137 + 0x1p-102*math.Abs(rh), true
}
