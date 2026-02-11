package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/parser"
	"barn/types"
	"fmt"
)

// Evaluator walks the AST and evaluates expressions/statements
type Evaluator struct {
	env      *Environment
	builtins *builtins.Registry
	store    *db.Store
}

// NewEvaluator creates a new evaluator with a fresh environment
func NewEvaluator() *Evaluator {
	store := db.NewStore()
	registry := builtins.NewRegistry()
	registry.RegisterObjectBuiltins(store)
	registry.RegisterPropertyBuiltins(store)
	registry.RegisterVerbBuiltins(store)
	registry.RegisterCryptoBuiltins(store)
	registry.RegisterSystemBuiltins(store)
	e := &Evaluator{
		env:      NewEnvironment(),
		builtins: registry,
		store:    store,
	}
	e.RegisterEvalBuiltin()
	e.RegisterPassBuiltin()
	e.registerVerbCaller()
	return e
}

// NewEvaluatorWithEnv creates a new evaluator with a given environment
func NewEvaluatorWithEnv(env *Environment) *Evaluator {
	store := db.NewStore()
	registry := builtins.NewRegistry()
	registry.RegisterObjectBuiltins(store)
	registry.RegisterPropertyBuiltins(store)
	registry.RegisterVerbBuiltins(store)
	registry.RegisterCryptoBuiltins(store)
	registry.RegisterSystemBuiltins(store)
	e := &Evaluator{
		env:      env,
		builtins: registry,
		store:    store,
	}
	e.RegisterEvalBuiltin()
	e.RegisterPassBuiltin()
	e.registerVerbCaller()
	return e
}

// NewEvaluatorWithEnvAndStore creates a new evaluator with a given environment and store
func NewEvaluatorWithEnvAndStore(env *Environment, store *db.Store) *Evaluator {
	registry := builtins.NewRegistry()
	registry.RegisterObjectBuiltins(store)
	registry.RegisterPropertyBuiltins(store)
	registry.RegisterVerbBuiltins(store)
	registry.RegisterCryptoBuiltins(store)
	registry.RegisterSystemBuiltins(store)
	e := &Evaluator{
		env:      env,
		builtins: registry,
		store:    store,
	}
	e.RegisterEvalBuiltin()
	e.RegisterPassBuiltin()
	e.registerVerbCaller()
	return e
}

// NewEvaluatorWithStore creates a new evaluator with a given store
func NewEvaluatorWithStore(store *db.Store) *Evaluator {
	registry := builtins.NewRegistry()
	registry.RegisterObjectBuiltins(store)
	registry.RegisterPropertyBuiltins(store)
	registry.RegisterVerbBuiltins(store)
	registry.RegisterCryptoBuiltins(store)
	registry.RegisterSystemBuiltins(store)
	e := &Evaluator{
		env:      NewEnvironment(),
		builtins: registry,
		store:    store,
	}
	e.RegisterEvalBuiltin()
	e.RegisterPassBuiltin()
	e.registerVerbCaller()
	return e
}

// registerVerbCaller registers the verb caller callback on the builtin registry
func (e *Evaluator) registerVerbCaller() {
	e.builtins.SetVerbCaller(func(objID types.ObjID, verbName string, args []types.Value, ctx *types.TaskContext) types.Result {
		return e.CallVerb(objID, verbName, args, ctx)
	})
}

// Eval evaluates an AST node and returns a Result
// All evaluation methods follow this pattern:
// - Accept *TaskContext for tick counting and permissions
// - Return Result (not raw Value) to unify error handling and control flow
// - Check tick limit before processing
func (e *Evaluator) Eval(node parser.Node, ctx *types.TaskContext) types.Result {
	// Tick counting - protect against infinite loops
	if !ctx.ConsumeTick() {
		return types.Err(types.E_MAXREC)
	}

	// Dispatch based on node type
	switch n := node.(type) {
	case *parser.LiteralExpr:
		return e.literal(n, ctx)
	case *parser.IdentifierExpr:
		return e.identifier(n, ctx)
	case *parser.UnaryExpr:
		return e.unary(n, ctx)
	case *parser.BinaryExpr:
		return e.binary(n, ctx)
	case *parser.TernaryExpr:
		return e.ternary(n, ctx)
	case *parser.AssignExpr:
		return e.assign(n, ctx)
	case *parser.ParenExpr:
		return e.Eval(n.Expr, ctx)
	case *parser.BuiltinCallExpr:
		return e.builtinCall(n, ctx)
	case *parser.IndexExpr:
		return e.index(n, ctx)
	case *parser.RangeExpr:
		return e.rangeExpr(n, ctx)
	case *parser.IndexMarkerExpr:
		return e.indexMarker(n, ctx)
	case *parser.PropertyExpr:
		return e.property(n, ctx)
	case *parser.VerbCallExpr:
		return e.verbCall(n, ctx)
	case *parser.SpliceExpr:
		return e.splice(n, ctx)
	case *parser.CatchExpr:
		return e.catch(n, ctx)
	case *parser.ListExpr:
		return e.listExpr(n, ctx)
	case *parser.ListRangeExpr:
		return e.listRangeExpr(n, ctx)
	case *parser.MapExpr:
		return e.mapExpr(n, ctx)
	default:
		// Unknown node type - this should never happen if parser is correct
		return types.Err(types.E_TYPE)
	}
}

