package builtins

import (
	"net/url"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

func builtinUrlEncode(ctx *Execution, args []types.Value) types.Result {
	if len(args) < 1 || len(args) > 2 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	spacePlus := false
	if len(args) == 2 {
		spacePlus = args[1].Truthy()
	}
	if spacePlus {
		return types.Ok(types.NewStr(url.QueryEscape(args[0].Str())))
	}
	return types.Ok(types.NewStr(strings.ReplaceAll(url.QueryEscape(args[0].Str()), "+", "%20")))
}

func builtinUrlDecode(ctx *Execution, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	return types.Ok(types.NewStr(toastURLDecode(args[0].Str())))
}

func toastURLDecode(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+2 >= len(s) {
			out = append(out, s[i])
			continue
		}
		hi, okHi := fromHex(s[i+1])
		lo, okLo := fromHex(s[i+2])
		if !okHi || !okLo {
			out = append(out, s[i])
			continue
		}
		b := hi<<4 | lo
		if b == 0 {
			break
		}
		out = append(out, b)
		i += 2
	}
	return string(out)
}

func fromHex(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	default:
		return 0, false
	}
}
