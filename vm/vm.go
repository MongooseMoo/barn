package vm

import (
	"barn/builtins"
	"barn/db"
	"barn/task"
	"barn/trace"
	"barn/types"
	"fmt"
	"strings"
	"time"
)

// MooError wraps an ErrorCode as a Go error
type MooError struct {
	Code types.ErrorCode
}

func (e MooError) Error() string {
	return fmt.Sprintf("E_%d", e.Code)
}

// VMException carries a structured exception value alongside an error code.
// Used for propagating builtin raise() payloads into except variables.
type VMException struct {
	Code  types.ErrorCode
	Value types.Value
}

func (e VMException) Error() string {
	return e.Code.String()
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
	Stack     []types.Value      // Operand stack
	SP        int                // Stack pointer
	Frames    []*StackFrame      // Call stack
	FP        int                // Frame pointer
	Store     *db.Store          // Object store
	Builtins  *builtins.Registry // Builtin function registry
	Context   *types.TaskContext // Task context for builtins
	TickLimit int64              // Maximum ticks before E_MAXREC
	Ticks     int64              // Current tick count

	yielded     bool         // VM has yielded control (suspend/fork)
	yieldResult types.Result // Why we yielded
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
	VerbLoc      types.ObjID   // Object where the current verb is defined (for pass())
	Args         []types.Value // Original args passed to this verb (for pass() inheritance)
	LoopStack    []LoopState   // Nested loop state
	ExceptStack  []Handler     // Exception handlers
	PendingError error         // Error saved during finally execution

	// Saved context fields — restored when this frame is popped (Return / HandleError).
	// Only set for verb-call frames (not the initial frame).
	IsVerbCall      bool        // True if this frame was pushed by executeCallVerb
	SavedThisObj    types.ObjID // ctx.ThisObj before verb call
	SavedThisValue  types.Value // ctx.ThisValue before verb call
	SavedVerb       string      // ctx.Verb before verb call
	SavedProgrammer types.ObjID // ctx.Programmer before verb call
	SavedIsWizard   bool        // ctx.IsWizard before verb call
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

// Run executes a program and returns the result.
// The returned Result encodes the flow control: FlowReturn for normal completion,
// FlowException for uncaught errors, FlowSuspend when a suspend() yields control,
// and FlowFork when a fork statement yields control.
func (vm *VM) Run(prog *Program) types.Result {
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

	// Initialize locals to unbound (reading before assignment raises E_VARNF)
	for i := range frame.Locals {
		frame.Locals[i] = types.UnboundValue{}
	}

	vm.Frames = append(vm.Frames, frame)
	vm.FP = 0
	vm.syncContextTicks()

	return vm.executeLoop()
}

// RunWithVerbContext executes a program with verb context variables pre-populated
// in the initial frame. This is used by the scheduler for top-level verb execution
// (command verbs and server hooks like do_login_command).
func (vm *VM) RunWithVerbContext(prog *Program, thisObj types.ObjID, player types.ObjID, caller types.ObjID, verbName string, verbLoc types.ObjID, args []types.Value) types.Result {
	frame := vm.PrepareVerbFrame(prog, thisObj, player, caller, verbName, verbLoc, args)

	// Pre-populate verb context variables
	setLocalByName(frame, prog, "this", types.NewObj(thisObj))
	setLocalByName(frame, prog, "player", types.NewObj(player))
	setLocalByName(frame, prog, "caller", types.NewObj(caller))
	setLocalByName(frame, prog, "verb", types.NewStr(verbName))
	setLocalByName(frame, prog, "args", types.NewList(args))
	vm.syncContextTicks()

	return vm.executeLoop()
}

func (vm *VM) syncContextTicks() {
	if vm.Context == nil {
		return
	}
	left := vm.TickLimit - vm.Ticks
	if left < 0 {
		left = 0
	}
	vm.Context.TicksRemaining = left
}

// PrepareVerbFrame creates and pushes an initial frame for a verb without starting
// execution. Returns the frame so the caller can set additional local variables
// (e.g. argstr, dobjstr, etc.) before calling ExecuteLoop().
func (vm *VM) PrepareVerbFrame(prog *Program, thisObj types.ObjID, player types.ObjID, caller types.ObjID, verbName string, verbLoc types.ObjID, args []types.Value) *StackFrame {
	frame := &StackFrame{
		Program:     prog,
		IP:          0,
		BasePointer: vm.SP,
		Locals:      make([]types.Value, prog.NumLocals),
		This:        thisObj,
		Player:      player,
		Verb:        verbName,
		Caller:      caller,
		VerbLoc:     verbLoc,
		Args:        args,
		LoopStack:   make([]LoopState, 0, 4),
		ExceptStack: make([]Handler, 0, 4),
	}

	// Initialize locals to unbound (reading before assignment raises E_VARNF)
	for i := range frame.Locals {
		frame.Locals[i] = types.UnboundValue{}
	}

	vm.Frames = append(vm.Frames, frame)
	vm.FP = 0
	return frame
}

// ExecuteLoop starts the VM's execution loop. Use this after PrepareVerbFrame
// to begin execution after setting up initial variables.
func (vm *VM) ExecuteLoop() types.Result {
	return vm.executeLoop()
}

// SetLocalByNamePublic is a public wrapper for setLocalByName, allowing the scheduler
// to set local variables in a frame before execution starts.
func SetLocalByNamePublic(frame *StackFrame, prog *Program, name string, value types.Value) {
	setLocalByName(frame, prog, name, value)
}

// IsYielded returns whether the VM has yielded (suspended or forked) and needs Resume().
func (vm *VM) IsYielded() bool {
	return vm.yielded
}

// Resume continues execution after a yield (suspend or fork).
// The VM's PC and stack are still intact from the yield point.
func (vm *VM) Resume() types.Result {
	vm.yielded = false
	vm.yieldResult = types.Result{}
	return vm.executeLoop()
}

// SetResumeValue replaces the top-of-stack value that was pushed when a
// builtin returned FlowSuspend. By default the VM pushes 0 (correct for
// suspend()), but read() needs to deliver the input line string. Call this
// before Resume().
func (vm *VM) SetResumeValue(val types.Value) {
	if vm.SP > 0 {
		vm.Stack[vm.SP-1] = val
	}
}

// SetForkResult sets the fork variable in the current frame to the child task ID.
// This should be called after the scheduler creates the child task, before Resume().
func (vm *VM) SetForkResult(childTaskID int64) {
	if vm.yieldResult.Flow == types.FlowFork && vm.yieldResult.ForkInfo != nil {
		varName := vm.yieldResult.ForkInfo.VarName
		if varName != "" {
			frame := vm.CurrentFrame()
			if frame != nil {
				setLocalByName(frame, frame.Program, varName, types.NewInt(childTaskID))
			}
		}
	}
}

// executeLoop is the core execution loop shared by Run() and Resume().
func (vm *VM) executeLoop() types.Result {
	for len(vm.Frames) > 0 {
		if err := vm.Step(); err != nil {
			// Capture line number before HandleError may pop frames
			line := vm.CurrentLine()
			// Snapshot activation stack before unwind so callers can inspect
			// the full trace on uncaught exceptions.
			var stackSnapshot interface{}
			vmStack := vm.snapshotActivationFrames(line)
			if len(vmStack) > 0 {
				stackSnapshot = vmStack
			} else if vm.Context != nil && vm.Context.Task != nil {
				if t, ok := vm.Context.Task.(*task.Task); ok {
					stackSnapshot = t.GetCallStack()
				}
			}
			// Handle error
			if !vm.HandleError(err) {
				// Extract error code, preferring the typed MooError
				var errCode types.ErrorCode
				if mooErr, ok := err.(MooError); ok {
					errCode = mooErr.Code
				} else if vmErr, ok := err.(VMException); ok {
					errCode = vmErr.Code
				} else {
					errCode = extractErrorCode(err)
					if errCode == types.E_NONE {
						errCode = types.E_EXEC
					}
				}
				return types.Result{
					Flow:      types.FlowException,
					Error:     errCode,
					Val:       types.NewStr(vm.annotateError(err, line).Error()),
					CallStack: stackSnapshot,
				}
			}
		}

		// Check if VM yielded (suspend/fork)
		if vm.yielded {
			// Sync line numbers so task_stack() reports accurate lines
			// for suspended tasks.
			vm.syncTaskLineNumbers()
			return vm.yieldResult
		}

		// Check tick limit
		if vm.Ticks >= vm.TickLimit {
			line := vm.CurrentLine()
			_ = vm.annotateError(fmt.Errorf("E_MAXREC: tick limit exceeded"), line)
			return types.Result{
				Flow:  types.FlowException,
				Error: types.E_MAXREC,
				Val:   types.NewStr("E_MAXREC: tick limit exceeded"),
			}
		}
	}

	// Return result
	if vm.SP > 0 {
		return types.Result{Flow: types.FlowReturn, Val: vm.Pop()}
	}

	return types.Result{Flow: types.FlowReturn, Val: types.IntValue{Val: 0}}
}

// syncTaskLineNumbers updates the task's CallStack line numbers from the VM's
// current frame IPs.  This must be called before any code that reads
// task.CallStack line numbers (callers(), task_stack(), traceback building).
func (vm *VM) syncTaskLineNumbers() {
	if vm.Context == nil || vm.Context.Task == nil {
		return
	}
	t, ok := vm.Context.Task.(*task.Task)
	if !ok {
		return
	}

	// VM verb-call frames map 1:1 to task CallStack entries (the initial
	// eval frame has IsVerbCall=false and is not in the task CallStack).
	var lineNumbers []int
	for _, frame := range vm.Frames {
		if !frame.IsVerbCall {
			continue
		}
		line := 1
		if frame.Program != nil {
			ip := frame.IP - 1
			if ip < 0 {
				ip = 0
			}
			line = frame.Program.LineForIP(ip)
		}
		if line < 1 {
			line = 1
		}
		lineNumbers = append(lineNumbers, line)
	}
	t.UpdateCallStackLineNumbers(lineNumbers)
}

// buildTraceback returns a MOO list of stack frames suitable for the 4th
// element of a caught exception value.  Frames are ordered innermost-first
// (the verb where the error occurred comes first).  Only real verb frames
// are included — eval infrastructure is excluded.
func (vm *VM) buildTraceback() types.Value {
	if vm.Context == nil || vm.Context.Task == nil {
		return types.NewList([]types.Value{})
	}
	t, ok := vm.Context.Task.(*task.Task)
	if !ok {
		return types.NewList([]types.Value{})
	}

	stack := t.GetCallStack()
	frames := make([]types.Value, 0, len(stack))
	for i := len(stack) - 1; i >= 0; i-- {
		f := stack[i]
		if f.ServerInitiated {
			continue
		}
		frames = append(frames, f.ToList())
	}
	return types.NewList(frames)
}

// snapshotActivationFrames captures the current VM call chain as activation
// frames for traceback formatting.
func (vm *VM) snapshotActivationFrames(topLine int) []task.ActivationFrame {
	if len(vm.Frames) == 0 {
		return nil
	}

	stack := make([]task.ActivationFrame, 0, len(vm.Frames))
	for i, frame := range vm.Frames {
		line := 1
		if i == len(vm.Frames)-1 {
			line = topLine
		} else if frame.Program != nil {
			// For caller frames, IP points at the next instruction to execute.
			// Use IP-1 so traceback lines point at the call site that led here.
			ip := frame.IP - 1
			if ip < 0 {
				ip = 0
			}
			line = frame.Program.LineForIP(ip)
		}

		stack = append(stack, task.ActivationFrame{
			This:       frame.This,
			ThisValue:  nil,
			Player:     frame.Player,
			Programmer: types.ObjNothing,
			Caller:     frame.Caller,
			Verb:       frame.Verb,
			VerbLoc:    frame.VerbLoc,
			Args:       frame.Args,
			LineNumber: line,
			SourceLine: vm.sourceLineForFrame(frame, line),
		})
	}

	return stack
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
		vm.syncContextTicks()
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
		val := vm.CurrentFrame().Locals[idx]
		if _, unbound := val.(types.UnboundValue); unbound {
			return MooError{Code: types.E_VARNF}
		}
		vm.Push(val)

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
	case OP_INDEX_MARKER:
		return vm.executeIndexMarker()
	case OP_LIST_RANGE:
		return vm.executeListRange()
	case OP_LIST_APPEND:
		return vm.executeListAppend()
	case OP_LIST_EXTEND:
		return vm.executeListExtend()
	case OP_SPLICE:
		return vm.executeSplice()

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

	// Fork
	case OP_FORK:
		return vm.executeFork()

	// Pass (parent verb call)
	case OP_PASS:
		return vm.executePass()

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

// CurrentLine returns the source line number for the current instruction pointer.
// Returns 0 if no line information is available.
func (vm *VM) CurrentLine() int {
	frame := vm.CurrentFrame()
	if frame == nil || frame.Program == nil {
		return 0
	}
	// IP has already been incremented past the opcode, so use IP-1
	// to find the line for the instruction being executed.
	ip := frame.IP - 1
	if ip < 0 {
		ip = 0
	}
	return frame.Program.LineForIP(ip)
}

// annotateError wraps an error with source line information if available.
// If the line is 0 (no line info), the original error is returned unchanged.
func (vm *VM) annotateError(err error, line int) error {
	if line > 0 {
		return fmt.Errorf("%w (line %d)", err, line)
	}
	return err
}

func (vm *VM) sourceLineForFrame(frame *StackFrame, line int) string {
	if frame == nil || frame.Program == nil || line <= 0 {
		return ""
	}
	if line > len(frame.Program.Source) {
		return ""
	}
	return strings.TrimSpace(frame.Program.Source[line-1])
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

	// If this was a verb-call frame, restore context and pop activation frame
	if frame.IsVerbCall && vm.Context != nil {
		trace.VerbReturn(frame.This, frame.Verb, value)
		vm.Context.ThisObj = frame.SavedThisObj
		vm.Context.ThisValue = frame.SavedThisValue
		vm.Context.Verb = frame.SavedVerb
		vm.Context.Programmer = frame.SavedProgrammer
		vm.Context.IsWizard = frame.SavedIsWizard

		// Pop activation frame from task call stack
		if vm.Context.Task != nil {
			if t, ok := vm.Context.Task.(*task.Task); ok {
				t.PopFrame()
			}
		}
	}

	vm.SP = frame.BasePointer
	vm.Frames = vm.Frames[:len(vm.Frames)-1]
	vm.Push(value)
}

// HandleError handles an error by looking for exception handlers.
// Searches the current frame's ExceptStack first, then unwinds through caller
// frames if no handler is found. This supports cross-frame exception propagation
// for native verb calls.
func (vm *VM) HandleError(err error) bool {
	// Extract error code
	errCode := types.E_NONE
	var exceptionValue types.Value
	if vmErr, ok := err.(VMException); ok {
		errCode = vmErr.Code
		exceptionValue = vmErr.Value
	} else if mooErr, ok := err.(MooError); ok {
		errCode = mooErr.Code
	} else {
		// Try to parse error code from error message (e.g. "E_DIV: division by zero")
		errCode = extractErrorCode(err)
	}

	// Snapshot traceback BEFORE any unwinding.  Sync line numbers first so
	// the traceback contains accurate call-site lines.
	vm.syncTaskLineNumbers()
	traceback := vm.buildTraceback()

	// Build or augment the 4-element exception value: {code, message, value, traceback}
	if exceptionValue == nil {
		exceptionValue = types.NewList([]types.Value{
			types.NewErr(errCode),
			types.NewStr(errCode.Message()),
			types.NewInt(0),
			traceback,
		})
	} else if listVal, ok := exceptionValue.(types.ListValue); ok {
		// raise() produces a 3-element list; append traceback as 4th element.
		elems := make([]types.Value, 0, 4)
		for i := 1; i <= listVal.Len() && i <= 3; i++ {
			elems = append(elems, listVal.Get(i))
		}
		for len(elems) < 3 {
			elems = append(elems, types.NewInt(0))
		}
		elems = append(elems, traceback)
		exceptionValue = types.NewList(elems)
	}

	// Search through frames from top (current) to bottom (initial)
	for len(vm.Frames) > 0 {
		frame := vm.CurrentFrame()
		if frame == nil {
			return false
		}

		// Search this frame's ExceptStack (innermost handler first)
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
					frame.Locals[handler.VarIndex] = exceptionValue
				}

				return true
			}
		}

		// No handler in this frame. If there are caller frames, pop this frame
		// and continue searching. This implements cross-frame exception unwinding.
		if len(vm.Frames) <= 1 {
			if frame.IsVerbCall {
				trace.Exception(frame.This, frame.Verb, errCode)
			}
			// This is the bottom frame — no more frames to unwind into
			return false
		}

		// Pop the current frame (unwind): reset SP to BasePointer, remove frame.
		// Do NOT push a return value — we're unwinding due to an error.
		// If this was a verb-call frame, restore context and pop activation frame.
		if frame.IsVerbCall && vm.Context != nil {
			trace.Exception(frame.This, frame.Verb, errCode)
			vm.Context.ThisObj = frame.SavedThisObj
			vm.Context.ThisValue = frame.SavedThisValue
			vm.Context.Verb = frame.SavedVerb
			vm.Context.Programmer = frame.SavedProgrammer
			vm.Context.IsWizard = frame.SavedIsWizard

			if vm.Context.Task != nil {
				if t, ok := vm.Context.Task.(*task.Task); ok {
					t.PopFrame()
				}
			}
		}
		vm.SP = frame.BasePointer
		vm.Frames = vm.Frames[:len(vm.Frames)-1]
		// Continue searching in the caller frame
	}

	// No frames left
	return false
}

