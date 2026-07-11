package bytecode

import (
	"fmt"
	"math"
	"strconv"

	"barn/types"
	"barn/verb"
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

// Registry is the narrow interface the compiler needs from the builtins
// registry: resolve a builtin function name to its numeric ID at compile time.
// Defined here so the bytecode package does NOT import barn/builtins (which would
// create an import cycle, since builtins imports bytecode). *builtins.Registry
// satisfies this structurally. The single GetID call happens once per verb at
// COMPILE time, never on the per-instruction execution hot path.
type Registry interface {
	GetID(name string) (int, bool)
}

// Compiler lowers semantic verb nodes to bytecode.
type Compiler struct {
	program           *Program
	constants         map[string]int    // Constant deduplication (Value.String() -> index)
	variables         map[string]int    // Variable name -> index mapping
	loops             []LoopContext     // Loop context stack for break/continue
	scopes            []Scope           // Variable scope stack
	tempCount         int               // Counter for unique temporary variable names
	registry          Registry          // Builtin function registry for name->ID resolution
	indexContextVar   int               // Variable slot used by index-boundary compilation (-1 = none)
	indexBoundaryMode indexBoundaryMode // Controls ^/$ compilation semantics in current context
	lastLine          int               // Last emitted line number for LineInfo deduplication
	err               error             // First overflow/limit error; checked at Compile boundaries
}

type indexBoundaryMode int

const (
	// ^ => 1, $ => collection length from indexContextVar.
	indexBoundaryModeLength indexBoundaryMode = iota
	// ^/$ => first/last for maps, 1/length for list/string using collection in indexContextVar.
	indexBoundaryModeCollection
)

// LoopContext tracks loop compilation state
type LoopContext struct {
	Label         string
	ValueName     string
	IndexName     string
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
		constants:         make(map[string]int),
		variables:         make(map[string]int),
		loops:             make([]LoopContext, 0, 8),
		scopes:            make([]Scope, 0, 8),
		indexContextVar:   -1,
		indexBoundaryMode: indexBoundaryModeLength,
	}
}

// NewCompilerWithRegistry creates a new compiler with a builtins registry
// for resolving builtin function names to IDs at compile time.
func NewCompilerWithRegistry(registry Registry) *Compiler {
	c := NewCompiler()
	c.registry = registry
	return c
}

// Compile compiles a node to a Program
func (c *Compiler) Compile(node verb.Node) (*Program, error) {
	// Initialize global scope
	c.beginScope()

	// Compile the node
	if err := c.compileNode(node); err != nil {
		return nil, err
	}

	// Check for accumulated overflow errors
	if c.err != nil {
		return nil, c.err
	}

	// If the node is a loop statement (which pushes its result), use OP_RETURN
	// to return the loop value. Otherwise use implicit return 0.
	if stmt, ok := node.(verb.Stmt); ok && isLoopStmt(stmt) {
		c.emit(OP_RETURN)
	} else {
		c.emit(OP_RETURN_NONE)
	}

	// End global scope
	c.endScope()

	return c.program, nil
}

