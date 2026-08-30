package builtins

import (
	"fmt"
	"log/slog"
	"os"

	"sort"

	"github.com/MongooseMoo/barn/types"
)

type functionSignature struct {
	minArg   int64
	maxArg   int64
	argTypes []int64
}

var knownFunctionSignatures = map[string]functionSignature{
	"chparent":                  {minArg: 2, maxArg: 2, argTypes: []int64{-1, int64(types.TYPE_OBJ)}},
	"chparents":                 {minArg: 2, maxArg: 2, argTypes: []int64{-1, int64(types.TYPE_LIST)}},
	"curl":                      {minArg: 1, maxArg: 3, argTypes: []int64{int64(types.TYPE_STR), -1, int64(types.TYPE_INT)}},
	"generate_json":             {minArg: 1, maxArg: 3, argTypes: []int64{-1, int64(types.TYPE_STR), -1}},
	"shutdown":                  {minArg: 0, maxArg: 2, argTypes: []int64{int64(types.TYPE_STR), -1}},
	"simplex_noise":             {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_LIST)}},
	"spellcheck":                {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_STR)}},
	"url_encode":                {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_STR)}},
	"url_decode":                {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_STR)}},
	"typeof":                    {minArg: 1, maxArg: 1, argTypes: []int64{-1}},
	"function_info":             {minArg: 0, maxArg: 1, argTypes: []int64{int64(types.TYPE_STR)}},
	"notify":                    {minArg: 2, maxArg: 4, argTypes: []int64{int64(types.TYPE_OBJ), int64(types.TYPE_STR), -1, -1}},
	"read_http":                 {minArg: 1, maxArg: 2, argTypes: []int64{int64(types.TYPE_STR), int64(types.TYPE_OBJ)}},
	"sqlite_open":               {minArg: 1, maxArg: 2, argTypes: []int64{int64(types.TYPE_STR), int64(types.TYPE_INT)}},
	"sqlite_close":              {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_INT)}},
	"sqlite_handles":            {minArg: 0, maxArg: 0, argTypes: []int64{}},
	"sqlite_info":               {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_INT)}},
	"sqlite_query":              {minArg: 2, maxArg: 3, argTypes: []int64{int64(types.TYPE_INT), int64(types.TYPE_STR), -1}},
	"sqlite_execute":            {minArg: 3, maxArg: 3, argTypes: []int64{int64(types.TYPE_INT), int64(types.TYPE_STR), int64(types.TYPE_LIST)}},
	"sqlite_last_insert_row_id": {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_INT)}},
	"sqlite_limit":              {minArg: 3, maxArg: 3, argTypes: []int64{int64(types.TYPE_INT), -1, int64(types.TYPE_INT)}},
	"sqlite_interrupt":          {minArg: 1, maxArg: 1, argTypes: []int64{int64(types.TYPE_INT)}},
	"server_version":            {minArg: 0, maxArg: 1, argTypes: []int64{-1}},
	"connected_players":         {minArg: 0, maxArg: 1, argTypes: []int64{-1}},
	"read_stdin":                {minArg: 0, maxArg: 0, argTypes: []int64{}},
}

var hiddenFunctionInfoExtensions = map[string]struct{}{
	"capitalize":        {},
	"connection_option": {},
	"downcase":          {},
	"implode":           {},
	"ltrim":             {},
	"mapmerge":          {},
	"read_stdin":        {},
	"rtrim":             {},
	"trim":              {},
	"unique":            {},
	"upcase":            {},
}

func functionInfoEntry(name string, sig functionSignature) types.Value {
	argTypes := make([]types.Value, 0, len(sig.argTypes))
	for _, t := range sig.argTypes {
		argTypes = append(argTypes, types.NewInt(t))
	}
	return types.NewList([]types.Value{
		types.NewStr(name),
		types.NewInt(sig.minArg),
		types.NewInt(sig.maxArg),
		types.NewList(argTypes),
	})
}

func lookupFunctionSignature(name string) (functionSignature, bool) {
	if sig, ok := knownFunctionSignatures[name]; ok {
		return sig, true
	}
	if sig, ok := generatedFunctionSignatures[name]; ok {
		return sig, true
	}
	return functionSignature{}, false
}

func signatureForFunction(name string) functionSignature {
	if sig, ok := lookupFunctionSignature(name); ok {
		return sig
	}
	return functionSignature{
		minArg:   0,
		maxArg:   -1,
		argTypes: []int64{-1},
	}
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

func validateKnownFunctionArgs(name string, sig functionSignature, args []types.Value) types.ErrorCode {
	if int64(len(args)) < sig.minArg {
		return types.E_ARGS
	}
	if sig.maxArg >= 0 && int64(len(args)) > sig.maxArg {
		return types.E_ARGS
	}
	for i, expected := range sig.argTypes {
		if i >= len(args) {
			break
		}
		if name == "next_recycled_object" && expected == int64(types.TYPE_OBJ) {
			if args[i].Type() == types.TYPE_INT {
				continue
			}
		}
		if !valueMatchesFunctionArgType(args[i], expected) {
			return types.E_TYPE
		}
	}
	return types.E_NONE
}

func knownFunctionArgError(sig functionSignature, args []types.Value, code types.ErrorCode) types.Result {
	if code != types.E_ARGS || sig.minArg != sig.maxArg {
		return types.Err(code)
	}
	message := fmt.Sprintf("Incorrect number of arguments (expected %d; got %d)", sig.minArg, len(args))
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
			if _, hidden := hiddenFunctionInfoExtensions[name]; hidden {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		entries := make([]types.Value, 0, len(names))
		for _, name := range names {
			entries = append(entries, functionInfoEntry(name, signatureForFunction(name)))
		}
		return types.Ok(types.NewList(entries))
	}

	if args[0].Type() != types.TYPE_STR {
		return types.Err(types.E_TYPE)
	}
	name := args[0].Str()
	if _, hidden := hiddenFunctionInfoExtensions[name]; hidden {
		return types.Err(types.E_INVARG)
	}
	if _, found := r.Get(name); !found {
		return types.Err(types.E_INVARG)
	}
	return types.Ok(functionInfoEntry(name, signatureForFunction(name)))
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