// executeFork handles OP_FORK: evaluate delay, yield control to the scheduler.
//
// Bytecode format: OP_FORK <varIdx:byte> <bodyLen:short>
// Stack: [delay] (delay value on top)
//
// Yields a FlowFork result with ForkInfo containing the fork body location,
// delay, and variable name. The scheduler should:
//  1. Create the child task (fork body)
//  2. Call SetForkResult(childTaskID) on the VM
//  3. Call Resume() to continue execution after the fork
//
// The fork variable is NOT set here — it is set by SetForkResult() with the
// actual child task ID assigned by the scheduler.
func (vm *VM) executeFork() error {
	varIdx := int(vm.ReadByte())
	bodyLen := vm.ReadShort()

	// Pop and validate the delay value
	delay := vm.Pop()

	var delaySeconds float64
	switch v := delay.(type) {
	case types.IntValue:
		if v.Val < 0 {
			return fmt.Errorf("E_INVARG: fork delay must be non-negative")
		}
		delaySeconds = float64(v.Val)
	case types.FloatValue:
		if v.Val < 0 {
			return fmt.Errorf("E_INVARG: fork delay must be non-negative")
		}
		delaySeconds = v.Val
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

	// Record the fork body's bytecode position for the scheduler.
	// The body starts at the current IP and runs for bodyLen bytes.
	frame := vm.CurrentFrame()
	forkBodyIP := frame.IP
	forkBodyLen := int(bodyLen)

	// Skip over the fork body — the parent continues after the fork
	frame.IP += forkBodyLen

	// Build ForkInfo for the scheduler.
	// Include the parent program and a locals snapshot so the scheduler can
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

	forkInfo := &types.ForkInfo{
		Delay:   time.Duration(delaySeconds * float64(time.Second)),
		VarName: varName,
		Body:    [3]interface{}{frame.Program, forkBodyIP, forkBodyLen}, // parent program, offset, length
		ThisObj: thisObj,
		Player:  playerObj,
		Caller:  callerObj,
		Verb:    verbStr,
		VerbLoc: frame.VerbLoc,
	}
	// Store locals snapshot in Variables map for the scheduler
	forkInfo.Variables = make(map[string]types.Value, len(frame.Program.VarNames))
	for i, name := range frame.Program.VarNames {
		if i < len(localsCopy) {
			forkInfo.Variables[name] = localsCopy[i]
		}
	}

	// Yield to the scheduler
	vm.yielded = true
	vm.yieldResult = types.Result{
		Flow:     types.FlowFork,
		ForkInfo: forkInfo,
	}

	return nil
}

// executeTryExcept handles OP_TRY_EXCEPT: push exception handlers onto ExceptStack
func (vm *VM) executeTryExcept() error {
	frame := vm.CurrentFrame()
	numClauses := int(vm.ReadByte())
	handlers := make([]Handler, numClauses)

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

		handlers[i] = Handler{
			Type:      HandlerExcept,
			HandlerIP: handlerIP,
			Codes:     codes,
			VarIndex:  varIndex,
		}
	}

	// Push in reverse source order so reverse scan in HandleError honors
	// "first matching except clause wins".
	for i := numClauses - 1; i >= 0; i-- {
		frame.ExceptStack = append(frame.ExceptStack, handlers[i])
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
