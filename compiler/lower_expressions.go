package compiler

import (
	"fmt"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/verb"
)

// compileLiteral compiles a literal value
func (c *lowerer) compileLiteral(n *verb.LiteralExpr) error {
	// Check if it's a small integer that can use immediate opcode
	if n.Kind == verb.LiteralInt {
		c.emitIntLiteral(n.IntValue)
		return nil
	}

	value, err := valueFromLiteral(n)
	if err != nil {
		return err
	}

	// Otherwise push from constant pool
	c.emitConstant(value)
	return nil
}

// builtinConstants maps MOO type constant names to their integer values.
// These are always available in any scope without explicit declaration.
var builtinConstants = map[string]types.Value{
	"INT":   types.NewInt(int64(types.TYPE_INT)),
	"NUM":   types.NewInt(int64(types.TYPE_INT)), // alias for INT
	"OBJ":   types.NewInt(int64(types.TYPE_OBJ)),
	"STR":   types.NewInt(int64(types.TYPE_STR)),
	"ERR":   types.NewInt(int64(types.TYPE_ERR)),
	"LIST":  types.NewInt(int64(types.TYPE_LIST)),
	"FLOAT": types.NewInt(int64(types.TYPE_FLOAT)),
	"MAP":   types.NewInt(int64(types.TYPE_MAP)),
	"ANON":  types.NewInt(int64(types.TYPE_ANON)),
	"WAIF":  types.NewInt(int64(types.TYPE_WAIF)),
	"BOOL":  types.NewInt(int64(types.TYPE_BOOL)),
}

// compileIdentifier compiles a variable reference
func (c *lowerer) compileIdentifier(n *verb.IdentifierExpr) error {
	// User variables take precedence over built-in type constants so code can
	// intentionally use names like NUM/INT as loop counters or temporaries.
	if idx, ok := c.resolveVariable(n.Name); ok {
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(idx))
		return nil
	}

	// Check for built-in type constants (OBJ, STR, INT, etc.)
	if val, ok := builtinConstants[canonicalIdentifier(n.Name)]; ok {
		c.emitConstant(val)
		return nil
	}

	// Variable not found - this will be a runtime error (E_VARNF)
	// For now, declare it (MOO has dynamic scoping)
	idx := c.declareVariable(n.Name)

	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(idx))
	return nil
}

// compileUnary compiles a unary expression
func (c *lowerer) compileUnary(n *verb.UnaryExpr) error {
	// Toast folds a negated float literal before constant-pool insertion. This is
	// observable for signed zero: a standalone -0.0 keeps its sign, while an
	// earlier +0.0 constant wins the pool's equality-based zero deduplication.
	// Integer literals fold the same way, so `-5` is one tick-free push rather
	// than a push and an OP_UNARY_MINUS.
	if n.Operator == verb.UnaryNegate {
		if literal, ok := n.Operand.(*verb.LiteralExpr); ok && literal.Kind == verb.LiteralFloat {
			c.emitConstant(types.NewFloat(-literal.FloatValue))
			return nil
		}
		if literal, ok := n.Operand.(*verb.LiteralExpr); ok && literal.Kind == verb.LiteralInt {
			c.emitIntLiteral(-literal.IntValue)
			return nil
		}
	}

	// Compile operand
	if err := c.compileNode(n.Operand); err != nil {
		return err
	}

	// Emit operator
	switch n.Operator {
	case verb.UnaryNegate:
		c.emit(bytecode.OP_NEG)
	case verb.UnaryNot:
		c.emit(bytecode.OP_NOT)
	case verb.UnaryBitwiseNot:
		c.emit(bytecode.OP_BITNOT)
	default:
		return fmt.Errorf("unknown unary operator: %v", n.Operator)
	}

	return nil
}

