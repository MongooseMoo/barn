package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/parser"
	"barn/types"
	"fmt"
)

// Compiler compiles AST nodes to bytecode
type Compiler struct {
	program        *Program
	constants      map[string]int       // Constant deduplication (Value.String() -> index)
	variables      map[string]int       // Variable name -> index mapping
	loops          []LoopContext        // Loop context stack for break/continue
	scopes         []Scope              // Variable scope stack
	tempCount      int                  // Counter for unique temporary variable names
	registry       *builtins.Registry   // Builtin function registry for name->ID resolution
	indexContextVar int                 // Variable slot holding collection length for $ resolution (-1 = none)
	lastLine       int                  // Last emitted line number for LineInfo deduplication
}

// LoopContext tracks loop compilation state
type LoopContext struct {
	Label         string
	BreakJumps    []int // Patch locations for break jumps (forward jumps past loop end)
	ContinueJumps []int // Patch locations for continue jumps (forward jumps to increment)
	ContinueIP    int   // Target IP for continue (0 = use ContinueJumps for forward patching)
	StartIP       int   // Loop condition start (for backward jump at end of body)
	ResultVar     int   // Variable slot holding loop result (from break expr or default 0)
}

// Scope tracks variables in a lexical scope
type Scope struct {
	Variables map[string]int
	Parent    *Scope
}

// NewCompiler creates a new compiler
func NewCompiler() *Compiler {
	return &Compiler{
		program: &Program{
			Code:      make([]byte, 0, 256),
			Constants: make([]types.Value, 0, 32),
			VarNames:  make([]string, 0, 16),
			LineInfo:  make([]LineEntry, 0, 32),
		},
		constants:       make(map[string]int),
		variables:       make(map[string]int),
		loops:           make([]LoopContext, 0, 8),
		scopes:          make([]Scope, 0, 8),
		indexContextVar: -1,
	}
}

// NewCompilerWithRegistry creates a new compiler with a builtins registry
// for resolving builtin function names to IDs at compile time.
func NewCompilerWithRegistry(registry *builtins.Registry) *Compiler {
	c := NewCompiler()
	c.registry = registry
	return c
}

// Compile compiles a node to a Program
func (c *Compiler) Compile(node parser.Node) (*Program, error) {
	// Initialize global scope
	c.beginScope()

	// Compile the node
	if err := c.compileNode(node); err != nil {
		return nil, err
	}

	// If the node is a loop statement (which pushes its result), use OP_RETURN
	// to return the loop value. Otherwise use implicit return 0.
	if stmt, ok := node.(parser.Stmt); ok && isLoopStmt(stmt) {
		c.emit(OP_RETURN)
	} else {
		c.emit(OP_RETURN_NONE)
	}

	// End global scope
	c.endScope()

	return c.program, nil
}

// CompileStatements compiles a slice of statements (e.g. a verb body) to a Program.
// An implicit "return 0" is appended if no explicit return is present (MOO verbs
// return 0 by default). When the last statement is a loop, its result value
// (from break expr or default 0) is used as the implicit return value, matching
// tree-walker EvalStatements behavior.
// VarNames is populated from the compiler's variable table.
func (c *Compiler) CompileStatements(stmts []parser.Stmt) (*Program, error) {
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

		// If the last statement is a loop, it pushed its result onto the stack.
		// Use OP_RETURN to return that value (matches tree-walker lastResult behavior).
		if isLoopStmt(last) {
			c.emit(OP_RETURN)
		} else {
			c.emit(OP_RETURN_NONE)
		}
	} else {
		c.emit(OP_RETURN_NONE)
	}

	c.endScope()

	// VarNames is already populated by declareVariable via compileBlock,
	// but ensure the mapping is complete by building from the variables map.
	// The compiler's declareVariable already appends to program.VarNames in order,
	// so program.VarNames[idx] == name for all entries in c.variables.
	// No extra work needed here — VarNames is populated incrementally.

	return c.program, nil
}

// CompileVerbBytecode compiles a verb's AST to bytecode, caching the result on the verb.
// If the verb already has cached bytecode, returns it immediately.
// If the verb has no parsed AST, parses the source code first via db.CompileVerb.
// Returns the compiled *Program or an error.
func CompileVerbBytecode(verb *db.Verb, registry *builtins.Registry) (*Program, error) {
	// Check cache first
	if verb.BytecodeCache != nil {
		if prog, ok := verb.BytecodeCache.(*Program); ok {
			return prog, nil
		}
	}

	// Ensure verb has parsed AST
	if verb.Program == nil {
		vp, errs := db.CompileVerb(verb.Code)
		if errs != nil {
			return nil, fmt.Errorf("parse error: %v", errs[0])
		}
		verb.Program = vp
	}

	// Compile AST to bytecode
	c := NewCompilerWithRegistry(registry)
	prog, err := c.CompileStatements(verb.Program.Statements)
	if err != nil {
		return nil, fmt.Errorf("bytecode compile error: %w", err)
	}

	// Cache the result
	verb.BytecodeCache = prog
	return prog, nil
}

// emit adds an opcode to the bytecode
func (c *Compiler) emit(op OpCode) int {
	pos := len(c.program.Code)
	c.program.Code = append(c.program.Code, byte(op))
	return pos
}

// emitByte adds a byte to the bytecode
func (c *Compiler) emitByte(b byte) {
	c.program.Code = append(c.program.Code, b)
}

// emitShort adds a 2-byte short to the bytecode (big-endian)
func (c *Compiler) emitShort(s uint16) {
	c.program.Code = append(c.program.Code, byte(s>>8), byte(s))
}

// emitConstant adds a constant and emits OP_PUSH
func (c *Compiler) emitConstant(v types.Value) {
	idx := c.addConstant(v)
	c.emit(OP_PUSH)
	c.emitByte(byte(idx))
}

// addConstant adds a value to the constant pool (with deduplication)
func (c *Compiler) addConstant(v types.Value) int {
	// Check if constant already exists
	key := v.String()
	if idx, ok := c.constants[key]; ok {
		return idx
	}

	// Add new constant
	idx := len(c.program.Constants)
	c.program.Constants = append(c.program.Constants, v)
	c.constants[key] = idx
	return idx
}

// emitJump emits a jump instruction and returns the offset to patch
func (c *Compiler) emitJump(op OpCode) int {
	c.emit(op)
	c.emitShort(0xFFFF) // Placeholder offset
	return len(c.program.Code) - 2
}

// patchJump patches a jump instruction to jump to current location
func (c *Compiler) patchJump(offset int) {
	jump := len(c.program.Code) - offset - 2
	if jump > 0xFFFF {
		panic("jump too large")
	}
	c.program.Code[offset] = byte(jump >> 8)
	c.program.Code[offset+1] = byte(jump)
}

// currentOffset returns the current bytecode offset
func (c *Compiler) currentOffset() int {
	return len(c.program.Code)
}

// trackLine records a line number entry if the given AST node's line differs
// from the last recorded line. This populates Program.LineInfo so that runtime
// errors can include source line numbers.
func (c *Compiler) trackLine(node parser.Node) {
	line := node.Position().Line
	if line > 0 && line != c.lastLine {
		c.program.LineInfo = append(c.program.LineInfo, LineEntry{
			StartIP: len(c.program.Code),
			Line:    line,
		})
		c.lastLine = line
	}
}

// beginScope starts a new variable scope
func (c *Compiler) beginScope() {
	scope := Scope{
		Variables: make(map[string]int),
	}
	if len(c.scopes) > 0 {
		scope.Parent = &c.scopes[len(c.scopes)-1]
	}
	c.scopes = append(c.scopes, scope)
}

// endScope ends the current variable scope
func (c *Compiler) endScope() {
	if len(c.scopes) > 0 {
		c.scopes = c.scopes[:len(c.scopes)-1]
	}
}

// declareVariable declares a variable in current scope
func (c *Compiler) declareVariable(name string) int {
	// Check if already exists in global variable table
	if idx, ok := c.variables[name]; ok {
		return idx
	}

	// Add to global variable table
	idx := len(c.program.VarNames)
	c.program.VarNames = append(c.program.VarNames, name)
	c.variables[name] = idx

	// Track max locals
	if idx+1 > c.program.NumLocals {
		c.program.NumLocals = idx + 1
	}

	// Add to current scope
	if len(c.scopes) > 0 {
		c.scopes[len(c.scopes)-1].Variables[name] = idx
	}

	return idx
}

