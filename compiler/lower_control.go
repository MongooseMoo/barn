package compiler

import (
	"fmt"
	"math"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/ir"
)

// loopContext tracks loop compilation state.
type loopContext struct {
	Label         string
	ValueName     string
	IndexName     string
	BreakJumps    []int // Patch locations for break jumps (forward jumps past loop end)
	ContinueJumps []int // Patch locations for continue jumps (forward jumps to increment)
	ContinueIP    int   // Target IP for continue (0 = use ContinueJumps for forward patching)
	StartIP       int   // Loop condition start (for backward jump at end of body)
	HandlerDepth  int   // Handlers retained by a break/continue to this loop
}

// beginLoop starts a new loop context.
func (c *lowerer) beginLoop(label, valueName, indexName string) {
	c.loops = append(c.loops, loopContext{
		Label:         label,
		ValueName:     valueName,
		IndexName:     indexName,
		StartIP:       c.currentOffset(),
		HandlerDepth:  c.handlerDepth,
		ContinueIP:    0, // 0 = not set yet; will use ContinueJumps for forward patching
		BreakJumps:    make([]int, 0, 4),
		ContinueJumps: make([]int, 0, 4),
	})
}

// endLoop ends the current loop context and patches all break jumps to current location
func (c *lowerer) endLoop() {
	if len(c.loops) > 0 {
		loop := &c.loops[len(c.loops)-1]
		// Patch all break jumps to point to current location (after the loop)
		for _, offset := range loop.BreakJumps {
			c.patchWide(offset, c.currentOffset())
		}
		c.loops = c.loops[:len(c.loops)-1]
	}
}

// currentLoop returns the current loop context
func (c *lowerer) currentLoop() *loopContext {
	if len(c.loops) == 0 {
		return nil
	}
	return &c.loops[len(c.loops)-1]
}

// findLoop finds a loop by label (or innermost if label is empty)
// findLoopByTarget finds a loop by explicit label or by loop variable/index name.
func (c *lowerer) findLoopByTarget(name string) *loopContext {
	if name == "" {
		return c.currentLoop()
	}

	target := canonicalIdentifier(name)
	for i := len(c.loops) - 1; i >= 0; i-- {
		loop := &c.loops[i]
		if canonicalIdentifier(loop.Label) == target ||
			canonicalIdentifier(loop.ValueName) == target ||
			canonicalIdentifier(loop.IndexName) == target {
			return loop
		}
	}
	return nil
}

func (c *lowerer) compileIf(n *ir.IfStmt) error {
	// Compile condition
	if err := c.compileNode(n.Condition); err != nil {
		return err
	}

	// Jump to the semantic else branch if false.
	elseJump := c.emitJump(bytecode.OP_JUMP_IF_FALSE)

	// Compile then branch
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	if len(n.Else) == 0 {
		c.patchJump(elseJump)
		return nil
	}

	endJump := c.emitJump(bytecode.OP_JUMP)
	c.patchJump(elseJump)
	if c.importLayout != nil {
		c.importLayout.resetBranch(c)
	}
	if err := c.compileBlock(n.Else); err != nil {
		return err
	}
	c.patchJump(endJump)

	return nil
}

func (c *lowerer) compileWhile(n *ir.WhileStmt) error {
	// Declare temp variable for loop result (break expr value or default 0)
	resultVar := c.declareInternalVariable(c.tempVar("loop_result"))
	// Initialize to 0 (default loop result when no break expr)
	if op, ok := bytecode.MakeImmediateOpcode(0); ok {
		c.emit(op)
	}
	c.emitStoreLocal(resultVar)

	// Start loop
	c.beginLoop(n.Label, "", "")
	loopStart := c.currentOffset()
	// For while loops, continue jumps back to condition check
	c.currentLoop().ContinueIP = loopStart

	// Compile condition
	if err := c.compileNode(n.Condition); err != nil {
		return err
	}

	// Exit loop if false. A named loop's test is Toast's EOP_WHILE_ID, an
	// extended opcode whose tick is not tested against the budget.
	if n.Label != "" {
		// EOP_WHILE_ID binds the complete condition value on every test,
		// including the false value which terminates the loop.
		c.emit(bytecode.OP_DUP)
		c.emitStoreLocal(c.declareVariable(n.Label))
		c.emitTicks(1 | bytecode.TicksUnchecked)
	}
	exitJump := c.emitJump(bytecode.OP_JUMP_IF_FALSE)

	// Compile body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// Jump back to start (backward jump)
	c.emit(bytecode.OP_LOOP_WIDE)
	// After reading opcode + uint32, IP = currentOffset + 4.
	// We want IP - offset = loopStart.
	offset := c.currentOffset() + 4 - loopStart
	c.emitWideInt(offset)

	// Patch exit jump
	c.patchJump(exitJump)

	// End loop and push result
	c.endLoop()
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(resultVar))
	return nil
}