// compileBinary compiles a binary expression
func (c *lowerer) compileBinary(n *verb.BinaryExpr) error {
	// Short-circuit for && and ||
	if n.Operator == verb.BinaryAnd {
		return c.compileShortCircuitAnd(n)
	}
	if n.Operator == verb.BinaryOr {
		return c.compileShortCircuitOr(n)
	}

	// Compile left operand
	if err := c.compileNode(n.Left); err != nil {
		return err
	}

	// Compile right operand
	if err := c.compileNode(n.Right); err != nil {
		return err
	}

	// Emit operator
	switch n.Operator {
	case verb.BinaryAdd:
		c.emit(bytecode.OP_ADD)
	case verb.BinarySubtract:
		c.emit(bytecode.OP_SUB)
	case verb.BinaryMultiply:
		c.emit(bytecode.OP_MUL)
	case verb.BinaryDivide:
		c.emit(bytecode.OP_DIV)
	case verb.BinaryModulo:
		c.emit(bytecode.OP_MOD)
	case verb.BinaryPower:
		c.emit(bytecode.OP_POW)
	case verb.BinaryEqual:
		c.emit(bytecode.OP_EQ)
	case verb.BinaryNotEqual:
		c.emit(bytecode.OP_NE)
	case verb.BinaryLess:
		c.emit(bytecode.OP_LT)
	case verb.BinaryLessEqual:
		c.emit(bytecode.OP_LE)
	case verb.BinaryGreater:
		c.emit(bytecode.OP_GT)
	case verb.BinaryGreaterEqual:
		c.emit(bytecode.OP_GE)
	case verb.BinaryIn:
		c.emit(bytecode.OP_IN)
	case verb.BinaryBitAnd:
		c.emit(bytecode.OP_BITAND)
	case verb.BinaryBitOr:
		c.emit(bytecode.OP_BITOR)
	case verb.BinaryBitXor:
		c.emit(bytecode.OP_BITXOR)
	case verb.BinaryShiftLeft:
		c.emit(bytecode.OP_SHL)
	case verb.BinaryShiftRight:
		c.emit(bytecode.OP_SHR)
	default:
		return fmt.Errorf("unknown binary operator: %v", n.Operator)
	}

	return nil
}

// compileShortCircuitAnd compiles && with short-circuit evaluation
func (c *lowerer) compileShortCircuitAnd(n *verb.BinaryExpr) error {
	// Compile left
	if err := c.compileNode(n.Left); err != nil {
		return err
	}

	// If false, skip right and leave false on stack
	skipJump := c.emitJump(bytecode.OP_AND)

	// Compile right
	if err := c.compileNode(n.Right); err != nil {
		return err
	}

	// Patch jump
	c.patchJump(skipJump)
	return nil
}

// compileShortCircuitOr compiles || with short-circuit evaluation
func (c *lowerer) compileShortCircuitOr(n *verb.BinaryExpr) error {
	// Compile left
	if err := c.compileNode(n.Left); err != nil {
		return err
	}

	// If true, skip right and leave true on stack
	skipJump := c.emitJump(bytecode.OP_OR)

	// Compile right
	if err := c.compileNode(n.Right); err != nil {
		return err
	}

	// Patch jump
	c.patchJump(skipJump)
	return nil
}

// compileTernary compiles a ternary expression
func (c *lowerer) compileTernary(n *verb.TernaryExpr) error {
	// Compile condition
	if err := c.compileNode(n.Condition); err != nil {
		return err
	}

	// Jump to else if false
	elseJump := c.emitJump(bytecode.OP_JUMP_IF_FALSE)

	// Compile then branch
	if err := c.compileNode(n.ThenExpr); err != nil {
		return err
	}

	// Jump over else
	endJump := c.emitJump(bytecode.OP_JUMP)

	// Compile else branch
	c.patchJump(elseJump)
	if err := c.compileNode(n.ElseExpr); err != nil {
		return err
	}

	// Patch end jump
	c.patchJump(endJump)
	return nil
}

// Stub implementations for other compile methods
// These will be completed based on the actual requirements

