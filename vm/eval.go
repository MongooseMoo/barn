package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/parser"
	"barn/types"
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
		return e.evalLiteral(n, ctx)
	case *parser.IdentifierExpr:
		return e.evalIdentifier(n, ctx)
	case *parser.UnaryExpr:
		return e.evalUnary(n, ctx)
	case *parser.BinaryExpr:
		return e.evalBinary(n, ctx)
	case *parser.TernaryExpr:
		return e.evalTernary(n, ctx)
	case *parser.AssignExpr:
		return e.evalAssign(n, ctx)
	case *parser.ParenExpr:
		return e.Eval(n.Expr, ctx)
	case *parser.BuiltinCallExpr:
		return e.evalBuiltinCall(n, ctx)
	case *parser.IndexExpr:
		return e.evalIndex(n, ctx)
	case *parser.RangeExpr:
		return e.evalRange(n, ctx)
	case *parser.IndexMarkerExpr:
		return e.evalIndexMarker(n, ctx)
	case *parser.PropertyExpr:
		return e.evalProperty(n, ctx)
	case *parser.VerbCallExpr:
		return e.evalVerbCall(n, ctx)
	case *parser.SpliceExpr:
		return e.evalSplice(n, ctx)
	case *parser.CatchExpr:
		return e.evalCatch(n, ctx)
	case *parser.ListExpr:
		return e.evalListExpr(n, ctx)
	case *parser.ListRangeExpr:
		return e.evalListRangeExpr(n, ctx)
	case *parser.MapExpr:
		return e.evalMapExpr(n, ctx)
	default:
		// Unknown node type - this should never happen if parser is correct
		return types.Err(types.E_TYPE)
	}
}

// evalLiteral evaluates a literal expression
// Literals are already Values, just wrap in Result
func (e *Evaluator) evalLiteral(node *parser.LiteralExpr, ctx *types.TaskContext) types.Result {
	return types.Ok(node.Value)
}

// evalIdentifier looks up a variable by name
// Returns E_VARNF if the variable is not defined
func (e *Evaluator) evalIdentifier(node *parser.IdentifierExpr, ctx *types.TaskContext) types.Result {
	val, ok := e.env.Get(node.Name)
	if !ok {
		return types.Err(types.E_VARNF)
	}
	return types.Ok(val)
}

// evalUnary evaluates a unary expression
// Implements: - (negation), ! (logical not), ~ (bitwise not)
func (e *Evaluator) evalUnary(node *parser.UnaryExpr, ctx *types.TaskContext) types.Result {
	// Evaluate operand
	operandResult := e.Eval(node.Operand, ctx)
	if !operandResult.IsNormal() {
		return operandResult // Propagate error/control flow
	}

	operand := operandResult.Val

	switch node.Operator {
	case parser.TOKEN_MINUS:
		// Unary minus: -x
		return evalUnaryMinus(operand)

	case parser.TOKEN_NOT:
		// Logical not: !x
		return evalUnaryNot(operand)

	case parser.TOKEN_BITNOT:
		// Bitwise not: ~x
		return evalBitwiseNot(operand)

	default:
		return types.Err(types.E_TYPE)
	}
}

// evalBinary evaluates a binary expression
// Handles arithmetic, comparison, logical, and bitwise operators
func (e *Evaluator) evalBinary(node *parser.BinaryExpr, ctx *types.TaskContext) types.Result {
	// Short-circuit evaluation for && and ||
	if node.Operator == parser.TOKEN_AND || node.Operator == parser.TOKEN_OR {
		return e.evalLogical(node, ctx)
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
		return evalAdd(left, right)
	case parser.TOKEN_MINUS:
		return evalSubtract(left, right)
	case parser.TOKEN_STAR:
		return evalMultiply(left, right)
	case parser.TOKEN_SLASH:
		return evalDivide(left, right)
	case parser.TOKEN_PERCENT:
		return evalModulo(left, right)
	case parser.TOKEN_CARET:
		return evalPower(left, right)

	// Comparison
	case parser.TOKEN_EQ:
		return evalEqual(left, right)
	case parser.TOKEN_NE:
		return evalNotEqual(left, right)
	case parser.TOKEN_LT:
		return evalLessThan(left, right)
	case parser.TOKEN_LE:
		return evalLessThanEqual(left, right)
	case parser.TOKEN_GT:
		return evalGreaterThan(left, right)
	case parser.TOKEN_GE:
		return evalGreaterThanEqual(left, right)
	case parser.TOKEN_IN:
		return evalIn(left, right)

	// Bitwise
	case parser.TOKEN_BITAND:
		return evalBitwiseAnd(left, right)
	case parser.TOKEN_BITOR:
		return evalBitwiseOr(left, right)
	case parser.TOKEN_BITXOR:
		return evalBitwiseXor(left, right)
	case parser.TOKEN_LSHIFT:
		return evalLeftShift(left, right)
	case parser.TOKEN_RSHIFT:
		return evalRightShift(left, right)

	default:
		return types.Err(types.E_TYPE)
	}
}