// compileRangeLoop compiles: for x in [start..end] ... endfor
// Compiles to equivalent while loop pattern.
func (c *lowerer) compileRangeLoop(n *ir.RangeLoopStmt) error {
	// Native range state has a private next cursor and end bound. The user's
	// loop variable can be assigned in the body without changing iteration.
	endVar := c.declareInternalVariable(c.tempVar("end"))
	nextVar := c.declareInternalVariable(c.tempVar("range_next"))
	valueVar := c.declareVariable(n.Value)

	// Evaluate both bounds from left to right before binding the user variable.
	if err := c.compileNode(n.Start); err != nil {
		return err
	}
	if err := c.compileNode(n.End); err != nil {
		return err
	}
	c.emitStoreLocal(endVar)
	c.emitStoreLocal(nextVar)

	// Loop start
	c.beginLoop(n.Label, n.Value, "")
	loopStart := c.currentOffset()
	c.currentLoop().ContinueIP = loopStart

	// Condition: if value > end, jump to exit. Fused FOR_RANGE_CHECK replaces the
	// GET_VAR/GET_VAR/LE/JUMP_IF_FALSE sequence (same compare semantics, one dispatch).
	c.emit(bytecode.OP_FOR_RANGE_CHECK_WIDE)
	c.emitByte(byte(nextVar))
	c.emitByte(byte(endVar))
	exitJump := c.currentOffset()
	c.emitWide(^uint32(0)) // exit offset placeholder, patched below

	// Bind the value and advance the private state before executing the body,
	// exactly as native OP_FOR_RANGE does. The existing NEXT opcode's zero
	// backward distance performs only its overflow-safe advance; its semantics
	// for historical checkpoint programs remain unchanged.
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(nextVar))
	c.emitStoreLocal(valueVar)
	c.emit(bytecode.OP_FOR_RANGE_NEXT_WIDE)
	c.emitByte(byte(nextVar))
	c.emitByte(byte(endVar))
	c.emitWideInt(0)
	if c.importLayout != nil {
		c.importLayout.flush(c)
		c.importLayout.locals[nextVar] = barnImportValue{origin: NativeValueOrigin{Kind: NativeRangeNext, Node: n}}
		c.importLayout.locals[endVar] = barnImportValue{origin: NativeValueOrigin{Kind: NativeRangeLimit, Node: n}}
	}

	// Body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// The cursor was advanced before the body, including before continue.
	c.emit(bytecode.OP_LOOP_WIDE)
	offset := c.currentOffset() + 4 - loopStart
	c.emitWideInt(offset)

	// Patch exit
	c.patchJump(exitJump)
	c.endLoop()
	// Break carries no value, so the loop's result is the constant zero.
	op, _ := bytecode.MakeImmediateOpcode(0)
	c.emit(op)
	return nil
}

