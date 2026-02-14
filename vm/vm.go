package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/types"
	"fmt"
)

// MooError wraps an ErrorCode as a Go error
type MooError struct {
	Code types.ErrorCode
}

func (e MooError) Error() string {
	return fmt.Sprintf("E_%d", e.Code)
}

// extractErrorCode parses an error code from an error message string.
// Handles messages like "E_DIV: division by zero" or "E_TYPE: ..."
func extractErrorCode(err error) types.ErrorCode {
	msg := err.Error()
	// Look for "E_XXX" at the start or after a space
	for _, prefix := range []string{
		"E_TYPE", "E_DIV", "E_PERM", "E_PROPNF", "E_VERBNF", "E_VARNF",
		"E_INVIND", "E_RECMOVE", "E_MAXREC", "E_RANGE", "E_ARGS",
		"E_NACC", "E_INVARG", "E_QUOTA", "E_FLOAT", "E_FILE", "E_EXEC",
		"E_INTRPT",
	} {
		if len(msg) >= len(prefix) && msg[:len(prefix)] == prefix {
			if code, ok := types.ErrorFromString(prefix); ok {
				return code
			}
		}
	}
	return types.E_NONE
}

// VM represents the bytecode virtual machine
type VM struct {
	Stack     []types.Value         // Operand stack
	SP        int                   // Stack pointer
	Frames    []*StackFrame         // Call stack
	FP        int                   // Frame pointer
	Store     *db.Store             // Object store
	Builtins  *builtins.Registry    // Builtin function registry
	Context   *types.TaskContext    // Task context for builtins
	TickLimit int64                 // Maximum ticks before E_MAXREC
	Ticks     int64                 // Current tick count
}

// StackFrame represents a call frame
type StackFrame struct {
	Program      *Program      // Bytecode program
	IP           int           // Instruction pointer
	BasePointer  int           // Stack base for this frame
	Locals       []types.Value // Local variables
	This         types.ObjID   // Current object
	Player       types.ObjID   // Player context
	Verb         string        // Verb name
	Caller       types.ObjID   // Calling object
	LoopStack    []LoopState   // Nested loop state
	ExceptStack  []Handler     // Exception handlers
	PendingError error         // Error saved during finally execution
}

// NewVM creates a new virtual machine
func NewVM(store *db.Store, registry *builtins.Registry) *VM {
	return &VM{
		Stack:     make([]types.Value, 0, 256),
		SP:        0,
		Frames:    make([]*StackFrame, 0, 16),
		FP:        0,
		Store:     store,
		Builtins:  registry,
		TickLimit: 30000,
		Ticks:     0,
	}
}

// Run executes a program and returns the result
func (vm *VM) Run(prog *Program) (types.Value, error) {
	// Create initial frame
	frame := &StackFrame{
		Program:     prog,
		IP:          0,
		BasePointer: vm.SP,
		Locals:      make([]types.Value, prog.NumLocals),
		This:        types.ObjNothing,
		Player:      types.ObjNothing,
		Verb:        "",
		Caller:      types.ObjNothing,
		LoopStack:   make([]LoopState, 0, 4),
		ExceptStack: make([]Handler, 0, 4),
	}

	// Initialize locals to 0
	for i := range frame.Locals {
		frame.Locals[i] = types.IntValue{Val: 0}
	}

	vm.Frames = append(vm.Frames, frame)
	vm.FP = 0

	// Execute until done
	for len(vm.Frames) > 0 {
		if err := vm.Step(); err != nil {
			// Handle error
			if !vm.HandleError(err) {
				return nil, err
			}
		}

		// Check tick limit
		if vm.Ticks >= vm.TickLimit {
			return nil, fmt.Errorf("E_MAXREC: tick limit exceeded")
		}
	}

	// Return result
	if vm.SP > 0 {
		return vm.Pop(), nil
	}

	return types.IntValue{Val: 0}, nil
}