// resolveVariable resolves a variable name to its index
func (c *Compiler) resolveVariable(name string) (int, bool) {
	idx, ok := c.variables[name]
	return idx, ok
}

// beginLoop starts a new loop context.
// resultVar is the local slot that holds the loop's result value (from break expr or default 0).
func (c *Compiler) beginLoop(label string, resultVar int) {
	c.loops = append(c.loops, LoopContext{
		Label:         label,
		StartIP:       c.currentOffset(),
		ContinueIP:    0, // 0 = not set yet; will use ContinueJumps for forward patching
		BreakJumps:    make([]int, 0, 4),
		ContinueJumps: make([]int, 0, 4),
		ResultVar:     resultVar,
	})
}

// endLoop ends the current loop context and patches all break jumps to current location
func (c *Compiler) endLoop() {
	if len(c.loops) > 0 {
		loop := &c.loops[len(c.loops)-1]
		// Patch all break jumps to point to current location (after the loop)
		for _, offset := range loop.BreakJumps {
			c.patchJump(offset)
		}
		c.loops = c.loops[:len(c.loops)-1]
	}
}

// currentLoop returns the current loop context
func (c *Compiler) currentLoop() *LoopContext {
	if len(c.loops) == 0 {
		return nil
	}
	return &c.loops[len(c.loops)-1]
}

// findLoop finds a loop by label (or innermost if label is empty)
func (c *Compiler) findLoop(label string) *LoopContext {
	if label == "" {
		return c.currentLoop()
	}

	for i := len(c.loops) - 1; i >= 0; i-- {
		if c.loops[i].Label == label {
			return &c.loops[i]
		}
	}
	return nil
}

// compileNode dispatches compilation based on node type
func (c *Compiler) compileNode(node parser.Node) error {
	// Track source line for runtime error reporting
	c.trackLine(node)

	switch n := node.(type) {
	// Expressions
	case *parser.LiteralExpr:
		return c.compileLiteral(n)
	case *parser.IdentifierExpr:
		return c.compileIdentifier(n)
	case *parser.UnaryExpr:
		return c.compileUnary(n)
	case *parser.BinaryExpr:
		return c.compileBinary(n)
	case *parser.TernaryExpr:
		return c.compileTernary(n)
	case *parser.ParenExpr:
		return c.compileNode(n.Expr)
	case *parser.AssignExpr:
		return c.compileAssign(n)
	case *parser.BuiltinCallExpr:
		return c.compileBuiltinCall(n)
	case *parser.IndexExpr:
		return c.compileIndex(n)
	case *parser.RangeExpr:
		return c.compileRange(n)
	case *parser.IndexMarkerExpr:
		return c.compileIndexMarker(n)
	case *parser.PropertyExpr:
		return c.compileProperty(n)
	case *parser.VerbCallExpr:
		return c.compileVerbCall(n)
	case *parser.SpliceExpr:
		return c.compileSplice(n)
	case *parser.CatchExpr:
		return c.compileCatch(n)
	case *parser.ListExpr:
		return c.compileList(n)
	case *parser.ListRangeExpr:
		return c.compileListRange(n)
	case *parser.MapExpr:
		return c.compileMap(n)

	// Statements
	case *parser.ExprStmt:
		return c.compileExprStmt(n)
	case *parser.IfStmt:
		return c.compileIf(n)
	case *parser.WhileStmt:
		return c.compileWhile(n)
	case *parser.ForStmt:
		return c.compileFor(n)
	case *parser.BreakStmt:
		return c.compileBreak(n)
	case *parser.ContinueStmt:
		return c.compileContinue(n)
	case *parser.ReturnStmt:
		return c.compileReturn(n)
	case *parser.TryExceptStmt:
		return c.compileTryExcept(n)
	case *parser.TryFinallyStmt:
		return c.compileTryFinally(n)
	case *parser.TryExceptFinallyStmt:
		return c.compileTryExceptFinally(n)
	case *parser.ScatterStmt:
		return c.compileScatter(n)
	case *parser.ForkStmt:
		return c.compileFork(n)

	default:
		return fmt.Errorf("unknown node type: %T", node)
	}
}

// compileLiteral compiles a literal value
func (c *Compiler) compileLiteral(n *parser.LiteralExpr) error {
	// Check if it's a small integer that can use immediate opcode
	if intVal, ok := n.Value.(types.IntValue); ok {
		val := int(intVal.Val)
		if op, ok := MakeImmediateOpcode(val); ok {
			c.emit(op)
			return nil
		}
	}

	// Otherwise push from constant pool
	c.emitConstant(n.Value)
	return nil
}

// compileIdentifier compiles a variable reference
func (c *Compiler) compileIdentifier(n *parser.IdentifierExpr) error {
	idx, ok := c.resolveVariable(n.Name)
	if !ok {
		// Variable not found - this will be a runtime error (E_VARNF)
		// For now, declare it (MOO has dynamic scoping)
		idx = c.declareVariable(n.Name)
	}

	c.emit(OP_GET_VAR)
	c.emitByte(byte(idx))
	return nil
}

// compileUnary compiles a unary expression
func (c *Compiler) compileUnary(n *parser.UnaryExpr) error {
	// Compile operand
	if err := c.compileNode(n.Operand); err != nil {
		return err
	}

	// Emit operator
	switch n.Operator {
	case parser.TOKEN_MINUS:
		c.emit(OP_NEG)
	case parser.TOKEN_NOT:
		c.emit(OP_NOT)
	case parser.TOKEN_BITNOT:
		c.emit(OP_BITNOT)
	default:
		return fmt.Errorf("unknown unary operator: %v", n.Operator)
	}

	return nil
}

// compileBinary compiles a binary expression
func (c *Compiler) compileBinary(n *parser.BinaryExpr) error {
	// Short-circuit for && and ||
	if n.Operator == parser.TOKEN_AND {
		return c.compileShortCircuitAnd(n)
	}
	if n.Operator == parser.TOKEN_OR {
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
	case parser.TOKEN_PLUS:
		c.emit(OP_ADD)
	case parser.TOKEN_MINUS:
		c.emit(OP_SUB)
	case parser.TOKEN_STAR:
		c.emit(OP_MUL)
	case parser.TOKEN_SLASH:
		c.emit(OP_DIV)
	case parser.TOKEN_PERCENT:
		c.emit(OP_MOD)
	case parser.TOKEN_CARET:
		c.emit(OP_POW)
	case parser.TOKEN_EQ:
		c.emit(OP_EQ)
	case parser.TOKEN_NE:
		c.emit(OP_NE)
	case parser.TOKEN_LT:
		c.emit(OP_LT)
	case parser.TOKEN_LE:
		c.emit(OP_LE)
	case parser.TOKEN_GT:
		c.emit(OP_GT)
	case parser.TOKEN_GE:
		c.emit(OP_GE)
	case parser.TOKEN_IN:
		c.emit(OP_IN)
	case parser.TOKEN_BITAND:
		c.emit(OP_BITAND)
	case parser.TOKEN_BITOR:
		c.emit(OP_BITOR)
	case parser.TOKEN_BITXOR:
		c.emit(OP_BITXOR)
	case parser.TOKEN_LSHIFT:
		c.emit(OP_SHL)
	case parser.TOKEN_RSHIFT:
		c.emit(OP_SHR)
	default:
		return fmt.Errorf("unknown binary operator: %v", n.Operator)
	}

	return nil
}

// compileShortCircuitAnd compiles && with short-circuit evaluation
func (c *Compiler) compileShortCircuitAnd(n *parser.BinaryExpr) error {
	// Compile left
	if err := c.compileNode(n.Left); err != nil {
		return err
	}

	// If false, skip right and leave false on stack
	skipJump := c.emitJump(OP_AND)

	// Compile right
	if err := c.compileNode(n.Right); err != nil {
		return err
	}

	// Patch jump
	c.patchJump(skipJump)
	return nil
}

// compileShortCircuitOr compiles || with short-circuit evaluation
func (c *Compiler) compileShortCircuitOr(n *parser.BinaryExpr) error {
	// Compile left
	if err := c.compileNode(n.Left); err != nil {
		return err
	}

	// If true, skip right and leave true on stack
	skipJump := c.emitJump(OP_OR)

	// Compile right
	if err := c.compileNode(n.Right); err != nil {
		return err
	}

	// Patch jump
	c.patchJump(skipJump)
	return nil
}