// evalLogical evaluates && and || with short-circuit semantics
func (e *Evaluator) evalLogical(node *parser.BinaryExpr, ctx *types.TaskContext) types.Result {
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

// evalTernary evaluates a ternary expression: cond ? true_expr | false_expr
func (e *Evaluator) evalTernary(node *parser.TernaryExpr, ctx *types.TaskContext) types.Result {
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

// evalAssign evaluates an assignment expression: target = value
// Supports variable assignment (simple and nested scopes)
func (e *Evaluator) evalAssign(node *parser.AssignExpr, ctx *types.TaskContext) types.Result {
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
		return e.evalAssignProperty(target, value, ctx)

	case *parser.IndexExpr:
		// Index assignment: list[i] = value, str[i] = char, map[key] = value
		return e.evalAssignIndex(target, value, ctx)

	case *parser.RangeExpr:
		// Range assignment: list[1..3] = vals, str[1..3] = substr
		return e.evalAssignRange(target, value, ctx)

	default:
		// Other assignment targets not supported
		return types.Err(types.E_TYPE)
	}
}

// evalBuiltinCall evaluates a builtin function call
func (e *Evaluator) evalBuiltinCall(node *parser.BuiltinCallExpr, ctx *types.TaskContext) types.Result {
	// Look up the builtin function
	fn, ok := e.builtins.Get(node.Name)
	if !ok {
		// Builtin not found
		return types.Err(types.E_VERBNF)
	}

	// Evaluate all arguments
	args := make([]types.Value, len(node.Args))
	for i, argExpr := range node.Args {
		argResult := e.Eval(argExpr, ctx)
		if !argResult.IsNormal() {
			return argResult // Propagate error/control flow
		}
		args[i] = argResult.Val
	}

	// Call the builtin function
	return fn(ctx, args)
}

// evalIndexMarker evaluates an index marker (^ or $)
// For lists/strings: ^ = 1, $ = length
// For maps: ^ = first key, $ = last key
func (e *Evaluator) evalIndexMarker(node *parser.IndexMarkerExpr, ctx *types.TaskContext) types.Result {
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

// EvalString parses and evaluates a string of MOO code
// This is used by the eval() builtin
func (e *Evaluator) EvalString(code string, ctx *types.TaskContext) types.Result {
	// Parse the code
	p := parser.NewParser(code)
	stmts, err := p.ParseProgram()
	if err != nil {
		// Parse error -> E_INVARG
		return types.Err(types.E_INVARG)
	}

	// Evaluate all statements using EvalStatements
	result := e.EvalStatements(stmts, ctx)

	// Handle FlowReturn - extract the value
	if result.Flow == types.FlowReturn {
		return types.Ok(result.Val)
	}

	return result
}

// evalSplice evaluates a splice expression: @expr
// Splice is only valid in specific contexts (list literals, function args, scatter)
// When evaluated standalone, it simply returns E_TYPE as per spec
func (e *Evaluator) evalSplice(node *parser.SpliceExpr, ctx *types.TaskContext) types.Result {
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

// evalCatch evaluates a catch expression: `expr ! codes => default`
// Catches errors matching codes and returns default (or the error if no default)
func (e *Evaluator) evalCatch(node *parser.CatchExpr, ctx *types.TaskContext) types.Result {
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

// evalListExpr evaluates a list expression: {expr, expr, ...}
// Handles splice (@expr) by expanding its elements into the list
func (e *Evaluator) evalListExpr(node *parser.ListExpr, ctx *types.TaskContext) types.Result {
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

	return types.Ok(types.NewList(elements))
}

// evalListRangeExpr evaluates a range list expression: {start..end}
// Generates a list of integers from start to end (inclusive)
// Accepts both integers and objects (which are treated as their ID values)
func (e *Evaluator) evalListRangeExpr(node *parser.ListRangeExpr, ctx *types.TaskContext) types.Result {
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

// evalMapExpr evaluates a map expression: [key -> value, ...]
func (e *Evaluator) evalMapExpr(node *parser.MapExpr, ctx *types.TaskContext) types.Result {
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

	return types.Ok(types.NewMap(pairs))
}

// Note: Operator implementation functions (evalAdd, evalSubtract, etc.)
// are defined in operators.go to keep this file focused on the evaluation structure
