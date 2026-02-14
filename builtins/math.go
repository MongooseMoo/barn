package builtins

import (
	"barn/types"
	"fmt"
	"math"
	"math/rand"
)

// ============================================================================
// LAYER 7.3: MATH BUILTINS
// ============================================================================

// builtinAbs returns absolute value
// abs(number) -> int|float
func builtinAbs(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinMin(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinMax(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinRandom(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinSqrt(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinSin(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinCos(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinTan(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinAsin(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinAcos(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinAtan(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinSinh(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinCosh(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinTanh(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinExp(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinLog(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinLog10(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinCeil(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinFloor(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinTrunc(ctx *types.TaskContext, args []types.Value) types.Result {
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
func builtinFloatstr(ctx *types.TaskContext, args []types.Value) types.Result {
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
