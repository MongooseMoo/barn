package eval

import (
	"barn/parser"
	"barn/types"
	"fmt"
)

// EvalStatements evaluates a sequence of statements
func (e *Evaluator) EvalStatements(stmts []parser.Stmt, ctx *types.TaskContext) types.Result {
	for _, stmt := range stmts {
		result := e.EvalStmt(stmt, ctx)
		// Propagate control flow (return, break, continue, error)
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

	switch s := stmt.(type) {
	case *parser.ExprStmt:
		return e.evalExprStmt(s, ctx)
	case *parser.IfStmt:
		return e.evalIfStmt(s, ctx)
	case *parser.WhileStmt:
		return e.evalWhileStmt(s, ctx)
	case *parser.ForStmt:
		return e.evalForStmt(s, ctx)
	case *parser.ReturnStmt:
		return e.evalReturnStmt(s, ctx)
	case *parser.BreakStmt:
		return e.evalBreakStmt(s, ctx)
	case *parser.ContinueStmt:
		return e.evalContinueStmt(s, ctx)
	case *parser.TryExceptStmt:
		return e.evalTryExceptStmt(s, ctx)
	case *parser.TryFinallyStmt:
		return e.evalTryFinallyStmt(s, ctx)
	case *parser.TryExceptFinallyStmt:
		return e.evalTryExceptFinallyStmt(s, ctx)
	case *parser.ScatterStmt:
		return e.evalScatterStmt(s, ctx)
	default:
		return types.Err(types.E_TYPE)
	}
}

// evalExprStmt evaluates an expression statement
func (e *Evaluator) evalExprStmt(stmt *parser.ExprStmt, ctx *types.TaskContext) types.Result {
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

// evalIfStmt evaluates if/elseif/else statements
func (e *Evaluator) evalIfStmt(stmt *parser.IfStmt, ctx *types.TaskContext) types.Result {
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

// evalWhileStmt evaluates while loops
func (e *Evaluator) evalWhileStmt(stmt *parser.WhileStmt, ctx *types.TaskContext) types.Result {
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

// evalForStmt evaluates for loops
func (e *Evaluator) evalForStmt(stmt *parser.ForStmt, ctx *types.TaskContext) types.Result {
	// Determine loop type: range, list, or map
	if stmt.RangeStart != nil {
		return e.evalForRange(stmt, ctx)
	} else {
		return e.evalForContainer(stmt, ctx)
	}
}

// evalForRange evaluates for loops over ranges: for x in [start..end]
func (e *Evaluator) evalForRange(stmt *parser.ForStmt, ctx *types.TaskContext) types.Result {
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

// evalForContainer evaluates for loops over lists, maps, and strings
func (e *Evaluator) evalForContainer(stmt *parser.ForStmt, ctx *types.TaskContext) types.Result {
	// Evaluate container expression
	containerResult := e.Eval(stmt.Container, ctx)
	if !containerResult.IsNormal() {
		return containerResult
	}

	container := containerResult.Val

	// Check if it's a list
	if list, ok := container.(types.ListValue); ok {
		return e.evalForList(stmt, &list, ctx)
	}

	// Check if it's a map
	if mapVal, ok := container.(types.MapValue); ok {
		return e.evalForMap(stmt, &mapVal, ctx)
	}

	// Check if it's a string
	if strVal, ok := container.(types.StrValue); ok {
		return e.evalForString(stmt, &strVal, ctx)
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

// evalForList evaluates for loops over lists
func (e *Evaluator) evalForList(stmt *parser.ForStmt, list *types.ListValue, ctx *types.TaskContext) types.Result {
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

// evalForMap evaluates for loops over maps
func (e *Evaluator) evalForMap(stmt *parser.ForStmt, mapVal *types.MapValue, ctx *types.TaskContext) types.Result {
	// Take a snapshot - mutations during iteration don't affect us
	pairs := mapVal.Pairs()

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

// evalForString evaluates for loops over strings (iterating characters)
func (e *Evaluator) evalForString(stmt *parser.ForStmt, strVal *types.StrValue, ctx *types.TaskContext) types.Result {
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

// evalReturnStmt evaluates return statements
func (e *Evaluator) evalReturnStmt(stmt *parser.ReturnStmt, ctx *types.TaskContext) types.Result {
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

// evalBreakStmt evaluates break statements
func (e *Evaluator) evalBreakStmt(stmt *parser.BreakStmt, ctx *types.TaskContext) types.Result {
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

// evalContinueStmt evaluates continue statements
func (e *Evaluator) evalContinueStmt(stmt *parser.ContinueStmt, ctx *types.TaskContext) types.Result {
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

// evalTryExceptStmt evaluates try/except statements
func (e *Evaluator) evalTryExceptStmt(stmt *parser.TryExceptStmt, ctx *types.TaskContext) types.Result {
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

// evalTryFinallyStmt evaluates try/finally statements
func (e *Evaluator) evalTryFinallyStmt(stmt *parser.TryFinallyStmt, ctx *types.TaskContext) types.Result {
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

// evalTryExceptFinallyStmt evaluates try/except/finally statements
func (e *Evaluator) evalTryExceptFinallyStmt(stmt *parser.TryExceptFinallyStmt, ctx *types.TaskContext) types.Result {
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

// evalScatterStmt evaluates scatter assignment: {a, ?b, @rest} = list
func (e *Evaluator) evalScatterStmt(stmt *parser.ScatterStmt, ctx *types.TaskContext) types.Result {
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