// Step executes a single instruction
func (vm *VM) Step() error {
	frame := vm.CurrentFrame()
	if frame == nil {
		return fmt.Errorf("no active frame")
	}

	if frame.IP >= len(frame.Program.Code) {
		// End of program - implicit return 0
		vm.Return(types.IntValue{Val: 0})
		return nil
	}

	op := OpCode(frame.Program.Code[frame.IP])
	frame.IP++

	// Count ticks for expensive operations
	if CountsTick(op) {
		vm.Ticks++
	}

	return vm.Execute(op)
}

// Execute dispatches an opcode
func (vm *VM) Execute(op OpCode) error {
	// Check for immediate integer
	if IsImmediateInt(op) {
		val := GetImmediateValue(op)
		vm.Push(types.IntValue{Val: int64(val)})
		return nil
	}

	switch op {
	// Stack operations
	case OP_PUSH:
		idx := vm.ReadByte()
		vm.Push(vm.CurrentFrame().Program.Constants[idx])

	case OP_POP:
		vm.Pop()

	case OP_DUP:
		vm.Push(vm.Peek(0))

	// Variable operations
	case OP_GET_VAR:
		idx := vm.ReadByte()
		vm.Push(vm.CurrentFrame().Locals[idx])

	case OP_SET_VAR:
		idx := vm.ReadByte()
		vm.CurrentFrame().Locals[idx] = vm.Pop()

	// Property operations
	case OP_GET_PROP:
		return vm.executeGetProp()
	case OP_SET_PROP:
		return vm.executeSetProp()

	// Arithmetic operations
	case OP_ADD:
		return vm.executeAdd()
	case OP_SUB:
		return vm.executeSub()
	case OP_MUL:
		return vm.executeMul()
	case OP_DIV:
		return vm.executeDiv()
	case OP_MOD:
		return vm.executeMod()
	case OP_POW:
		return vm.executePow()
	case OP_NEG:
		return vm.executeNeg()

	// Comparison operations
	case OP_EQ:
		return vm.executeEq()
	case OP_NE:
		return vm.executeNe()
	case OP_LT:
		return vm.executeLt()
	case OP_LE:
		return vm.executeLe()
	case OP_GT:
		return vm.executeGt()
	case OP_GE:
		return vm.executeGe()
	case OP_IN:
		return vm.executeIn()

	// Logical operations
	case OP_NOT:
		return vm.executeNot()
	case OP_AND:
		return vm.executeAnd()
	case OP_OR:
		return vm.executeOr()

	// Bitwise operations
	case OP_BITOR:
		return vm.executeBitOr()
	case OP_BITAND:
		return vm.executeBitAnd()
	case OP_BITXOR:
		return vm.executeBitXor()
	case OP_BITNOT:
		return vm.executeBitNot()
	case OP_SHL:
		return vm.executeShl()
	case OP_SHR:
		return vm.executeShr()

	// Control flow
	case OP_JUMP:
		offset := vm.ReadShort()
		vm.CurrentFrame().IP += int(offset)

	case OP_JUMP_IF_FALSE:
		offset := vm.ReadShort()
		if !vm.Pop().Truthy() {
			vm.CurrentFrame().IP += int(offset)
		}

	case OP_JUMP_IF_TRUE:
		offset := vm.ReadShort()
		if vm.Pop().Truthy() {
			vm.CurrentFrame().IP += int(offset)
		}

	case OP_RETURN:
		val := vm.Pop()
		vm.Return(val)

	case OP_LOOP:
		offset := vm.ReadShort()
		vm.CurrentFrame().IP -= int(offset)

	case OP_RETURN_NONE:
		vm.Return(types.IntValue{Val: 0})

	// Collection operations
	case OP_INDEX:
		return vm.executeIndex()
	case OP_INDEX_SET:
		return vm.executeIndexSet()
	case OP_RANGE:
		return vm.executeRange()
	case OP_RANGE_SET:
		return vm.executeRangeSet()
	case OP_MAKE_LIST:
		return vm.executeMakeList()
	case OP_MAKE_MAP:
		return vm.executeMakeMap()
	case OP_LENGTH:
		return vm.executeLength()
	case OP_LIST_RANGE:
		return vm.executeListRange()
	case OP_LIST_APPEND:
		return vm.executeListAppend()
	case OP_LIST_EXTEND:
		return vm.executeListExtend()

	// Scatter assignment
	case OP_SCATTER:
		return vm.executeScatter()

	// Iteration preparation
	case OP_ITER_PREP:
		return vm.executeIterPrep()

	// Builtin calls
	case OP_CALL_BUILTIN:
		return vm.executeCallBuiltin()

	// Verb calls
	case OP_CALL_VERB:
		return vm.executeCallVerb()

	// Exception handling
	case OP_TRY_EXCEPT:
		return vm.executeTryExcept()
	case OP_END_EXCEPT:
		vm.executeEndExcept()
	case OP_TRY_FINALLY:
		return vm.executeTryFinally()
	case OP_END_FINALLY:
		return vm.executeEndFinally()

	default:
		return fmt.Errorf("unknown opcode: %s (%d)", op.String(), op)
	}

	return nil
}