func (c *lowerer) compileBuiltinCall(n *verb.BuiltinCallExpr) error {
	if c.registry == nil {
		return fmt.Errorf("builtin call compilation requires a builtins registry")
	}
	funcID, ok := c.registry[canonicalIdentifier(n.Name)]
	if !ok {
		return &UnknownBuiltinError{Name: n.Name, Position: n.Pos}
	}

	// Special-case pass(): emit bytecode.OP_PASS instead of bytecode.OP_CALL_BUILTIN.
	// bytecode.OP_PASS is handled natively by the VM — looks up the parent verb,
	// compiles it to bytecode, and pushes a new frame.
	if canonicalIdentifier(n.Name) == canonicalIdentifier("pass") {
		hasSplice := hasSpliceArgs(n.Args)
		if !hasSplice && len(n.Args) > 254 {
			return fmt.Errorf("too many arguments (max 254)")
		}

		if hasSplice {
			// Build a single flattened arg list on-stack; bytecode.OP_PASS 0xFF consumes it.
			c.emit(bytecode.OP_MAKE_LIST)
			c.emitByte(0)
			for _, arg := range n.Args {
				if splice, ok := arg.(*verb.SpliceExpr); ok {
					if err := c.compileNode(splice.Expr); err != nil {
						return err
					}
					c.emit(bytecode.OP_LIST_EXTEND)
				} else {
					if err := c.compileNode(arg); err != nil {
						return err
					}
					c.emit(bytecode.OP_LIST_APPEND)
				}
			}
			c.emit(bytecode.OP_PASS)
			c.emitByte(0xFF)
			return nil
		}

		// Compile fixed arguments directly onto the stack.
		for _, arg := range n.Args {
			if err := c.compileNode(arg); err != nil {
				return err
			}
		}
		c.emit(bytecode.OP_PASS)
		c.emitByte(byte(len(n.Args)))
		return nil
	}

	// Check if any argument is a splice expression
	hasSplice := hasSpliceArgs(n.Args)

	// Check argument count overflow (emitted as single byte, 0xFF reserved for splice)
	if !hasSplice && len(n.Args) > 254 {
		return fmt.Errorf("too many arguments (max 254)")
	}

	if hasSplice {
		// Splice path: build args list incrementally using bytecode.OP_LIST_APPEND/EXTEND
		c.emit(bytecode.OP_MAKE_LIST)
		c.emitByte(0)
		for _, arg := range n.Args {
			if splice, ok := arg.(*verb.SpliceExpr); ok {
				if err := c.compileNode(splice.Expr); err != nil {
					return err
				}
				c.emit(bytecode.OP_LIST_EXTEND)
			} else {
				if err := c.compileNode(arg); err != nil {
					return err
				}
				c.emit(bytecode.OP_LIST_APPEND)
			}
		}
		// argc=0xFF signals that args list is on top of stack
		c.emitCallBuiltin(funcID, 0xFF)
	} else {
		// Fast path: no splices, push args directly
		for _, arg := range n.Args {
			if err := c.compileNode(arg); err != nil {
				return err
			}
		}
		c.emitCallBuiltin(funcID, byte(len(n.Args)))
	}

	return nil
}

func (c *lowerer) compileIndex(n *verb.IndexExpr) error {
	// Compile collection
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// If the index contains ^ or $, set up an index context variable with the collection.
	// Stack: [coll] -> DUP -> [coll, coll] -> SET_VAR -> [coll]
	hasIndexBoundary := containsIndexBoundary(n.Index)
	oldContextVar := c.indexContextVar
	oldBoundaryContext := c.indexBoundaryContext
	if hasIndexBoundary {
		tempIdx := c.declareInternalVariable(c.tempVar("idxctx"))
		c.emit(bytecode.OP_DUP)
		c.emitStoreLocal(tempIdx)
		c.indexContextVar = tempIdx
		c.indexBoundaryContext = indexBoundaryIndex
	}

	// Compile index
	if err := c.compileNode(n.Index); err != nil {
		return err
	}

	// Restore previous context
	c.indexContextVar = oldContextVar
	c.indexBoundaryContext = oldBoundaryContext

	// Emit index operation
	c.emit(bytecode.OP_INDEX)
	return nil
}

func (c *lowerer) compileRange(n *verb.RangeExpr) error {
	// Compile collection
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// If start or end contains ^ or $, retain the collection as the index
	// context. bytecode.OP_INDEX_MARKER then preserves the semantic FIRST/LAST operation
	// in bytecode while resolving it against the collection at runtime.
	// Stack: [coll] -> DUP -> [coll, coll] -> SET_VAR -> [coll]
	hasIndexBoundary := containsIndexBoundary(n.Start) || containsIndexBoundary(n.End)
	oldContextVar := c.indexContextVar
	oldBoundaryContext := c.indexBoundaryContext
	if hasIndexBoundary {
		tempIdx := c.declareInternalVariable(c.tempVar("rngctx"))
		c.emit(bytecode.OP_DUP)
		c.emitStoreLocal(tempIdx)
		c.indexContextVar = tempIdx
		c.indexBoundaryContext = indexBoundaryRange
	}

	// Compile start
	if err := c.compileNode(n.Start); err != nil {
		return err
	}

	// Compile end
	if err := c.compileNode(n.End); err != nil {
		return err
	}

	// Restore previous context
	c.indexContextVar = oldContextVar
	c.indexBoundaryContext = oldBoundaryContext

	// Emit range operation
	c.emit(bytecode.OP_RANGE)
	return nil
}

func (c *lowerer) compileProperty(n *verb.PropertyExpr) error {
	// Compile the object expression (pushes object onto stack)
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	if n.Property != "" {
		// Static property: obj.prop
		c.emitStaticNameOperation(bytecode.OP_GET_PROP, bytecode.OP_GET_PROP_WIDE, n.Property)
	} else if n.PropertyExpr != nil {
		// Dynamic property: obj.(expr)
		// Compile the property name expression (pushes string onto stack)
		if err := c.compileNode(n.PropertyExpr); err != nil {
			return err
		}
		c.emit(bytecode.OP_GET_PROP_DYNAMIC)
	} else {
		return fmt.Errorf("property expression has neither static name nor dynamic expression")
	}

	return nil
}

