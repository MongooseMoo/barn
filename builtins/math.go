package builtins

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"barn/kernel"
	"barn/types"
)

// ============================================================================
// LAYER 7.3: MATH BUILTINS
// ============================================================================

// builtinAbs returns absolute value
// abs(number) -> int|float
func builtinAbs(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	switch v := args[0].(type) {
	case types.IntValue:
		if v.Val < 0 {
			return types.Ok(types.IntValue{Val: -v.Val})
		}
		return types.Ok(v)
	case types.FloatValue:
		return types.Ok(types.FloatValue{Val: math.Abs(v.Val)})
	default:
		return types.Err(types.E_TYPE)
	}
}

// builtinMin returns the smallest value
// min(num1, num2, ...) -> int|float
func builtinMin(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) == 0 {
		return types.Err(types.E_ARGS)
	}

	switch first := args[0].(type) {
	case types.IntValue:
		minVal := first
		for i := 1; i < len(args); i++ {
			v, ok := args[i].(types.IntValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			if v.Val < minVal.Val {
				minVal = v
			}
		}
		return types.Ok(minVal)
	case types.FloatValue:
		minVal := first
		for i := 1; i < len(args); i++ {
			v, ok := args[i].(types.FloatValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			if v.Val < minVal.Val {
				minVal = v
			}
		}
		return types.Ok(minVal)
	default:
		return types.Err(types.E_TYPE)
	}
}

// builtinMax returns the largest value
// max(num1, num2, ...) -> int|float
func builtinMax(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) == 0 {
		return types.Err(types.E_ARGS)
	}

	switch first := args[0].(type) {
	case types.IntValue:
		maxVal := first
		for i := 1; i < len(args); i++ {
			v, ok := args[i].(types.IntValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			if v.Val > maxVal.Val {
				maxVal = v
			}
		}
		return types.Ok(maxVal)
	case types.FloatValue:
		maxVal := first
		for i := 1; i < len(args); i++ {
			v, ok := args[i].(types.FloatValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			if v.Val > maxVal.Val {
				maxVal = v
			}
		}
		return types.Ok(maxVal)
	default:
		return types.Err(types.E_TYPE)
	}
}

// builtinRandom returns a random integer
// random() -> int (32-bit)
// random(max) -> int (1 to max)
// random(min, max) -> int (min to max)
func builtinRandom(ctx *kernel.TaskContext, args []types.Value) types.Result {
	switch len(args) {
	case 0:
		// Random positive integer in full 64-bit range [1, MaxInt64]
		// Use rand.Int63n(MaxInt64) which gives [0, MaxInt64-1], then add 1
		const maxInt64 = 9223372036854775807
		return types.Ok(types.IntValue{Val: rand.Int63n(maxInt64) + 1})

	case 1:
		// Random in [1, max]
		maxV, ok := args[0].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		if maxV.Val <= 0 {
			return types.Err(types.E_INVARG) // Must be positive
		}
		return types.Ok(types.IntValue{Val: rand.Int63n(maxV.Val) + 1})

	case 2:
		// Random in [min, max]
		minV, ok1 := args[0].(types.IntValue)
		maxV, ok2 := args[1].(types.IntValue)
		if !ok1 || !ok2 {
			return types.Err(types.E_TYPE)
		}
		if minV.Val > maxV.Val {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.IntValue{Val: minV.Val + rand.Int63n(maxV.Val-minV.Val+1)})

	default:
		return types.Err(types.E_ARGS)
	}
}

// builtinSqrt returns square root
// sqrt(value) -> float
func builtinSqrt(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val
	if f < 0 {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.FloatValue{Val: math.Sqrt(f)})
}

// builtinSin returns sine of angle (radians)
// sin(angle) -> float
func builtinSin(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	return types.Ok(types.FloatValue{Val: math.Sin(f)})
}

// builtinCos returns cosine of angle (radians)
// cos(angle) -> float
func builtinCos(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	return types.Ok(types.FloatValue{Val: math.Cos(f)})
}

// builtinTan returns tangent of angle (radians)
// tan(angle) -> float
func builtinTan(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	result := math.Tan(f)
	if math.IsInf(result, 0) {
		return types.Err(types.E_FLOAT)
	}

	return types.Ok(types.FloatValue{Val: result})
}

// builtinAsin returns arc sine
// asin(value) -> float
func builtinAsin(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val
	if f < -1 || f > 1 {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.FloatValue{Val: math.Asin(f)})
}

// builtinAcos returns arc cosine
// acos(value) -> float
func builtinAcos(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val
	if f < -1 || f > 1 {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.FloatValue{Val: math.Acos(f)})
}

// builtinAtan returns arc tangent
// atan(value) -> float
// atan(y, x) -> float (two-argument form)
func builtinAtan(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) == 0 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if len(args) == 1 {
		fv, ok := args[0].(types.FloatValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		return types.Ok(types.FloatValue{Val: math.Atan(fv.Val)})
	}

	// Two-argument form
	yv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	xv, ok := args[1].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.FloatValue{Val: math.Atan2(yv.Val, xv.Val)})
}

// builtinSinh returns hyperbolic sine
// sinh(value) -> float
func builtinSinh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	return types.Ok(types.FloatValue{Val: math.Sinh(f)})
}

