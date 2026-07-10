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

	switch args[0].Type() {
	case types.TYPE_INT:
		if args[0].Int() < 0 {
			return types.Ok(types.NewInt(-args[0].Int()))
		}
		return types.Ok(args[0])
	case types.TYPE_FLOAT:
		return types.Ok(types.NewFloat(math.Abs(args[0].Float())))
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

	switch args[0].Type() {
	case types.TYPE_INT:
		minVal := args[0]
		for i := 1; i < len(args); i++ {
			if args[i].Type() != types.TYPE_INT {
				return types.Err(types.E_TYPE)
			}
			if args[i].Int() < minVal.Int() {
				minVal = args[i]
			}
		}
		return types.Ok(minVal)
	case types.TYPE_FLOAT:
		minVal := args[0]
		for i := 1; i < len(args); i++ {
			if args[i].Type() != types.TYPE_FLOAT {
				return types.Err(types.E_TYPE)
			}
			if args[i].Float() < minVal.Float() {
				minVal = args[i]
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

	switch args[0].Type() {
	case types.TYPE_INT:
		maxVal := args[0]
		for i := 1; i < len(args); i++ {
			if args[i].Type() != types.TYPE_INT {
				return types.Err(types.E_TYPE)
			}
			if args[i].Int() > maxVal.Int() {
				maxVal = args[i]
			}
		}
		return types.Ok(maxVal)
	case types.TYPE_FLOAT:
		maxVal := args[0]
		for i := 1; i < len(args); i++ {
			if args[i].Type() != types.TYPE_FLOAT {
				return types.Err(types.E_TYPE)
			}
			if args[i].Float() > maxVal.Float() {
				maxVal = args[i]
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
		return types.Ok(types.NewInt(rand.Int63n(maxInt64) + 1))

	case 1:
		// Random in [1, max]
		if args[0].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		maxV := args[0].Int()
		if maxV <= 0 {
			return types.Err(types.E_INVARG) // Must be positive
		}
		return types.Ok(types.NewInt(rand.Int63n(maxV) + 1))

	case 2:
		// Random in [min, max]
		if args[0].Type() != types.TYPE_INT || args[1].Type() != types.TYPE_INT {
			return types.Err(types.E_TYPE)
		}
		minV := args[0].Int()
		maxV := args[1].Int()
		if minV > maxV {
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.NewInt(minV + rand.Int63n(maxV-minV+1)))

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

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()
	if f < 0 {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewFloat(math.Sqrt(f)))
}

// builtinSin returns sine of angle (radians)
// sin(angle) -> float
func builtinSin(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	return types.Ok(types.NewFloat(math.Sin(f)))
}

// builtinCos returns cosine of angle (radians)
// cos(angle) -> float
func builtinCos(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	return types.Ok(types.NewFloat(math.Cos(f)))
}

// builtinTan returns tangent of angle (radians)
// tan(angle) -> float
func builtinTan(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	result := math.Tan(f)
	if math.IsInf(result, 0) {
		return types.Err(types.E_FLOAT)
	}

	return types.Ok(types.NewFloat(result))
}

// builtinAsin returns arc sine
// asin(value) -> float
func builtinAsin(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()
	if f < -1 || f > 1 {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewFloat(math.Asin(f)))
}

// builtinAcos returns arc cosine
// acos(value) -> float
func builtinAcos(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()
	if f < -1 || f > 1 {
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewFloat(math.Acos(f)))
}

// builtinAtan returns arc tangent
// atan(value) -> float
// atan(y, x) -> float (two-argument form)
func builtinAtan(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) == 0 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}

	if len(args) == 1 {
		if args[0].Type() != types.TYPE_FLOAT {
			return types.Err(types.E_TYPE)
		}
		return types.Ok(types.NewFloat(math.Atan(args[0].Float())))
	}

	// Two-argument form
	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}

	return types.Ok(types.NewFloat(math.Atan2(args[0].Float(), args[1].Float())))
}

// builtinSinh returns hyperbolic sine
// sinh(value) -> float
func builtinSinh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	return types.Ok(types.NewFloat(math.Sinh(f)))
}

// builtinCosh returns hyperbolic cosine
// cosh(value) -> float
func builtinCosh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	return types.Ok(types.NewFloat(math.Cosh(f)))
}

// builtinTanh returns hyperbolic tangent
// tanh(value) -> float
func builtinTanh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	return types.Ok(types.NewFloat(math.Tanh(f)))
}

// builtinExp returns e raised to power
// exp(value) -> float
func builtinExp(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	result := math.Exp(f)
	if math.IsInf(result, 0) {
		return types.Err(types.E_FLOAT)
	}

	return types.Ok(types.NewFloat(result))
}

// builtinLog returns natural logarithm
// log(value) -> float
func builtinLog(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()
	if f <= 0 {
		if f == 0 {
			return types.Err(types.E_FLOAT)
		}
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewFloat(math.Log(f)))
}

