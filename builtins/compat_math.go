package builtins

import (
	"barn/types"
	"math"
	mathrand "math/rand"
	"regexp"
	"strings"
	"time"
)

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var ansiTagRe = regexp.MustCompile(`\[([^\[\]]+)\]`)

var ansiTags = map[string]string{
	"black":    "\x1b[30m",
	"red":      "\x1b[31m",
	"green":    "\x1b[32m",
	"yellow":   "\x1b[33m",
	"blue":     "\x1b[34m",
	"purple":   "\x1b[35m",
	"magenta":  "\x1b[35m",
	"cyan":     "\x1b[36m",
	"white":    "\x1b[37m",
	"gray":     "\x1b[90m",
	"grey":     "\x1b[90m",
	"b:black":  "\x1b[40m",
	"b:red":    "\x1b[41m",
	"b:green":  "\x1b[42m",
	"b:yellow": "\x1b[43m",
	"b:blue":   "\x1b[44m",
	"b:purple": "\x1b[45m",
	"b:magenta":"\x1b[45m",
	"b:cyan":   "\x1b[46m",
	"b:white":  "\x1b[47m",
	"bold":     "\x1b[1m",
	"unbold":   "\x1b[22m",
	"bright":   "\x1b[1m",
	"unbright": "\x1b[22m",
	"underline":"\x1b[4m",
	"inverse":  "\x1b[7m",
	"blink":    "\x1b[5m",
	"unblink":  "\x1b[25m",
	"normal":   "\x1b[0m",
	"beep":     "\a",
	"random":   "\x1b[37m",
	"null":     "",
}

func builtinAcosh(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	f := toNumericFloat(args[0])
	if math.IsNaN(f) {
		return types.Err(types.E_TYPE)
	}
	if f < 1 {
		return types.Err(types.E_FLOAT)
	}
	return types.Ok(types.NewFloat(math.Acosh(f)))
}

func builtinAsinh(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	f := toNumericFloat(args[0])
	if math.IsNaN(f) {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Asinh(f)))
}

func builtinAtanh(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	f := toNumericFloat(args[0])
	if math.IsNaN(f) {
		return types.Err(types.E_TYPE)
	}
	if f <= -1 || f >= 1 {
		return types.Err(types.E_FLOAT)
	}
	return types.Ok(types.NewFloat(math.Atanh(f)))
}

func builtinAtan2(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 {
		return types.Err(types.E_ARGS)
	}
	y := toNumericFloat(args[0])
	x := toNumericFloat(args[1])
	if math.IsNaN(y) || math.IsNaN(x) {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Atan2(y, x)))
}

func builtinCbrt(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	f := toNumericFloat(args[0])
	if math.IsNaN(f) {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Cbrt(f)))
}

func builtinRound(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	f := toNumericFloat(args[0])
	if math.IsNaN(f) {
		return types.Err(types.E_TYPE)
	}
	if len(args) == 1 {
		return types.Ok(types.NewInt(int64(math.Round(f))))
	}
	places, ok := args[1].(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	if places.Val < 0 || places.Val > 15 {
		return types.Err(types.E_RANGE)
	}
	scale := math.Pow(10, float64(places.Val))
	return types.Ok(types.NewFloat(math.Round(f*scale) / scale))
}

func builtinFrandom(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 0 {
		return types.Err(types.E_ARGS)
	}
	return types.Ok(types.NewFloat(mathrand.Float64()))
}

func builtinReseedRandom(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}
	seed := time.Now().UnixNano()
	if len(args) == 1 {
		v, ok := args[0].(types.IntValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		seed = v.Val
	}
	mathrand.Seed(seed)
	return types.Ok(types.NewInt(0))
}

func builtinChr(ctx *types.TaskContext, args []types.Value) types.Result {
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
			encodeByte(&out, byte(n))
		case types.StrValue:
			for _, b := range []byte(val.Value()) {
				encodeByte(&out, b)
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

func builtinAllMembers(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinDistance(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 2 && len(args) != 4 {
		return types.Err(types.E_ARGS)
	}
	if len(args) == 2 {
		x := toNumericFloat(args[0])
		y := toNumericFloat(args[1])
		if math.IsNaN(x) || math.IsNaN(y) {
			return types.Err(types.E_TYPE)
		}
		return types.Ok(types.NewFloat(math.Hypot(x, y)))
	}
	x1 := toNumericFloat(args[0])
	y1 := toNumericFloat(args[1])
	x2 := toNumericFloat(args[2])
	y2 := toNumericFloat(args[3])
	if math.IsNaN(x1) || math.IsNaN(y1) || math.IsNaN(x2) || math.IsNaN(y2) {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewFloat(math.Hypot(x2-x1, y2-y1)))
}

func builtinRelativeHeading(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 4 {
		return types.Err(types.E_ARGS)
	}
	x1 := toNumericFloat(args[0])
	y1 := toNumericFloat(args[1])
	x2 := toNumericFloat(args[2])
	y2 := toNumericFloat(args[3])
	if math.IsNaN(x1) || math.IsNaN(y1) || math.IsNaN(x2) || math.IsNaN(y2) {
		return types.Err(types.E_TYPE)
	}
	deg := math.Atan2(y2-y1, x2-x1) * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	return types.Ok(types.NewFloat(deg))
}

func builtinSimplexNoise(ctx *types.TaskContext, args []types.Value) types.Result {
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

func builtinParseAnsi(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	converted := ansiTagRe.ReplaceAllStringFunc(s.Value(), func(tag string) string {
		name := strings.ToLower(tag[1 : len(tag)-1])
		if code, ok := ansiTags[name]; ok {
			return code
		}
		return tag
	})
	return types.Ok(types.NewStr(converted))
}

func builtinRemoveAnsi(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	s, ok := args[0].(types.StrValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}
	strippedTags := ansiTagRe.ReplaceAllStringFunc(s.Value(), func(tag string) string {
		name := strings.ToLower(tag[1 : len(tag)-1])
		if _, ok := ansiTags[name]; ok {
			return ""
		}
		return tag
	})
	return types.Ok(types.NewStr(ansiEscapeRe.ReplaceAllString(strippedTags, "")))
}
