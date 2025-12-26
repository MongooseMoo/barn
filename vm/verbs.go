package vm

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

	var objID types.ObjID
	isPrimitive := false

	// Check if target is an object or a primitive with a prototype
	objVal, ok := objResult.Val.(types.ObjValue)
	if ok {
		objID = objVal.ID()
	} else {
		// Not an object - check if it's a primitive with a prototype
		protoID := e.getPrimitivePrototype(objResult.Val)
		if protoID == types.ObjNothing {
			// No prototype for this type
			return types.Err(types.E_TYPE)
		}
		objID = protoID
		isPrimitive = true
	}

	// Suppress unused variable warning
	_ = isPrimitive

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

	// Update environment variables for the verb call
	// These need to be accessible from within the verb code
	oldVerbEnv, _ := e.env.Get("verb")
	oldThisEnv, _ := e.env.Get("this")
	oldCallerEnv, _ := e.env.Get("caller")
	oldArgsEnv, _ := e.env.Get("args")

	e.env.Set("verb", types.NewStr(expr.Verb))
	e.env.Set("this", types.NewObj(objID)) // this = object the verb was called ON (not where defined)
	e.env.Set("caller", types.NewObj(ctx.ThisObj)) // caller = old this
	e.env.Set("args", types.NewList(args))

	// TODO: Set up dobj, iobj, etc.

	// Execute the verb
	result := e.evalStatements(verb.Program.Statements, ctx)

	// Restore environment
	if oldVerbEnv != nil {
		e.env.Set("verb", oldVerbEnv)
	}
	if oldThisEnv != nil {
		e.env.Set("this", oldThisEnv)
	}
	if oldCallerEnv != nil {
		e.env.Set("caller", oldCallerEnv)
	}
	if oldArgsEnv != nil {
		e.env.Set("args", oldArgsEnv)
	}

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

// CallVerb calls a verb on an object directly without needing to parse an expression
// This is used by builtins like create() to call :initialize
// Returns the result of the verb call, or E_VERBNF if verb not found
func (e *Evaluator) CallVerb(objID types.ObjID, verbName string, args []types.Value, ctx *types.TaskContext) types.Result {
	// Check if object is valid
	if !e.store.Valid(objID) {
		return types.Err(types.E_INVIND)
	}

	// Look up the verb
	verb, defObjID, err := e.store.FindVerb(objID, verbName)
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
				Verb:       verbName,
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
	ctx.Verb = verbName

	// Update environment variables for the verb call
	oldVerbEnv, _ := e.env.Get("verb")
	oldThisEnv, _ := e.env.Get("this")
	oldCallerEnv, _ := e.env.Get("caller")
	oldArgsEnv, _ := e.env.Get("args")

	e.env.Set("verb", types.NewStr(verbName))
	e.env.Set("this", types.NewObj(objID)) // this = object the verb was called ON (not where defined)
	e.env.Set("caller", types.NewObj(ctx.ThisObj)) // caller = old this
	e.env.Set("args", types.NewList(args))

	// Execute the verb
	result := e.evalStatements(verb.Program.Statements, ctx)

	// Restore environment
	if oldVerbEnv != nil {
		e.env.Set("verb", oldVerbEnv)
	}
	if oldThisEnv != nil {
		e.env.Set("this", oldThisEnv)
	}
	if oldCallerEnv != nil {
		e.env.Set("caller", oldCallerEnv)
	}
	if oldArgsEnv != nil {
		e.env.Set("args", oldArgsEnv)
	}

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

// getPrimitivePrototype returns the prototype object ID for a primitive value
// Returns ObjNothing if no prototype is configured for this type
func (e *Evaluator) getPrimitivePrototype(val types.Value) types.ObjID {
	// Get #0 (system object)
	sysObj := e.store.Get(0)
	if sysObj == nil {
		return types.ObjNothing
	}

	// Determine the prototype property name based on value type
	var propName string
	switch val.(type) {
	case types.IntValue:
		propName = "int_proto"
	case types.FloatValue:
		propName = "float_proto"
	case types.StrValue:
		propName = "str_proto"
	case types.ListValue:
		propName = "list_proto"
	case types.MapValue:
		propName = "map_proto"
	case types.ErrValue:
		propName = "err_proto"
	case types.BoolValue:
		propName = "bool_proto"
	default:
		return types.ObjNothing
	}

	// Look up the prototype property on #0
	prop, ok := sysObj.Properties[propName]
	if !ok || prop == nil {
		return types.ObjNothing
	}

	// Get the object ID from the property value
	if objVal, ok := prop.Value.(types.ObjValue); ok {
		protoID := objVal.ID()
		// Check if the prototype object is valid
		if e.store.Valid(protoID) {
			return protoID
		}
	}

	return types.ObjNothing
}
