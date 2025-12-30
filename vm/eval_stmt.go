package vm

import (
	"barn/parser"
	"barn/task"
	"barn/types"
	"fmt"
	"sort"
	"strings"
	"time"
)

// EvalStatements evaluates a sequence of statements
func (e *Evaluator) EvalStatements(stmts []parser.Stmt, ctx *types.TaskContext) types.Result {
	// Set up context variables from task context
	e.env.Set("player", types.NewObj(ctx.Player))
	for _, stmt := range stmts {
		result := e.EvalStmt(stmt, ctx)
		// FlowFork: fork schedules a background task, but the parent continues execution
		if result.Flow == types.FlowFork {
			// If we have a ForkCreator (scheduler), create the child task and assign fork variable
			if ctx.Task != nil {
				if t, ok := ctx.Task.(*task.Task); ok && t.ForkCreator != nil && result.ForkInfo != nil {
					childID := t.ForkCreator.CreateForkedTask(t, result.ForkInfo)

					// If named fork, store child ID in parent's variable
					if result.ForkInfo.VarName != "" {
						e.env.Set(result.ForkInfo.VarName, types.NewInt(childID))
					}
				}
			}
			// Parent continues execution
			continue
		}
		// FlowSuspend: for synchronous execution, sleep and continue
		if result.Flow == types.FlowSuspend {
			// Get suspend duration from result.Val
			var seconds float64
			if fv, ok := result.Val.(types.FloatValue); ok {
				seconds = fv.Val
			} else if iv, ok := result.Val.(types.IntValue); ok {
				seconds = float64(iv.Val)
			}
			// Sleep for the duration (timed suspend)
			if seconds > 0 {
				time.Sleep(time.Duration(seconds * float64(time.Second)))
			}
			// Continue to next statement after suspend
			continue
		}
		// Propagate other control flow (return, break, continue, error)
		if !result.IsNormal() {
			return result
		}
	}
	// Normal completion - return 0 (default)
	return types.Ok(types.NewInt(0))
}

// EvalStmt evaluates a single statement
func (e *Evaluator) EvalStmt(stmt parser.Stmt, ctx *types.TaskContext) types.Result {
	// Tick counting
	if !ctx.ConsumeTick() {
		return types.Err(types.E_MAXREC)
	}

	// Update line number in current activation frame
	if ctx.Task != nil {
		if t, ok := ctx.Task.(*task.Task); ok {
			t.UpdateLineNumber(stmt.Position().Line)
		}
	}

	switch s := stmt.(type) {
	case *parser.ExprStmt:
		return e.exprStmt(s, ctx)
	case *parser.IfStmt:
		return e.ifStmt(s, ctx)
	case *parser.WhileStmt:
		return e.whileStmt(s, ctx)
	case *parser.ForStmt:
		return e.forStmt(s, ctx)
	case *parser.ReturnStmt:
		return e.returnStmt(s, ctx)
	case *parser.BreakStmt:
		return e.breakStmt(s, ctx)
	case *parser.ContinueStmt:
		return e.continueStmt(s, ctx)
	case *parser.TryExceptStmt:
		return e.tryExceptStmt(s, ctx)
	case *parser.TryFinallyStmt:
		return e.tryFinallyStmt(s, ctx)
	case *parser.TryExceptFinallyStmt:
		return e.tryExceptFinallyStmt(s, ctx)
	case *parser.ScatterStmt:
		return e.scatterStmt(s, ctx)
	case *parser.ForkStmt:
		return e.forkStmt(s, ctx)
	default:
		return types.Err(types.E_TYPE)
	}
}

// exprStmt evaluates an expression statement
func (e *Evaluator) exprStmt(stmt *parser.ExprStmt, ctx *types.TaskContext) types.Result {
	if stmt.Expr == nil {
		// Empty statement
		return types.Ok(types.NewInt(0))
	}

	// Evaluate expression and discard result (unless it's an error/control flow)
	result := e.Eval(stmt.Expr, ctx)
	if !result.IsNormal() {
		return result
	}

	// Normal expression - discard value, continue
	return types.Ok(types.NewInt(0))
}