// CompileProgram compiles a semantic verb program to bytecode.
// An implicit "return 0" is appended if no explicit return is present (MOO verbs
// return 0 by default). When the last statement is a loop, its result value
// (from break expr or default 0) is used as the implicit return value.
// VarNames is populated from the compiler's variable table.
func (c *Compiler) CompileProgram(program *verb.Program) (*Program, error) {
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
		// Use OP_RETURN to return that value.
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

// CompileVerbBytecode compiles verb source (code lines) to bytecode, backed by a
// content-addressed cache.
//
// In the relocated topology the bytecode package owns the cache and the parser
// bridge. The caller passes only persistent state (the source lines) plus the
// registry; bytecode no longer reaches into a db/store.Verb struct, so it does
// NOT import db/store.
//
// The cache is keyed by a hash of the RAW stored source (the code lines). On a
// hit the cached *Program is returned directly (a cheap map lookup, no parse, no
// compile). On a miss the source is parsed + compiled, the result is stored, and
// returned. Correctness is automatic: changed source hashes to a new key and
// recompiles; eviction (LRU) is memory-management only and at worst forces a
// recompile. The cached *Program is immutable (all per-execution state lives on
// the VM StackFrame), so sharing it across executions is safe.
//
// Returns the compiled *Program or an error.
func CompileVerbBytecode(code []string, registry Registry) (*Program, error) {
	key := hashCode(code)
	if prog, ok := verbProgramCache.get(key); ok {
		return prog, nil
	}

	vp, errs := CompileVerb(code)
	if errs != nil {
		// errs[0] is already Toast-formatted ("Line N:  syntax error").
		return nil, fmt.Errorf("%s", errs[0])
	}

	c := NewCompilerWithRegistry(registry)
	prog, err := c.CompileProgram(&verb.Program{Statements: vp.Statements})
	if err != nil {
		return nil, fmt.Errorf("bytecode compile error: %w", err)
	}
	if len(code) > 0 {
		prog.Source = append([]string(nil), code...)
	}

	verbProgramCache.put(key, prog)
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

// emitConstant adds a constant and emits OP_PUSH.
// If the constant pool overflows, c.err is set by addConstant.
func (c *Compiler) emitConstant(v types.Value) {
	idx := c.addConstant(v)
	c.emit(OP_PUSH)
	c.emitByte(byte(idx))
}

// addConstant adds a value to the constant pool (with deduplication).
// If the constant pool exceeds 255 entries, sets c.err and returns 0
// as a safe fallback index.
func (c *Compiler) addConstant(v types.Value) int {
	// Check if constant already exists
	key := v.String()
	if idx, ok := c.constants[key]; ok {
		return idx
	}

	// Check overflow before adding
	idx := len(c.program.Constants)
	if idx > 255 {
		if c.err == nil {
			c.err = fmt.Errorf("too many constants (max 255)")
		}
		return 0 // safe fallback; c.err will be checked at Compile boundary
	}

	// Add new constant
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

// patchJump patches a jump instruction to jump to current location.
// If the jump offset exceeds 0xFFFF, sets c.err instead of panicking.
func (c *Compiler) patchJump(offset int) {
	jump := len(c.program.Code) - offset - 2
	if jump > 0xFFFF {
		if c.err == nil {
			c.err = fmt.Errorf("jump offset too large (max 65535, got %d)", jump)
		}
		return
	}
	c.program.Code[offset] = byte(jump >> 8)
	c.program.Code[offset+1] = byte(jump)
}

// currentOffset returns the current bytecode offset
func (c *Compiler) currentOffset() int {
	return len(c.program.Code)
}

// trackLine records a line number entry if the semantic node's line differs
// from the last recorded line. This populates Program.LineInfo so that runtime
// errors can include source line numbers.
func (c *Compiler) trackLine(node verb.Node) {
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

// declareVariable declares a variable in current scope.
// If the variable count exceeds 255 (the maximum addressable by a single byte),
// sets c.err and returns 0 as a safe fallback index.
func (c *Compiler) declareVariable(name string) int {
	// Check if already exists in global variable table
	if idx, ok := c.variables[name]; ok {
		return idx
	}

	// Check overflow before adding
	idx := len(c.program.VarNames)
	if idx > 255 {
		if c.err == nil {
			c.err = fmt.Errorf("too many local variables (max 255)")
		}
		return 0 // safe fallback; c.err will be checked at Compile boundary
	}

	// Add to global variable table
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
func (c *Compiler) beginLoop(label string, resultVar int, valueName, indexName string) {
	c.loops = append(c.loops, LoopContext{
		Label:         label,
		ValueName:     valueName,
		IndexName:     indexName,
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

// findLoopByTarget finds a loop by explicit label or by loop variable/index name.
func (c *Compiler) findLoopByTarget(name string) *LoopContext {
	if name == "" {
		return c.currentLoop()
	}

	for i := len(c.loops) - 1; i >= 0; i-- {
		loop := &c.loops[i]
		if loop.Label == name || loop.ValueName == name || loop.IndexName == name {
			return loop
		}
	}
	return nil
}

// compileNode dispatches compilation based on node type
func (c *Compiler) compileNode(node verb.Node) error {
	// Bail out early if an overflow error has been recorded
	if c.err != nil {
		return c.err
	}

	// Guard against nil nodes (e.g. an empty expression statement).
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
	case *verb.ExprStmt:
		return c.compileExprStmt(n)
	case *verb.IfStmt:
		return c.compileIf(n)
	case *verb.WhileStmt:
		return c.compileWhile(n)
	case *verb.ForStmt:
		return c.compileFor(n)
	case *verb.BreakStmt:
		return c.compileBreak(n)
	case *verb.ContinueStmt:
		return c.compileContinue(n)
	case *verb.ReturnStmt:
		return c.compileReturn(n)
	case *verb.TryStmt:
		return c.compileTry(n)
	case *verb.ScatterStmt:
		return c.compileScatter(n)
	case *verb.ForkStmt:
		return c.compileFork(n)

	default:
		return fmt.Errorf("unknown node type: %T", node)
	}
}

// compileLiteral compiles a literal value
func (c *Compiler) compileLiteral(n *verb.LiteralExpr) error {
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

// emitIntLiteral emits bytecode for an integer literal without consuming
// constant-pool slots for large integers.
func (c *Compiler) emitIntLiteral(v int64) {
	if op, ok := MakeImmediateOpcode(int(v)); ok {
		c.emit(op)
		return
	}

	// Avoid overflow when negating MinInt64.
	if v == math.MinInt64 {
		c.emitConstant(types.NewInt(v))
		return
	}
	if v < 0 {
		c.emitIntLiteral(-v)
		c.emit(OP_NEG)
		return
	}

	// Build positive integers from decimal digits:
	// n = (((d0 * 10) + d1) * 10 + d2) ...
	digits := strconv.FormatInt(v, 10)
	c.emitIntLiteral(int64(digits[0] - '0'))
	for i := 1; i < len(digits); i++ {
		c.emitIntLiteral(10)
		c.emit(OP_MUL)

		d := int64(digits[i] - '0')
		if d != 0 {
			c.emitIntLiteral(d)
			c.emit(OP_ADD)
		}
	}
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
func (c *Compiler) compileIdentifier(n *verb.IdentifierExpr) error {
	// User variables take precedence over built-in type constants so code can
	// intentionally use names like NUM/INT as loop counters or temporaries.
	if idx, ok := c.resolveVariable(n.Name); ok {
		c.emit(OP_GET_VAR)
		c.emitByte(byte(idx))
		return nil
	}

	// Check for built-in type constants (OBJ, STR, INT, etc.)
	if val, ok := builtinConstants[n.Name]; ok {
		c.emitConstant(val)
		return nil
	}

	// Variable not found - this will be a runtime error (E_VARNF)
	// For now, declare it (MOO has dynamic scoping)
	idx := c.declareVariable(n.Name)

	c.emit(OP_GET_VAR)
	c.emitByte(byte(idx))
	return nil
}

// compileUnary compiles a unary expression
func (c *Compiler) compileUnary(n *verb.UnaryExpr) error {
	// Compile operand
	if err := c.compileNode(n.Operand); err != nil {
		return err
	}

	// Emit operator
	switch n.Operator {
	case verb.UnaryNegate:
		c.emit(OP_NEG)
	case verb.UnaryNot:
		c.emit(OP_NOT)
	case verb.UnaryBitwiseNot:
		c.emit(OP_BITNOT)
	default:
		return fmt.Errorf("unknown unary operator: %v", n.Operator)
	}

	return nil
}

// compileBinary compiles a binary expression
func (c *Compiler) compileBinary(n *verb.BinaryExpr) error {
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
		c.emit(OP_ADD)
	case verb.BinarySubtract:
		c.emit(OP_SUB)
	case verb.BinaryMultiply:
		c.emit(OP_MUL)
	case verb.BinaryDivide:
		c.emit(OP_DIV)
	case verb.BinaryModulo:
		c.emit(OP_MOD)
	case verb.BinaryPower:
		c.emit(OP_POW)
	case verb.BinaryEqual:
		c.emit(OP_EQ)
	case verb.BinaryNotEqual:
		c.emit(OP_NE)
	case verb.BinaryLess:
		c.emit(OP_LT)
	case verb.BinaryLessEqual:
		c.emit(OP_LE)
	case verb.BinaryGreater:
		c.emit(OP_GT)
	case verb.BinaryGreaterEqual:
		c.emit(OP_GE)
	case verb.BinaryIn:
		c.emit(OP_IN)
	case verb.BinaryBitAnd:
		c.emit(OP_BITAND)
	case verb.BinaryBitOr:
		c.emit(OP_BITOR)
	case verb.BinaryBitXor:
		c.emit(OP_BITXOR)
	case verb.BinaryShiftLeft:
		c.emit(OP_SHL)
	case verb.BinaryShiftRight:
		c.emit(OP_SHR)
	default:
		return fmt.Errorf("unknown binary operator: %v", n.Operator)
	}

	return nil
}

// compileShortCircuitAnd compiles && with short-circuit evaluation
func (c *Compiler) compileShortCircuitAnd(n *verb.BinaryExpr) error {
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
func (c *Compiler) compileShortCircuitOr(n *verb.BinaryExpr) error {
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
func (c *Compiler) compileTernary(n *verb.TernaryExpr) error {
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
func (c *Compiler) compileAssign(n *verb.AssignExpr) error {
	// Compile value
	if err := c.compileNode(n.Value); err != nil {
		return err
	}

	// Duplicate value (assignment returns the value)
	c.emit(OP_DUP)

	// Handle different target types
	switch target := n.Target.(type) {
	case *verb.IdentifierExpr:
		// Simple variable assignment
		idx := c.declareVariable(target.Name)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(idx))
	case *verb.ListExpr:
		// Scatter-style assignment expression:
		//   {a, b, c} = value
		// Assignment expression result remains on stack (the RHS value).
		names := make([]string, len(target.Elements))
		for i, elem := range target.Elements {
			id, ok := elem.(*verb.IdentifierExpr)
			if !ok {
				return fmt.Errorf("invalid assignment target: %T", target)
			}
			names[i] = id.Name
		}

		listVar := c.declareVariable(c.tempVar("scatter_assign_list"))
		// Stack: [value, value_copy] -> store value_copy.
		c.emit(OP_SET_VAR)
		c.emitByte(byte(listVar))
		// Stack: [value]

		// Validate shape exactly matches required count.
		c.emit(OP_GET_VAR)
		c.emitByte(byte(listVar))
		c.emit(OP_SCATTER)
		c.emitByte(byte(len(names))) // required
		c.emitByte(0)                // optional
		c.emitByte(0)                // hasRest

		for i, name := range names {
			c.emit(OP_GET_VAR)
			c.emitByte(byte(listVar))
			c.emitConstant(types.NewInt(int64(i + 1)))
			c.emit(OP_INDEX)
			idx := c.declareVariable(name)
			c.emit(OP_SET_VAR)
			c.emitByte(byte(idx))
		}
	case *verb.IndexExpr:
		// Index assignment: coll[idx] = value  OR  nested: coll[i][j]... = value
		// Walk the IndexExpr chain to find the base variable and collect indices
		var indices []verb.Expr
		var baseExpr verb.Expr = target
		for {
			ie, ok := baseExpr.(*verb.IndexExpr)
			if !ok {
				break
			}
			indices = append(indices, ie.Index)
			baseExpr = ie.Expr
		}

		// Determine base type: variable or property
		var baseVarIdx int
		var basePropExpr *verb.PropertyExpr

		if baseIdent, ok := baseExpr.(*verb.IdentifierExpr); ok {
			// Variable-based: x[i] = val
			baseVarIdx = c.declareVariable(baseIdent.Name)
		} else if propExpr, ok := baseExpr.(*verb.PropertyExpr); ok {
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
			oldContextVar := c.indexContextVar
			oldMarkerMode := c.indexBoundaryMode
			if containsIndexBoundary(indices[0]) {
				tempIdx := c.declareVariable(c.tempVar("idxsetctx"))
				c.emit(OP_GET_VAR)
				c.emitByte(byte(baseVarIdx))
				c.emit(OP_SET_VAR)
				c.emitByte(byte(tempIdx))
				c.indexContextVar = tempIdx
				c.indexBoundaryMode = indexBoundaryModeCollection
			}
			if err := c.compileNode(indices[0]); err != nil {
				return err
			}
			c.indexContextVar = oldContextVar
			c.indexBoundaryMode = oldMarkerMode
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
				oldContextVar := c.indexContextVar
				oldMarkerMode := c.indexBoundaryMode
				if containsIndexBoundary(indices[k]) {
					tempIdx := c.declareVariable(c.tempVar("nestedidxctx"))
					c.emit(OP_GET_VAR)
					c.emitByte(byte(baseVarIdx))
					c.emit(OP_SET_VAR)
					c.emitByte(byte(tempIdx))
					c.indexContextVar = tempIdx
					c.indexBoundaryMode = indexBoundaryModeCollection
				}
				if err := c.compileNode(indices[k]); err != nil {
					return err
				}
				c.indexContextVar = oldContextVar
				c.indexBoundaryMode = oldMarkerMode
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
	case *verb.PropertyExpr:
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
	case *verb.RangeExpr:
		// Range assignment: coll[start..end] = value
		if nestedIndex, ok := target.Expr.(*verb.IndexExpr); ok {
			return c.compileNestedRangeAssign(nestedIndex, target.Start, target.End)
		}

		var varIdx int
		var basePropExpr *verb.PropertyExpr

		if baseIdent, ok := target.Expr.(*verb.IdentifierExpr); ok {
			// Variable-based: x[2..3] = val
			varIdx = c.declareVariable(baseIdent.Name)
		} else if propExpr, ok := target.Expr.(*verb.PropertyExpr); ok {
			// Property-based: obj.prop[2..3] = val
			// Read the property into a temp, do range-set on temp, write back.
			basePropExpr = propExpr

			// Stack currently: [value, value_copy]
			// Store value_copy into temp so we can use the stack for GET_PROP
			tmpValHold := c.declareVariable("__prop_range_val")
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
			varIdx = c.declareVariable("__prop_range_base")
			c.emit(OP_SET_VAR)
			c.emitByte(byte(varIdx))
			// Stack: [value]

			// Restore the value_copy onto the stack for the range assignment code below
			c.emit(OP_GET_VAR)
			c.emitByte(byte(tmpValHold))
			// Stack: [value, value_copy]
		} else {
			return fmt.Errorf("range assignment target must be a variable or property")
		}

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

		// If the base was a property, write the modified temp back to the property
		if basePropExpr != nil {
			// Stack: [value]
			// Load the modified base temp (now has the updated collection)
			c.emit(OP_GET_VAR)
			c.emitByte(byte(varIdx))
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
	default:
		return fmt.Errorf("invalid assignment target: %T", target)
	}

	return nil
}

// compileRangeIndex compiles a range index expression with numeric boundary semantics.
// In range context, ^ means 1 and $ means collection length.
func (c *Compiler) compileRangeIndex(expr verb.Expr, varIdx int) error {
	if !containsIndexBoundary(expr) {
		return c.compileNode(expr)
	}

	oldContextVar := c.indexContextVar
	oldMarkerMode := c.indexBoundaryMode

	tempIdx := c.declareVariable(c.tempVar("rngsetctx"))
	c.emit(OP_GET_VAR)
	c.emitByte(byte(varIdx))
	c.emit(OP_LENGTH)
	c.emit(OP_SET_VAR)
	c.emitByte(byte(tempIdx))

	c.indexContextVar = tempIdx
	c.indexBoundaryMode = indexBoundaryModeLength

	err := c.compileNode(expr)

	c.indexContextVar = oldContextVar
	c.indexBoundaryMode = oldMarkerMode

	return err
}

// compileNestedRangeAssign compiles one-level nested range assignment:
//
//	outer[idx][start..end] = value
//
// by desugaring through temporary variables and existing INDEX/RANGE_SET opcodes.
func (c *Compiler) compileNestedRangeAssign(indexExpr *verb.IndexExpr, start, end verb.Expr) error {
	// For now, support one nested index level (x[i][a..b]); deeper forms can be added later.
	if _, deeper := indexExpr.Expr.(*verb.IndexExpr); deeper {
		return fmt.Errorf("range assignment target nesting depth > 1 is not supported")
	}

	var baseVarIdx int
	var basePropExpr *verb.PropertyExpr

	// If the base is a property, load it into a temp base variable.
	if baseIdent, ok := indexExpr.Expr.(*verb.IdentifierExpr); ok {
		baseVarIdx = c.declareVariable(baseIdent.Name)
	} else if propExpr, ok := indexExpr.Expr.(*verb.PropertyExpr); ok {
		basePropExpr = propExpr

		// Stack currently: [value, value_copy]
		tmpValHold := c.declareVariable("__prop_nested_range_val")
		c.emit(OP_SET_VAR)
		c.emitByte(byte(tmpValHold))
		// Stack: [value]

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

		baseVarIdx = c.declareVariable("__prop_nested_range_base")
		c.emit(OP_SET_VAR)
		c.emitByte(byte(baseVarIdx))
		// Stack: [value]

		c.emit(OP_GET_VAR)
		c.emitByte(byte(tmpValHold))
		// Stack: [value, value_copy]
	} else {
		return fmt.Errorf("range assignment target must be a variable or property")
	}

	// Preserve value_copy for RANGE_SET while keeping original assigned value on stack.
	tmpAssignedVal := c.declareVariable("__nested_range_assigned")
	c.emit(OP_SET_VAR)
	c.emitByte(byte(tmpAssignedVal))
	// Stack: [value]

	// Resolve outer index once and store it.
	oldContextVar := c.indexContextVar
	oldMarkerMode := c.indexBoundaryMode
	if containsIndexBoundary(indexExpr.Index) {
		tempIdx := c.declareVariable(c.tempVar("nestedrangectx"))
		c.emit(OP_GET_VAR)
		c.emitByte(byte(baseVarIdx))
		c.emit(OP_SET_VAR)
		c.emitByte(byte(tempIdx))
		c.indexContextVar = tempIdx
		c.indexBoundaryMode = indexBoundaryModeCollection
	}
	if err := c.compileNode(indexExpr.Index); err != nil {
		return err
	}
	c.indexContextVar = oldContextVar
	c.indexBoundaryMode = oldMarkerMode

	tmpOuterIndex := c.declareVariable("__nested_range_index")
	c.emit(OP_SET_VAR)
	c.emitByte(byte(tmpOuterIndex))
	// Stack: [value]

	// Load outer[idx] into a temp inner collection.
	c.emit(OP_GET_VAR)
	c.emitByte(byte(baseVarIdx))
	c.emit(OP_GET_VAR)
	c.emitByte(byte(tmpOuterIndex))
	c.emit(OP_INDEX)
	tmpInnerVar := c.declareVariable("__nested_range_inner")
	c.emit(OP_SET_VAR)
	c.emitByte(byte(tmpInnerVar))
	// Stack: [value]

	// Perform inner range assignment.
	c.emit(OP_GET_VAR)
	c.emitByte(byte(tmpAssignedVal))
	if err := c.compileRangeIndex(start, tmpInnerVar); err != nil {
		return err
	}
	if err := c.compileRangeIndex(end, tmpInnerVar); err != nil {
		return err
	}
	c.emit(OP_RANGE_SET)
	c.emitByte(byte(tmpInnerVar))
	// Stack: [value]

	// Write modified inner collection back to outer[idx].
	c.emit(OP_GET_VAR)
	c.emitByte(byte(tmpInnerVar))
	c.emit(OP_GET_VAR)
	c.emitByte(byte(tmpOuterIndex))
	c.emit(OP_INDEX_SET)
	c.emitByte(byte(baseVarIdx))
	// Stack: [value]

	// If base was a property, persist modified base temp back onto the object property.
	if basePropExpr != nil {
		c.emit(OP_GET_VAR)
		c.emitByte(byte(baseVarIdx))
		if err := c.compileNode(basePropExpr.Expr); err != nil {
			return err
		}
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
	}

	return nil
}

// Stub implementations for other compile methods
// These will be completed based on the actual requirements

func (c *Compiler) compileBuiltinCall(n *verb.BuiltinCallExpr) error {
	if c.registry == nil {
		return fmt.Errorf("builtin call compilation requires a builtins registry")
	}

	// Special-case pass(): emit OP_PASS instead of OP_CALL_BUILTIN.
	// OP_PASS is handled natively by the VM — looks up the parent verb,
	// compiles it to bytecode, and pushes a new frame.
	if n.Name == "pass" {
		hasSplice := hasSpliceArgs(n.Args)
		if !hasSplice && len(n.Args) > 254 {
			return fmt.Errorf("too many arguments (max 254)")
		}

		if hasSplice {
			// Build a single flattened arg list on-stack; OP_PASS 0xFF consumes it.
			c.emit(OP_MAKE_LIST)
			c.emitByte(0)
			for _, arg := range n.Args {
				if splice, ok := arg.(*verb.SpliceExpr); ok {
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
			c.emit(OP_PASS)
			c.emitByte(0xFF)
			return nil
		}

		// Compile fixed arguments directly onto the stack.
		for _, arg := range n.Args {
			if err := c.compileNode(arg); err != nil {
				return err
			}
		}
		c.emit(OP_PASS)
		c.emitByte(byte(len(n.Args)))
		return nil
	}

	// Resolve function name to numeric ID at compile time
	funcID, ok := c.registry.GetID(n.Name)
	if !ok {
		return &UnknownBuiltinError{Name: n.Name, Line: n.Pos.Line}
	}

	// Check builtin function ID overflow (emitted as single byte)
	if funcID > 255 {
		return fmt.Errorf("too many builtin functions (id %d exceeds max 255)", funcID)
	}

	// Check if any argument is a splice expression
	hasSplice := hasSpliceArgs(n.Args)

	// Check argument count overflow (emitted as single byte, 0xFF reserved for splice)
	if !hasSplice && len(n.Args) > 254 {
		return fmt.Errorf("too many arguments (max 254)")
	}

	if hasSplice {
		// Splice path: build args list incrementally using OP_LIST_APPEND/EXTEND
		c.emit(OP_MAKE_LIST)
		c.emitByte(0)
		for _, arg := range n.Args {
			if splice, ok := arg.(*verb.SpliceExpr); ok {
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

func (c *Compiler) compileIndex(n *verb.IndexExpr) error {
	// Compile collection
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// If the index contains ^ or $, set up an index context variable with the collection.
	// Stack: [coll] -> DUP -> [coll, coll] -> SET_VAR -> [coll]
	hasIndexBoundary := containsIndexBoundary(n.Index)
	oldContextVar := c.indexContextVar
	oldMarkerMode := c.indexBoundaryMode
	if hasIndexBoundary {
		tempIdx := c.declareVariable(c.tempVar("idxctx"))
		c.emit(OP_DUP)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(tempIdx))
		c.indexContextVar = tempIdx
		c.indexBoundaryMode = indexBoundaryModeCollection
	}

	// Compile index
	if err := c.compileNode(n.Index); err != nil {
		return err
	}

	// Restore previous context
	c.indexContextVar = oldContextVar
	c.indexBoundaryMode = oldMarkerMode

	// Emit index operation
	c.emit(OP_INDEX)
	return nil
}

func (c *Compiler) compileRange(n *verb.RangeExpr) error {
	// Compile collection
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// If start or end contains ^ or $, set up an index context variable with collection length.
	// Stack: [coll] -> DUP -> [coll, coll] -> LENGTH -> [coll, len] -> SET_VAR -> [coll]
	hasIndexBoundary := containsIndexBoundary(n.Start) || containsIndexBoundary(n.End)
	oldContextVar := c.indexContextVar
	oldMarkerMode := c.indexBoundaryMode
	if hasIndexBoundary {
		tempIdx := c.declareVariable(c.tempVar("rngctx"))
		c.emit(OP_DUP)
		c.emit(OP_LENGTH)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(tempIdx))
		c.indexContextVar = tempIdx
		c.indexBoundaryMode = indexBoundaryModeLength
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
	c.indexBoundaryMode = oldMarkerMode

	// Emit range operation
	c.emit(OP_RANGE)
	return nil
}

func (c *Compiler) compileIndexBoundary(n *verb.IndexBoundaryExpr) error {
	if c.indexContextVar >= 0 && c.indexBoundaryMode == indexBoundaryModeCollection {
		c.emit(OP_GET_VAR)
		c.emitByte(byte(c.indexContextVar))
		c.emit(OP_INDEX_MARKER)
		if n.Boundary == verb.IndexFirst {
			c.emitByte(0)
		} else {
			c.emitByte(1)
		}
		return nil
	}

	if n.Boundary == verb.IndexFirst {
		c.emitConstant(types.NewInt(1))
		return nil
	}

	// $ resolves to collection length in length mode.
	if c.indexContextVar >= 0 {
		c.emit(OP_GET_VAR)
		c.emitByte(byte(c.indexContextVar))
	} else {
		// No index context (shouldn't happen for well-formed index/range expressions).
		// Fall back to literal -1 (will produce E_RANGE at runtime).
		c.emitConstant(types.NewInt(-1))
	}

	return nil
}

func (c *Compiler) compileProperty(n *verb.PropertyExpr) error {
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

func (c *Compiler) compileVerbCall(n *verb.VerbCallExpr) error {
	// Compile the object expression (pushes object onto stack)
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Check if any argument is a splice expression
	hasSplice := hasSpliceArgs(n.Args)

	// Check argument count overflow (emitted as single byte, 0xFF reserved for splice)
	if !hasSplice && len(n.Args) > 254 {
		return fmt.Errorf("too many verb arguments (max 254)")
	}

	if hasSplice {
		// Splice path: build args list incrementally using OP_LIST_APPEND/EXTEND
		c.emit(OP_MAKE_LIST)
		c.emitByte(0)
		for _, arg := range n.Args {
			if splice, ok := arg.(*verb.SpliceExpr); ok {
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

func (c *Compiler) compileSplice(n *verb.SpliceExpr) error {
	// Compile the expression to splice
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Emit splice operation
	c.emit(OP_SPLICE)
	return nil
}

func (c *Compiler) compileCatch(n *verb.CatchExpr) error {
	// Catch expressions (`expr ! codes => default`) are compiled as a
	// single-clause try/except that leaves the result on the stack.
	//
	// With default:
	//   OP_TRY_EXCEPT 1 [codes...] [0 = no var] [handler_ip:short]
	//   [expr]
	//   OP_END_EXCEPT 1
	//   OP_JUMP [end]
	//   handler_ip: [default expr]
	//   end:
	//
	// Without default (return the error value):
	//   OP_TRY_EXCEPT 1 [codes...] [var+1] [handler_ip:short]
	//   [expr]
	//   OP_END_EXCEPT 1
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
	c.emitByte(1) // 1 clause, matching the OP_TRY_EXCEPT above

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
		// No default: return the captured error code (first element of exception list).
		c.emit(OP_GET_VAR)
		c.emitByte(byte(errVarIdx))
		if op, ok := MakeImmediateOpcode(1); ok {
			c.emit(op)
		}
		c.emit(OP_INDEX)
	}

	// Patch end jump
	c.patchJump(endJump)

	return nil
}

func (c *Compiler) compileExprStmt(n *verb.ExprStmt) error {
	// Guard against nil expression (e.g. bare semicolons)
	if n.Expr == nil {
		return nil
	}

	// Effect-context fast path: a simple variable assignment used as a statement
	// does not need its value. Emit the store directly, skipping the assignment's
	// value-preserving OP_DUP and the trailing OP_POP (dead value shuffling that
	// otherwise dominates assignment-heavy loops). Complex targets (scatter /
	// index / property) keep the general value-producing path below.
	if assign, ok := n.Expr.(*verb.AssignExpr); ok {
		if ident, ok := assign.Target.(*verb.IdentifierExpr); ok {
			// Self-concat idiom: s = s + expr. This statement does not need the
			// assignment's result value, so emit a string-append opcode directly.
			// Runtime type checks preserve normal `+` errors if either operand is
			// not a string.
			if bin, ok := assign.Value.(*verb.BinaryExpr); ok && bin.Operator == verb.BinaryAdd {
				if leftIdent, ok := bin.Left.(*verb.IdentifierExpr); ok && leftIdent.Name == ident.Name {
					idx := c.declareVariable(ident.Name)
					c.emit(OP_GET_VAR)
					c.emitByte(byte(idx))
					if err := c.compileNode(bin.Right); err != nil {
						return err
					}
					c.emit(OP_STRING_APPEND)
					c.emit(OP_SET_VAR)
					c.emitByte(byte(idx))
					return nil
				}
			}

			// Self-append idiom: v = {@v, e1, ...}. Instead of building a fresh
			// list (copy all of v via LIST_EXTEND) and reassigning, append the
			// trailing elements directly onto v. With the in-place Append path
			// this turns an O(n^2) build loop into amortized O(n); the result is
			// identical (v's elements followed by the trailing items).
			if list, ok := assign.Value.(*verb.ListExpr); ok && len(list.Elements) > 0 {
				if sp, ok := list.Elements[0].(*verb.SpliceExpr); ok {
					if spIdent, ok := sp.Expr.(*verb.IdentifierExpr); ok && spIdent.Name == ident.Name {
						idx := c.declareVariable(ident.Name)
						c.emit(OP_GET_VAR)
						c.emitByte(byte(idx))
						for _, elem := range list.Elements[1:] {
							if splice, ok := elem.(*verb.SpliceExpr); ok {
								if err := c.compileNode(splice.Expr); err != nil {
									return err
								}
								c.emit(OP_LIST_EXTEND)
							} else {
								if err := c.compileNode(elem); err != nil {
									return err
								}
								c.emit(OP_LIST_APPEND)
							}
						}
						c.emit(OP_SET_VAR)
						c.emitByte(byte(idx))
						return nil
					}
				}
			}

			if err := c.compileNode(assign.Value); err != nil {
				return err
			}
			idx := c.declareVariable(ident.Name)
			c.emit(OP_SET_VAR)
			c.emitByte(byte(idx))
			return nil
		}
	}

	// Compile expression
	if err := c.compileNode(n.Expr); err != nil {
		return err
	}

	// Pop result (expression statement doesn't use result)
	c.emit(OP_POP)
	return nil
}

func (c *Compiler) compileIf(n *verb.IfStmt) error {
	// Compile condition
	if err := c.compileNode(n.Condition); err != nil {
		return err
	}

	// Jump to the semantic else branch if false.
	elseJump := c.emitJump(OP_JUMP_IF_FALSE)

	// Compile then branch
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	if len(n.Else) == 0 {
		c.patchJump(elseJump)
		return nil
	}

	endJump := c.emitJump(OP_JUMP)
	c.patchJump(elseJump)
	if err := c.compileBlock(n.Else); err != nil {
		return err
	}
	c.patchJump(endJump)

	return nil
}

func (c *Compiler) compileWhile(n *verb.WhileStmt) error {
	// Declare temp variable for loop result (break expr value or default 0)
	resultVar := c.declareVariable(c.tempVar("loop_result"))
	// Initialize to 0 (default loop result when no break expr)
	if op, ok := MakeImmediateOpcode(0); ok {
		c.emit(op)
	}
	c.emit(OP_SET_VAR)
	c.emitByte(byte(resultVar))

	// Start loop
	c.beginLoop(n.Label, resultVar, "", "")
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

func (c *Compiler) compileFor(n *verb.ForStmt) error {
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

// hasSpliceArgs checks if any argument in a list is a splice expression.
func hasSpliceArgs(args []verb.Expr) bool {
	for _, arg := range args {
		if _, ok := arg.(*verb.SpliceExpr); ok {
			return true
		}
	}
	return false
}

// containsIndexBoundary reports whether expr contains a ^/$ index boundary bound
// to the *current* indexing context — i.e. one not already shadowed by a
// nested index/range expression's own brackets (which establish their own
// context for any ^/$ inside them). It must recurse into every expression
// kind that can hold a child expression so a boundary nested arbitrarily deep
// (e.g. inside a function call argument, list/map literal element, or
// assignment value) is still detected; under-detection silently falls back
// to an unbound boundary (compiles to a literal -1), not a compile error.
// Over-detection is harmless: nested index/range expressions always push
// and restore their own context around their own Index/Start/End fields.
func containsIndexBoundary(expr verb.Expr) bool {
	switch n := expr.(type) {
	case nil:
		return false
	case *verb.IndexBoundaryExpr:
		return n.Boundary == verb.IndexLast || n.Boundary == verb.IndexFirst
	case *verb.BinaryExpr:
		return containsIndexBoundary(n.Left) || containsIndexBoundary(n.Right)
	case *verb.UnaryExpr:
		return containsIndexBoundary(n.Operand)
	case *verb.TernaryExpr:
		return containsIndexBoundary(n.Condition) || containsIndexBoundary(n.ThenExpr) || containsIndexBoundary(n.ElseExpr)
	case *verb.IndexExpr:
		// n.Index is scoped to this IndexExpr's own brackets; only n.Expr
		// (the collection being indexed) is in the enclosing context.
		return containsIndexBoundary(n.Expr)
	case *verb.RangeExpr:
		// n.Start/n.End are scoped to this RangeExpr's own brackets.
		return containsIndexBoundary(n.Expr)
	case *verb.PropertyExpr:
		return containsIndexBoundary(n.Expr) || containsIndexBoundary(n.PropertyExpr)
	case *verb.VerbCallExpr:
		if containsIndexBoundary(n.Expr) || containsIndexBoundary(n.VerbExpr) {
			return true
		}
		for _, arg := range n.Args {
			if containsIndexBoundary(arg) {
				return true
			}
		}
		return false
	case *verb.BuiltinCallExpr:
		for _, arg := range n.Args {
			if containsIndexBoundary(arg) {
				return true
			}
		}
		return false
	case *verb.SpliceExpr:
		return containsIndexBoundary(n.Expr)
	case *verb.CatchExpr:
		return containsIndexBoundary(n.Expr) || containsIndexBoundary(n.Default)
	case *verb.AssignExpr:
		return containsIndexBoundary(n.Target) || containsIndexBoundary(n.Value)
	case *verb.ListExpr:
		for _, el := range n.Elements {
			if containsIndexBoundary(el) {
				return true
			}
		}
		return false
	case *verb.ListRangeExpr:
		return containsIndexBoundary(n.Start) || containsIndexBoundary(n.End)
	case *verb.MapExpr:
		for _, pair := range n.Pairs {
			if containsIndexBoundary(pair.Key) || containsIndexBoundary(pair.Value) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// compileForRange compiles: for x in [start..end] ... endfor
// Compiles to equivalent while loop pattern.
func (c *Compiler) compileForRange(n *verb.ForStmt) error {
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
	c.beginLoop(n.Label, resultVar, n.Value, "")
	loopStart := c.currentOffset()

	// Condition: if value > end, jump to exit. Fused FOR_RANGE_CHECK replaces the
	// GET_VAR/GET_VAR/LE/JUMP_IF_FALSE sequence (same compare semantics, one dispatch).
	c.emit(OP_FOR_RANGE_CHECK)
	c.emitByte(byte(valueVar))
	c.emitByte(byte(endVar))
	exitJump := c.currentOffset()
	c.emitShort(0xFFFF) // exit offset placeholder, patched below

	// Body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// Patch continue jumps to point here (the increment section)
	// continue in a for-range should increment before re-checking condition
	for _, offset := range c.currentLoop().ContinueJumps {
		c.patchJump(offset)
	}

	// Increment + loop back: value += 1; jump to condition. Fused FOR_RANGE_NEXT
	// replaces GET_VAR/IMM/ADD/SET_VAR/LOOP (same +1 semantics, counts one tick).
	c.emit(OP_FOR_RANGE_NEXT)
	c.emitByte(byte(valueVar))
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
func (c *Compiler) compileForList(n *verb.ForStmt) error {
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
	c.beginLoop(n.Label, resultVar, n.Value, n.Index)
	loopStart := c.currentOffset()

	// Condition: if idx > len, jump to exit. Fused FOR_RANGE_CHECK (idx and len are
	// ints) replaces GET_VAR/GET_VAR/LE/JUMP_IF_FALSE — same opcode used by range-for.
	c.emit(OP_FOR_RANGE_CHECK)
	c.emitByte(byte(idxVar))
	c.emitByte(byte(lenVar))
	exitJump := c.currentOffset()
	c.emitShort(0xFFFF) // exit offset placeholder, patched below

	// Load current element into the loop variable(s). Fused element-load replaces
	// GET_VAR(list)/GET_VAR(idx)/INDEX plus the value/index extraction: idx is
	// provably in [1..len] so the bounds-checked INDEX dispatch is unnecessary.
	if hasIndex {
		// elem is always a {value, key/index} pair (OP_ITER_PREP guarantees isPairs).
		c.emit(OP_FOR_LIST_LOAD_KV)
		c.emitByte(byte(listVar))
		c.emitByte(byte(idxVar))
		c.emitByte(byte(valueVar))
		c.emitByte(byte(indexVar))
	} else {
		// value = isPairs ? elem[1] : elem (isPairs resolved at runtime).
		c.emit(OP_FOR_LIST_LOAD)
		c.emitByte(byte(listVar))
		c.emitByte(byte(idxVar))
		c.emitByte(byte(valueVar))
		c.emitByte(byte(isPairsVar))
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

	// Increment + loop back: idx += 1; jump to condition. Fused FOR_RANGE_NEXT
	// replaces GET_VAR/IMM/ADD/SET_VAR/LOOP (same opcode used by range-for).
	c.emit(OP_FOR_RANGE_NEXT)
	c.emitByte(byte(idxVar))
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

func (c *Compiler) compileBreak(n *verb.BreakStmt) error {
	// Mirror compileContinue: an explicit loop name must resolve to an
	// enclosing loop, otherwise raise "Invalid loop name" (ToastStunt
	// parser.y:1205-1206, check_loop_name LOOP_BREAK).
	loop := c.findLoopByTarget(n.Label)
	if loop == nil && n.Label != "" {
		return fmt.Errorf("Invalid loop name")
	}
	if loop == nil {
		return fmt.Errorf("break outside of loop")
	}

	// Emit a forward jump past the loop end (will be patched by endLoop)
	patchOffset := c.emitJump(OP_JUMP)
	loop.BreakJumps = append(loop.BreakJumps, patchOffset)
	return nil
}

func (c *Compiler) compileContinue(n *verb.ContinueStmt) error {
	loop := c.findLoopByTarget(n.Label)
	if loop == nil && n.Label != "" {
		return fmt.Errorf("Invalid loop name")
	}
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

func (c *Compiler) compileReturn(n *verb.ReturnStmt) error {
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

func (c *Compiler) compileTry(n *verb.TryStmt) error {
	finallyIPPatch := -1
	if n.Finalizer != nil {
		c.emit(OP_TRY_FINALLY)
		finallyIPPatch = len(c.program.Code)
		c.emitShort(0xFFFF)
	}

	if len(n.Handlers) == 0 {
		if err := c.compileBlock(n.Body); err != nil {
			return err
		}
	} else {
		numHandlers := len(n.Handlers)
		c.emit(OP_TRY_EXCEPT)
		c.emitByte(byte(numHandlers))

		handlerOffsetPatches := make([]int, numHandlers)
		for i, handler := range n.Handlers {
			if handler.IsAny {
				c.emitByte(0)
			} else {
				codes, err := lowerErrorNames(handler.Codes)
				if err != nil {
					return err
				}
				c.emitByte(byte(len(codes)))
				for _, code := range codes {
					c.emitByte(byte(code))
				}
			}

			if handler.Variable != "" {
				idx := c.declareVariable(handler.Variable)
				c.emitByte(byte(idx + 1))
			} else {
				c.emitByte(0)
			}

			handlerOffsetPatches[i] = len(c.program.Code)
			c.emitShort(0xFFFF)
		}

		if err := c.compileBlock(n.Body); err != nil {
			return err
		}

		c.emit(OP_END_EXCEPT)
		c.emitByte(byte(numHandlers))
		endHandlersJump := c.emitJump(OP_JUMP)
		handlerEndJumps := make([]int, 0, numHandlers-1)

		for i, handler := range n.Handlers {
			handlerIP := c.currentOffset()
			c.program.Code[handlerOffsetPatches[i]] = byte(handlerIP >> 8)
			c.program.Code[handlerOffsetPatches[i]+1] = byte(handlerIP)

			if err := c.compileBlock(handler.Body); err != nil {
				return err
			}
			if i < numHandlers-1 {
				handlerEndJumps = append(handlerEndJumps, c.emitJump(OP_JUMP))
			}
		}

		c.patchJump(endHandlersJump)
		for _, jump := range handlerEndJumps {
			c.patchJump(jump)
		}
	}

	if n.Finalizer != nil {
		c.emit(OP_END_FINALLY)
		finallyIP := c.currentOffset()
		c.program.Code[finallyIPPatch] = byte(finallyIP >> 8)
		c.program.Code[finallyIPPatch+1] = byte(finallyIP)
		if err := c.compileBlock(n.Finalizer.Body); err != nil {
			return err
		}
		c.emit(OP_END_FINALLY)
	}

	return nil
}

func (c *Compiler) compileScatter(n *verb.ScatterStmt) error {
	// Scatter assignment: {a, ?b, @rest} = list
	//
	// Runtime strategy:
	// 1. Validate list shape via OP_SCATTER.
	// 2. Track two cursors (left/right) into the list.
	// 3. Bind suffix targets (after @rest) from the right.
	// 4. Bind prefix targets from the left.
	// 5. Bind @rest to the remaining slice between left..right.
	numRequired := 0
	numOptional := 0
	restIndex := -1
	for i, target := range n.Targets {
		if target.Rest {
			restIndex = i
			continue
		}
		if target.Optional {
			numOptional++
		} else {
			numRequired++
		}
	}
	hasRest := restIndex >= 0

	listVar := c.declareVariable(c.tempVar("scatter_list"))
	lenVar := c.declareVariable(c.tempVar("scatter_len"))
	leftVar := c.declareVariable(c.tempVar("scatter_left"))
	rightVar := c.declareVariable(c.tempVar("scatter_right"))

	if err := c.compileNode(n.Value); err != nil {
		return err
	}
	c.emit(OP_DUP)
	c.emit(OP_SET_VAR)
	c.emitByte(byte(listVar))

	c.emit(OP_SCATTER)
	c.emitByte(byte(numRequired))
	c.emitByte(byte(numOptional))
	if hasRest {
		c.emitByte(1)
	} else {
		c.emitByte(0)
	}

	// len = length(list)
	c.emit(OP_GET_VAR)
	c.emitByte(byte(listVar))
	c.emit(OP_LENGTH)
	c.emit(OP_SET_VAR)
	c.emitByte(byte(lenVar))

	// left = 1
	if op, ok := MakeImmediateOpcode(1); ok {
		c.emit(op)
	}
	c.emit(OP_SET_VAR)
	c.emitByte(byte(leftVar))

	// right = len
	c.emit(OP_GET_VAR)
	c.emitByte(byte(lenVar))
	c.emit(OP_SET_VAR)
	c.emitByte(byte(rightVar))

	// countRequired returns number of required non-rest targets in [start, end].
	countRequired := func(start, end int) int {
		count := 0
		for i := start; i <= end && i < len(n.Targets); i++ {
			if i < 0 {
				continue
			}
			target := n.Targets[i]
			if !target.Rest && !target.Optional {
				count++
			}
		}
		return count
	}

	emitAssignFrom := func(targetVar, indexVar int) {
		c.emit(OP_GET_VAR)
		c.emitByte(byte(listVar))
		c.emit(OP_GET_VAR)
		c.emitByte(byte(indexVar))
		c.emit(OP_INDEX)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(targetVar))
	}

	emitDec := func(varIdx int) {
		c.emit(OP_GET_VAR)
		c.emitByte(byte(varIdx))
		if op, ok := MakeImmediateOpcode(1); ok {
			c.emit(op)
		}
		c.emit(OP_SUB)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(varIdx))
	}

	emitInc := func(varIdx int) {
		c.emit(OP_GET_VAR)
		c.emitByte(byte(varIdx))
		if op, ok := MakeImmediateOpcode(1); ok {
			c.emit(op)
		}
		c.emit(OP_ADD)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(varIdx))
	}

	emitOptionalCondition := func(requiredReserve int) {
		// (right - left + 1) > requiredReserve
		c.emit(OP_GET_VAR)
		c.emitByte(byte(rightVar))
		c.emit(OP_GET_VAR)
		c.emitByte(byte(leftVar))
		c.emit(OP_SUB)
		if op, ok := MakeImmediateOpcode(1); ok {
			c.emit(op)
		}
		c.emit(OP_ADD)
		c.emitConstant(types.NewInt(int64(requiredReserve)))
		c.emit(OP_GT)
	}

	emitOptionalMissingValue := func(target verb.ScatterTarget, targetVar int) error {
		if target.Default != nil {
			if err := c.compileNode(target.Default); err != nil {
				return err
			}
			c.emit(OP_SET_VAR)
			c.emitByte(byte(targetVar))
		}
		// When no default is specified, leave the variable as-is.
		return nil
	}

	// Bind suffix targets from the right when @rest is present.
	if hasRest {
		for i := len(n.Targets) - 1; i > restIndex; i-- {
			target := n.Targets[i]
			if target.Rest {
				continue
			}
			targetVar := c.declareVariable(target.Name)
			if target.Optional {
				requiredBefore := countRequired(0, i-1)
				emitOptionalCondition(requiredBefore)
				elseJump := c.emitJump(OP_JUMP_IF_FALSE)

				emitAssignFrom(targetVar, rightVar)
				emitDec(rightVar)
				endJump := c.emitJump(OP_JUMP)

				c.patchJump(elseJump)
				if err := emitOptionalMissingValue(target, targetVar); err != nil {
					return err
				}
				c.patchJump(endJump)
			} else {
				emitAssignFrom(targetVar, rightVar)
				emitDec(rightVar)
			}
		}
	}

	// Bind prefix targets from the left.
	prefixEnd := len(n.Targets) - 1
	if hasRest {
		prefixEnd = restIndex - 1
	}
	for i := 0; i <= prefixEnd; i++ {
		target := n.Targets[i]
		if target.Rest {
			continue
		}

		targetVar := c.declareVariable(target.Name)
		if target.Optional {
			requiredAfter := countRequired(i+1, prefixEnd)
			emitOptionalCondition(requiredAfter)
			elseJump := c.emitJump(OP_JUMP_IF_FALSE)

			emitAssignFrom(targetVar, leftVar)
			emitInc(leftVar)
			endJump := c.emitJump(OP_JUMP)

			c.patchJump(elseJump)
			if err := emitOptionalMissingValue(target, targetVar); err != nil {
				return err
			}
			c.patchJump(endJump)
		} else {
			emitAssignFrom(targetVar, leftVar)
			emitInc(leftVar)
		}
	}

	// Bind @rest to the remaining middle slice.
	if hasRest {
		restTarget := n.Targets[restIndex]
		restVar := c.declareVariable(restTarget.Name)

		c.emit(OP_GET_VAR)
		c.emitByte(byte(leftVar))
		c.emit(OP_GET_VAR)
		c.emitByte(byte(rightVar))
		c.emit(OP_LE)
		elseJump := c.emitJump(OP_JUMP_IF_FALSE)

		c.emit(OP_GET_VAR)
		c.emitByte(byte(listVar))
		c.emit(OP_GET_VAR)
		c.emitByte(byte(leftVar))
		c.emit(OP_GET_VAR)
		c.emitByte(byte(rightVar))
		c.emit(OP_RANGE)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(restVar))
		endJump := c.emitJump(OP_JUMP)

		c.patchJump(elseJump)
		c.emit(OP_MAKE_LIST)
		c.emitByte(0)
		c.emit(OP_SET_VAR)
		c.emitByte(byte(restVar))
		c.patchJump(endJump)
	}

	return nil
}

func (c *Compiler) compileFork(n *verb.ForkStmt) error {
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
func isLoopStmt(stmt verb.Stmt) bool {
	switch stmt.(type) {
	case *verb.WhileStmt, *verb.ForStmt:
		return true
	default:
		return false
	}
}

func (c *Compiler) compileBlock(stmts []verb.Stmt) error {
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

// compileList compiles a list literal incrementally:
// start with {}, then append regular elements and extend splices.
func (c *Compiler) compileList(n *verb.ListExpr) error {
	// Start with an empty list on the stack.
	c.emit(OP_MAKE_LIST)
	c.emitByte(0)

	for _, elem := range n.Elements {
		if splice, ok := elem.(*verb.SpliceExpr); ok {
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
func (c *Compiler) compileListRange(n *verb.ListRangeExpr) error {
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
func (c *Compiler) compileMap(n *verb.MapExpr) error {
	// Build map incrementally in a temp local via OP_INDEX_SET.
	tmp := c.declareVariable(c.tempVar("maplit"))
	c.emit(OP_MAKE_MAP)
	c.emitByte(0)
	c.emit(OP_SET_VAR)
	c.emitByte(byte(tmp))

	for _, pair := range n.Pairs {
		// OP_INDEX_SET pops index first, then value.
		if err := c.compileNode(pair.Value); err != nil {
			return err
		}
		if err := c.compileNode(pair.Key); err != nil {
			return err
		}
		c.emit(OP_INDEX_SET)
		c.emitByte(byte(tmp))
	}

	c.emit(OP_GET_VAR)
	c.emitByte(byte(tmp))
	return nil
}