// compileCollectionLoop compiles: for x in (expr) ... endfor
// Values and optional map keys are separate snapshots. List/string indexes
// come from the cursor; no per-element pair lists are needed.
func (c *lowerer) compileCollectionLoop(n *ir.CollectionLoopStmt) error {
	hasIndex := n.Index != ""

	// Hidden variables (unique per loop to support nesting)
	listVar := c.declareInternalVariable(c.tempVar("list"))
	keysVar := c.declareInternalVariable(c.tempVar("keys"))
	idxVar := c.declareInternalVariable(c.tempVar("idx"))
	lenVar := c.declareInternalVariable(c.tempVar("len"))
	valueVar := c.declareVariable(n.Value)
	var indexVar int
	if hasIndex {
		indexVar = c.declareVariable(n.Index)
	}

	// Declare temp variable for loop result (break expr value or default 0)
	resultVar := c.declareInternalVariable(c.tempVar("loop_result"))
	if op, ok := bytecode.MakeImmediateOpcode(0); ok {
		c.emit(op)
	}
	c.emitStoreLocal(resultVar)

	// Evaluate the container once, then prepare its iteration snapshots.
	if err := c.compileNode(n.Collection); err != nil {
		return err
	}
	c.emit(bytecode.OP_ITER_PREP_COLUMNS)
	// Stack: [values, keys-or-zero]. Map keys stay rooted even when the loop
	// does not bind them, preserving their lifetime across suspension.
	c.emitStoreLocal(keysVar)
	c.emitStoreLocal(listVar)

	// idx = 1
	if op, ok := bytecode.MakeImmediateOpcode(1); ok {
		c.emit(op)
	}
	c.emitStoreLocal(idxVar)

	// len = length(list)
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(listVar))
	c.emit(bytecode.OP_LENGTH)
	c.emitStoreLocal(lenVar)

	// Loop start
	c.beginLoop(n.Label, n.Value, n.Index)
	loopStart := c.currentOffset()

	// Condition: if idx > len, jump to exit. Fused FOR_LIST_CHECK (idx and len are
	// ints) replaces GET_VAR/GET_VAR/LE/JUMP_IF_FALSE; it is FOR_RANGE_CHECK with
	// the unchecked tick of Toast's EOP_FOR_LIST.
	c.emit(bytecode.OP_FOR_LIST_CHECK_WIDE)
	c.emitByte(byte(idxVar))
	c.emitByte(byte(lenVar))
	exitJump := c.currentOffset()
	c.emitWide(^uint32(0)) // exit offset placeholder, patched below

	// Load current element into the loop variable(s). Fused element-load replaces
	// GET_VAR(list)/GET_VAR(idx)/INDEX plus the value/index extraction: idx is
	// provably in [1..len] so the bounds-checked INDEX dispatch is unnecessary.
	if hasIndex {
		c.emit(bytecode.OP_FOR_LIST_LOAD_COLUMNS)
		c.emitByte(byte(listVar))
		c.emitByte(byte(idxVar))
		c.emitByte(byte(valueVar))
		c.emitByte(byte(indexVar))
		c.emitByte(byte(keysVar))
	} else {
		c.emit(bytecode.OP_FOR_LIST_LOAD_VALUE)
		c.emitByte(byte(listVar))
		c.emitByte(byte(idxVar))
		c.emitByte(byte(valueVar))
	}

	if c.importLayout != nil {
		c.importLayout.flush(c)
		c.importLayout.locals[idxVar] = barnImportValue{origin: NativeValueOrigin{Kind: NativeLoopCursor, Node: n}, projection: importCursor}
	}
	// Body
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}

	// Patch continue jumps to point here (the increment section)
	// continue in a for-list should increment before re-checking condition
	for _, offset := range c.currentLoop().ContinueJumps {
		c.patchWide(offset, c.currentOffset())
	}

	// Increment + loop back: idx += 1; jump to condition. Fused FOR_RANGE_NEXT
	// replaces GET_VAR/IMM/ADD/SET_VAR/LOOP (same opcode used by range-for).
	c.emit(bytecode.OP_FOR_RANGE_NEXT_WIDE)
	c.emitByte(byte(idxVar))
	c.emitByte(byte(lenVar))
	offset := c.currentOffset() + 4 - loopStart
	c.emitWideInt(offset)

	// Patch exit
	c.patchJump(exitJump)
	c.endLoop()
	// Push loop result onto stack
	c.emit(bytecode.OP_GET_VAR)
	c.emitByte(byte(resultVar))
	return nil
}

func (c *lowerer) compileBreak(n *ir.BreakStmt) error {
	// Mirror compileContinue: an explicit loop name must resolve to an
	// enclosing loop, otherwise raise "Invalid loop name" (ToastStunt
	// parser.y:1205-1206, check_loop_name LOOP_BREAK).
	loop := c.findLoopByTarget(n.Label)
	if loop == nil && n.Label != "" {
		//lint:ignore ST1005 This diagnostic matches Toast's externally visible compiler text.
		return fmt.Errorf("Invalid loop name")
	}
	if loop == nil {
		return fmt.Errorf("break outside of loop")
	}

	// Emit a forward jump past the loop end (will be patched by endLoop). It
	// stands for Toast's EOP_EXIT / EOP_EXIT_ID: one unchecked tick.
	c.emitTicks(1 | bytecode.TicksUnchecked)
	patchOffset := c.emitUnwindJump(loop.HandlerDepth, -1)
	loop.BreakJumps = append(loop.BreakJumps, patchOffset)
	return nil
}

