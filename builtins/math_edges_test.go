package builtins

import (
	"math"
	"strconv"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestMathFuncEdgesMatchToast pins rows observed on the stock WSL Toast oracle
// (issue #1096). want is Toast's value in its 15 significant digits.
func TestMathFuncEdgesMatchToast(t *testing.T) {
	ctx := newTestExecution()
	tests := []struct {
		name    string
		builtin BuiltinFunc
		arg     float64
		want    string
		wantErr types.ErrorCode
	}{
		{name: "sinh overflow", builtin: builtinSinh, arg: 3e300, wantErr: types.E_FLOAT},
		{name: "sinh negative overflow", builtin: builtinSinh, arg: -711, wantErr: types.E_FLOAT},
		{name: "cosh overflow", builtin: builtinCosh, arg: 3e300, wantErr: types.E_FLOAT},
		{name: "cosh just past threshold", builtin: builtinCosh, arg: 710.48, wantErr: types.E_FLOAT},
		{name: "cosh below threshold", builtin: builtinCosh, arg: 710.4, want: "1.66636428328065e+308"},
		{name: "sinh below threshold", builtin: builtinSinh, arg: -710.4, want: "-1.66636428328065e+308"},
		{name: "sinh at 710", builtin: builtinSinh, arg: 710, want: "1.11699738308086e+308"},
		{name: "cosh above Go exp overflow", builtin: builtinCosh, arg: 709.78, want: "8.96411397197258e+307"},
		{name: "exp overflow", builtin: builtinExp, arg: 710, wantErr: types.E_FLOAT},
		{name: "exp smallest subnormal", builtin: builtinExp, arg: -745, want: "4.94065645841247e-324"},
		{name: "exp near smallest normal", builtin: builtinExp, arg: -709, want: "1.21678075062342e-308"},
		{name: "exp underflow to zero", builtin: builtinExp, arg: -746, wantErr: types.E_FLOAT},
		{name: "log subnormal", builtin: builtinLog, arg: 1e-310, want: "-713.801378828154"},
		{name: "log smallest subnormal", builtin: builtinLog, arg: 5e-324, want: "-744.440071921381"},
		{name: "log10 subnormal", builtin: builtinLog10, arg: 1e-310, want: "-310"},
		{name: "log10 smallest subnormal", builtin: builtinLog10, arg: 5e-324, want: "-323.306215343116"},
		{name: "log zero pole", builtin: builtinLog, arg: 0, wantErr: types.E_FLOAT},
		{name: "log negative zero pole", builtin: builtinLog, arg: math.Copysign(0, -1), wantErr: types.E_FLOAT},
		{name: "log negative subnormal domain", builtin: builtinLog, arg: -1e-310, wantErr: types.E_INVARG},
		{name: "log10 negative zero pole", builtin: builtinLog10, arg: math.Copysign(0, -1), wantErr: types.E_FLOAT},
		{name: "sqrt domain", builtin: builtinSqrt, arg: -1e-300, wantErr: types.E_INVARG},
		{name: "asin domain", builtin: builtinAsin, arg: 1.0000000000000002, wantErr: types.E_INVARG},
		{name: "acos domain", builtin: builtinAcos, arg: -1.0000000000000002, wantErr: types.E_INVARG},
		{name: "acosh domain", builtin: builtinAcosh, arg: 0.9999999999999999, wantErr: types.E_INVARG},
		{name: "atanh pole", builtin: builtinAtanh, arg: -1, wantErr: types.E_FLOAT},
		{name: "atanh domain", builtin: builtinAtanh, arg: 1.0000000000000002, wantErr: types.E_INVARG},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.builtin(ctx, []types.Value{types.NewFloat(test.arg)})
			if test.wantErr != types.E_NONE {
				if result.Flow != types.FlowException || result.Error != test.wantErr {
					t.Fatalf("result = flow %v error %v value %v, want %v", result.Flow, result.Error, result.Val, test.wantErr)
				}
				return
			}
			if !result.IsNormal() || result.Val.Type() != types.TYPE_FLOAT {
				t.Fatalf("result = flow %v error %v value %v, want %s", result.Flow, result.Error, result.Val, test.want)
			}
			if got := strconv.FormatFloat(result.Val.Float(), 'g', 15, 64); got != test.want {
				t.Fatalf("result = %s, want %s", got, test.want)
			}
		})
	}
}

// TestMathExpFiniteToTrueOverflow covers the range where Go's amd64 math.Exp
// already returns +Inf although e^x is finite.
func TestMathExpFiniteToTrueOverflow(t *testing.T) {
	for _, x := range []float64{709.5, 709.7, 709.78} {
		if r := mathExp(x); math.IsInf(r, 0) || r < 1e308 {
			t.Errorf("mathExp(%v) = %v, want finite above 1e308", x, r)
		}
	}
}