// builtinCosh returns hyperbolic cosine
// cosh(value) -> float
func builtinCosh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	return types.Ok(types.FloatValue{Val: math.Cosh(f)})
}

// builtinTanh returns hyperbolic tangent
// tanh(value) -> float
func builtinTanh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	return types.Ok(types.FloatValue{Val: math.Tanh(f)})
}

// builtinExp returns e raised to power
// exp(value) -> float
func builtinExp(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	result := math.Exp(f)
	if math.IsInf(result, 0) {
		return types.Err(types.E_FLOAT)
	}

	return types.Ok(types.FloatValue{Val: result})
}

// builtinLog returns natural logarithm
// log(value) -> float
func builtinLog(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val
	if f <= 0 {
		if f == 0 {
			return types.Err(types.E_FLOAT)
		}
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.FloatValue{Val: math.Log(f)})
}

// builtinLog10 returns base-10 logarithm
// log10(value) -> float
func builtinLog10(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val
	if f <= 0 {
		if f == 0 {
			return types.Err(types.E_FLOAT)
		}
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.FloatValue{Val: math.Log10(f)})
}

// builtinCeil rounds up to nearest integer
// ceil(float) -> float
func builtinCeil(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	return types.Ok(types.FloatValue{Val: math.Ceil(f)})
}

// builtinFloor rounds down to nearest integer
// floor(float) -> float
func builtinFloor(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	return types.Ok(types.FloatValue{Val: math.Floor(f)})
}

// builtinTrunc truncates towards zero
// trunc(float) -> float
func builtinTrunc(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	return types.Ok(types.FloatValue{Val: math.Trunc(f)})
}

// builtinFloatstr formats a float as a string
// floatstr(float, precision [, scientific]) -> str
func builtinFloatstr(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val

	precV, ok := args[1].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	precision := int(precV.Val)
	if precision < 0 || precision > 19 {
		return types.Err(types.E_INVARG)
	}

	scientific := false
	if len(args) == 3 {
		scientific = args[2].Truthy()
	}

	var result string
	if scientific {
		result = fmt.Sprintf("%.*e", precision, f)
	} else {
		result = fmt.Sprintf("%.*f", precision, f)
	}

	return types.Ok(types.NewStr(result))
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// toNumericFloat converts a value to float64 for math operations
// Returns NaN if not numeric
func toNumericFloat(v types.Value) float64 {
	switch val := v.(type) {
	case types.IntValue:
		return float64(val.Val)
	case types.FloatValue:
		return val.Val
	default:
		return math.NaN()
	}
}

func builtinAcosh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val
	if f < 1 {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewFloat(math.Acosh(f)))
}

func builtinAsinh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val
	return types.Ok(types.NewFloat(math.Asinh(f)))
}

func builtinAtanh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	f := fv.Val
	if f == -1 || f == 1 {
		return types.Err(types.E_FLOAT)
	}
	if f < -1 || f > 1 {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewFloat(math.Atanh(f)))
}

func builtinAtan2(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	yv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	xv, ok := args[1].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Atan2(yv.Val, xv.Val)))
}

func builtinCbrt(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Cbrt(fv.Val)))
}

func builtinRound(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	fv, ok := args[0].(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Round(fv.Val)))
}