// compileTernary compiles a ternary expression
func (c *Compiler) compileTernary(n *parser.TernaryExpr) error {
	// Compile condition
	if err := c.compileNode(n.Condition); err != nil {
		return err
	}

	// Jump to else if false
	elseJump := c.emitJump(OP_JUMP_IF_FALSE)

	// Compile then branch
	if err := c.compileNode(n.ThenExpr); err != nil {
		return err
	}

	// Jump over else
	endJump := c.emitJump(OP_JUMP)

	// Compile else branch
	c.patchJump(elseJump)
	if err := c.compileNode(n.ElseExpr); err != nil {
		return err
	}

	// Patch end jump
	c.patchJump(endJump)
	return nil
}

// compileAssign compiles an assignment expression
func (c *Compiler) compileAssign(n *parser.AssignExpr) error {
	// Compile value
	if err := c.compileNode(n.Value); err != nil {
		return err
	}

	// Duplicate value (assignment returns the value)
	c.emit(OP_DUP)

	// Handle different target types
	switch target := n.Target.(type) {
	case *parser.IdentifierExpr:
		// Simple variable assignment
		idx := c.declareVariable(target.Name)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(idx))
	case *parser.IndexExpr:
		// Index assignment: coll[idx] = value  OR  nested: coll[i][j]... = value
		// Walk the IndexExpr chain to find the base variable and collect indices
		var indices []parser.Expr
		var baseExpr parser.Expr = target
		for {
			ie, ok := baseExpr.(*parser.IndexExpr)
			if !ok {
				break
			}
			indices = append(indices, ie.Index)
			baseExpr = ie.Expr
		}

		// Determine base type: variable or property
		var baseVarIdx int
		var basePropExpr *parser.PropertyExpr

		if baseIdent, ok := baseExpr.(*parser.IdentifierExpr); ok {
			// Variable-based: x[i] = val
			baseVarIdx = c.declareVariable(baseIdent.Name)
		} else if propExpr, ok := baseExpr.(*parser.PropertyExpr); ok {
			// Property-based: obj.prop[i] = val
			// Read the property value into a temp variable, use it as the base,
			// then write the modified temp back to the property after index ops.
			basePropExpr = propExpr

			// Stack currently: [value, value_copy]
			// Store value_copy into temp so we can use the stack for GET_PROP
			tmpValHold := c.declareVariable("__prop_idx_val")
			c.emit(OP_SET_VAR)
			c.emitByte(byte(tmpValHold))
			// Stack: [value]

			// Compile obj expression, emit GET_PROP to read current property value
			if err := c.compileNode(propExpr.Expr); err != nil {
				return err
			}
			if propExpr.Property != "" {
				propIdx := c.addConstant(types.NewStr(propExpr.Property))
				c.emit(OP_GET_PROP)
				c.emitByte(byte(propIdx))
			} else if propExpr.PropertyExpr != nil {
				if err := c.compileNode(propExpr.PropertyExpr); err != nil {
					return err
				}
				c.emit(OP_GET_PROP)
				c.emitByte(0xFF)
			} else {
				return fmt.Errorf("property expression has neither static name nor dynamic expression")
			}
			// Stack: [value, prop_value]

			// Store property value into a temp that acts as the "base variable"
			baseVarIdx = c.declareVariable("__prop_idx_base")
			c.emit(OP_SET_VAR)
			c.emitByte(byte(baseVarIdx))
			// Stack: [value]

			// Restore the value_copy onto the stack for the index assignment code below
			c.emit(OP_GET_VAR)
			c.emitByte(byte(tmpValHold))
			// Stack: [value, value_copy]
		} else {
			return fmt.Errorf("index assignment target must be a variable or property")
		}

		// indices are collected outermost-first: for x[i][j], indices = [j, i]
		// Reverse to get base-to-deepest order: [i, j]
		for left, right := 0, len(indices)-1; left < right; left, right = left+1, right-1 {
			indices[left], indices[right] = indices[right], indices[left]
		}

		depth := len(indices)

		if depth == 1 {
			// Single-level index assignment (original fast path)
			// Stack currently: [value, value_copy]
			// Compile the index expression -> [value, value_copy, index]
			if err := c.compileNode(indices[0]); err != nil {
				return err
			}
			// VM will: pop index, pop value_copy, read coll from locals[baseVarIdx],
			// set coll[index] = value_copy, store modified coll back
			c.emit(OP_INDEX_SET)
			c.emitByte(byte(baseVarIdx))
		} else {
			// Nested index assignment: x[i1][i2]...[iN] = val
			// Desugar into temp variables using existing opcodes.
			//
			// Stack currently: [value, value_copy]

			// 1. Store value_copy into a temp variable
			tmpVal := c.declareVariable("__nested_val")
			c.emit(OP_SET_VAR)
			c.emitByte(byte(tmpVal))
			// Stack: [value]

			// 2. Evaluate each index into a temp variable
			tmpIndices := make([]int, depth)
			for k := 0; k < depth; k++ {
				if err := c.compileNode(indices[k]); err != nil {
					return err
				}
				tmpIndices[k] = c.declareVariable(fmt.Sprintf("__nested_idx_%d", k))
				c.emit(OP_SET_VAR)
				c.emitByte(byte(tmpIndices[k]))
			}
			// Stack: [value]

			// 3. Traverse down: read intermediate collections
			// For x[i][j][k], we need intermediates:
			//   inter_0 = x[i]          (depth-2 intermediates needed)
			//   inter_1 = inter_0[j]
			// Then set: inter_1[k] = val, inter_0[j] = inter_1, x[i] = inter_0
			tmpInter := make([]int, depth-1)
			for k := 0; k < depth-1; k++ {
				if k == 0 {
					// Read from base variable
					c.emit(OP_GET_VAR)
					c.emitByte(byte(baseVarIdx))
				} else {
					// Read from previous intermediate
					c.emit(OP_GET_VAR)
					c.emitByte(byte(tmpInter[k-1]))
				}
				c.emit(OP_GET_VAR)
				c.emitByte(byte(tmpIndices[k]))
				c.emit(OP_INDEX)
				tmpInter[k] = c.declareVariable(fmt.Sprintf("__nested_inter_%d", k))
				c.emit(OP_SET_VAR)
				c.emitByte(byte(tmpInter[k]))
			}
			// Stack: [value]

			// 4. Set at deepest level: lastIntermediate[lastIndex] = val
			c.emit(OP_GET_VAR)
			c.emitByte(byte(tmpVal))
			c.emit(OP_GET_VAR)
			c.emitByte(byte(tmpIndices[depth-1]))
			c.emit(OP_INDEX_SET)
			c.emitByte(byte(tmpInter[depth-2]))
			// Stack: [value]

			// 5. Rebuild going back up
			for k := depth - 2; k >= 1; k-- {
				// tmpInter[k-1][tmpIndices[k]] = tmpInter[k]
				c.emit(OP_GET_VAR)
				c.emitByte(byte(tmpInter[k]))
				c.emit(OP_GET_VAR)
				c.emitByte(byte(tmpIndices[k]))
				c.emit(OP_INDEX_SET)
				c.emitByte(byte(tmpInter[k-1]))
			}

			// 6. Set base: x[tmpIndices[0]] = tmpInter[0]
			c.emit(OP_GET_VAR)
			c.emitByte(byte(tmpInter[0]))
			c.emit(OP_GET_VAR)
			c.emitByte(byte(tmpIndices[0]))
			c.emit(OP_INDEX_SET)
			c.emitByte(byte(baseVarIdx))
			// Stack: [value] (the original value remains as expression result)
		}

		// If the base was a property, write the modified temp back to the property
		if basePropExpr != nil {
			// Stack: [value]
			// Load the modified base temp (now has the updated collection)
			c.emit(OP_GET_VAR)
			c.emitByte(byte(baseVarIdx))
			// Stack: [value, modified_collection]

			// Compile the object expression again
			if err := c.compileNode(basePropExpr.Expr); err != nil {
				return err
			}
			// Stack: [value, modified_collection, obj]

			// Emit SET_PROP: pops obj, pops modified_collection, writes property
			if basePropExpr.Property != "" {
				propIdx := c.addConstant(types.NewStr(basePropExpr.Property))
				c.emit(OP_SET_PROP)
				c.emitByte(byte(propIdx))
			} else if basePropExpr.PropertyExpr != nil {
				if err := c.compileNode(basePropExpr.PropertyExpr); err != nil {
					return err
				}
				c.emit(OP_SET_PROP)
				c.emitByte(0xFF)
			}
			// Stack: [value] (original assigned value remains as expression result)
		}
	case *parser.PropertyExpr:
		// Property assignment: obj.prop = value
		// Stack currently: [value, value_copy]
		// Compile the object expression -> [value, value_copy, obj]
		if err := c.compileNode(target.Expr); err != nil {
			return err
		}

		if target.Property != "" {
			// Static property: obj.prop = value
			propIdx := c.addConstant(types.NewStr(target.Property))
			c.emit(OP_SET_PROP)
			c.emitByte(byte(propIdx))
		} else if target.PropertyExpr != nil {
			// Dynamic property: obj.(expr) = value
			if err := c.compileNode(target.PropertyExpr); err != nil {
				return err
			}
			c.emit(OP_SET_PROP)
			c.emitByte(0xFF) // dynamic property name on stack
		} else {
			return fmt.Errorf("property expression has neither static name nor dynamic expression")
		}
	case *parser.RangeExpr:
		// Range assignment: coll[start..end] = value
		// Find the base variable
		baseIdent, ok := target.Expr.(*parser.IdentifierExpr)
		if !ok {
			return fmt.Errorf("range assignment target must be a variable")
		}

		// Ensure the base variable is declared
		varIdx := c.declareVariable(baseIdent.Name)

		// Stack currently: [value, value_copy]
		// Compile start index, resolving $ to collection length
		if err := c.compileRangeIndex(target.Start, varIdx); err != nil {
			return err
		}
		// Stack: [value, value_copy, start]

		// Compile end index, resolving $ to collection length
		if err := c.compileRangeIndex(target.End, varIdx); err != nil {
			return err
		}
		// Stack: [value, value_copy, start, end]

		// Emit OP_RANGE_SET with variable index
		// VM will: pop end, start, value_copy; read coll from locals[varIdx];
		// replace coll[start..end] with value_copy; store back to locals[varIdx]
		// The original 'value' remains on stack as expression result
		c.emit(OP_RANGE_SET)
		c.emitByte(byte(varIdx))
	default:
		return fmt.Errorf("invalid assignment target: %T", target)
	}

	return nil
}

