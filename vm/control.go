package vm

import (
	"fmt"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
	"strings"
	"time"
)

// executeFork handles OP_FORK: evaluate delay, then yield to the execution engine.
//
// Bytecode format: OP_FORK <varIdx:byte> <bodyLen:short> (legacy) or
// OP_FORK_WIDE <varIdx:byte> <bodyLen:uint32> or
// OP_FORK_LOCAL_WIDE <varIdx:uint16> <bodyLen:uint32> (new compilation).
// Stack: [delay] (delay value on top)
//
// Yields a FlowFork result with ForkInfo containing the fork body location,
// delay, and variable name. The execution engine should:
//  1. Create the child task (fork body)
//  2. Call SetForkResult(childTaskID) on the VM
//  3. Call Resume() to continue execution after the fork
//
// The fork variable is NOT set here — it is set by SetForkResult() with the
// actual child task ID assigned by the execution engine.
func (vm *VM) executeFork(wide, wideLocal bool) error {
	var varIdx int
	if wideLocal {
		varIdx = int(vm.ReadShort())
	} else {
		varIdx = int(vm.FetchByte())
	}
	bodyLen := vm.readControlFlowOperand(wide)

	// Pop and validate the delay value
	delay := vm.Pop()

	var delaySeconds float64
	switch delay.Type() {
	case types.TYPE_INT:
		if delay.Int() < 0 {
			return fmt.Errorf("E_INVARG: fork delay must be non-negative")
		}
		delaySeconds = float64(delay.Int())
	case types.TYPE_FLOAT:
		if delay.Float() < 0 {
			return fmt.Errorf("E_INVARG: fork delay must be non-negative")
		}
		delaySeconds = delay.Float()
	default:
		return fmt.Errorf("E_TYPE: fork delay must be numeric")
	}

	// Resolve variable name from index
	var varName string
	if varIdx > 0 {
		frame := vm.CurrentFrame()
		if varIdx-1 < len(frame.Program.VarNames) {
			varName = frame.Program.VarNames[varIdx-1]
		}
	}

	// Record the fork body's bytecode position for the execution engine.
	// The body starts at the current IP and runs for bodyLen bytes.
	frame := vm.CurrentFrame()
	forkBodyIP := frame.IP
	forkBodyLen := bodyLen

	// Skip over the fork body — the parent continues after the fork
	frame.IP += forkBodyLen

	// Build ForkInfo for the execution engine.
	// Include the parent program and a locals snapshot so the execution engine can
	// create a child VM with the forked bytecode range and variable state.
	localsCopy := make([]types.Value, len(frame.Locals))
	copy(localsCopy, frame.Locals)

	// Populate context fields from the current frame
	var thisObj types.ObjID = types.ObjNothing
	var playerObj types.ObjID = types.ObjNothing
	var callerObj types.ObjID = types.ObjNothing
	var verbStr string
	thisObj = frame.This
	playerObj = frame.Player
	callerObj = frame.Caller
	verbStr = frame.Verb
	if vm.Context != nil {
		if vm.Context.Player != types.ObjNothing {
			playerObj = vm.Context.Player
		}
	}
	thisValue := types.None
	if vm.Context != nil {
		thisValue = vm.Context.ThisValue
	}

	forkInfo := &types.ForkInfo{
		Delay:       time.Duration(delaySeconds * float64(time.Second)),
		VarName:     varName,
		Body:        [3]interface{}{frame.Program, forkBodyIP, forkBodyLen}, // parent program, offset, length
		SourceLines: sourceLinesForFork(frame.Program, forkBodyIP, forkBodyLen),
		ThisObj:     thisObj,
		ThisValue:   thisValue,
		Player:      playerObj,
		Caller:      callerObj,
		Verb:        verbStr,
		VerbLoc:     frame.VerbLoc,
	}
	// Store locals snapshot in Variables map for the execution engine.
	forkInfo.Variables = make(map[string]types.Value, len(frame.Program.VarNames))
	for i, name := range frame.Program.VarNames {
		if i < len(localsCopy) {
			forkInfo.Variables[name] = localsCopy[i]
		}
	}

	// Yield to the execution engine.
	vm.yielded = true
	vm.yieldResult = types.Result{
		Flow:     types.FlowFork,
		ForkInfo: forkInfo,
	}

	return nil
}

func sourceLinesForFork(program *bytecode.Program, bodyIP, bodyLen int) []string {
	if program == nil || len(program.Source) == 0 || bodyLen <= 0 {
		return nil
	}

	startLine := program.LineForIP(bodyIP)
	endLine := program.LineForIP(bodyIP + bodyLen - 1)
	if startLine <= 0 || endLine <= 0 {
		return nil
	}
	if endLine < startLine {
		endLine = startLine
	}
	if startLine > len(program.Source) {
		return nil
	}
	if endLine > len(program.Source) {
		endLine = len(program.Source)
	}
	if startLine == endLine {
		if body := oneLineForkBody(program.Source[startLine-1]); body != "" {
			return []string{body}
		}
	}

	lines := make([]string, 0, endLine-startLine+1)
	for i := startLine; i <= endLine; i++ {
		lines = append(lines, program.Source[i-1])
	}
	return lines
}