// CurrentFrame returns the current stack frame
func (vm *VM) CurrentFrame() *StackFrame {
	if len(vm.Frames) == 0 {
		return nil
	}
	return vm.Frames[len(vm.Frames)-1]
}

// Push pushes a value onto the stack
func (vm *VM) Push(v types.Value) {
	if vm.SP >= len(vm.Stack) {
		vm.Stack = append(vm.Stack, v)
	} else {
		vm.Stack[vm.SP] = v
	}
	vm.SP++
}

// Pop pops a value from the stack
func (vm *VM) Pop() types.Value {
	if vm.SP == 0 {
		panic("stack underflow")
	}
	vm.SP--
	return vm.Stack[vm.SP]
}

// Peek peeks at a value on the stack (0 = top)
func (vm *VM) Peek(offset int) types.Value {
	if vm.SP-1-offset < 0 {
		panic("stack underflow")
	}
	return vm.Stack[vm.SP-1-offset]
}

// PopN pops N values from the stack
func (vm *VM) PopN(n int) []types.Value {
	if vm.SP < n {
		panic("stack underflow")
	}
	values := make([]types.Value, n)
	for i := n - 1; i >= 0; i-- {
		values[i] = vm.Pop()
	}
	return values
}

// ReadByte reads a byte from the current instruction stream
func (vm *VM) ReadByte() byte {
	frame := vm.CurrentFrame()
	b := frame.Program.Code[frame.IP]
	frame.IP++
	return b
}

// ReadShort reads a 2-byte short from the current instruction stream
func (vm *VM) ReadShort() uint16 {
	frame := vm.CurrentFrame()
	hi := frame.Program.Code[frame.IP]
	lo := frame.Program.Code[frame.IP+1]
	frame.IP += 2
	return uint16(hi)<<8 | uint16(lo)
}

// Return returns from the current frame
func (vm *VM) Return(value types.Value) {
	if len(vm.Frames) == 0 {
		return
	}

	frame := vm.Frames[len(vm.Frames)-1]
	vm.SP = frame.BasePointer
	vm.Frames = vm.Frames[:len(vm.Frames)-1]
	vm.Push(value)
}