// compileRangeIndex compiles a range index expression, resolving $ and ^ markers
// to the collection length or 1 respectively.
func (c *Compiler) compileRangeIndex(expr parser.Expr, varIdx int) error {
	if marker, ok := expr.(*parser.IndexMarkerExpr); ok {
		if marker.Marker == parser.TOKEN_CARET {
			// ^ resolves to 1
			c.emitConstant(types.IntValue{Val: 1})
		} else if marker.Marker == parser.TOKEN_DOLLAR {
			// $ resolves to length of the collection variable
			c.emit(OP_GET_VAR)
			c.emitByte(byte(varIdx))
			c.emit(OP_LENGTH)
		} else {
			return fmt.Errorf("unknown index marker")
		}
		return nil
	}
	// Normal expression
	return c.compileNode(expr)
}

// Stub implementations for other compile methods
// These will be completed based on the actual requirements

func (c *Compiler) compileBuiltinCall(n *parser.BuiltinCallExpr) error {
	if c.registry == nil {
		return fmt.Errorf("builtin call compilation requires a builtins registry")
	}

	// Resolve function name to numeric ID at compile time
	funcID, ok := c.registry.GetID(n.Name)
	if !ok {
		return fmt.Errorf("unknown builtin function: %s", n.Name)
	}

	// Check if any argument is a splice expression
	hasSplice := false
	for _, arg := range n.Args {
		if _, ok := arg.(*parser.SpliceExpr); ok {
			hasSplice = true
			break
		}
	}

	if hasSplice {
		// Splice path: build args list incrementally using OP_LIST_APPEND/EXTEND
		c.emit(OP_MAKE_LIST)
		c.emitByte(0)
		for _, arg := range n.Args {
			if splice, ok := arg.(*parser.SpliceExpr); ok {
				if err := c.compileNode(splice.Expr); err != nil {
					return err
				}
				c.emit(OP_LIST_EXTEND)
			} else {
				if err := c.compileNode(arg); err != nil {
					return err
				}
				c.emit(OP_LIST_APPEND)
			}
		}
		// argc=0xFF signals that args list is on top of stack
		c.emit(OP_CALL_BUILTIN)
		c.emitByte(byte(funcID))
		c.emitByte(0xFF)
	} else {
		// Fast path: no splices, push args directly
		for _, arg := range n.Args {
			if err := c.compileNode(arg); err != nil {
				return err
			}
		}
		c.emit(OP_CALL_BUILTIN)
		c.emitByte(byte(funcID))
		c.emitByte(byte(len(n.Args)))
	}

	return nil
}

func (c *Compiler) compileIndex(n *parser.IndexExpr) error {
	// Compile collection
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// If the index contains $, set up an index context variable with the collection length.
	// Stack: [coll] -> DUP -> [coll, coll] -> LENGTH -> [coll, len] -> SET_VAR -> [coll]
	hasDollar := containsDollarMarker(n.Index)
	oldContextVar := c.indexContextVar
	if hasDollar {
		tempIdx := c.declareVariable(c.tempVar("idxctx"))
		c.emit(OP_DUP)
		c.emit(OP_LENGTH)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(tempIdx))
		c.indexContextVar = tempIdx
	}

	// Compile index
	if err := c.compileNode(n.Index); err != nil {
		return err
	}

	// Restore previous context
	c.indexContextVar = oldContextVar

	// Emit index operation
	c.emit(OP_INDEX)
	return nil
}

func (c *Compiler) compileRange(n *parser.RangeExpr) error {
	// Compile collection
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// If start or end contains $, set up an index context variable.
	// Stack: [coll] -> DUP -> [coll, coll] -> LENGTH -> [coll, len] -> SET_VAR -> [coll]
	hasDollar := containsDollarMarker(n.Start) || containsDollarMarker(n.End)
	oldContextVar := c.indexContextVar
	if hasDollar {
		tempIdx := c.declareVariable(c.tempVar("rngctx"))
		c.emit(OP_DUP)
		c.emit(OP_LENGTH)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(tempIdx))
		c.indexContextVar = tempIdx
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

	// Emit range operation
	c.emit(OP_RANGE)
	return nil
}

func (c *Compiler) compileIndexMarker(n *parser.IndexMarkerExpr) error {
	if n.Marker == parser.TOKEN_CARET {
		// ^ always resolves to 1 for lists/strings
		c.emitConstant(types.IntValue{Val: 1})
	} else if n.Marker == parser.TOKEN_DOLLAR {
		// $ resolves to collection length
		if c.indexContextVar >= 0 {
			// Read context: length was pre-computed and stored in a temp variable
			c.emit(OP_GET_VAR)
			c.emitByte(byte(c.indexContextVar))
		} else {
			// No index context (shouldn't happen for well-formed index/range expressions)
			// Fall back to literal -1 (will produce E_RANGE at runtime)
			c.emitConstant(types.IntValue{Val: -1})
		}
	} else {
		return fmt.Errorf("unknown index marker type")
	}
	return nil
}

func (c *Compiler) compileProperty(n *parser.PropertyExpr) error {
	// Compile the object expression (pushes object onto stack)
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	if n.Property != "" {
		// Static property: obj.prop
		// Push property name as a string constant, then emit OP_GET_PROP
		propIdx := c.addConstant(types.NewStr(n.Property))
		c.emit(OP_GET_PROP)
		c.emitByte(byte(propIdx))
	} else if n.PropertyExpr != nil {
		// Dynamic property: obj.(expr)
		// Compile the property name expression (pushes string onto stack)
		if err := c.compileNode(n.PropertyExpr); err != nil {
			return err
		}
		// Use 0xFF to signal "property name is on top of stack"
		c.emit(OP_GET_PROP)
		c.emitByte(0xFF)
	} else {
		return fmt.Errorf("property expression has neither static name nor dynamic expression")
	}

	return nil
}

