package eval

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
	return &Evaluator{
		env:      NewEnvironment(),
		builtins: registry,
		store:    store,
	}
}

// NewEvaluatorWithEnv creates a new evaluator with a given environment
func NewEvaluatorWithEnv(env *Environment) *Evaluator {
	store := db.NewStore()
	registry := builtins.NewRegistry()
	registry.RegisterObjectBuiltins(store)
	registry.RegisterPropertyBuiltins(store)
	registry.RegisterVerbBuiltins(store)
	return &Evaluator{
		env:      env,
		builtins: registry,
		store:    store,
	}
}

// NewEvaluatorWithStore creates a new evaluator with a given store
func NewEvaluatorWithStore(store *db.Store) *Evaluator {
	registry := builtins.NewRegistry()
	registry.RegisterObjectBuiltins(store)
	registry.RegisterPropertyBuiltins(store)
	registry.RegisterVerbBuiltins(store)
	return &Evaluator{
		env:      NewEnvironment(),
		builtins: registry,
		store:    store,
	}
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
		// Index assignment: list[i] = value (Phase 4)
		// Not yet implemented - will add in indexing phase
		return types.Err(types.E_TYPE)

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
// These can only be meaningful in an indexing context, so we return E_TYPE here
// The actual resolution happens in evalIndex and evalRange
func (e *Evaluator) evalIndexMarker(node *parser.IndexMarkerExpr, ctx *types.TaskContext) types.Result {
	// Index markers should only appear inside [] expressions
	// If we're evaluating one directly, it's an error
	return types.Err(types.E_TYPE)
}

// GetEnvironment returns the evaluator's environment (for testing)
func (e *Evaluator) GetEnvironment() *Environment {
	return e.env
}

// Note: Operator implementation functions (evalAdd, evalSubtract, etc.)
// are defined in operators.go to keep this file focused on the evaluation structure