// literal evaluates a literal expression
// Literals are already Values, just wrap in Result
func (e *Evaluator) literal(node *parser.LiteralExpr, ctx *types.TaskContext) types.Result {
	return types.Ok(node.Value)
}

// identifier looks up a variable by name
// Returns E_VARNF if the variable is not defined
func (e *Evaluator) identifier(node *parser.IdentifierExpr, ctx *types.TaskContext) types.Result {
	val, ok := e.env.Get(node.Name)
	if !ok {
		return types.Err(types.E_VARNF)
	}
	return types.Ok(val)
}

// unary evaluates a unary expression
// Implements: - (negation), ! (logical not), ~ (bitwise not)
func (e *Evaluator) unary(node *parser.UnaryExpr, ctx *types.TaskContext) types.Result {
	// Evaluate operand
	operandResult := e.Eval(node.Operand, ctx)
	if !operandResult.IsNormal() {
		return operandResult // Propagate error/control flow
	}

	operand := operandResult.Val

	switch node.Operator {
	case parser.TOKEN_MINUS:
		// Unary minus: -x
		return unaryMinus(operand)

	case parser.TOKEN_NOT:
		// Logical not: !x
		return unaryNot(operand)

	case parser.TOKEN_BITNOT:
		// Bitwise not: ~x
		return bitwiseNot(operand)

	default:
		return types.Err(types.E_TYPE)
	}
}

// binary evaluates a binary expression
// Handles arithmetic, comparison, logical, and bitwise operators
func (e *Evaluator) binary(node *parser.BinaryExpr, ctx *types.TaskContext) types.Result {
	// Short-circuit evaluation for && and ||
	if node.Operator == parser.TOKEN_AND || node.Operator == parser.TOKEN_OR {
		return e.logical(node, ctx)
	}

	// Evaluate both operands
	leftResult := e.Eval(node.Left, ctx)
	if !leftResult.IsNormal() {
		return leftResult // Propagate error/control flow
	}

	rightResult := e.Eval(node.Right, ctx)
	if !rightResult.IsNormal() {
		return rightResult // Propagate error/control flow
	}

	left := leftResult.Val
	right := rightResult.Val

	// Dispatch to operator-specific handlers
	switch node.Operator {
	// Arithmetic
	case parser.TOKEN_PLUS:
		return add(left, right)
	case parser.TOKEN_MINUS:
		return subtract(left, right)
	case parser.TOKEN_STAR:
		return multiply(left, right)
	case parser.TOKEN_SLASH:
		return divide(left, right)
	case parser.TOKEN_PERCENT:
		return modulo(left, right)
	case parser.TOKEN_CARET:
		return power(left, right)

	// Comparison
	case parser.TOKEN_EQ:
		return equal(left, right)
	case parser.TOKEN_NE:
		return notEqual(left, right)
	case parser.TOKEN_LT:
		return lessThan(left, right)
	case parser.TOKEN_LE:
		return lessThanEqual(left, right)
	case parser.TOKEN_GT:
		return greaterThan(left, right)
	case parser.TOKEN_GE:
		return greaterThanEqual(left, right)
	case parser.TOKEN_IN:
		return inOp(left, right)

	// Bitwise
	case parser.TOKEN_BITAND:
		return bitwiseAnd(left, right)
	case parser.TOKEN_BITOR:
		return bitwiseOr(left, right)
	case parser.TOKEN_BITXOR:
		return bitwiseXor(left, right)
	case parser.TOKEN_LSHIFT:
		return leftShift(left, right)
	case parser.TOKEN_RSHIFT:
		return rightShift(left, right)

	default:
		return types.Err(types.E_TYPE)
	}
}