func (c *Compiler) compileVerbCall(n *parser.VerbCallExpr) error {
	// Compile the object expression (pushes object onto stack)
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Check if any argument is a splice expression
	hasSplice := false
	for _, arg := range n.Args {
		if _, ok := arg.(*parser.SpliceExpr); ok {
			hasSplice = true
			break
		}
	}

	if hasSplice {
		// Splice path: build args list incrementally using OP_LIST_APPEND/EXTEND
		c.emit(OP_MAKE_LIST)
		c.emitByte(0)
		for _, arg := range n.Args {
			if splice, ok := arg.(*parser.SpliceExpr); ok {
				if err := c.compileNode(splice.Expr); err != nil {
					return err
				}
				c.emit(OP_LIST_EXTEND)
			} else {
				if err := c.compileNode(arg); err != nil {
					return err
				}
				c.emit(OP_LIST_APPEND)
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

	// For dynamic verb names, compile the verb name expression onto the stack
	// before emitting the opcode (so the VM can pop it)
	isDynamic := n.Verb == "" && n.VerbExpr != nil
	if isDynamic {
		if err := c.compileNode(n.VerbExpr); err != nil {
			return err
		}
	}

	// Emit OP_CALL_VERB with verb name index and argument count
	// Format: OP_CALL_VERB <verb_name_idx:byte> <argc:byte>
	// verb_name_idx = 0xFF means dynamic (verb name on top of stack)
	// argc = 0xFF means args list is on top of stack (splice mode)
	c.emit(OP_CALL_VERB)

	if isDynamic {
		c.emitByte(0xFF) // signal: verb name is on stack
	} else if n.Verb != "" {
		verbIdx := c.addConstant(types.NewStr(n.Verb))
		c.emitByte(byte(verbIdx))
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

func (c *Compiler) compileSplice(n *parser.SpliceExpr) error {
	// Compile the expression to splice
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Emit splice operation
	c.emit(OP_SPLICE)
	return nil
}

func (c *Compiler) compileCatch(n *parser.CatchExpr) error {
	// Catch expressions (`expr ! codes => default`) are compiled as a
	// single-clause try/except that leaves the result on the stack.
	//
	// With default:
	//   OP_TRY_EXCEPT 1 [codes...] [0 = no var] [handler_ip:short]
	//   [expr]
	//   OP_END_EXCEPT
	//   OP_JUMP [end]
	//   handler_ip: [default expr]
	//   end:
	//
	// Without default (return the error value):
	//   OP_TRY_EXCEPT 1 [codes...] [var+1] [handler_ip:short]
	//   [expr]
	//   OP_END_EXCEPT
	//   OP_JUMP [end]
	//   handler_ip: OP_GET_VAR [var]   (error was stored by HandleError)
	//   end:

	// For the no-default case, we need a temp variable to receive the error
	var errVarIdx int
	if n.Default == nil {
		errVarIdx = c.declareVariable(c.tempVar("catch_err"))
	}

	// Emit OP_TRY_EXCEPT with 1 clause
	c.emit(OP_TRY_EXCEPT)
	c.emitByte(1) // 1 clause

	// Emit catch codes
	c.emitByte(byte(len(n.Codes)))
	for _, code := range n.Codes {
		c.emitByte(byte(code))
	}

	// Variable index: 0 means no variable, idx+1 means store in local[idx]
	if n.Default == nil {
		c.emitByte(byte(errVarIdx + 1))
	} else {
		c.emitByte(0) // no variable needed
	}

	// Handler IP placeholder (absolute)
	handlerIPPatch := len(c.program.Code)
	c.emitShort(0xFFFF)

	// Compile the main expression
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Normal path: pop the except handler
	c.emit(OP_END_EXCEPT)

	// Jump past the handler body
	endJump := c.emitJump(OP_JUMP)

	// Patch handler IP to point here
	handlerIP := c.currentOffset()
	c.program.Code[handlerIPPatch] = byte(handlerIP >> 8)
	c.program.Code[handlerIPPatch+1] = byte(handlerIP)

	// Handler body
	if n.Default != nil {
		// Evaluate default expression
		if err := c.compileNode(n.Default); err != nil {
			return err
		}
	} else {
		// No default: push the captured error value
		c.emit(OP_GET_VAR)
		c.emitByte(byte(errVarIdx))
	}

	// Patch end jump
	c.patchJump(endJump)

	return nil
}

func (c *Compiler) compileExprStmt(n *parser.ExprStmt) error {
	// Compile expression
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Pop result (expression statement doesn't use result)
	c.emit(OP_POP)
	return nil
}

func (c *Compiler) compileIf(n *parser.IfStmt) error {
	// Compile condition
	if err := c.compileNode(n.Condition); err != nil {
		return err
	}

	// Jump to next clause if false
	elseJump := c.emitJump(OP_JUMP_IF_FALSE)

	// Compile then branch
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// Jump over else branches
	endJumps := []int{c.emitJump(OP_JUMP)}

	// Compile elseif chains
	c.patchJump(elseJump)
	for _, elseif := range n.ElseIfs {
		// Compile elseif condition
		if err := c.compileNode(elseif.Condition); err != nil {
			return err
		}

		// Jump to next clause if false
		nextJump := c.emitJump(OP_JUMP_IF_FALSE)

		// Compile elseif body
		if err := c.compileBlock(elseif.Body); err != nil {
			return err
		}

		// Jump to end
		endJumps = append(endJumps, c.emitJump(OP_JUMP))
		c.patchJump(nextJump)
	}

	// Compile else branch
	if n.Else != nil {
		if err := c.compileBlock(n.Else); err != nil {
			return err
		}
	}

	// Patch all end jumps
	for _, jump := range endJumps {
		c.patchJump(jump)
	}

	return nil
}

func (c *Compiler) compileWhile(n *parser.WhileStmt) error {
	// Declare temp variable for loop result (break expr value or default 0)
	resultVar := c.declareVariable(c.tempVar("loop_result"))
	// Initialize to 0 (default loop result when no break expr)
	if op, ok := MakeImmediateOpcode(0); ok {
		c.emit(op)
	}
	c.emit(OP_SET_VAR)
	c.emitByte(byte(resultVar))

	// Start loop
	c.beginLoop(n.Label, resultVar)
	loopStart := c.currentOffset()
	// For while loops, continue jumps back to condition check
	c.currentLoop().ContinueIP = loopStart

	// Compile condition
	if err := c.compileNode(n.Condition); err != nil {
		return err
	}

	// Exit loop if false
	exitJump := c.emitJump(OP_JUMP_IF_FALSE)

	// Compile body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// Jump back to start (backward jump)
	c.emit(OP_LOOP)
	// After reading opcode + short, IP = currentOffset + 2
	// We want IP - offset = loopStart, so offset = currentOffset + 2 - loopStart
	offset := c.currentOffset() + 2 - loopStart
	c.emitShort(uint16(offset))

	// Patch exit jump
	c.patchJump(exitJump)

	// End loop and push result
	c.endLoop()
	c.emit(OP_GET_VAR)
	c.emitByte(byte(resultVar))
	return nil
}

func (c *Compiler) compileFor(n *parser.ForStmt) error {
	if n.RangeStart != nil {
		return c.compileForRange(n)
	}
	return c.compileForList(n)
}

// tempVar generates a unique temporary variable name
func (c *Compiler) tempVar(prefix string) string {
	c.tempCount++
	return fmt.Sprintf("__%s_%d__", prefix, c.tempCount)
}

// containsDollarMarker checks if an expression tree contains a $ index marker.
// Used to determine if compileIndex/compileRange needs to set up an index context.
func containsDollarMarker(expr parser.Expr) bool {
	switch n := expr.(type) {
	case *parser.IndexMarkerExpr:
		return n.Marker == parser.TOKEN_DOLLAR
	case *parser.BinaryExpr:
		return containsDollarMarker(n.Left) || containsDollarMarker(n.Right)
	case *parser.UnaryExpr:
		return containsDollarMarker(n.Operand)
	case *parser.ParenExpr:
		return containsDollarMarker(n.Expr)
	case *parser.TernaryExpr:
		return containsDollarMarker(n.Condition) || containsDollarMarker(n.ThenExpr) || containsDollarMarker(n.ElseExpr)
	default:
		return false
	}
}

// compileForRange compiles: for x in [start..end] ... endfor
// Compiles to equivalent while loop pattern.
func (c *Compiler) compileForRange(n *parser.ForStmt) error {
	// Hidden variable for end bound
	endVar := c.declareVariable(c.tempVar("end"))
	valueVar := c.declareVariable(n.Value)

	// Declare temp variable for loop result (break expr value or default 0)
	resultVar := c.declareVariable(c.tempVar("loop_result"))
	if op, ok := MakeImmediateOpcode(0); ok {
		c.emit(op)
	}
	c.emit(OP_SET_VAR)
	c.emitByte(byte(resultVar))

	// Evaluate end and store
	if err := c.compileNode(n.RangeEnd); err != nil {
		return err
	}
	c.emit(OP_SET_VAR)
	c.emitByte(byte(endVar))

	// Evaluate start and store as loop variable
	if err := c.compileNode(n.RangeStart); err != nil {
		return err
	}
	c.emit(OP_SET_VAR)
	c.emitByte(byte(valueVar))

	// Loop start
	c.beginLoop(n.Label, resultVar)
	loopStart := c.currentOffset()

	// Condition: x <= end
	c.emit(OP_GET_VAR)
	c.emitByte(byte(valueVar))
	c.emit(OP_GET_VAR)
	c.emitByte(byte(endVar))
	c.emit(OP_LE)
	exitJump := c.emitJump(OP_JUMP_IF_FALSE)

	// Body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// Patch continue jumps to point here (the increment section)
	// continue in a for-range should increment before re-checking condition
	for _, offset := range c.currentLoop().ContinueJumps {
		c.patchJump(offset)
	}

	// Increment: x = x + 1
	c.emit(OP_GET_VAR)
	c.emitByte(byte(valueVar))
	if op, ok := MakeImmediateOpcode(1); ok {
		c.emit(op)
	}
	c.emit(OP_ADD)
	c.emit(OP_SET_VAR)
	c.emitByte(byte(valueVar))

	// Loop back
	c.emit(OP_LOOP)
	offset := c.currentOffset() + 2 - loopStart
	c.emitShort(uint16(offset))

	// Patch exit
	c.patchJump(exitJump)
	c.endLoop()
	// Push loop result onto stack
	c.emit(OP_GET_VAR)
	c.emitByte(byte(resultVar))
	return nil
}

// compileForList compiles: for x in (expr) ... endfor
// Handles lists, maps, and strings via OP_ITER_PREP runtime type dispatch.
// When an index/key variable is present (for v, k in ...), OP_ITER_PREP wraps
// elements as {value, key/index} pairs and the loop extracts both components.
func (c *Compiler) compileForList(n *parser.ForStmt) error {
	hasIndex := n.Index != ""

	// Hidden variables (unique per loop to support nesting)
	listVar := c.declareVariable(c.tempVar("list"))
	isPairsVar := c.declareVariable(c.tempVar("pairs"))
	idxVar := c.declareVariable(c.tempVar("idx"))
	lenVar := c.declareVariable(c.tempVar("len"))
	valueVar := c.declareVariable(n.Value)
	var indexVar int
	if hasIndex {
		indexVar = c.declareVariable(n.Index)
	}

	// Declare temp variable for loop result (break expr value or default 0)
	resultVar := c.declareVariable(c.tempVar("loop_result"))
	if op, ok := MakeImmediateOpcode(0); ok {
		c.emit(op)
	}
	c.emit(OP_SET_VAR)
	c.emitByte(byte(resultVar))

	// Evaluate container, then OP_ITER_PREP normalizes it
	if err := c.compileNode(n.Container); err != nil {
		return err
	}
	c.emit(OP_ITER_PREP)
	if hasIndex {
		c.emitByte(1)
	} else {
		c.emitByte(0)
	}
	// Stack now has: [normalizedList, isPairsFlag]
	// Store isPairs flag, then store list
	c.emit(OP_SET_VAR)
	c.emitByte(byte(isPairsVar))
	c.emit(OP_SET_VAR)
	c.emitByte(byte(listVar))

	// idx = 1
	if op, ok := MakeImmediateOpcode(1); ok {
		c.emit(op)
	}
	c.emit(OP_SET_VAR)
	c.emitByte(byte(idxVar))

	// len = length(list)
	c.emit(OP_GET_VAR)
	c.emitByte(byte(listVar))
	c.emit(OP_LENGTH)
	c.emit(OP_SET_VAR)
	c.emitByte(byte(lenVar))

	// Loop start
	c.beginLoop(n.Label, resultVar)
	loopStart := c.currentOffset()

	// Condition: idx <= len
	c.emit(OP_GET_VAR)
	c.emitByte(byte(idxVar))
	c.emit(OP_GET_VAR)
	c.emitByte(byte(lenVar))
	c.emit(OP_LE)
	exitJump := c.emitJump(OP_JUMP_IF_FALSE)

	// Get current element: elem = list[idx]
	c.emit(OP_GET_VAR)
	c.emitByte(byte(listVar))
	c.emit(OP_GET_VAR)
	c.emitByte(byte(idxVar))
	c.emit(OP_INDEX)
	// Stack: [elem]

	if hasIndex {
		// isPairs is always 1 when hasIndex is true (OP_ITER_PREP guarantees this)
		// elem is {value, key/index} pair
		// Store the pair temporarily via DUP
		c.emit(OP_DUP)
		// Stack: [elem, elem]
		// Extract value = elem[1]
		if op, ok := MakeImmediateOpcode(1); ok {
			c.emit(op)
		}
		c.emit(OP_INDEX)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(valueVar))
		// Stack: [elem]
		// Extract key/index = elem[2]
		if op, ok := MakeImmediateOpcode(2); ok {
			c.emit(op)
		}
		c.emit(OP_INDEX)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(indexVar))
	} else {
		// Check isPairs at runtime: if pairs, extract elem[1]; else use elem directly
		c.emit(OP_GET_VAR)
		c.emitByte(byte(isPairsVar))
		noPairsJump := c.emitJump(OP_JUMP_IF_FALSE)
		// isPairs == true: elem is {value, key}, extract value = elem[1]
		if op, ok := MakeImmediateOpcode(1); ok {
			c.emit(op)
		}
		c.emit(OP_INDEX)
		assignJump := c.emitJump(OP_JUMP)
		// isPairs == false: elem is already the value
		c.patchJump(noPairsJump)
		// No-op: elem is already on stack
		c.patchJump(assignJump)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(valueVar))
	}

	// Body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// Patch continue jumps to point here (the increment section)
	// continue in a for-list should increment before re-checking condition
	for _, offset := range c.currentLoop().ContinueJumps {
		c.patchJump(offset)
	}

	// Increment: idx = idx + 1
	c.emit(OP_GET_VAR)
	c.emitByte(byte(idxVar))
	if op, ok := MakeImmediateOpcode(1); ok {
		c.emit(op)
	}
	c.emit(OP_ADD)
	c.emit(OP_SET_VAR)
	c.emitByte(byte(idxVar))

	// Loop back
	c.emit(OP_LOOP)
	offset := c.currentOffset() + 2 - loopStart
	c.emitShort(uint16(offset))

	// Patch exit
	c.patchJump(exitJump)
	c.endLoop()
	// Push loop result onto stack
	c.emit(OP_GET_VAR)
	c.emitByte(byte(resultVar))
	return nil
}

func (c *Compiler) compileBreak(n *parser.BreakStmt) error {
	loop := c.findLoop(n.Label)
	if loop == nil {
		return fmt.Errorf("break outside of loop")
	}

	// If break has a value expression, compile it and store to loop result variable.
	// Otherwise the result variable already holds the default (0).
	if n.Value != nil {
		if err := c.compileNode(n.Value); err != nil {
			return err
		}
		c.emit(OP_SET_VAR)
		c.emitByte(byte(loop.ResultVar))
	}

	// Emit a forward jump past the loop end (will be patched by endLoop)
	patchOffset := c.emitJump(OP_JUMP)
	loop.BreakJumps = append(loop.BreakJumps, patchOffset)
	return nil
}

func (c *Compiler) compileContinue(n *parser.ContinueStmt) error {
	loop := c.findLoop(n.Label)
	if loop == nil {
		return fmt.Errorf("continue outside of loop")
	}

	if loop.ContinueIP > 0 {
		// ContinueIP is known (while loops) -- emit backward jump directly
		c.emit(OP_LOOP)
		// After reading opcode + short, IP = currentOffset + 2
		// We want IP - offset = ContinueIP, so offset = currentOffset + 2 - ContinueIP
		offset := c.currentOffset() + 2 - loop.ContinueIP
		c.emitShort(uint16(offset))
	} else {
		// ContinueIP not yet known (for loops) -- emit forward jump, patch later
		patchOffset := c.emitJump(OP_JUMP)
		loop.ContinueJumps = append(loop.ContinueJumps, patchOffset)
	}
	return nil
}

func (c *Compiler) compileReturn(n *parser.ReturnStmt) error {
	if n.Value != nil {
		// Compile return value
		if err := c.compileNode(n.Value); err != nil {
			return err
		}
		c.emit(OP_RETURN)
	} else {
		// Return 0
		c.emit(OP_RETURN_NONE)
	}
	return nil
}

func (c *Compiler) compileTryExcept(n *parser.TryExceptStmt) error {
	// Bytecode layout:
	//   OP_TRY_EXCEPT <num_clauses>
	//     per clause: <num_codes> <code1> <code2>... <var_index+1> <handler_offset:short>
	//   [body]
	//   OP_END_EXCEPT
	//   OP_JUMP <end_offset>  (skip past handler blocks on normal path)
	//   [handler 1 body]
	//   OP_JUMP <end_offset>
	//   [handler 2 body]
	//   OP_JUMP <end_offset>
	//   ...
	//   <end>

	numClauses := len(n.Excepts)

	// Emit OP_TRY_EXCEPT with clause count
	c.emit(OP_TRY_EXCEPT)
	c.emitByte(byte(numClauses))

	// Emit clause metadata with placeholder handler offsets
	clauseOffsetPatches := make([]int, numClauses)
	clauseVarIndices := make([]int, numClauses)

	for i, except := range n.Excepts {
		if except.IsAny {
			c.emitByte(0) // 0 codes = catch any
		} else {
			c.emitByte(byte(len(except.Codes)))
			for _, code := range except.Codes {
				c.emitByte(byte(code))
			}
		}

		// Variable index (0 = no variable, 1+ = index+1)
		if except.Variable != "" {
			idx := c.declareVariable(except.Variable)
			clauseVarIndices[i] = idx
			c.emitByte(byte(idx + 1)) // +1 so 0 means "no variable"
		} else {
			clauseVarIndices[i] = -1
			c.emitByte(0) // no variable
		}

		// Placeholder for handler IP (absolute offset in code)
		clauseOffsetPatches[i] = len(c.program.Code)
		c.emitShort(0xFFFF)
	}

	// Compile try body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// OP_END_EXCEPT pops handlers from ExceptStack
	c.emit(OP_END_EXCEPT)

	// Jump past all handler blocks (normal path)
	endJump := c.emitJump(OP_JUMP)

	// Compile each handler clause body
	handlerEndJumps := make([]int, 0, numClauses)
	for i, except := range n.Excepts {
		// Patch the handler offset to point here
		handlerIP := c.currentOffset()
		c.program.Code[clauseOffsetPatches[i]] = byte(handlerIP >> 8)
		c.program.Code[clauseOffsetPatches[i]+1] = byte(handlerIP)

		_ = except // metadata already handled above

		// Compile handler body
		if err := c.compileBlock(except.Body); err != nil {
			return err
		}

		// Jump to end (past remaining handlers)
		if i < numClauses-1 {
			hEndJump := c.emitJump(OP_JUMP)
			handlerEndJumps = append(handlerEndJumps, hEndJump)
		}
	}

	// Patch all end jumps
	c.patchJump(endJump)
	for _, j := range handlerEndJumps {
		c.patchJump(j)
	}

	return nil
}

func (c *Compiler) compileTryFinally(n *parser.TryFinallyStmt) error {
	// Bytecode layout:
	//   OP_TRY_FINALLY <finally_ip:short>
	//   [body]
	//   OP_END_FINALLY  (normal path: pop handler, fall through to finally code)
	//   <finally_ip>:   (error path entry point)
	//   [finally block]
	//   OP_END_FINALLY  (re-raise PendingError if set)

	// Emit OP_TRY_FINALLY with placeholder for finally IP
	c.emit(OP_TRY_FINALLY)
	finallyIPPatch := len(c.program.Code)
	c.emitShort(0xFFFF)

	// Compile try body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// OP_END_FINALLY on normal path: pop the handler
	c.emit(OP_END_FINALLY)

	// Patch finally IP to point here (the finally block entry for error path)
	finallyIP := c.currentOffset()
	c.program.Code[finallyIPPatch] = byte(finallyIP >> 8)
	c.program.Code[finallyIPPatch+1] = byte(finallyIP)

	// Compile finally block
	if err := c.compileBlock(n.Finally); err != nil {
		return err
	}

	// OP_END_FINALLY at end of finally block: re-raise pending error if any
	c.emit(OP_END_FINALLY)

	return nil
}

func (c *Compiler) compileTryExceptFinally(n *parser.TryExceptFinallyStmt) error {
	// This is a combination: wrap try/except inside try/finally.
	// Desugar to: try { try { body } except ... endtry } finally { ... } endtry
	//
	// Bytecode layout:
	//   OP_TRY_FINALLY <finally_ip:short>
	//   OP_TRY_EXCEPT <num_clauses> [clause metadata...]
	//   [body]
	//   OP_END_EXCEPT
	//   OP_JUMP <past_handlers>
	//   [handler bodies...]
	//   <past_handlers>:
	//   OP_END_FINALLY
	//   [finally block]

	// Outer: try/finally
	c.emit(OP_TRY_FINALLY)
	finallyIPPatch := len(c.program.Code)
	c.emitShort(0xFFFF)

	// Inner: try/except (reuse compileTryExcept logic inline)
	numClauses := len(n.Excepts)
	c.emit(OP_TRY_EXCEPT)
	c.emitByte(byte(numClauses))

	clauseOffsetPatches := make([]int, numClauses)
	for i, except := range n.Excepts {
		if except.IsAny {
			c.emitByte(0)
		} else {
			c.emitByte(byte(len(except.Codes)))
			for _, code := range except.Codes {
				c.emitByte(byte(code))
			}
		}
		if except.Variable != "" {
			idx := c.declareVariable(except.Variable)
			c.emitByte(byte(idx + 1))
		} else {
			c.emitByte(0)
		}
		clauseOffsetPatches[i] = len(c.program.Code)
		c.emitShort(0xFFFF)
	}

	// Compile try body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// End except handlers (normal path)
	c.emit(OP_END_EXCEPT)
	endExceptJump := c.emitJump(OP_JUMP)

	// Compile handler bodies
	handlerEndJumps := make([]int, 0, numClauses)
	for i, except := range n.Excepts {
		handlerIP := c.currentOffset()
		c.program.Code[clauseOffsetPatches[i]] = byte(handlerIP >> 8)
		c.program.Code[clauseOffsetPatches[i]+1] = byte(handlerIP)

		if err := c.compileBlock(except.Body); err != nil {
			return err
		}

		if i < numClauses-1 {
			hEndJump := c.emitJump(OP_JUMP)
			handlerEndJumps = append(handlerEndJumps, hEndJump)
		}
	}

	c.patchJump(endExceptJump)
	for _, j := range handlerEndJumps {
		c.patchJump(j)
	}

	// OP_END_FINALLY on normal path: pop handler
	c.emit(OP_END_FINALLY)

	// Patch finally IP
	finallyIP := c.currentOffset()
	c.program.Code[finallyIPPatch] = byte(finallyIP >> 8)
	c.program.Code[finallyIPPatch+1] = byte(finallyIP)

	// Compile finally block
	if err := c.compileBlock(n.Finally); err != nil {
		return err
	}

	// OP_END_FINALLY at end: re-raise pending error if any
	c.emit(OP_END_FINALLY)

	return nil
}

func (c *Compiler) compileScatter(n *parser.ScatterStmt) error {
	// Scatter assignment: {a, ?b, @rest} = list
	//
	// Strategy:
	//   1. Compile RHS and store in temp variable
	//   2. Emit OP_SCATTER to validate type and length
	//   3. For each target, emit index/default/range assignment using existing opcodes
	//
	// OP_SCATTER pops the list from stack, validates:
	//   - Value is a list (E_TYPE if not)
	//   - Length >= numRequired (E_ARGS if too few)
	//   - If no @rest: length <= numRequired + numOptional (E_ARGS if too many)
	// Then pushes nothing (list is accessed via temp var for assignments).

	// Count required, optional, and rest targets
	numRequired := 0
	numOptional := 0
	hasRest := false
	for _, target := range n.Targets {
		if target.Rest {
			hasRest = true
		} else if target.Optional {
			numOptional++
		} else {
			numRequired++
		}
	}

	// Compile and store RHS in a temp variable
	listVar := c.declareVariable(c.tempVar("scatter_list"))
	if err := c.compileNode(n.Value); err != nil {
		return err
	}
	c.emit(OP_DUP) // OP_SCATTER will pop one copy; temp var gets the other
	c.emit(OP_SET_VAR)
	c.emitByte(byte(listVar))

	// Emit OP_SCATTER for validation (pops the list from stack)
	c.emit(OP_SCATTER)
	c.emitByte(byte(numRequired))
	c.emitByte(byte(numOptional))
	if hasRest {
		c.emitByte(1)
	} else {
		c.emitByte(0)
	}

	// Now emit assignment code for each target
	// elemIdx tracks the 1-based index into the list (matching tree-walker's elemIdx + 1)
	elemIdx := 1
	for _, target := range n.Targets {
		if target.Rest {
			// Rest is handled after all other targets
			continue
		}

		targetVar := c.declareVariable(target.Name)

		if target.Optional {
			// Optional target: check if list has enough elements
			// if length(list) >= elemIdx then list[elemIdx] else default
			c.emit(OP_GET_VAR)
			c.emitByte(byte(listVar))
			c.emit(OP_LENGTH)
			c.emitConstant(types.IntValue{Val: int64(elemIdx)})
			c.emit(OP_GE) // length >= elemIdx?
			elseJump := c.emitJump(OP_JUMP_IF_FALSE)

			// Has value: list[elemIdx]
			c.emit(OP_GET_VAR)
			c.emitByte(byte(listVar))
			c.emitConstant(types.IntValue{Val: int64(elemIdx)})
			c.emit(OP_INDEX)
			c.emit(OP_SET_VAR)
			c.emitByte(byte(targetVar))
			endJump := c.emitJump(OP_JUMP)

			// Default branch
			c.patchJump(elseJump)
			if target.Default != nil {
				if err := c.compileNode(target.Default); err != nil {
					return err
				}
			} else {
				c.emitConstant(types.IntValue{Val: 0})
			}
			c.emit(OP_SET_VAR)
			c.emitByte(byte(targetVar))

			c.patchJump(endJump)
		} else {
			// Required target: list[elemIdx]
			c.emit(OP_GET_VAR)
			c.emitByte(byte(listVar))
			c.emitConstant(types.IntValue{Val: int64(elemIdx)})
			c.emit(OP_INDEX)
			c.emit(OP_SET_VAR)
			c.emitByte(byte(targetVar))
		}

		elemIdx++
	}

	// Handle @rest target
	if hasRest {
		for _, target := range n.Targets {
			if !target.Rest {
				continue
			}
			restVar := c.declareVariable(target.Name)
			// if elemIdx > length(list): rest = {}
			// else: rest = list[elemIdx..length(list)]
			c.emit(OP_GET_VAR)
			c.emitByte(byte(listVar))
			c.emit(OP_LENGTH)
			c.emitConstant(types.IntValue{Val: int64(elemIdx)})
			c.emit(OP_GE) // length >= elemIdx?
			elseJump := c.emitJump(OP_JUMP_IF_FALSE)

			// Has remaining elements: rest = list[elemIdx..length(list)]
			c.emit(OP_GET_VAR)
			c.emitByte(byte(listVar))
			c.emitConstant(types.IntValue{Val: int64(elemIdx)})
			c.emit(OP_GET_VAR)
			c.emitByte(byte(listVar))
			c.emit(OP_LENGTH)
			c.emit(OP_RANGE)
			c.emit(OP_SET_VAR)
			c.emitByte(byte(restVar))
			endJump := c.emitJump(OP_JUMP)

			// No remaining elements: rest = {}
			c.patchJump(elseJump)
			c.emit(OP_MAKE_LIST)
			c.emitByte(0) // empty list
			c.emit(OP_SET_VAR)
			c.emitByte(byte(restVar))

			c.patchJump(endJump)
			break // only one @rest allowed
		}
	}

	return nil
}

func (c *Compiler) compileFork(n *parser.ForkStmt) error {
	// Fork statement: fork [name] (delay) body endfork
	//
	// Bytecode layout:
	//   [delay expression]         -- evaluates delay, pushes onto stack
	//   OP_FORK <varIdx> <bodyLen:short>  -- pops delay, validates, sets var=0, jumps over body
	//   [body statements]          -- compiled but skipped at runtime (for future scheduling)
	//
	// varIdx: 0 = anonymous fork, idx+1 = store task ID (0) in locals[idx]
	// bodyLen: number of bytes to skip past the fork body

	// Compile the delay expression
	if err := c.compileNode(n.Delay); err != nil {
		return err
	}

	// Determine variable index
	var varIdx int
	if n.VarName != "" {
		varIdx = c.declareVariable(n.VarName) + 1 // +1 so 0 means "no variable"
	}

	// Emit OP_FORK with variable index and placeholder body length
	c.emit(OP_FORK)
	c.emitByte(byte(varIdx))
	bodyLenPatch := len(c.program.Code)
	c.emitShort(0xFFFF) // placeholder for body length

	// Compile the fork body (will be skipped at runtime but compiled for future use)
	bodyStart := c.currentOffset()
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}
	bodyEnd := c.currentOffset()

	// Patch body length
	bodyLen := bodyEnd - bodyStart
	c.program.Code[bodyLenPatch] = byte(bodyLen >> 8)
	c.program.Code[bodyLenPatch+1] = byte(bodyLen)

	return nil
}

// isLoopStmt returns true if a statement node is a loop (pushes a result value).
func isLoopStmt(stmt parser.Stmt) bool {
	switch stmt.(type) {
	case *parser.WhileStmt, *parser.ForStmt:
		return true
	default:
		return false
	}
}

func (c *Compiler) compileBlock(stmts []parser.Stmt) error {
	for _, stmt := range stmts {
		if err := c.compileNode(stmt); err != nil {
			return err
		}
		// Loop statements push their result value onto the stack.
		// In block context (if/try/loop bodies), discard it to keep the stack clean.
		if isLoopStmt(stmt) {
			c.emit(OP_POP)
		}
	}
	return nil
}

// compileList compiles a list literal: {expr, expr, ...}
// If no splice (@) elements are present, uses the fast path: push all elements
// then OP_MAKE_LIST <count>.
// If splices are present, builds incrementally: start with empty list, then
// OP_LIST_APPEND for each regular element and OP_LIST_EXTEND for each splice.
func (c *Compiler) compileList(n *parser.ListExpr) error {
	// Check if any element is a splice expression
	hasSplice := false
	for _, elem := range n.Elements {
		if _, ok := elem.(*parser.SpliceExpr); ok {
			hasSplice = true
			break
		}
	}

	if !hasSplice {
		// Fast path: no splices, use existing OP_MAKE_LIST
		for _, elem := range n.Elements {
			if err := c.compileNode(elem); err != nil {
				return err
			}
		}
		c.emit(OP_MAKE_LIST)
		c.emitByte(byte(len(n.Elements)))
		return nil
	}

	// Splice path: build list incrementally
	// Start with an empty list on the stack
	c.emit(OP_MAKE_LIST)
	c.emitByte(0)

	for _, elem := range n.Elements {
		if splice, ok := elem.(*parser.SpliceExpr); ok {
			// Splice: compile inner expression, then extend
			if err := c.compileNode(splice.Expr); err != nil {
				return err
			}
			c.emit(OP_LIST_EXTEND)
		} else {
			// Regular element: compile, then append
			if err := c.compileNode(elem); err != nil {
				return err
			}
			c.emit(OP_LIST_APPEND)
		}
	}

	return nil
}

// compileListRange compiles a range list: {start..end}
// Emits: [start] [end] OP_LIST_RANGE
// VM handler builds the list at runtime.
func (c *Compiler) compileListRange(n *parser.ListRangeExpr) error {
	// Compile start expression
	if err := c.compileNode(n.Start); err != nil {
		return err
	}

	// Compile end expression
	if err := c.compileNode(n.End); err != nil {
		return err
	}

	// Emit OP_LIST_RANGE: pops end, start; pushes {start..end} list
	c.emit(OP_LIST_RANGE)
	return nil
}

// compileMap compiles a map literal: [key -> value, ...]
func (c *Compiler) compileMap(n *parser.MapExpr) error {
	// Compile each key-value pair (in order)
	for _, pair := range n.Pairs {
		if err := c.compileNode(pair.Key); err != nil {
			return err
		}
		if err := c.compileNode(pair.Value); err != nil {
			return err
		}
	}
	// Emit MAKE_MAP with pair count
	c.emit(OP_MAKE_MAP)
	c.emitByte(byte(len(n.Pairs)))
	return nil
}
