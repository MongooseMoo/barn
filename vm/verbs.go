package vm

import (
	"barn/db"
	"barn/parser"
	"barn/task"
	"barn/types"
	"fmt"
)

func (e *Evaluator) verbCall(expr *parser.VerbCallExpr, ctx *types.TaskContext) types.Result {
	// Evaluate the object expression
	objResult := e.Eval(expr.Expr, ctx)
	if !objResult.IsNormal() {
		return objResult
	}

	var objID types.ObjID
	var primitiveValue types.Value
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
		primitiveValue = objResult.Val // Save the primitive value to use as 'this'
	}

	// Check if object is valid
	if !e.store.Valid(objID) {
		return types.Err(types.E_INVIND)
	}

	// Evaluate arguments, handling splice expressions (@arg)
	var args []types.Value
	for _, argExpr := range expr.Args {
		// Check if this is a splice expression
		if splice, ok := argExpr.(*parser.SpliceExpr); ok {
			// Evaluate the splice operand
			spliceResult := e.Eval(splice.Expr, ctx)
			if !spliceResult.IsNormal() {
				return spliceResult
			}
			// Splice requires a LIST operand
			if spliceResult.Val.Type() != types.TYPE_LIST {
				return types.Err(types.E_TYPE)
			}
			// Expand all elements from the spliced list into args
			list := spliceResult.Val.(types.ListValue)
			for i := 1; i <= list.Len(); i++ {
				args = append(args, list.Get(i))
			}
		} else {
			// Regular argument - evaluate and append
			argResult := e.Eval(argExpr, ctx)
			if !argResult.IsNormal() {
				return argResult
			}
			args = append(args, argResult.Val)
		}
	}

	// Get verb name (static or dynamic)
	verbName := expr.Verb
	if verbName == "" && expr.VerbExpr != nil {
		// Dynamic verb name - evaluate the expression
		verbResult := e.Eval(expr.VerbExpr, ctx)
		if !verbResult.IsNormal() {
			return verbResult
		}
		// The verb name must be a string
		strVal, ok := verbResult.Val.(types.StrValue)
		if !ok {
			return types.Err(types.E_TYPE)
		}
		verbName = strVal.Value()
	}

	// Look up the verb (in EvalVerbCall - handles expr:verb(args))
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
			fmt.Printf("[COMPILE FAIL] verbCall: #%d:%s compile errors:\n", objID, verbName)
			for i, err := range errors {
				fmt.Printf("[COMPILE FAIL]   %d: %v\n", i, err)
			}
			return types.Err(types.E_VERBNF)
		}
		verb.Program = program
	}

	// Push activation frame onto call stack (if we have a task)
	// NOTE: We do NOT use defer PopFrame() because we want to preserve the call stack
	// when an exception occurs, so the scheduler can extract it for tracebacks.
	// We explicitly pop the frame only on successful completion.
	var framePushed bool
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			// For primitives, 'This' should be the primitive object ID (the prototype)
			// but ThisValue should hold the actual primitive value for callers() and queued_tasks()
			frame := task.ActivationFrame{
				This:            defObjID,
				ThisValue:       primitiveValue, // Store primitive value for prototype calls
				Player:          ctx.Player,
				Programmer:      ctx.Programmer,
				Caller:          ctx.ThisObj, // The object that called this verb
				Verb:            verbName,
				VerbLoc:         defObjID,
				Args:            args,
				LineNumber:      0,     // TODO: Track line numbers during execution
				ServerInitiated: false, // This is a MOO-code verb call, not server-initiated
			}
			t.PushFrame(frame)
			framePushed = true
		}
	}

	// Set up verb call context
	oldThis := ctx.ThisObj
	oldThisValue := ctx.ThisValue
	oldVerb := ctx.Verb
	ctx.ThisObj = objID // this = object the verb was called on
	if isPrimitive {
		ctx.ThisValue = primitiveValue
	} else {
		ctx.ThisValue = nil
	}
	ctx.Verb = verbName

	// Update environment variables for the verb call
	// These need to be accessible from within the verb code
	oldVerbEnv, _ := e.env.Get("verb")
	oldThisEnv, _ := e.env.Get("this")
	oldCallerEnv, _ := e.env.Get("caller")
	oldArgsEnv, _ := e.env.Get("args")

	e.env.Set("verb", types.NewStr(verbName))
	// For primitives, 'this' should be the primitive value itself, not the prototype object
	if isPrimitive {
		e.env.Set("this", primitiveValue)
	} else {
		e.env.Set("this", types.NewObj(objID))
	}
	e.env.Set("caller", types.NewObj(oldThis)) // caller = previous this
	e.env.Set("args", types.NewList(args))

	// TODO: Set up dobj, iobj, etc.

	// Execute the verb
	result := e.statements(verb.Program.Statements, ctx)

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
	ctx.ThisValue = oldThisValue
	ctx.Verb = oldVerb

	// Pop frame only on successful completion
	// For errors (FlowException), leave the frame on the stack for traceback
	if result.Flow != types.FlowException && framePushed {
		if t, ok := ctx.Task.(*task.Task); ok {
			t.PopFrame()
		}
	}

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