// logical evaluates && and || with short-circuit semantics
func (e *Evaluator) logical(node *parser.BinaryExpr, ctx *types.TaskContext) types.Result {
	// Evaluate left operand
	leftResult := e.Eval(node.Left, ctx)
	if !leftResult.IsNormal() {
		return leftResult // Propagate error/control flow
	}

	left := leftResult.Val

	switch node.Operator {
	case parser.TOKEN_AND:
		// Short-circuit: if left is falsy, return left without evaluating right
		if !left.Truthy() {
			return types.Ok(left)
		}
		// Left is truthy, evaluate and return right
		return e.Eval(node.Right, ctx)

	case parser.TOKEN_OR:
		// Short-circuit: if left is truthy, return left without evaluating right
		if left.Truthy() {
			return types.Ok(left)
		}
		// Left is falsy, evaluate and return right
		return e.Eval(node.Right, ctx)

	default:
		return types.Err(types.E_TYPE)
	}
}

// ternary evaluates a ternary expression: cond ? true_expr | false_expr
func (e *Evaluator) ternary(node *parser.TernaryExpr, ctx *types.TaskContext) types.Result {
	// Evaluate condition
	condResult := e.Eval(node.Condition, ctx)
	if !condResult.IsNormal() {
		return condResult // Propagate error/control flow
	}

	// Choose which branch to evaluate based on truthiness
	if condResult.Val.Truthy() {
		return e.Eval(node.ThenExpr, ctx)
	} else {
		return e.Eval(node.ElseExpr, ctx)
	}
}

// assign evaluates an assignment expression: target = value
// Supports variable assignment (simple and nested scopes)
func (e *Evaluator) assign(node *parser.AssignExpr, ctx *types.TaskContext) types.Result {
	// Evaluate the value to assign
	valueResult := e.Eval(node.Value, ctx)
	if !valueResult.IsNormal() {
		return valueResult // Propagate error/control flow
	}

	value := valueResult.Val

	// Handle different assignment targets
	switch target := node.Target.(type) {
	case *parser.IdentifierExpr:
		// Variable assignment
		e.env.Set(target.Name, value)
		return types.Ok(value)

	case *parser.PropertyExpr:
		// Property assignment: obj.property = value
		return e.assignProperty(target, value, ctx)

	case *parser.IndexExpr:
		// Index assignment: list[i] = value, str[i] = char, map[key] = value
		return e.assignIndex(target, value, ctx)

	case *parser.RangeExpr:
		// Range assignment: list[1..3] = vals, str[1..3] = substr
		return e.assignRange(target, value, ctx)

	default:
		// Other assignment targets not supported
		return types.Err(types.E_TYPE)
	}
}

// builtinCall evaluates a builtin function call
func (e *Evaluator) builtinCall(node *parser.BuiltinCallExpr, ctx *types.TaskContext) types.Result {
	// Look up the builtin function
	fn, ok := e.builtins.Get(node.Name)
	if !ok {
		// Builtin not found
		fmt.Printf("[BUILTIN NOT FOUND] %s\n", node.Name)
		return types.Err(types.E_VERBNF)
	}

	// Evaluate all arguments, handling splice expressions (@arg)
	var args []types.Value
	for _, argExpr := range node.Args {
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
				return argResult // Propagate error/control flow
			}
			args = append(args, argResult.Val)
		}
	}

	// Call the builtin function
	result := fn(ctx, args)
	if result.Flow == types.FlowException && result.Error == types.E_INVARG {
		fmt.Printf("[BUILTIN E_INVARG] %s returned E_INVARG\n", node.Name)
	}
	return result
}