func (c *lowerer) compileVerbCall(n *verb.VerbCallExpr) error {
	// Compile the object expression (pushes object onto stack)
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Dynamic verb names are evaluated after the object but before arguments.
	// Hold the name in a temporary so it can still be placed above the finished
	// argument stack for bytecode.OP_CALL_VERB.
	isDynamic := n.Verb == "" && n.VerbExpr != nil
	nameVar := -1
	if isDynamic {
		if err := c.compileNode(n.VerbExpr); err != nil {
			return err
		}
		nameVar = c.declareInternalVariable(c.tempVar("verbcallname"))
		c.emitStoreLocal(nameVar)
	}

	// Check if any argument is a splice expression
	hasSplice := hasSpliceArgs(n.Args)

	// Check argument count overflow (emitted as single byte, 0xFF reserved for splice)
	if !hasSplice && len(n.Args) > 254 {
		return fmt.Errorf("too many verb arguments (max 254)")
	}

	if hasSplice {
		// Splice path: build args list incrementally using bytecode.OP_LIST_APPEND/EXTEND
		c.emit(bytecode.OP_MAKE_LIST)
		c.emitByte(0)
		for _, arg := range n.Args {
			if splice, ok := arg.(*verb.SpliceExpr); ok {
				if err := c.compileNode(splice.Expr); err != nil {
					return err
				}
				c.emit(bytecode.OP_LIST_EXTEND)
			} else {
				if err := c.compileNode(arg); err != nil {
					return err
				}
				c.emit(bytecode.OP_LIST_APPEND)
			}
		}
	} else {
		// Fast path: no splices, push args directly
		for _, arg := range n.Args {
			if err := c.compileNode(arg); err != nil {
				return err
			}
		}
	}

	if isDynamic {
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(nameVar))
	}

	// Static names use a compact or wide constant operand. Dynamic names have
	// their own opcode, so constant index 255 is never confused with stack mode.
	if isDynamic {
		c.emit(bytecode.OP_CALL_VERB_DYNAMIC)
	} else if n.Verb != "" {
		c.emitStaticNameOperation(bytecode.OP_CALL_VERB, bytecode.OP_CALL_VERB_WIDE, n.Verb)
	} else {
		return fmt.Errorf("verb call has neither static name nor dynamic expression")
	}

	if hasSplice {
		c.emitByte(0xFF) // signal: args list is on stack
	} else {
		c.emitByte(byte(len(n.Args)))
	}

	return nil
}

func (c *lowerer) compileSplice(n *verb.SpliceExpr) error {
	// Compile the expression to splice
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Emit splice operation
	c.emit(bytecode.OP_SPLICE)
	return nil
}

func (c *lowerer) compileCatch(n *verb.CatchExpr) error {
	// Catch expressions (`expr ! codes => default`) are compiled as a
	// single-clause try/except that leaves the result on the stack.
	//
	// With default:
	//   bytecode.OP_TRY_EXCEPT_LOCAL_WIDE 1 [codes...] [0 = no var:uint16] [handler_ip:uint32]
	//   [expr]
	//   bytecode.OP_END_EXCEPT 1
	//   bytecode.OP_JUMP [end]
	//   handler_ip: [default expr]
	//   end:
	//
	// Without default (return the error value):
	//   bytecode.OP_TRY_EXCEPT_LOCAL_WIDE 1 [codes...] [var+1:uint16] [handler_ip:uint32]
	//   [expr]
	//   bytecode.OP_END_EXCEPT 1
	//   bytecode.OP_JUMP [end]
	//   handler_ip: bytecode.OP_GET_VAR [var]   (error was stored by HandleError)
	//   end:

	// For the no-default case, we need a temp variable to receive the error
	var errVarIdx int
	if n.Default == nil {
		errVarIdx = c.declareInternalVariable(c.tempVar("catch_err"))
	}

	// Emit bytecode.OP_TRY_EXCEPT with 1 clause
	c.emit(bytecode.OP_TRY_EXCEPT_LOCAL_WIDE)
	c.emitByte(1) // 1 clause

	codes, err := lowerErrorNames(n.Codes)
	if err != nil {
		return err
	}

	// Emit catch codes
	if n.IsAny {
		c.emitByte(0)
	} else {
		c.emitByte(byte(len(codes)))
	}
	for _, code := range codes {
		c.emitByte(byte(code))
	}

	// Variable index: 0 means no variable, idx+1 means store in local[idx]
	if n.Default == nil {
		c.emitShort(uint16(errVarIdx + 1))
	} else {
		c.emitShort(0) // no variable needed
	}

	// Handler IP placeholder (absolute)
	handlerIPPatch := len(c.program.Code)
	c.emitWide(^uint32(0))

	// Compile the main expression
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Normal path: pop the except handler
	c.emit(bytecode.OP_END_EXCEPT)
	c.emitByte(1) // 1 clause, matching the bytecode.OP_TRY_EXCEPT above

	// Jump past the handler body
	endJump := c.emitJump(bytecode.OP_JUMP)

	// Patch handler IP to point here
	handlerIP := c.currentOffset()
	c.patchWide(handlerIPPatch, handlerIP)

	// Handler body
	if n.Default != nil {
		// Evaluate default expression
		if err := c.compileNode(n.Default); err != nil {
			return err
		}
	} else {
		// No default: return the captured error code (first element of exception list).
		c.emit(bytecode.OP_GET_VAR)
		c.emitByte(byte(errVarIdx))
		if op, ok := bytecode.MakeImmediateOpcode(1); ok {
			c.emit(op)
		}
		c.emit(bytecode.OP_INDEX)
	}

	// Patch end jump
	c.patchJump(endJump)

	return nil
}