func oneLineForkBody(source string) string {
	lower := strings.ToLower(source)
	forkIdx := strings.Index(lower, "fork")
	if forkIdx < 0 {
		return ""
	}
	headerEnd := strings.Index(source[forkIdx:], ")")
	if headerEnd < 0 {
		return ""
	}
	bodyStart := forkIdx + headerEnd + 1
	endIdx := strings.Index(strings.ToLower(source[bodyStart:]), "endfork")
	if endIdx < 0 {
		return ""
	}
	body := strings.TrimSpace(source[bodyStart : bodyStart+endIdx])
	body = strings.TrimPrefix(body, ";")
	body = strings.TrimSpace(body)
	return body
}

// executeTryExcept handles OP_TRY_EXCEPT: push exception handlers onto ExceptStack
func (vm *VM) executeTryExcept(wide, wideLocal bool) error {
	frame := vm.CurrentFrame()
	numClauses := int(vm.FetchByte())
	handlers := make([]bytecode.Handler, numClauses)

	for i := 0; i < numClauses; i++ {
		numCodes := int(vm.FetchByte())
		codes := make([]types.ErrorCode, numCodes)
		for j := 0; j < numCodes; j++ {
			codes[j] = types.ErrorCode(vm.FetchByte())
		}

		var varOperand int
		if wideLocal {
			varOperand = int(vm.ReadShort())
		} else {
			varOperand = int(vm.FetchByte())
		}
		varIndex := varOperand - 1 // 0 = no variable -> -1

		// Read handler IP (absolute).
		handlerIP := vm.readControlFlowOperand(wide)

		handlers[i] = bytecode.Handler{
			Type:       bytecode.HandlerExcept,
			HandlerIP:  handlerIP,
			Codes:      codes,
			VarIndex:   varIndex,
			StackDepth: vm.SP,
		}
	}

	// Push in reverse source order so reverse scan in HandleError honors
	// "first matching except clause wins".
	for i := numClauses - 1; i >= 0; i-- {
		frame.ExceptStack = append(frame.ExceptStack, handlers[i])
	}

	return nil
}

// executeEndExcept handles OP_END_EXCEPT <num_clauses>: pop exactly the
// handlers pushed by the matching OP_TRY_EXCEPT. Popping "while top is
// Except-type" instead of an exact count is wrong whenever a try/except is
// nested inside another try/except's body (e.g. a backtick catch-expression
// evaluated inside an outer try) — closing the inner block would also
// consume the outer block's still-live handlers, since both are
// HandlerExcept-typed with nothing else distinguishing them.
func (vm *VM) executeEndExcept() error {
	frame := vm.CurrentFrame()
	numClauses := int(vm.FetchByte())
	if numClauses > len(frame.ExceptStack) {
		return fmt.Errorf("internal error: END_EXCEPT wants to pop %d handlers from stack of %d", numClauses, len(frame.ExceptStack))
	}
	frame.ExceptStack = frame.ExceptStack[:len(frame.ExceptStack)-numClauses]
	return nil
}

// executeTryFinally handles OP_TRY_FINALLY: push a finally handler
func (vm *VM) executeTryFinally(wide bool) error {
	frame := vm.CurrentFrame()

	// Read finally IP (absolute)
	finallyIP := vm.readControlFlowOperand(wide)

	handler := bytecode.Handler{
		Type:       bytecode.HandlerFinally,
		HandlerIP:  finallyIP,
		VarIndex:   -1,
		StackDepth: vm.SP,
	}
	frame.ExceptStack = append(frame.ExceptStack, handler)

	return nil
}

// executeEndFinally handles OP_END_FINALLY <finally_ip>.
// This opcode appears twice in try/finally bytecode:
// 1. After the try body (normal path): pop its matching handler from ExceptStack
// 2. After the finally block: re-raise PendingError if set
func (vm *VM) executeEndFinally(wide bool) error {
	frame := vm.CurrentFrame()

	// Read the matching finally IP (absolute). Both END_FINALLY instructions
	// carry the same identity as their OP_TRY_FINALLY so an inner block cannot
	// consume an enclosing block's still-live handler.
	finallyIP := vm.readControlFlowOperand(wide)

	// If this block's finally handler is on top of the stack, pop it (normal path).
	if len(frame.ExceptStack) > 0 {
		top := frame.ExceptStack[len(frame.ExceptStack)-1]
		if top.Type == bytecode.HandlerFinally && top.HandlerIP == finallyIP {
			frame.ExceptStack = frame.ExceptStack[:len(frame.ExceptStack)-1]
			return nil
		}
	}

	// This block's handler was already removed while unwinding. Re-raise the
	// pending error without consuming an enclosing finally handler.
	if frame.PendingError != nil {
		err := frame.PendingError
		frame.PendingError = nil
		return err
	}

	return nil
}