// indexMarker evaluates an index marker (^ or $)
// For lists/strings: ^ = 1, $ = length
// For maps: ^ = first key, $ = last key
func (e *Evaluator) indexMarker(node *parser.IndexMarkerExpr, ctx *types.TaskContext) types.Result {
	// Check if we have an indexing context
	// IndexContext = -1 means "not in an indexing context"
	// IndexContext >= 0 means we're indexing a collection of that length (0 for empty)
	if ctx.IndexContext < 0 {
		// No indexing context - error
		return types.Err(types.E_TYPE)
	}

	if node.Marker == parser.TOKEN_CARET {
		// ^ resolves to first key for maps, or 1 for lists/strings
		if ctx.MapFirstKey != nil {
			return types.Ok(ctx.MapFirstKey)
		}
		return types.Ok(types.NewInt(1))
	} else if node.Marker == parser.TOKEN_DOLLAR {
		// $ resolves to last key for maps, or length for lists/strings
		if ctx.MapLastKey != nil {
			return types.Ok(ctx.MapLastKey)
		}
		return types.Ok(types.NewInt(int64(ctx.IndexContext)))
	}
	return types.Err(types.E_TYPE)
}

// GetEnvironment returns the evaluator's environment (for testing)
func (e *Evaluator) GetEnvironment() *Environment {
	return e.env
}

// VerbContext contains the context for a verb execution
type VerbContext struct {
	Player  types.ObjID
	This    types.ObjID
	Caller  types.ObjID
	Verb    string
	Args    []string
	Argstr  string
	Dobj    types.ObjID
	Dobjstr string
	Iobj    types.ObjID
	Iobjstr string
	Prepstr string
}

// SetVerbContext sets up the environment for verb execution
func (e *Evaluator) SetVerbContext(vc *VerbContext) {
	e.env.Set("player", types.NewObj(vc.Player))
	e.env.Set("this", types.NewObj(vc.This))
	e.env.Set("caller", types.NewObj(vc.Caller))
	e.env.Set("verb", types.NewStr(vc.Verb))
	e.env.Set("argstr", types.NewStr(vc.Argstr))

	// Convert string args to Value list
	argList := make([]types.Value, len(vc.Args))
	for i, arg := range vc.Args {
		argList[i] = types.NewStr(arg)
	}
	e.env.Set("args", types.NewList(argList))

	e.env.Set("dobj", types.NewObj(vc.Dobj))
	e.env.Set("dobjstr", types.NewStr(vc.Dobjstr))
	e.env.Set("iobj", types.NewObj(vc.Iobj))
	e.env.Set("iobjstr", types.NewStr(vc.Iobjstr))
	e.env.Set("prepstr", types.NewStr(vc.Prepstr))
}

// EvalString parses and evaluates a string of MOO code
// This is used by the eval() builtin
// Returns Result with either:
// - FlowNormal/FlowReturn: successful evaluation
// - FlowException: runtime error (Error field set)
// - FlowParseError: syntax error (Val contains list of error strings)
func (e *Evaluator) EvalString(code string, ctx *types.TaskContext) types.Result {
	// Parse the code as statements
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		// Return parse error with error message list (Toast format)
		// Format: "Line <n>:  <message>"
		errorMsg := fmt.Sprintf("Line 1:  %s", err.Error())
		return types.Result{
			Flow: types.FlowParseError,
			Val: types.NewList([]types.Value{types.NewStr(errorMsg)}),
		}
	}

	// Evaluate all statements using EvalStatements
	result := e.EvalStatements(stmts, ctx)

	// Handle FlowReturn - extract the value and convert to normal flow
	if result.Flow == types.FlowReturn {
		return types.Ok(result.Val)
	}

	// Handle FlowNormal - already has the right value (or 0)
	if result.Flow == types.FlowNormal {
		return types.Ok(result.Val)
	}

	// Propagate errors and other control flow
	return result
}

// splice evaluates a splice expression: @expr
// Splice is only valid in specific contexts (list literals, function args, scatter)
// When evaluated standalone, it simply returns E_TYPE as per spec
func (e *Evaluator) splice(node *parser.SpliceExpr, ctx *types.TaskContext) types.Result {
	// Evaluate the expression
	result := e.Eval(node.Expr, ctx)
	if !result.IsNormal() {
		return result
	}

	// Splice requires a LIST operand
	if result.Val.Type() != types.TYPE_LIST {
		return types.Err(types.E_TYPE)
	}

	// In standalone context, splice just returns the list
	// The actual splicing happens in list construction and function calls
	return result
}