// hasSpliceArgs checks if any argument in a list is a splice expression.
func hasSpliceArgs(args []verb.Expr) bool {
	for _, arg := range args {
		if _, ok := arg.(*verb.SpliceExpr); ok {
			return true
		}
	}
	return false
}

// compileList compiles a list literal incrementally, as Toast does: the first
// element becomes a singleton list (OP_MAKE_SINGLETON_LIST, one tick) or, when
// spliced, is checked to be a list (OP_CHECK_LIST_FOR_SPLICE, one tick); later
// elements are appended or spliced tick-free.
func (c *lowerer) compileList(n *verb.ListExpr) error {
	if len(n.Elements) == 0 {
		c.emit(bytecode.OP_MAKE_LIST)
		c.emitByte(0)
		return nil
	}
	if splice, ok := n.Elements[0].(*verb.SpliceExpr); ok {
		if err := c.compileNode(splice.Expr); err != nil {
			return err
		}
		c.emit(bytecode.OP_SPLICE)
	} else {
		if err := c.compileNode(n.Elements[0]); err != nil {
			return err
		}
		c.emit(bytecode.OP_MAKE_LIST)
		c.emitByte(1)
	}

	for _, elem := range n.Elements[1:] {
		if splice, ok := elem.(*verb.SpliceExpr); ok {
			// Splice: compile inner expression, then extend
			if err := c.compileNode(splice.Expr); err != nil {
				return err
			}
			c.emit(bytecode.OP_LIST_EXTEND)
		} else {
			// Regular element: compile, then append
			if err := c.compileNode(elem); err != nil {
				return err
			}
			c.emit(bytecode.OP_LIST_APPEND)
		}
	}

	return nil
}

// compileListRange compiles a range list: {start..end}
// Emits: [start] [end] bytecode.OP_LIST_RANGE
// VM handler builds the list at runtime.
func (c *lowerer) compileListRange(n *verb.ListRangeExpr) error {
	// Compile start expression
	if err := c.compileNode(n.Start); err != nil {
		return err
	}

	// Compile end expression
	if err := c.compileNode(n.End); err != nil {
		return err
	}

	// Emit bytecode.OP_LIST_RANGE: pops end, start; pushes {start..end} list
	c.emit(bytecode.OP_LIST_RANGE)
	return nil
}

// compileMap compiles a map literal: [key -> value, ...]
func (c *lowerer) compileMap(n *verb.MapExpr) error {
	// Build map incrementally in a temp local via bytecode.OP_INDEX_SET.
	// Toast's OP_MAP_CREATE / OP_MAP_INSERT are tick-free.
	tmp := c.declareInternalVariable(c.tempVar("maplit"))
	c.emit(bytecode.OP_MAKE_MAP)
	c.emitByte(0)
	c.emitStoreLocal(tmp)

	for _, pair := range n.Pairs {
		// bytecode.OP_INDEX_SET pops index first, then value.
		if err := c.compileNode(pair.Value); err != nil {
			return err
		}
		if err := c.compileNode(pair.Key); err != nil {
			return err
		}
		c.emitTicks(0)
		c.emit(bytecode.OP_INDEX_SET)
		c.emitByte(byte(tmp))
	}

	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(tmp))
	c.emit(bytecode.OP_CHECK_MAP_LIMIT)
	return nil
}
