package eval

import (
	"barn/db"
	"barn/parser"
	"barn/task"
	"barn/types"
)

// evalVerbCall evaluates a verb call expression: obj:verb(args)
func (e *Evaluator) evalVerbCall(expr *parser.VerbCallExpr, ctx *types.TaskContext) types.Result {
	// Evaluate the object expression
	objResult := e.Eval(expr.Expr, ctx)
	if !objResult.IsNormal() {
		return objResult
	}

	// Must be an object
	objVal, ok := objResult.Val.(types.ObjValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	objID := objVal.ID()

	// Check if object is valid
	if !e.store.Valid(objID) {
		return types.Err(types.E_INVIND)
	}

	// Evaluate arguments
	args := make([]types.Value, len(expr.Args))
	for i, argExpr := range expr.Args {
		argResult := e.Eval(argExpr, ctx)
		if !argResult.IsNormal() {
			return argResult
		}
		args[i] = argResult.Val
	}

	// Look up the verb
	verb, defObjID, err := e.store.FindVerb(objID, expr.Verb)
	if err != nil {
		return types.Err(types.E_VERBNF)
	}

	// Check execute permission
	if !verb.Perms.Has(db.VerbExecute) {
		// TODO: Check if caller is owner or wizard
		return types.Err(types.E_PERM)
	}

	// Compile verb if not already compiled
	if verb.Program == nil {
		program, errors := db.CompileVerb(verb.Code)
		if len(errors) > 0 {
			// Compilation error - return E_VERBNF (verb is broken)
			return types.Err(types.E_VERBNF)
		}
		verb.Program = program
	}

	// Push activation frame onto call stack (if we have a task)
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			frame := task.ActivationFrame{
				This:       defObjID,
				Player:     ctx.Player,
				Programmer: ctx.Programmer,
				Caller:     ctx.ThisObj, // The object that called this verb
				Verb:       expr.Verb,
				VerbLoc:    defObjID,
				Args:       args,
				LineNumber: 0, // TODO: Track line numbers during execution
			}
			t.PushFrame(frame)
			defer t.PopFrame()
		}
	}

	// Set up verb call context
	oldThis := ctx.ThisObj
	oldVerb := ctx.Verb
	ctx.ThisObj = defObjID // this = object where verb is defined
	ctx.Verb = expr.Verb

	// TODO: Set up dobj, iobj, etc.

	// Execute the verb
	result := e.evalStatements(verb.Program.Statements, ctx)

	// Restore context
	ctx.ThisObj = oldThis
	ctx.Verb = oldVerb

	// If the verb returned, extract the value
	if result.Flow == types.FlowReturn {
		return types.Ok(result.Val)
	}

	// If normal completion, return 0
	if result.IsNormal() {
		return types.Ok(types.NewInt(0))
	}

	// Propagate errors, break, continue
	return result
}

// evalStatements executes a sequence of statements
func (e *Evaluator) evalStatements(stmts []parser.Stmt, ctx *types.TaskContext) types.Result {
	var result types.Result
	for _, stmt := range stmts {
		result = e.EvalStmt(stmt, ctx)
		if !result.IsNormal() {
			return result
		}
	}
	return types.Ok(types.NewInt(0))
}
