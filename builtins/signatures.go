package builtins

import (
	"fmt"
	"log/slog"
	"os"

	"sort"

	"github.com/MongooseMoo/barn/types"
)

func functionInfoEntry(name string, sig Signature) types.Value {
	argTypes := make([]types.Value, 0, len(sig.ArgTypes))
	for _, t := range sig.ArgTypes {
		argTypes = append(argTypes, types.NewInt(t))
	}
	return types.NewList([]types.Value{
		types.NewStr(name),
		types.NewInt(sig.MinArgs),
		types.NewInt(sig.MaxArgs),
		types.NewList(argTypes),
	})
}

// isObjectRef reports whether v is an object reference (regular or anonymous).
// The pre-de-box code used a single ObjValue type whose assertion matched both
// TYPE_OBJ and TYPE_ANON, so callers that asserted ObjValue accepted anonymous
// references too; this preserves that exact behavior.
func isObjectRef(v types.Value) bool {
	t := v.Type()
	return t == types.TYPE_OBJ || t == types.TYPE_ANON
}

func valueMatchesFunctionArgType(v types.Value, expected int64) bool {
	switch expected {
	case -1:
		return true
	case -2:
		t := v.Type()
		return t == types.TYPE_INT || t == types.TYPE_FLOAT
	default:
		return int64(v.Type()) == expected
	}
}

func validateFunctionArgs(sig Signature, args []types.Value) types.ErrorCode {
	if int64(len(args)) < sig.MinArgs {
		return types.E_ARGS
	}
	if sig.MaxArgs >= 0 && int64(len(args)) > sig.MaxArgs {
		return types.E_ARGS
	}
	for i, expected := range sig.ArgTypes {
		if i >= len(args) {
			break
		}
		matchedAlternative := false
		for _, alternative := range sig.Alternatives[i] {
			if valueMatchesFunctionArgType(args[i], alternative) {
				matchedAlternative = true
				break
			}
		}
		if matchedAlternative {
			continue
		}
		if !valueMatchesFunctionArgType(args[i], expected) {
			return types.E_TYPE
		}
	}
	if sig.VariadicType != nil {
		for i := len(sig.ArgTypes); i < len(args); i++ {
			if !valueMatchesFunctionArgType(args[i], *sig.VariadicType) {
				return types.E_TYPE
			}
		}
	}
	return types.E_NONE
}

func functionArgError(sig Signature, args []types.Value, code types.ErrorCode) types.Result {
	if code != types.E_ARGS || sig.MinArgs != sig.MaxArgs || sig.PlainArityError {
		return types.Err(code)
	}
	message := fmt.Sprintf("Incorrect number of arguments (expected %d; got %d)", sig.MinArgs, len(args))
	return types.Result{
		Flow:  types.FlowException,
		Error: code,
		Val: types.NewList([]types.Value{
			types.NewErr(code),
			types.NewStr(message),
			types.NewInt(0),
		}),
	}
}

func builtinFunctionInfo(ctx *Execution, args []types.Value) types.Result {
	r := ctx.Registry
	if r == nil {
		return types.Err(types.E_INVARG)
	}

	if len(args) > 1 {
		return types.Err(types.E_ARGS)
	}

	if len(args) == 0 {
		names := make([]string, 0, len(r.funcs))
		for name := range r.funcs {
			if r.entries[r.nameToID[name]].visibility == Hidden {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		entries := make([]types.Value, 0, len(names))
		for _, name := range names {
			entries = append(entries, functionInfoEntry(name, r.entries[r.nameToID[name]].sig))
		}
		return types.Ok(types.NewList(entries))
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	name := args[0].Str()
	id, found := r.nameToID[name]
	if !found || r.entries[id].visibility == Hidden {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(functionInfoEntry(name, r.entries[id].sig))
}

// debugCallFunction gates temporary call_function failure logging (shares the
// BARN_DEBUG_RETRY diagnosis env with the store/engine instrumentation).
var debugCallFunction = os.Getenv("BARN_DEBUG_RETRY") != ""

func builtinCallFunction(ctx *Execution, args []types.Value) types.Result {
	r := ctx.Registry
	if r == nil {
		return types.Err(types.E_INVARG)
	}

	if len(args) < 1 {
		return types.Err(types.E_ARGS)
	}
	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	name := args[0].Str()
	fn, found := r.Get(name)
	if !found {
		return types.Err(types.E_INVARG)
	}
	result := fn(ctx, args[1:])
	if debugCallFunction && result.Flow == types.FlowException {
		slog.Warn("DEBUG-CALLFN", slog.String("fn", name),
			slog.String("error", types.NewErr(result.Error).String()),
			slog.Int("nargs", len(args)-1))
	}
	if name == "max_object" && result.IsNormal() {
		if result.Val.Type() == types.TYPE_INT {
			return types.Ok(types.NewObj(types.ObjID(result.Val.Int())))
		}
	}
	return result
}