// statements executes a sequence of statements
func (e *Evaluator) statements(stmts []parser.Stmt, ctx *types.TaskContext) types.Result {
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

	// Push activation frame EARLY, before verb lookup, so errors get proper traceback
	// We'll update VerbLoc after we find where the verb is defined
	// NOTE: We do NOT use defer PopFrame() because we want to keep the frame on error
	// so that scheduler.CallVerb can extract the call stack for tracebacks
	// The scheduler is responsible for popping frames after extracting the call stack
	var framePushed bool
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			frame := task.ActivationFrame{
				This:            objID,              // The object we're calling on
				Player:          ctx.Player,
				Programmer:      ctx.Programmer,
				Caller:          ctx.ThisObj,        // The object that called this verb
				Verb:            verbName,
				VerbLoc:         objID,              // Will be updated if verb found
				Args:            args,
				LineNumber:      1,                  // Line 1 for verb call
				ServerInitiated: ctx.ServerInitiated, // Mark server-initiated frames
			}
			t.PushFrame(frame)
			framePushed = true
		}
	}

	// Look up the verb (in CallVerb - direct verb invocation)
	verb, defObjID, err := e.store.FindVerb(objID, verbName)
	if err != nil {
		// Don't pop frame - leave it for traceback
		return types.Err(types.E_VERBNF)
	}

	// Update VerbLoc in the frame now that we know where the verb is defined
	if framePushed {
		if t, ok := ctx.Task.(*task.Task); ok {
			if topFrame := t.GetTopFrame(); topFrame != nil {
				topFrame.VerbLoc = defObjID
			}
		}
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
			fmt.Printf("[COMPILE ERROR] Failed to compile verb %s on #%d:\n", verbName, defObjID)
			for i, err := range errors {
				fmt.Printf("[COMPILE ERROR]   %d: %v\n", i, err)
			}
			fmt.Printf("[COMPILE ERROR] Verb code:\n")
			for i, line := range verb.Code {
				fmt.Printf("[COMPILE ERROR]   %d: %s\n", i, line)
			}
			return types.Err(types.E_VERBNF)
		}
		verb.Program = program
	}

	// Set up verb call context
	oldThis := ctx.ThisObj
	oldVerb := ctx.Verb
	ctx.ThisObj = objID // this = object the verb was called on
	ctx.Verb = verbName

	// Update environment variables for the verb call
	oldVerbEnv, _ := e.env.Get("verb")
	oldThisEnv, _ := e.env.Get("this")
	oldCallerEnv, _ := e.env.Get("caller")
	oldArgsEnv, _ := e.env.Get("args")
	oldPlayerEnv, _ := e.env.Get("player")

	e.env.Set("verb", types.NewStr(verbName))
	e.env.Set("this", types.NewObj(objID))    // this = object the verb was called ON (not where defined)
	e.env.Set("caller", types.NewObj(oldThis)) // caller = previous this
	e.env.Set("args", types.NewList(args))
	e.env.Set("player", types.NewObj(ctx.Player)) // player from context

	// Execute the verb
	result := e.statements(verb.Program.Statements, ctx)

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
	if oldPlayerEnv != nil {
		e.env.Set("player", oldPlayerEnv)
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