// ifStmt evaluates if/elseif/else statements
func (e *Evaluator) ifStmt(stmt *parser.IfStmt, ctx *types.TaskContext) types.Result {
	// Evaluate main condition
	condResult := e.Eval(stmt.Condition, ctx)
	if !condResult.IsNormal() {
		return condResult
	}

	if condResult.Val.Truthy() {
		// Execute if body
		return e.EvalStatements(stmt.Body, ctx)
	}

	// Try elseif clauses
	for _, elseIf := range stmt.ElseIfs {
		elseIfCondResult := e.Eval(elseIf.Condition, ctx)
		if !elseIfCondResult.IsNormal() {
			return elseIfCondResult
		}

		if elseIfCondResult.Val.Truthy() {
			return e.EvalStatements(elseIf.Body, ctx)
		}
	}

	// Execute else body if present
	if stmt.Else != nil {
		return e.EvalStatements(stmt.Else, ctx)
	}

	// No condition matched, no else - return normal
	return types.Ok(types.NewInt(0))
}

// whileStmt evaluates while loops
func (e *Evaluator) whileStmt(stmt *parser.WhileStmt, ctx *types.TaskContext) types.Result {
	for {
		// Evaluate condition
		condResult := e.Eval(stmt.Condition, ctx)
		if !condResult.IsNormal() {
			return condResult
		}

		// Check if condition is falsy - exit loop
		if !condResult.Val.Truthy() {
			break
		}

		// Execute body
		bodyResult := e.EvalStatements(stmt.Body, ctx)

		// Handle control flow
		switch bodyResult.Flow {
		case types.FlowReturn, types.FlowException:
			// Propagate return or error
			return bodyResult
		case types.FlowBreak:
			// Check if break targets this loop (or any loop if no label)
			if bodyResult.Label == "" || bodyResult.Label == stmt.Label {
				// Break value becomes loop value, or 0 if no value
				if bodyResult.Val != nil {
					return types.Ok(bodyResult.Val)
				}
				return types.Ok(types.NewInt(0))
			}
			// Break targets outer loop - propagate
			return bodyResult
		case types.FlowContinue:
			// Check if continue targets this loop
			if bodyResult.Label == "" || bodyResult.Label == stmt.Label {
				// Continue to next iteration
				continue
			}
			// Continue targets outer loop - propagate
			return bodyResult
		}
	}

	return types.Ok(types.NewInt(0))
}

// forStmt evaluates for loops
func (e *Evaluator) forStmt(stmt *parser.ForStmt, ctx *types.TaskContext) types.Result {
	// Determine loop type: range, list, or map
	if stmt.RangeStart != nil {
		return e.forRange(stmt, ctx)
	} else {
		return e.forContainer(stmt, ctx)
	}
}