func (c *lowerer) compileContinue(n *ir.ContinueStmt) error {
	loop := c.findLoopByTarget(n.Label)
	if loop == nil && n.Label != "" {
		//lint:ignore ST1005 This diagnostic matches Toast's externally visible compiler text.
		return fmt.Errorf("Invalid loop name")
	}
	if loop == nil {
		return fmt.Errorf("continue outside of loop")
	}

	// Like break, continue is Toast's EOP_EXIT / EOP_EXIT_ID.
	c.emitTicks(1 | bytecode.TicksUnchecked)
	if loop.ContinueIP > 0 {
		c.emitUnwindJump(loop.HandlerDepth, loop.ContinueIP)
	} else {
		// ContinueIP not yet known (for loops) -- emit forward jump, patch later
		patchOffset := c.emitUnwindJump(loop.HandlerDepth, -1)
		loop.ContinueJumps = append(loop.ContinueJumps, patchOffset)
	}
	return nil
}

func (c *lowerer) emitUnwindJump(depth, target int) int {
	if depth < 0 || depth > math.MaxUint16 {
		if c.err == nil {
			c.err = fmt.Errorf("too many active exception handlers")
		}
	}
	c.emit(bytecode.OP_UNWIND_JUMP)
	patch := c.currentOffset()
	if target < 0 {
		c.emitWide(^uint32(0))
	} else {
		c.emitWideInt(target)
	}
	c.emitShort(uint16(depth))
	return patch
}

func (c *lowerer) compileReturn(n *ir.ReturnStmt) error {
	if n.Value != nil {
		// Compile return value
		if err := c.compileNode(n.Value); err != nil {
			return err
		}
		c.emit(bytecode.OP_RETURN)
	} else {
		// Return 0
		c.emit(bytecode.OP_RETURN_NONE)
	}
	return nil
}

// prepareExceptCodes returns an except clause's OP_TRY_EXCEPT code bytes,
// count first. ANY has no codes, and a list of error literals is encoded
// inline. Any other list is evaluated here, before the try is entered, as
// Toast evaluates every arm's codes (code_gen.cc generate_codes), into a
// compiler temporary that the clause names with ExceptCodesInLocal.
func (c *lowerer) prepareExceptCodes(isAny bool, codes []ir.Expr) ([]byte, error) {
	if isAny {
		return []byte{0}, nil
	}
	errorCodes, literal, err := literalErrorCodes(codes)
	if err != nil {
		return nil, err
	}
	if literal && len(errorCodes) <= math.MaxUint8 {
		encoded := make([]byte, 0, 1+len(errorCodes))
		encoded = append(encoded, byte(len(errorCodes)))
		for _, code := range errorCodes {
			encoded = append(encoded, byte(code))
		}
		return encoded, nil
	}
	if err := c.compileList(&ir.ListExpr{Pos: codes[0].Position(), Elements: codes}); err != nil {
		return nil, err
	}
	slot := c.declareInternalVariable(c.tempVar("except_codes"))
	c.emitStoreLocal(slot)
	return []byte{2, bytecode.ExceptCodesInLocal, byte(slot)}, nil
}

