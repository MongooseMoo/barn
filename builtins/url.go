package builtins

import (
	"net/url"
	"strings"

	"barn/kernel"
	"barn/types"
)

func builtinUrlEncode(ctx *kernel.TaskContext, args []types.Value) types.Result {
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

func builtinUrlDecode(ctx *kernel.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	decoded, err := url.QueryUnescape(args[0].Str())
	if err != nil {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(types.NewStr(decoded))
}