// forRange evaluates for loops over ranges: for x in [start..end]
func (e *Evaluator) forRange(stmt *parser.ForStmt, ctx *types.TaskContext) types.Result {
	// Evaluate start
	startResult := e.Eval(stmt.RangeStart, ctx)
	if !startResult.IsNormal() {
		return startResult
	}

	startInt, ok := startResult.Val.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Evaluate end
	endResult := e.Eval(stmt.RangeEnd, ctx)
	if !endResult.IsNormal() {
		return endResult
	}

	endInt, ok := endResult.Val.(types.IntValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Iterate from start to end (inclusive)
	for i := startInt.Val; i <= endInt.Val; i++ {
		// Bind loop variable
		e.env.Set(stmt.Value, types.NewInt(i))

		// Execute body
		bodyResult := e.EvalStatements(stmt.Body, ctx)

		// Handle control flow
		switch bodyResult.Flow {
		case types.FlowReturn, types.FlowException:
			return bodyResult
		case types.FlowBreak:
			if forLoopLabelMatches(bodyResult.Label, stmt) {
				// Break value becomes loop value, or 0 if no value
				if bodyResult.Val != nil {
					return types.Ok(bodyResult.Val)
				}
				return types.Ok(types.NewInt(0))
			}
			return bodyResult
		case types.FlowContinue:
			if forLoopLabelMatches(bodyResult.Label, stmt) {
				continue
			}
			return bodyResult
		}
	}

	return types.Ok(types.NewInt(0))
}

// forContainer evaluates for loops over lists, maps, and strings
func (e *Evaluator) forContainer(stmt *parser.ForStmt, ctx *types.TaskContext) types.Result {
	// Evaluate container expression
	containerResult := e.Eval(stmt.Container, ctx)
	if !containerResult.IsNormal() {
		return containerResult
	}

	container := containerResult.Val

	// Check if it's a list
	if list, ok := container.(types.ListValue); ok {
		return e.forList(stmt, &list, ctx)
	}

	// Check if it's a map
	if mapVal, ok := container.(types.MapValue); ok {
		return e.forMap(stmt, &mapVal, ctx)
	}

	// Check if it's a string
	if strVal, ok := container.(types.StrValue); ok {
		return e.forString(stmt, &strVal, ctx)
	}

	// Not a list, map, or string - type error
	return types.Err(types.E_TYPE)
}

// forLoopLabelMatches checks if a break/continue label matches this for loop
// In MOO, the loop variable name(s) act as implicit labels for the loop
func forLoopLabelMatches(label string, stmt *parser.ForStmt) bool {
	if label == "" {
		return true // No label means innermost loop
	}
	if stmt.Label != "" && label == stmt.Label {
		return true // Explicit loop label matches
	}
	if label == stmt.Value {
		return true // Matches first loop variable
	}
	if stmt.Index != "" && label == stmt.Index {
		return true // Matches second loop variable (index/key)
	}
	return false
}

// forList evaluates for loops over lists
func (e *Evaluator) forList(stmt *parser.ForStmt, list *types.ListValue, ctx *types.TaskContext) types.Result {
	// Take a snapshot - mutations during iteration don't affect us
	elements := list.Elements()

	for i, elem := range elements {
		// Bind value
		e.env.Set(stmt.Value, elem)

		// Bind index if requested (1-based)
		if stmt.Index != "" {
			e.env.Set(stmt.Index, types.NewInt(int64(i+1)))
		}

		// Execute body
		bodyResult := e.EvalStatements(stmt.Body, ctx)

		// Handle control flow
		switch bodyResult.Flow {
		case types.FlowReturn, types.FlowException:
			return bodyResult
		case types.FlowBreak:
			if forLoopLabelMatches(bodyResult.Label, stmt) {
				// Break value becomes loop value, or 0 if no value
				if bodyResult.Val != nil {
					return types.Ok(bodyResult.Val)
				}
				return types.Ok(types.NewInt(0))
			}
			return bodyResult
		case types.FlowContinue:
			if forLoopLabelMatches(bodyResult.Label, stmt) {
				continue
			}
			return bodyResult
		}
	}

	return types.Ok(types.NewInt(0))
}

// forMap evaluates for loops over maps
func (e *Evaluator) forMap(stmt *parser.ForStmt, mapVal *types.MapValue, ctx *types.TaskContext) types.Result {
	// Take a snapshot - mutations during iteration don't affect us
	pairs := mapVal.Pairs()
	// Sort pairs by key in MOO canonical order
	sortForMapPairs(pairs)

	for _, pair := range pairs {
		key := pair[0]
		value := pair[1]

		// Bind value (first variable receives value)
		e.env.Set(stmt.Value, value)

		// Bind key if requested (second variable receives key)
		if stmt.Index != "" {
			e.env.Set(stmt.Index, key)
		}

		// Execute body
		bodyResult := e.EvalStatements(stmt.Body, ctx)

		// Handle control flow
		switch bodyResult.Flow {
		case types.FlowReturn, types.FlowException:
			return bodyResult
		case types.FlowBreak:
			if forLoopLabelMatches(bodyResult.Label, stmt) {
				// Break value becomes loop value, or 0 if no value
				if bodyResult.Val != nil {
					return types.Ok(bodyResult.Val)
				}
				return types.Ok(types.NewInt(0))
			}
			return bodyResult
		case types.FlowContinue:
			if forLoopLabelMatches(bodyResult.Label, stmt) {
				continue
			}
			return bodyResult
		}
	}

	return types.Ok(types.NewInt(0))
}

// forString evaluates for loops over strings (iterating characters)
func (e *Evaluator) forString(stmt *parser.ForStmt, strVal *types.StrValue, ctx *types.TaskContext) types.Result {
	// Get characters as runes for proper Unicode handling
	s := strVal.Value()
	runes := []rune(s)

	for i, r := range runes {
		// Bind value (character as string)
		e.env.Set(stmt.Value, types.NewStr(string(r)))

		// Bind index if requested (1-based)
		if stmt.Index != "" {
			e.env.Set(stmt.Index, types.NewInt(int64(i+1)))
		}

		// Execute body
		bodyResult := e.EvalStatements(stmt.Body, ctx)

		// Handle control flow
		switch bodyResult.Flow {
		case types.FlowReturn, types.FlowException:
			return bodyResult
		case types.FlowBreak:
			if forLoopLabelMatches(bodyResult.Label, stmt) {
				// Break value becomes loop value, or 0 if no value
				if bodyResult.Val != nil {
					return types.Ok(bodyResult.Val)
				}
				return types.Ok(types.NewInt(0))
			}
			return bodyResult
		case types.FlowContinue:
			if forLoopLabelMatches(bodyResult.Label, stmt) {
				continue
			}
			return bodyResult
		}
	}

	return types.Ok(types.NewInt(0))
}

// returnStmt evaluates return statements
func (e *Evaluator) returnStmt(stmt *parser.ReturnStmt, ctx *types.TaskContext) types.Result {
	var value types.Value

	if stmt.Value != nil {
		// Evaluate return expression
		result := e.Eval(stmt.Value, ctx)
		if !result.IsNormal() {
			return result
		}
		value = result.Val
	} else {
		// No expression - return 0
		value = types.NewInt(0)
	}

	return types.Return(value)
}

// breakStmt evaluates break statements
func (e *Evaluator) breakStmt(stmt *parser.BreakStmt, ctx *types.TaskContext) types.Result {
	// If there's a value expression, evaluate it
	var val types.Value
	if stmt.Value != nil {
		result := e.Eval(stmt.Value, ctx)
		if !result.IsNormal() {
			return result
		}
		val = result.Val
	}
	return types.Break(stmt.Label, val)
}

// continueStmt evaluates continue statements
func (e *Evaluator) continueStmt(stmt *parser.ContinueStmt, ctx *types.TaskContext) types.Result {
	return types.Continue(stmt.Label)
}

// EvalProgram is a convenience function to evaluate a program from source
func (e *Evaluator) EvalProgram(source string) (types.Value, error) {
	p := parser.NewParser(source)
	stmts, err := p.ParseProgram()
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	ctx := types.NewTaskContext()
	result := e.EvalStatements(stmts, ctx)

	if result.Flow == types.FlowException {
		errVal := types.NewErr(result.Error)
		return errVal, nil
	}

	if result.Flow == types.FlowReturn {
		return result.Val, nil
	}

	// Should not get break/continue outside of loops
	if result.Flow == types.FlowBreak || result.Flow == types.FlowContinue {
		return nil, fmt.Errorf("break/continue outside of loop")
	}

	return result.Val, nil
}

// tryExceptStmt evaluates try/except statements
func (e *Evaluator) tryExceptStmt(stmt *parser.TryExceptStmt, ctx *types.TaskContext) types.Result {
	// Execute try body
	result := e.EvalStatements(stmt.Body, ctx)

	// If no error, return normally
	if !result.IsError() {
		return result
	}

	// Error occurred - check except clauses
	errorCode := result.Error
	for _, except := range stmt.Excepts {
		// Check if this except clause handles this error
		if except.IsAny || e.matchesErrorCode(errorCode, except.Codes) {
			// Bind error to variable if specified
			if except.Variable != "" {
				e.env.Set(except.Variable, types.NewErr(errorCode))
			}

			// Execute except body
			return e.EvalStatements(except.Body, ctx)
		}
	}

	// No matching except clause - propagate error
	return result
}

// tryFinallyStmt evaluates try/finally statements
func (e *Evaluator) tryFinallyStmt(stmt *parser.TryFinallyStmt, ctx *types.TaskContext) types.Result {
	// Execute try body
	result := e.EvalStatements(stmt.Body, ctx)

	// Always execute finally block
	finallyResult := e.EvalStatements(stmt.Finally, ctx)

	// If finally returned/broke/continued/errored, that takes precedence
	if !finallyResult.IsNormal() {
		return finallyResult
	}

	// Otherwise return the try result (error or normal)
	return result
}

// tryExceptFinallyStmt evaluates try/except/finally statements
func (e *Evaluator) tryExceptFinallyStmt(stmt *parser.TryExceptFinallyStmt, ctx *types.TaskContext) types.Result {
	// Execute try body
	result := e.EvalStatements(stmt.Body, ctx)

	// If error occurred, try to catch it
	if result.IsError() {
		errorCode := result.Error
		for _, except := range stmt.Excepts {
			if except.IsAny || e.matchesErrorCode(errorCode, except.Codes) {
				// Bind error to variable if specified
				if except.Variable != "" {
					e.env.Set(except.Variable, types.NewErr(errorCode))
				}

				// Execute except body
				result = e.EvalStatements(except.Body, ctx)
				break
			}
		}
	}

	// Always execute finally block
	finallyResult := e.EvalStatements(stmt.Finally, ctx)

	// If finally returned/broke/continued/errored, that takes precedence
	if !finallyResult.IsNormal() {
		return finallyResult
	}

	// Otherwise return the result (from try or except)
	return result
}

// matchesErrorCode checks if an error code is in the list of codes
func (e *Evaluator) matchesErrorCode(code types.ErrorCode, codes []types.ErrorCode) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}

