package builtins

import (
	"barn/types"
)

func builtinUrlEncode(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if _, ok := args[0].(types.StrValue); !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Err(types.E_PERM)
}

func builtinUrlDecode(ctx *types.TaskContext, args []types.Value) types.Result {
	if len(args) != 1 {
		return types.Err(types.E_ARGS)
	}
	if _, ok := args[0].(types.StrValue); !ok {
		return types.Err(types.E_TYPE)
	}
	return types.Err(types.E_PERM)
}
