package vm

import (
	"barn/db"
	"barn/types"
	"strings"
)

// RegisterEvalBuiltin registers the eval() builtin function
// This must be called from the evaluator after the builtins registry is created
func (e *Evaluator) RegisterEvalBuiltin() {
	e.builtins.Register("eval", func(ctx *types.TaskContext, args []types.Value) types.Result {
		if len(args) < 1 {
			return types.Err(types.E_ARGS)
		}

		// eval() requires programmer permissions
		// Check if the programmer has the programmer flag
		progObj := e.store.Get(ctx.Programmer)
		if progObj == nil || !progObj.Flags.Has(db.FlagProgrammer) {
			return types.Err(types.E_PERM)
		}

		// All arguments must be strings
		var lines []string
		for _, arg := range args {
			strVal, ok := arg.(types.StrValue)
			if !ok {
				return types.Err(types.E_TYPE)
			}
			lines = append(lines, strVal.Value())
		}

		// Join with newlines
		code := strings.Join(lines, "\n")

		// Use the evaluator's EvalString method
		result := e.EvalString(code, ctx)

		// eval() returns {success, result}
		// success = 1 if evaluation succeeded, 0 if error

		// Handle parse/syntax errors - return {0, {error_messages}}
		if result.Flow == types.FlowParseError {
			return types.Ok(types.NewList([]types.Value{
				types.NewInt(0),
				result.Val, // Val contains list of error strings
			}))
		}

		// Handle runtime errors - return {0, error_code}
		if result.Flow == types.FlowException {
			return types.Ok(types.NewList([]types.Value{
				types.NewInt(0),
				types.NewErr(result.Error),
			}))
		}

		// Return {1, result_value}
		return types.Ok(types.NewList([]types.Value{
			types.NewInt(1),
			result.Val,
		}))
	})
}