// scatterStmt evaluates scatter assignment: {a, ?b, @rest} = list
func (e *Evaluator) scatterStmt(stmt *parser.ScatterStmt, ctx *types.TaskContext) types.Result {
	// Evaluate the value expression
	valueResult := e.Eval(stmt.Value, ctx)
	if !valueResult.IsNormal() {
		return valueResult
	}

	// Must be a list
	listVal, ok := valueResult.Val.(types.ListValue)
	if !ok {
		return types.Err(types.E_TYPE)
	}

	// Get list elements
	elements := listVal.Elements()
	elemIdx := 0

	// Track if we've seen @rest
	var restTarget *parser.ScatterTarget

	// Process targets
	for i := range stmt.Targets {
		target := &stmt.Targets[i]

		if target.Rest {
			restTarget = target
			continue // Process rest at the end
		}

		// Check if we have an element for this target
		if elemIdx >= len(elements) {
			if target.Optional {
				// Use default value or 0
				var val types.Value
				if target.Default != nil {
					defaultResult := e.Eval(target.Default, ctx)
					if !defaultResult.IsNormal() {
						return defaultResult
					}
					val = defaultResult.Val
				} else {
					val = types.NewInt(0)
				}
				e.env.Set(target.Name, val)
			} else {
				// Required target with no value
				return types.Err(types.E_ARGS)
			}
		} else {
			// Bind element to variable
			e.env.Set(target.Name, elements[elemIdx])
			elemIdx++
		}
	}

	// Handle @rest if present
	if restTarget != nil {
		// Collect remaining elements
		remaining := elements[elemIdx:]
		e.env.Set(restTarget.Name, types.NewList(remaining))
	} else {
		// If no @rest and extra elements, that's an error
		if elemIdx < len(elements) {
			return types.Err(types.E_ARGS)
		}
	}

	return types.Ok(types.NewInt(0))
}

