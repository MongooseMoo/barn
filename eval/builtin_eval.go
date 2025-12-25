package eval

import (
	"barn/types"
)

// RegisterEvalBuiltin registers the eval() builtin function
// This must be called from the evaluator after the builtins registry is created
func (e *Evaluator) RegisterEvalBuiltin() {
	e.builtins.Register("eval", func(ctx *types.TaskContext, args []types.Value) types.Result {
		if len(args) != 1 {
			return types.Err(types.E_ARGS)
		}

		strVal, ok := args[0].(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}

		code := strVal.Value()

		// Use the evaluator's EvalString method
		return e.EvalString(code, ctx)
	})
}