// builtinLog10 returns base-10 logarithm
// log10(value) -> float
func builtinLog10(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()
	if f <= 0 {
		if f == 0 {
			return types.Err(types.E_FLOAT)
		}
		return types.Err(types.E_INVARG)
	}

	return types.Ok(types.NewFloat(math.Log10(f)))
}

// builtinCeil rounds up to nearest integer
// ceil(float) -> float
func builtinCeil(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	return types.Ok(types.NewFloat(math.Ceil(f)))
}

// builtinFloor rounds down to nearest integer
// floor(float) -> float
func builtinFloor(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	return types.Ok(types.NewFloat(math.Floor(f)))
}

// builtinTrunc truncates towards zero
// trunc(float) -> float
func builtinTrunc(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	return types.Ok(types.NewFloat(math.Trunc(f)))
}

// builtinFloatstr formats a float as a string
// floatstr(float, precision [, scientific]) -> str
func builtinFloatstr(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 2 || len(args) > 3 {
		return types.Err(types.E_ARGS)
	}

	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()

	if args[1].Type() != types.TYPE_INT {
		return types.Err(types.E_TYPE)
	}
	precision := int(args[1].Int())
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
	switch v.Type() {
	case types.TYPE_INT:
		return float64(v.Int())
	case types.TYPE_FLOAT:
		return v.Float()
	default:
		return math.NaN()
	}
}

func builtinAcosh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()
	if f < 1 {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewFloat(math.Acosh(f)))
}

func builtinAsinh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()
	return types.Ok(types.NewFloat(math.Asinh(f)))
}

func builtinAtanh(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	f := args[0].Float()
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
	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	if args[1].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Atan2(args[0].Float(), args[1].Float())))
}

func builtinCbrt(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Cbrt(args[0].Float())))
}

func builtinRound(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Round(args[0].Float())))
}

func builtinFrandom(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	var min float64
	var max float64
	if len(args) == 1 {
		if args[0].Type() != types.TYPE_FLOAT {
			return types.Err(types.E_TYPE)
		}
		min = 0.0
		max = args[0].Float()
	} else {
		if args[0].Type() != types.TYPE_FLOAT {
			return types.Err(types.E_TYPE)
		}
		if args[1].Type() != types.TYPE_FLOAT {
			return types.Err(types.E_TYPE)
		}
		min = args[0].Float()
		max = args[1].Float()
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
		switch v.Type() {
		case types.TYPE_INT:
			n := v.Int()
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
		case types.TYPE_STR:
			for _, b := range []byte(v.Str()) {
				out.WriteByte(b)
			}
		case types.TYPE_LIST:
			for i := 1; i <= v.Len(); i++ {
				if err := appendValue(v.Get(i)); err != types.E_NONE {
					return err
				}
			}
		default:
			return types.E_INVARG
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
	if args[1].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	list := args[1]
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
			if needle.Type() == types.TYPE_STR && item.Type() == types.TYPE_STR {
				matched = strings.EqualFold(needle.Str(), item.Str())
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
	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	a := args[0]
	if args[1].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	b := args[1]
	if a.Len() != b.Len() || a.Len() == 0 {
		return types.Err(types.E_TYPE)
	}
	total := 0.0
	for i := 1; i <= a.Len(); i++ {
		var av float64
		switch a.Get(i).Type() {
		case types.TYPE_INT:
			av = float64(a.Get(i).Int())
		case types.TYPE_FLOAT:
			av = a.Get(i).Float()
		default:
			return types.Err(types.E_TYPE)
		}
		var bv float64
		switch b.Get(i).Type() {
		case types.TYPE_INT:
			bv = float64(b.Get(i).Int())
		case types.TYPE_FLOAT:
			bv = b.Get(i).Float()
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
	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	a := args[0]
	if args[1].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	b := args[1]
	if a.Len() != 3 || b.Len() != 3 {
		return types.Err(types.E_INVARG)
	}
	if a.Get(1).Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	ax := a.Get(1).Float()
	if a.Get(2).Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	ay := a.Get(2).Float()
	if a.Get(3).Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	az := a.Get(3).Float()
	if b.Get(1).Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	bx := b.Get(1).Float()
	if b.Get(2).Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	by := b.Get(2).Float()
	if b.Get(3).Type() != types.TYPE_FLOAT {
		return types.Err(types.E_TYPE)
	}
	bz := b.Get(3).Float()

	dx := bx - ax
	dy := by - ay
	dz := bz - az

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
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}
	coords := args[0].Elements()
	if len(coords) == 0 || len(coords) > 4 {
		return types.Ok(types.NewErr(types.E_TYPE))
	}
	seed := 0.0
	for i, coord := range coords {
		if coord.Type() != types.TYPE_FLOAT {
			return types.Err(types.E_TYPE)
		}
		seed += coord.Float() * float64(i+1) * 12.9898
	}
	noise := math.Sin(seed) * 43758.5453
	noise = noise - math.Floor(noise)
	return types.Ok(types.NewFloat(noise*2 - 1))
}