// catch evaluates a catch expression: `expr ! codes => default`
// Catches errors matching codes and returns default (or the error if no default)
func (e *Evaluator) catch(node *parser.CatchExpr, ctx *types.TaskContext) types.Result {
	// Evaluate the main expression
	result := e.Eval(node.Expr, ctx)

	// If no error, return the result
	if result.Flow != types.FlowException {
		return result
	}

	// Check if the error matches any of the catch codes
	for _, code := range node.Codes {
		if result.Error == code {
			// Error matches, return default if provided
			if node.Default != nil {
				return e.Eval(node.Default, ctx)
			}
			// No default, return the error value
			return types.Ok(types.NewErr(result.Error))
		}
	}

	// Error doesn't match, propagate it
	return result
}

// listExpr evaluates a list expression: {expr, expr, ...}
// Handles splice (@expr) by expanding its elements into the list
func (e *Evaluator) listExpr(node *parser.ListExpr, ctx *types.TaskContext) types.Result {
	var elements []types.Value

	for _, elem := range node.Elements {
		// Check if this is a splice expression
		if splice, ok := elem.(*parser.SpliceExpr); ok {
			// Evaluate the splice operand
			result := e.Eval(splice.Expr, ctx)
			if !result.IsNormal() {
				return result
			}

			// Splice requires a LIST operand
			if result.Val.Type() != types.TYPE_LIST {
				return types.Err(types.E_TYPE)
			}

			// Append all elements from the spliced list
			list := result.Val.(types.ListValue)
			for i := 1; i <= list.Len(); i++ {
				elements = append(elements, list.Get(i))
			}
		} else {
			// Regular expression - evaluate and append
			result := e.Eval(elem, ctx)
			if !result.IsNormal() {
				return result
			}
			elements = append(elements, result.Val)
		}
	}

	resultList := types.NewList(elements)

	// Check size limit
	if err := builtins.CheckListLimit(resultList); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(resultList)
}

// listRangeExpr evaluates a range list expression: {start..end}
// Generates a list of integers from start to end (inclusive)
// Accepts both integers and objects (which are treated as their ID values)
func (e *Evaluator) listRangeExpr(node *parser.ListRangeExpr, ctx *types.TaskContext) types.Result {
	// Evaluate start expression
	startResult := e.Eval(node.Start, ctx)
	if !startResult.IsNormal() {
		return startResult
	}

	// Evaluate end expression
	endResult := e.Eval(node.End, ctx)
	if !endResult.IsNormal() {
		return endResult
	}

	// Extract integer values (accept both INT and OBJ types)
	var start, end int64

	switch v := startResult.Val.(type) {
	case types.IntValue:
		start = v.Val
	case types.ObjValue:
		start = int64(v.ID())
	default:
		return types.Err(types.E_TYPE)
	}

	switch v := endResult.Val.(type) {
	case types.IntValue:
		end = v.Val
	case types.ObjValue:
		end = int64(v.ID())
	default:
		return types.Err(types.E_TYPE)
	}

	// Generate the list
	var elements []types.Value
	if start <= end {
		// Ascending range
		for i := start; i <= end; i++ {
			elements = append(elements, types.NewInt(i))
		}
	} else {
		// Descending range (or empty if start > end, but MOO allows descending)
		for i := start; i >= end; i-- {
			elements = append(elements, types.NewInt(i))
		}
	}

	return types.Ok(types.NewList(elements))
}

// mapExpr evaluates a map expression: [key -> value, ...]
func (e *Evaluator) mapExpr(node *parser.MapExpr, ctx *types.TaskContext) types.Result {
	pairs := make([][2]types.Value, 0, len(node.Pairs))

	for _, pair := range node.Pairs {
		// Evaluate key
		keyResult := e.Eval(pair.Key, ctx)
		if !keyResult.IsNormal() {
			return keyResult
		}

		// Validate key type - lists and maps cannot be map keys
		if !types.IsValidMapKey(keyResult.Val) {
			return types.Err(types.E_TYPE)
		}

		// Evaluate value
		valueResult := e.Eval(pair.Value, ctx)
		if !valueResult.IsNormal() {
			return valueResult
		}

		pairs = append(pairs, [2]types.Value{keyResult.Val, valueResult.Val})
	}

	resultMap := types.NewMap(pairs)

	// Check size limit
	if err := builtins.CheckMapLimit(resultMap); err != types.E_NONE {
		return types.Err(err)
	}

	return types.Ok(resultMap)
}

// Note: Operator implementation functions (evalAdd, evalSubtract, etc.)
// are defined in operators.go to keep this file focused on the evaluation structure