// HandleError handles an error by looking for exception handlers
func (vm *VM) HandleError(err error) bool {
	// Extract error code
	errCode := types.E_NONE
	if mooErr, ok := err.(MooError); ok {
		errCode = mooErr.Code
	} else {
		// Try to parse error code from error message (e.g. "E_DIV: division by zero")
		errCode = extractErrorCode(err)
	}

	frame := vm.CurrentFrame()
	if frame == nil {
		return false
	}

	// Search for handler (innermost first)
	for i := len(frame.ExceptStack) - 1; i >= 0; i-- {
		handler := frame.ExceptStack[i]

		if handler.Type == HandlerFinally {
			// Finally handler: run the finally block, then re-raise the error.
			// Pop this handler and everything above it.
			frame.ExceptStack = frame.ExceptStack[:i]
			// Save the pending error so after finally runs, we re-raise it
			frame.PendingError = err
			frame.IP = handler.HandlerIP
			return true
		}

		if handler.Type == HandlerExcept && handler.Matches(errCode) {
			// Found matching except handler - jump to it
			frame.ExceptStack = frame.ExceptStack[:i]
			frame.IP = handler.HandlerIP

			// Store error in variable if specified
			if handler.VarIndex >= 0 {
				frame.Locals[handler.VarIndex] = types.NewErr(errCode)
			}

			return true
		}
	}

	// No handler found
	return false
}

// executeTryExcept handles OP_TRY_EXCEPT: push exception handlers onto ExceptStack
func (vm *VM) executeTryExcept() error {
	frame := vm.CurrentFrame()
	numClauses := int(vm.ReadByte())

	for i := 0; i < numClauses; i++ {
		numCodes := int(vm.ReadByte())
		codes := make([]types.ErrorCode, numCodes)
		for j := 0; j < numCodes; j++ {
			codes[j] = types.ErrorCode(vm.ReadByte())
		}

		varByte := vm.ReadByte()
		varIndex := int(varByte) - 1 // 0 = no variable -> -1

		// Read handler IP (absolute)
		hi := frame.Program.Code[frame.IP]
		lo := frame.Program.Code[frame.IP+1]
		frame.IP += 2
		handlerIP := int(uint16(hi)<<8 | uint16(lo))

		handler := Handler{
			Type:      HandlerExcept,
			HandlerIP: handlerIP,
			Codes:     codes,
			VarIndex:  varIndex,
		}
		frame.ExceptStack = append(frame.ExceptStack, handler)
	}

	return nil
}

// executeEndExcept handles OP_END_EXCEPT: pop all except handlers for current try block
func (vm *VM) executeEndExcept() {
	frame := vm.CurrentFrame()
	// Pop all except handlers from the stack (they were pushed by the most recent OP_TRY_EXCEPT)
	// We pop from the end until we hit a non-Except handler or empty
	for len(frame.ExceptStack) > 0 {
		top := frame.ExceptStack[len(frame.ExceptStack)-1]
		if top.Type != HandlerExcept {
			break
		}
		frame.ExceptStack = frame.ExceptStack[:len(frame.ExceptStack)-1]
	}
}

// executeTryFinally handles OP_TRY_FINALLY: push a finally handler
func (vm *VM) executeTryFinally() error {
	frame := vm.CurrentFrame()

	// Read finally IP (absolute)
	hi := frame.Program.Code[frame.IP]
	lo := frame.Program.Code[frame.IP+1]
	frame.IP += 2
	finallyIP := int(uint16(hi)<<8 | uint16(lo))

	handler := Handler{
		Type:      HandlerFinally,
		HandlerIP: finallyIP,
		VarIndex:  -1,
	}
	frame.ExceptStack = append(frame.ExceptStack, handler)

	return nil
}

// executeEndFinally handles OP_END_FINALLY.
// This opcode appears twice in try/finally bytecode:
// 1. After the try body (normal path): pop handler from ExceptStack
// 2. After the finally block: re-raise PendingError if set
func (vm *VM) executeEndFinally() error {
	frame := vm.CurrentFrame()

	// If there's a finally handler on top of the stack, pop it (normal path)
	if len(frame.ExceptStack) > 0 {
		top := frame.ExceptStack[len(frame.ExceptStack)-1]
		if top.Type == HandlerFinally {
			frame.ExceptStack = frame.ExceptStack[:len(frame.ExceptStack)-1]
			return nil
		}
	}

	// No finally handler to pop. Check for pending error to re-raise.
	if frame.PendingError != nil {
		err := frame.PendingError
		frame.PendingError = nil
		return err
	}

	return nil
}