func (c *lowerer) compileTry(n *ir.TryStmt) error {
	finallyIPPatch := -1
	if n.Finalizer != nil {
		c.emit(bytecode.OP_TRY_FINALLY_WIDE)
		finallyIPPatch = len(c.program.Code)
		c.emitWide(^uint32(0))
		c.handlerDepth++
	}

	if len(n.Handlers) == 0 {
		if err := c.compileBlock(n.Body); err != nil {
			return err
		}
	} else {
		numHandlers := len(n.Handlers)
		clauseCodes := make([][]byte, numHandlers)
		for i, handler := range n.Handlers {
			codes, err := c.prepareExceptCodes(handler.IsAny, handler.Codes)
			if err != nil {
				return err
			}
			clauseCodes[i] = codes
		}

		c.emit(bytecode.OP_TRY_EXCEPT_LOCAL_WIDE)
		c.emitByte(byte(numHandlers))

		handlerOffsetPatches := make([]int, numHandlers)
		for i, handler := range n.Handlers {
			c.program.Code = append(c.program.Code, clauseCodes[i]...)

			if handler.Variable != "" {
				idx := c.declareVariable(handler.Variable)
				c.emitShort(uint16(idx + 1))
			} else {
				c.emitShort(0)
			}

			handlerOffsetPatches[i] = len(c.program.Code)
			c.emitWide(^uint32(0))
		}

		c.handlerDepth += numHandlers
		if err := c.compileBlock(n.Body); err != nil {
			return err
		}

		c.handlerDepth -= numHandlers
		c.emit(bytecode.OP_END_EXCEPT)
		c.emitByte(byte(numHandlers))
		endHandlersJump := c.emitJump(bytecode.OP_JUMP)
		handlerEndJumps := make([]int, 0, numHandlers-1)

		for i, handler := range n.Handlers {
			handlerIP := c.currentOffset()
			c.patchWide(handlerOffsetPatches[i], handlerIP)
			if c.importLayout != nil {
				c.importLayout.resetBranch(c)
			}

			if err := c.compileBlock(handler.Body); err != nil {
				return err
			}
			if i < numHandlers-1 {
				handlerEndJumps = append(handlerEndJumps, c.emitJump(bytecode.OP_JUMP))
			}
		}

		c.patchJump(endHandlersJump)
		for _, jump := range handlerEndJumps {
			c.patchJump(jump)
		}
	}

	if n.Finalizer != nil {
		c.handlerDepth--
		c.emit(bytecode.OP_END_FINALLY_WIDE)
		endFinallyIPPatch := len(c.program.Code)
		c.emitWide(^uint32(0))
		finallyIP := c.currentOffset()
		c.patchWide(finallyIPPatch, finallyIP)
		c.patchWide(endFinallyIPPatch, finallyIP)
		if err := c.compileBlock(n.Finalizer.Body); err != nil {
			return err
		}
		c.emit(bytecode.OP_END_FINALLY_WIDE)
		c.emitWideInt(finallyIP)
	}

	return nil
}

func (c *lowerer) compileFork(n *ir.ForkStmt) error {
	// Fork statement: fork [name] (delay) body endfork
	//
	// Bytecode layout:
	//   [delay expression]         -- evaluates delay, pushes onto stack
	//   bytecode.OP_FORK_LOCAL_WIDE <varIdx:uint16> <bodyLen:uint32> -- pops delay, validates, sets var=0, jumps over body
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

	// Emit the local-wide fork with variable index and placeholder body length.
	c.emit(bytecode.OP_FORK_LOCAL_WIDE)
	c.emitShort(uint16(varIdx))
	bodyLenPatch := len(c.program.Code)
	c.emitWide(^uint32(0)) // placeholder for body length

	// Compile the fork body (will be skipped at runtime but compiled for future use)
	bodyStart := c.currentOffset()
	parentDepth, parentLoops := c.handlerDepth, c.loops
	c.handlerDepth, c.loops = 0, nil
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}
	c.handlerDepth, c.loops = parentDepth, parentLoops
	bodyEnd := c.currentOffset()

	// Patch body length
	bodyLen := bodyEnd - bodyStart
	c.patchWide(bodyLenPatch, bodyLen)

	return nil
}

// isLoopStmt returns true if a statement node is a loop (pushes a result value).
func isLoopStmt(stmt ir.Stmt) bool {
	switch stmt.(type) {
	case *ir.WhileStmt, *ir.CollectionLoopStmt, *ir.RangeLoopStmt:
		return true
	default:
		return false
	}
}

func (c *lowerer) compileBlock(stmts []ir.Stmt) error {
	for _, stmt := range stmts {
		if err := c.compileNode(stmt); err != nil {
			return err
		}
		// Loop statements push their result value onto the stack.
		// In block context (if/try/loop bodies), discard it to keep the stack clean.
		if isLoopStmt(stmt) {
			c.emit(bytecode.OP_POP)
		}
	}
	return nil
}
