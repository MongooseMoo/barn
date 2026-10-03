package compiler

import (
	"fmt"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/verb"
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
	ResultVar     int   // Variable slot holding loop result (from break expr or default 0)
}

// beginLoop starts a new loop context.
// resultVar is the local slot that holds the loop's result value (from break expr or default 0).
func (c *lowerer) beginLoop(label string, resultVar int, valueName, indexName string) {
	c.loops = append(c.loops, loopContext{
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
func (c *lowerer) endLoop() {
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

func (c *lowerer) compileIf(n *verb.IfStmt) error {
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
	if err := c.compileBlock(n.Else); err != nil {
		return err
	}
	c.patchJump(endJump)

	return nil
}

func (c *lowerer) compileWhile(n *verb.WhileStmt) error {
	// Declare temp variable for loop result (break expr value or default 0)
	resultVar := c.declareInternalVariable(c.tempVar("loop_result"))
	// Initialize to 0 (default loop result when no break expr)
	if op, ok := bytecode.MakeImmediateOpcode(0); ok {
		c.emit(op)
	}
	c.emitStoreLocal(resultVar)

	// Start loop
	c.beginLoop(n.Label, resultVar, "", "")
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
func (c *lowerer) compileRangeLoop(n *verb.RangeLoopStmt) error {
	// Hidden variable for end bound
	endVar := c.declareInternalVariable(c.tempVar("end"))
	valueVar := c.declareVariable(n.Value)

	// Declare temp variable for loop result (break expr value or default 0)
	resultVar := c.declareInternalVariable(c.tempVar("loop_result"))
	if op, ok := bytecode.MakeImmediateOpcode(0); ok {
		c.emit(op)
	}
	c.emitStoreLocal(resultVar)

	// Evaluate end and store
	if err := c.compileNode(n.End); err != nil {
		return err
	}
	c.emitStoreLocal(endVar)

	// Evaluate start and store as loop variable. Toast's OP_FOR_RANGE binds the
	// variable itself, so this store is not an OP_PUT.
	if err := c.compileNode(n.Start); err != nil {
		return err
	}
	c.emitStoreLocal(valueVar)

	// Loop start
	c.beginLoop(n.Label, resultVar, n.Value, "")
	loopStart := c.currentOffset()

	// Condition: if value > end, jump to exit. Fused FOR_RANGE_CHECK replaces the
	// GET_VAR/GET_VAR/LE/JUMP_IF_FALSE sequence (same compare semantics, one dispatch).
	c.emit(bytecode.OP_FOR_RANGE_CHECK_WIDE)
	c.emitByte(byte(valueVar))
	c.emitByte(byte(endVar))
	exitJump := c.currentOffset()
	c.emitWide(^uint32(0)) // exit offset placeholder, patched below

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
	c.emit(bytecode.OP_FOR_RANGE_NEXT_WIDE)
	c.emitByte(byte(valueVar))
	c.emitByte(byte(endVar))
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

// compileCollectionLoop compiles: for x in (expr) ... endfor
// Values and optional map keys are separate snapshots. List/string indexes
// come from the cursor; no per-element pair lists are needed.
func (c *lowerer) compileCollectionLoop(n *verb.CollectionLoopStmt) error {
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
	c.beginLoop(n.Label, resultVar, n.Value, n.Index)
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

func (c *lowerer) compileBreak(n *verb.BreakStmt) error {
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
	patchOffset := c.emitJump(bytecode.OP_JUMP)
	loop.BreakJumps = append(loop.BreakJumps, patchOffset)
	return nil
}

func (c *lowerer) compileContinue(n *verb.ContinueStmt) error {
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
		// ContinueIP is known (while loops) -- emit backward jump directly
		c.emit(bytecode.OP_LOOP_WIDE)
		// After reading opcode + uint32, IP = currentOffset + 4.
		// We want IP - offset = ContinueIP.
		offset := c.currentOffset() + 4 - loop.ContinueIP
		c.emitWideInt(offset)
	} else {
		// ContinueIP not yet known (for loops) -- emit forward jump, patch later
		patchOffset := c.emitJump(bytecode.OP_JUMP)
		loop.ContinueJumps = append(loop.ContinueJumps, patchOffset)
	}
	return nil
}

func (c *lowerer) compileReturn(n *verb.ReturnStmt) error {
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

func (c *lowerer) compileTry(n *verb.TryStmt) error {
	finallyIPPatch := -1
	if n.Finalizer != nil {
		c.emit(bytecode.OP_TRY_FINALLY_WIDE)
		finallyIPPatch = len(c.program.Code)
		c.emitWide(^uint32(0))
	}

	if len(n.Handlers) == 0 {
		if err := c.compileBlock(n.Body); err != nil {
			return err
		}
	} else {
		numHandlers := len(n.Handlers)
		c.emit(bytecode.OP_TRY_EXCEPT_LOCAL_WIDE)
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
				c.emitShort(uint16(idx + 1))
			} else {
				c.emitShort(0)
			}

			handlerOffsetPatches[i] = len(c.program.Code)
			c.emitWide(^uint32(0))
		}

		if err := c.compileBlock(n.Body); err != nil {
			return err
		}

		c.emit(bytecode.OP_END_EXCEPT)
		c.emitByte(byte(numHandlers))
		endHandlersJump := c.emitJump(bytecode.OP_JUMP)
		handlerEndJumps := make([]int, 0, numHandlers-1)

		for i, handler := range n.Handlers {
			handlerIP := c.currentOffset()
			c.patchWide(handlerOffsetPatches[i], handlerIP)

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

func (c *lowerer) compileFork(n *verb.ForkStmt) error {
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
	if err := c.compileBlock(n.Body); err != nil {
		return err
	}
	bodyEnd := c.currentOffset()

	// Patch body length
	bodyLen := bodyEnd - bodyStart
	c.patchWide(bodyLenPatch, bodyLen)

	return nil
}

// isLoopStmt returns true if a statement node is a loop (pushes a result value).
func isLoopStmt(stmt verb.Stmt) bool {
	switch stmt.(type) {
	case *verb.WhileStmt, *verb.CollectionLoopStmt, *verb.RangeLoopStmt:
		return true
	default:
		return false
	}
}

func (c *lowerer) compileBlock(stmts []verb.Stmt) error {
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