// sortForMapPairs sorts map pairs by key in MOO canonical order:
// INT < OBJ < FLOAT < ERR < STR
// Within each type, sorted by value
func sortForMapPairs(pairs [][2]types.Value) {
	sort.Slice(pairs, func(i, j int) bool {
		return compareForMapKeys(pairs[i][0], pairs[j][0]) < 0
	})
}

// compareForMapKeys returns negative if a < b, 0 if equal, positive if a > b
// Order: INT (0) < OBJ (1) < FLOAT (2) < ERR (3) < STR (4)
func compareForMapKeys(a, b types.Value) int {
	typeOrder := func(v types.Value) int {
		switch v.Type() {
		case types.TYPE_INT:
			return 0
		case types.TYPE_OBJ:
			return 1
		case types.TYPE_FLOAT:
			return 2
		case types.TYPE_ERR:
			return 3
		case types.TYPE_STR:
			return 4
		default:
			return 5
		}
	}

	aOrder := typeOrder(a)
	bOrder := typeOrder(b)
	if aOrder != bOrder {
		return aOrder - bOrder
	}

	// Same type, compare values
	switch av := a.(type) {
	case types.IntValue:
		bv := b.(types.IntValue)
		if av.Val < bv.Val {
			return -1
		} else if av.Val > bv.Val {
			return 1
		}
		return 0
	case types.FloatValue:
		bv := b.(types.FloatValue)
		if av.Val < bv.Val {
			return -1
		} else if av.Val > bv.Val {
			return 1
		}
		return 0
	case types.ObjValue:
		bv := b.(types.ObjValue)
		if av.ID() < bv.ID() {
			return -1
		} else if av.ID() > bv.ID() {
			return 1
		}
		return 0
	case types.ErrValue:
		bv := b.(types.ErrValue)
		if av.Code() < bv.Code() {
			return -1
		} else if av.Code() > bv.Code() {
			return 1
		}
		return 0
	case types.StrValue:
		bv := b.(types.StrValue)
		// Case-insensitive comparison for strings
		return strings.Compare(strings.ToLower(av.Value()), strings.ToLower(bv.Value()))
	}
	return 0
}

