package compiler

import (
	"fmt"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/verb"
)

// UnknownBuiltinError is returned by the compiler when a verb references a
// builtin function name that the registry does not know. It carries the name
// and source line so callers (e.g. set_verb_code) can format the error exactly
// as ToastStunt does: "Line N:  Unknown built-in function: NAME".
type UnknownBuiltinError struct {
	Name string
	Line int
}

func (e *UnknownBuiltinError) Error() string {
	return fmt.Sprintf("unknown built-in function: %s", e.Name)
}

// lowerer lowers one validated semantic program to bytecode.
type lowerer struct {
	program              *bytecode.Program
	constants            map[string]int       // Constant deduplication (exact typed value -> index)
	variables            map[string]int       // Source variable name -> index mapping
	internalVariables    map[string]int       // Compiler-only local name -> index mapping
	loops                []loopContext        // Loop context stack for break/continue
	scopes               []scope              // Variable scope stack
	tempCount            int                  // Counter for unique temporary variable names
	propertyAssignDepth  int                  // Active property-assignment nesting depth for reusable temp slots
	registry             map[string]int       // Immutable builtin name-to-ID view
	indexContextVar      int                  // Variable slot used by index-boundary compilation (-1 = none)
	indexBoundaryContext indexBoundaryContext // Whether map boundaries resolve as keys or positions
	lastLine             int                  // Last emitted line number for LineInfo deduplication
	err                  error                // First overflow/limit error; checked at Compile boundaries
}

func newLowerer(registry map[string]int) *lowerer {
	return &lowerer{
		program: &bytecode.Program{
			Code:      make([]byte, 0, 256),
			Constants: make([]types.Value, 0, 32),
			VarNames:  make([]string, 0, 16),
			LineInfo:  make([]bytecode.LineEntry, 0, 32),
		},
		constants:         make(map[string]int),
		variables:         make(map[string]int),
		internalVariables: make(map[string]int),
		loops:             make([]loopContext, 0, 8),
		scopes:            make([]scope, 0, 8),
		registry:          registry,
		indexContextVar:   -1,
	}
}

// compileProgram compiles a semantic verb program to bytecode.
// An implicit "return 0" is appended if no explicit return is present (MOO verbs
// return 0 by default). When the last statement is a loop, its result value
// (from break expr or default 0) is used as the implicit return value.
// VarNames is populated from the compiler's variable table.
func (c *lowerer) compileProgram(program *verb.Program) (*bytecode.Program, error) {
	if err := verb.Validate(program); err != nil {
		return nil, err
	}
	stmts := program.Statements
	c.beginScope()

	if len(stmts) > 0 {
		// Compile all but the last statement using compileBlock (which pops loop results)
		if len(stmts) > 1 {
			if err := c.compileBlock(stmts[:len(stmts)-1]); err != nil {
				return nil, err
			}
		}

		// Compile the last statement directly (without auto-pop for loops)
		last := stmts[len(stmts)-1]
		if err := c.compileNode(last); err != nil {
			return nil, err
		}

		// Check for accumulated overflow errors
		if c.err != nil {
			return nil, c.err
		}

		// If the last statement is a loop, it pushed its result onto the stack.
		// Use bytecode.OP_RETURN to return that value.
		if isLoopStmt(last) {
			c.emit(bytecode.OP_RETURN)
		} else {
			c.emit(bytecode.OP_RETURN_NONE)
		}
	} else {
		c.emit(bytecode.OP_RETURN_NONE)
	}

	c.endScope()

	// VarNames is already populated by declareVariable via compileBlock,
	// but ensure the mapping is complete by building from the variables map.
	// The compiler's declareVariable already appends to program.VarNames in order,
	// so program.VarNames[idx] == name for all entries in c.variables.
	// No extra work needed here — VarNames is populated incrementally.
	if len(c.internalVariables) != 0 {
		if err := c.program.CompactInternalLocals(256 - len(c.internalVariables)); err != nil {
			return nil, err
		}
	}

	return c.program, nil
}

// compileNode dispatches compilation based on node type
func (c *lowerer) compileNode(node verb.Node) error {
	// Bail out early if an overflow error has been recorded
	if c.err != nil {
		return c.err
	}

	// Program validation supplies non-nil nodes; keep this internal guard.
	if node == nil {
		return fmt.Errorf("nil semantic node")
	}

	// Track source line for runtime error reporting
	c.trackLine(node)

	switch n := node.(type) {
	// Expressions
	case *verb.LiteralExpr:
		return c.compileLiteral(n)
	case *verb.IdentifierExpr:
		return c.compileIdentifier(n)
	case *verb.UnaryExpr:
		return c.compileUnary(n)
	case *verb.BinaryExpr:
		return c.compileBinary(n)
	case *verb.TernaryExpr:
		return c.compileTernary(n)
	case *verb.AssignExpr:
		return c.compileAssign(n)
	case *verb.BuiltinCallExpr:
		return c.compileBuiltinCall(n)
	case *verb.IndexExpr:
		return c.compileIndex(n)
	case *verb.RangeExpr:
		return c.compileRange(n)
	case *verb.IndexBoundaryExpr:
		return c.compileIndexBoundary(n)
	case *verb.PropertyExpr:
		return c.compileProperty(n)
	case *verb.VerbCallExpr:
		return c.compileVerbCall(n)
	case *verb.SpliceExpr:
		return c.compileSplice(n)
	case *verb.CatchExpr:
		return c.compileCatch(n)
	case *verb.ListExpr:
		return c.compileList(n)
	case *verb.ListRangeExpr:
		return c.compileListRange(n)
	case *verb.MapExpr:
		return c.compileMap(n)

	// Statements
	case *verb.EmptyStmt:
		return nil
	case *verb.ExprStmt:
		return c.compileExprStmt(n)
	case *verb.IfStmt:
		return c.compileIf(n)
	case *verb.WhileStmt:
		return c.compileWhile(n)
	case *verb.CollectionLoopStmt:
		return c.compileCollectionLoop(n)
	case *verb.RangeLoopStmt:
		return c.compileRangeLoop(n)
	case *verb.BreakStmt:
		return c.compileBreak(n)
	case *verb.ContinueStmt:
		return c.compileContinue(n)
	case *verb.ReturnStmt:
		return c.compileReturn(n)
	case *verb.TryStmt:
		return c.compileTry(n)
	case *verb.ForkStmt:
		return c.compileFork(n)

	default:
		return fmt.Errorf("unknown node type: %T", node)
	}
}
