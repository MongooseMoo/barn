package builtins

import (
	"math"
	"math/big"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestMathExpConstants checks the init-derived constants: ln2/64 against Go's
// math.Ln2 and the published digits of ln 2, and each table entry T by
// T^64 = 2^j.
func TestMathExpConstants(t *testing.T) {
	if got, want := expLn2NHi, float64(math.Ln2/expTableSize); got != want {
		t.Fatalf("expLn2NHi = %v, want %v", got, want)
	}
	ln2N := new(big.Float).SetPrec(200)
	ln2N.SetString("0.693147180559945309417232121458176568075500134360255254120680009")
	ln2N.Quo(ln2N, big.NewFloat(expTableSize))
	wantLo, _ := new(big.Float).Sub(ln2N, big.NewFloat(expLn2NHi)).Float64()
	if expLn2NLo != wantLo {
		t.Fatalf("expLn2NLo = %v, want %v", expLn2NLo, wantLo)
	}
	for j := range expTableSize {
		v := new(big.Float).SetPrec(300).SetFloat64(expTable[j][0])
		v.Add(v, big.NewFloat(expTable[j][1]))
		if math.Abs(expTable[j][1]) > 0x1p-53*expTable[j][0] {
			t.Fatalf("expTable[%d] = %v is not normalised", j, expTable[j])
		}
		p := new(big.Float).SetPrec(300).SetInt64(1)
		for range expTableSize {
			p.Mul(p, v)
		}
		// T = 2^(j/64)(1+d) gives T^64 = 2^j (1 + 64d); require |d| < 2^-104.
		rel := new(big.Float).SetMantExp(p, -j)
		rel.Sub(rel, big.NewFloat(1)).Abs(rel)
		if rel.Cmp(big.NewFloat(0x1p-98)) > 0 {
			t.Fatalf("expTable[%d]^64 / 2^%d - 1 = %v", j, j, rel)
		}
	}
}

// TestMathCRFastPathIsCorrectlyRounded compares each function's float64 fast
// path with its math/big slow path, which decides the rounding independently.
func TestMathCRFastPathIsCorrectlyRounded(t *testing.T) {
	rng := rand.New(rand.NewPCG(1110, 1))
	uniform := func(lo, hi float64) func() float64 {
		return func() float64 { return lo + rng.Float64()*(hi-lo) }
	}
	logUniform := func(lo, hi float64, signed bool) func() float64 {
		return func() float64 {
			v := math.Pow(10, lo+rng.Float64()*(hi-lo))
			if signed && rng.IntN(2) == 0 {
				v = -v
			}
			return v
		}
	}
	tests := []struct {
		name string
		fast func(float64) float64
		slow func(x float64, prec uint) *big.Float
		gens []func() float64
	}{
		{"exp", mathExp, func(x float64, p uint) *big.Float { return expBig(new(big.Float).SetFloat64(x), p) },
			[]func() float64{uniform(-745.2, 709.79), uniform(-30, 30), logUniform(-20, 2, true)}},
		{"log", mathLog, logBig,
			[]func() float64{logUniform(-323, 308, false), uniform(0.5, 2)}},
		{"log10", mathLog10, func(x float64, p uint) *big.Float {
			v := logBig(x, p+8)
			return v.Quo(v, logBig(10, p+8))
		}, []func() float64{logUniform(-323, 308, false), uniform(0.5, 20)}},
		{"sin", mathSin, func(x float64, p uint) *big.Float { s, _ := sinCosBig(x, p); return s },
			[]func() float64{uniform(-10, 10), logUniform(-20, 9, true), logUniform(9, 308, true)}},
		{"cos", mathCos, func(x float64, p uint) *big.Float { _, c := sinCosBig(x, p); return c },
			[]func() float64{uniform(-10, 10), logUniform(-20, 9, true), logUniform(9, 308, true)}},
		{"tan", mathTan, func(x float64, p uint) *big.Float { s, c := sinCosBig(x, p+2); return s.Quo(s, c) },
			[]func() float64{uniform(-10, 10), logUniform(-20, 9, true), logUniform(9, 308, true)}},
		{"asin", mathAsin, asinBig,
			[]func() float64{uniform(-1, 1), logUniform(-20, 0, true)}},
		{"acos", mathAcos, func(x float64, p uint) *big.Float {
			v := asinBig(x, p+64)
			return v.Sub(new(big.Float).SetPrec(p+128).Set(halfPiBig(p+128)), v)
		}, []func() float64{uniform(-1, 1), logUniform(-20, 0, true)}},
	}
	n := 2000
	if testing.Short() {
		n = 200
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, gen := range test.gens {
				for range n {
					x := gen()
					want := zivBig(func(p uint) *big.Float { return test.slow(x, p) })
					if got := test.fast(x); got != want {
						t.Fatalf("%s(%v) = %v, want %v", test.name, x, got, want)
					}
				}
			}
		})
	}
}

// TestMathExpMatchesToast pins exp results observed on the stock WSL Toast
// oracle (issue #1110), in Toast's 15 significant digits.
func TestMathExpMatchesToast(t *testing.T) {
	ctx := newTestExecution()
	tests := []struct {
		arg  float64
		want string
	}{
		{1.5, "4.48168907033806"},
		{700, "1.014232054735e+304"},
		{450.7359251584104, "5.65099719667398e+195"},
		{709.5, "1.35498631931463e+308"},
		{709.78, "1.79282279439452e+308"},
		{132.78155163351704, "4.63762013563591e+57"},
		{398.88134969770834, "1.70596103654619e+173"},
		{-620.1130545905793, "4.87890357717507e-270"},
		{-744.44007192138122, "4.94065645841247e-324"},
		{1e-300, "1"}, // Toast prints 1.0; FormatFloat 'g' drops the ".0"
		{math.Copysign(0, -1), "1"},
	}
	for _, test := range tests {
		result := builtinExp(ctx, []types.Value{types.NewFloat(test.arg)})
		if !result.IsNormal() || result.Val.Type() != types.TYPE_FLOAT {
			t.Fatalf("exp(%v) = flow %v error %v, want %s", test.arg, result.Flow, result.Error, test.want)
		}
		if got := strconv.FormatFloat(result.Val.Float(), 'g', 15, 64); got != test.want {
			t.Errorf("exp(%v) = %s, want %s", test.arg, got, test.want)
		}
	}
}

var benchMathSink float64

// BenchmarkMathCR times each correctly rounded function beside Go's math on
// the same 1024 arguments.
func BenchmarkMathCR(b *testing.B) {
	benches := []struct {
		name     string
		f, gof   func(float64) float64
		lo, span float64
	}{
		{"exp", mathExp, math.Exp, -700, 1400},
		{"log", mathLog, math.Log, 1e-3, 1e3},
		{"log10", mathLog10, math.Log10, 1e-3, 1e3},
		{"sin", mathSin, math.Sin, -10, 20},
		{"cos", mathCos, math.Cos, -10, 20},
		{"tan", mathTan, math.Tan, -10, 20},
		{"asin", mathAsin, math.Asin, -1, 2},
		{"acos", mathAcos, math.Acos, -1, 2},
	}
	args := make([]float64, 1024)
	for _, bench := range benches {
		rng := rand.New(rand.NewPCG(1, 2))
		for i := range args {
			args[i] = bench.lo + rng.Float64()*bench.span
		}
		for _, impl := range []struct {
			name string
			f    func(float64) float64
		}{{"barn", bench.f}, {"go", bench.gof}} {
			b.Run(bench.name+"/"+impl.name, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					benchMathSink += impl.f(args[i&1023])
				}
			})
		}
	}
}