func builtinFrandom(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	var min float64
	var max float64
	if len(args) == 1 {
		maxArg, ok := args[0].(types.FloatValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		min = 0.0
		max = maxArg.Val
	} else {
		minArg, ok := args[0].(types.FloatValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		maxArg, ok := args[1].(types.FloatValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		min = minArg.Val
		max = maxArg.Val
	}
	f := rand.Float64()
	return types.Ok(types.NewFloat(min + f*(max-min)))
}

func builtinReseedRandom(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	if !ctx.IsWizard {
		return types.Err(types.E_PERM)
	}
	rand.Seed(time.Now().UnixNano())
	return types.Ok(types.NewInt(0))
}

func builtinChr(ctx *kernel.TaskContext, args []types.Value) types.Result {
	var out strings.Builder

	var appendValue func(v types.Value) types.ErrorCode
	appendValue = func(v types.Value) types.ErrorCode {
		switch val := v.(type) {
		case types.IntValue:
			n := val.Val
			if n < 0 || n > 255 {
				return types.E_INVARG
			}
			if !ctx.IsWizard && (n < 32 || n > 254) {
				return types.E_INVARG
			}
			// chr() yields the RAW byte for the code point. The ~XX binary
			// notation is the job of encode_binary(), not chr(). A NUL cannot
			// live inside a MOO string, so chr(0) contributes nothing.
			if n != 0 {
				out.WriteByte(byte(n))
			}
		case types.StrValue:
			for _, b := range []byte(val.Value()) {
				out.WriteByte(b)
			}
		case types.ListValue:
			for i := 1; i <= val.Len(); i++ {
				if err := appendValue(val.Get(i)); err != types.E_NONE {
					return err
				}
			}
		default:
			return types.E_TYPE
		}
		return types.E_NONE
	}

	for _, arg := range args {
		if err := appendValue(arg); err != types.E_NONE {
			return types.Err(err)
		}
	}

	return types.Ok(types.NewStr(out.String()))
}

func builtinAllMembers(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	list, ok := args[1].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	caseMatters := true
	if len(args) == 3 {
		caseMatters = args[2].Truthy()
	}
	needle := args[0]
	result := make([]types.Value, 0)
	for i := 1; i <= list.Len(); i++ {
		item := list.Get(i)
		matched := false
		if !caseMatters {
			ns, nok := needle.(types.StrValue)
			is, iok := item.(types.StrValue)
			if nok && iok {
				matched = strings.EqualFold(ns.Value(), is.Value())
			}
		} else {
			matched = needle.Equal(item)
		}
		if matched {
			result = append(result, types.NewInt(int64(i)))
		}
	}
	return types.Ok(types.NewList(result))
}

func builtinDistance(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	a, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	b, ok := args[1].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if a.Len() != b.Len() || a.Len() == 0 {
		return types.Err(types.E_TYPE)
	}
	total := 0.0
	for i := 1; i <= a.Len(); i++ {
		var av float64
		switch v := a.Get(i).(type) {
		case types.IntValue:
			av = float64(v.Val)
		case types.FloatValue:
			av = v.Val
		default:
			return types.Err(types.E_TYPE)
		}
		var bv float64
		switch v := b.Get(i).(type) {
		case types.IntValue:
			bv = float64(v.Val)
		case types.FloatValue:
			bv = v.Val
		default:
			return types.Err(types.E_TYPE)
		}
		d := bv - av
		total += d * d
	}
	return types.Ok(types.NewFloat(math.Sqrt(total)))
}

func builtinRelativeHeading(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	a, ok := args[0].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	b, ok := args[1].(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if a.Len() != 3 || b.Len() != 3 {
		return types.Err(types.E_INVARG)
	}
	ax, ok := a.Get(1).(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	ay, ok := a.Get(2).(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	az, ok := a.Get(3).(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	bx, ok := b.Get(1).(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	by, ok := b.Get(2).(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	bz, ok := b.Get(3).(types.FloatValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	dx := bx.Val - ax.Val
	dy := by.Val - ay.Val
	dz := bz.Val - az.Val

	xy := math.Atan2(dy, dx) * 57.2957795130823
	if xy < 0.0 {
		xy += 360.0
	}
	z := math.Atan2(dz, math.Sqrt((dx*dx)+(dy*dy))) * 57.2957795130823

	return types.Ok(types.NewList([]types.Value{
		types.NewInt(int64(xy)),
		types.NewInt(int64(z)),
	}))
}

func builtinSimplexNoise(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}
	seed := 0.0
	for i, arg := range args {
		v := toNumericFloat(arg)
		if math.IsNaN(v) {
			return types.Err(types.E_TYPE)
		}
		seed += v * float64(i+1) * 12.9898
	}
	noise := math.Sin(seed) * 43758.5453
	noise = noise - math.Floor(noise)
	return types.Ok(types.NewFloat(noise*2 - 1))
}