// forkStmt evaluates a fork statement
func (e *Evaluator) forkStmt(stmt *parser.ForkStmt, ctx *types.TaskContext) types.Result {
	// 1. Evaluate delay expression
	delayResult := e.Eval(stmt.Delay, ctx)
	if !delayResult.IsNormal() {
		return delayResult
	}

	// 2. Convert delay to duration (must be numeric and >= 0)
	var delaySeconds float64
	switch v := delayResult.Val.(type) {
	case types.IntValue:
		delaySeconds = float64(v.Val)
	case types.FloatValue:
		delaySeconds = v.Val
	default:
		return types.Err(types.E_TYPE)
	}

	if delaySeconds < 0 {
		return types.Err(types.E_INVARG)
	}

	delay := time.Duration(delaySeconds * float64(time.Second))

	// 3. Deep copy variable environment
	allVars := e.env.GetAllVars()
	varEnv := make(map[string]types.Value)
	for k, v := range allVars {
		varEnv[k] = deepCopyValue(v)
	}

	// 4. Get caller from environment
	caller := types.ObjNothing
	if callerVal, ok := e.env.Get("caller"); ok {
		if callerObj, ok := callerVal.(types.ObjValue); ok {
			caller = callerObj.ID()
		}
	}

	// 5. Build fork info
	forkInfo := &types.ForkInfo{
		Body:      stmt.Body, // []parser.Stmt
		Delay:     delay,
		VarName:   stmt.VarName,
		Variables: varEnv,
		ThisObj:   ctx.ThisObj,
		Player:    ctx.Player,
		Caller:    caller,
		Verb:      ctx.Verb,
	}

	// 5. Return fork flow - scheduler will handle task creation
	return types.Fork(forkInfo)
}

// deepCopyValue creates a deep copy of a value
func deepCopyValue(v types.Value) types.Value {
	switch val := v.(type) {
	case types.ListValue:
		items := val.Elements()
		newItems := make([]types.Value, len(items))
		for i, item := range items {
			newItems[i] = deepCopyValue(item)
		}
		return types.NewList(newItems)
	case types.MapValue:
		// Deep copy map
		pairs := val.Pairs()
		newPairs := make([][2]types.Value, len(pairs))
		for i, pair := range pairs {
			newPairs[i] = [2]types.Value{
				deepCopyValue(pair[0]),
				deepCopyValue(pair[1]),
			}
		}
		return types.NewMap(newPairs)
	case types.WaifValue:
		// For waif, we need to deep copy properties
		// Since we can't iterate properties directly, we'll just return the value as-is for now
		// In a full implementation, WaifValue should expose a way to copy properties
		// TODO: Implement proper waif deep copy
		return val
	default:
		// Immutable types (int, float, str, obj, err, bool) don't need copying
		return v
	}
}
