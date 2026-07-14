package vm

import (
	"fmt"

	"barn/builtins"
	"barn/bytecode"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/trace"
	"barn/types"
)

// VM represents the bytecode virtual machine
type VM struct {
	Stack         []types.Value       // Operand stack
	SP            int                 // Stack pointer
	Frames        []*StackFrame       // Call stack
	FP            int                 // Frame pointer
	Store         *dbstore.Store      // Object store
	Builtins      *builtins.Registry  // Builtin function registry
	Context       *kernel.TaskContext // Task context for builtins
	TickLimit     int64               // Maximum ticks before E_MAXREC
	MaxStackDepth int                 // Maximum VM call frames before E_MAXREC
	Ticks         int64               // Current tick count
	PendingWaifs  []types.Value

	frame       *StackFrame  // Cached top of Frames; kept in sync by pushFrame/popFrame
	yielded     bool         // VM has yielded control (suspend/fork)
	yieldResult types.Result // Why we yielded
}

// pushFrame appends a call frame and updates the cached current-frame pointer.
func (vm *VM) pushFrame(f *StackFrame) {
	vm.Frames = append(vm.Frames, f)
	vm.frame = f
}

// popFrame removes the top call frame and updates the cached current-frame
// pointer to the new top (nil when the call stack is empty).
func (vm *VM) popFrame() {
	vm.Frames = vm.Frames[:len(vm.Frames)-1]
	if n := len(vm.Frames); n > 0 {
		vm.frame = vm.Frames[n-1]
	} else {
		vm.frame = nil
	}
}

// StackFrame represents a call frame
type StackFrame struct {
	Program      *bytecode.Program    // Bytecode program
	IP           int                  // Instruction pointer
	BasePointer  int                  // Stack base for this frame
	Locals       []types.Value        // Local variables
	This         types.ObjID          // Current object
	ThisValue    types.Value          // Actual non-object receiver for anonymous/waif/primitive calls; None for objects
	Player       types.ObjID          // Player context
	Verb         string               // Verb name as invoked (the `verb` variable; used by callers()/task_stack())
	StoredVerb   string               // Verb's stored name spec incl. wildcards (e.g. "eval*-d"); used by printed tracebacks
	Caller       types.ObjID          // Calling object
	VerbLoc      types.ObjID          // Object where the current verb is defined (for pass())
	Args         []types.Value        // Original args passed to this verb (for pass() inheritance)
	LoopStack    []bytecode.LoopState // Nested loop state
	ExceptStack  []bytecode.Handler   // Exception handlers
	PendingError error                // Error saved during finally execution
	VerbDebug    bool                 // Verb's 'd' flag: when false, runtime errors are pushed as values instead of raising exceptions

	// Saved context fields — restored when this frame is popped (Return / HandleError).
	// Only set for verb-call frames (not the initial frame).
	IsVerbCall      bool        // True if this frame was pushed by executeCallVerb
	IsEvalFrame     bool        // True if this frame was pushed by eval() builtin
	SavedThisObj    types.ObjID // ctx.ThisObj before verb call
	SavedThisValue  types.Value // ctx.ThisValue before verb call
	SavedVerb       string      // ctx.Verb before verb call
	SavedProgrammer types.ObjID // ctx.Programmer before verb call
	SavedIsWizard   bool        // ctx.IsWizard before verb call
}

// NewVM creates a new virtual machine
func NewVM(store *dbstore.Store, registry *builtins.Registry) *VM {
	return &VM{
		Stack:         make([]types.Value, 0, 256),
		SP:            0,
		Frames:        make([]*StackFrame, 0, 16),
		FP:            0,
		Store:         store,
		Builtins:      registry,
		TickLimit:     30000,
		MaxStackDepth: 50,
		Ticks:         0,
	}
}

func (vm *VM) checkFrameLimit() error {
	if vm.MaxStackDepth > 0 && len(vm.Frames) >= vm.MaxStackDepth {
		return MooError{Code: types.E_MAXREC}
	}
	return nil
}

// Run executes a program and returns the result.
// The returned Result encodes the flow control: FlowReturn for normal completion,
// FlowException for uncaught errors, FlowSuspend when a suspend() yields control,
// and FlowFork when a fork statement yields control.
func (vm *VM) Run(prog *bytecode.Program) types.Result {
	vm.ensureContextDependencies()

	// Create initial frame
	frame := &StackFrame{
		Program:     prog,
		IP:          0,
		BasePointer: vm.SP,
		Locals:      make([]types.Value, prog.NumLocals),
		This:        types.ObjNothing,
		ThisValue:   types.None,
		Player:      types.ObjNothing,
		Verb:        "",
		Caller:      types.ObjNothing,
		LoopStack:   make([]bytecode.LoopState, 0, 4),
		ExceptStack: make([]bytecode.Handler, 0, 4),
		VerbDebug:   true, // Default: errors propagate as exceptions
	}

	// Initialize locals to unbound (reading before assignment raises E_VARNF)
	for i := range frame.Locals {
		frame.Locals[i] = types.Unbound
	}

	vm.pushFrame(frame)
	vm.FP = 0
	vm.syncContextTicks()

	return vm.executeLoop()
}

// RunWithVerbContext executes a program with verb context variables pre-populated
// in the initial frame. This is used by the scheduler for top-level verb execution
// (command verbs and server hooks like do_login_command).
func (vm *VM) RunWithVerbContext(prog *bytecode.Program, thisObj types.ObjID, player types.ObjID, caller types.ObjID, verbName string, verbLoc types.ObjID, args []types.Value) types.Result {
	vm.ensureContextDependencies()

	frame := vm.PrepareVerbFrame(prog, thisObj, player, caller, verbName, verbLoc, args)

	// Pre-populate verb context variables
	SetLocalByName(frame, prog, "this", types.NewObj(thisObj))
	SetLocalByName(frame, prog, "player", types.NewObj(player))
	SetLocalByName(frame, prog, "caller", types.NewObj(caller))
	SetLocalByName(frame, prog, "verb", types.NewStr(verbName))
	SetLocalByName(frame, prog, "args", types.NewList(args))
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

func (vm *VM) ensureContextDependencies() {
	if vm.Context == nil {
		vm.Context = kernel.NewTaskContext()
	}
	vm.Context.Store = vm.Store
	vm.Context.Registry = vm.Builtins
}

// PrepareVerbFrame creates and pushes an initial frame for a verb without starting
// execution. Returns the frame so the caller can set additional local variables
// (e.g. argstr, dobjstr, etc.) before calling ExecuteLoop().
func (vm *VM) PrepareVerbFrame(prog *bytecode.Program, thisObj types.ObjID, player types.ObjID, caller types.ObjID, verbName string, verbLoc types.ObjID, args []types.Value) *StackFrame {
	frame := &StackFrame{
		Program:     prog,
		IP:          0,
		BasePointer: vm.SP,
		Locals:      make([]types.Value, prog.NumLocals),
		This:        thisObj,
		ThisValue:   types.None,
		Player:      player,
		Verb:        verbName,
		Caller:      caller,
		VerbLoc:     verbLoc,
		Args:        args,
		LoopStack:   make([]bytecode.LoopState, 0, 4),
		ExceptStack: make([]bytecode.Handler, 0, 4),
		VerbDebug:   true, // Default: errors propagate as exceptions
	}

	// Initialize locals to unbound (reading before assignment raises E_VARNF)
	for i := range frame.Locals {
		frame.Locals[i] = types.Unbound
	}

	vm.pushFrame(frame)
	vm.FP = 0
	return frame
}

// ExecuteLoop starts the VM's execution loop. Use this after PrepareVerbFrame
// to begin execution after setting up initial variables.
func (vm *VM) ExecuteLoop() types.Result {
	vm.ensureContextDependencies()
	return vm.executeLoop()
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
				SetLocalByName(frame, frame.Program, varName, types.NewInt(childTaskID))
			}
		}
	}
}

// executeLoop is the core execution loop shared by Run() and Resume().
func (vm *VM) executeLoop() types.Result {
	// Hot path. Two invariants let this loop skip per-opcode bookkeeping that
	// earlier revisions paid for on every single instruction:
	//
	//  1. vm.frame is nil exactly when vm.Frames is empty (maintained by
	//     pushFrame/popFrame), so the loop condition is the cheap pointer test
	//     `vm.frame != nil` instead of reloading len(vm.Frames) each iteration.
	//
	//  2. TERMINATOR INVARIANT: every compiled program ends in a frame-popping
	//     terminal opcode (OP_RETURN or OP_RETURN_NONE). The compiler emits one
	//     at the end of every program / verb / eval body (see
	//     bytecode/compiler.go Compile + CompileStatements) and
	//     Program.ExtractForkBody appends OP_RETURN_NONE to fork sub-programs.
	//     Execution therefore can never "fall off the end" of Code: it always
	//     reaches the terminator, which pops the frame and (for OP_RETURN_NONE)
	//     yields the MOO default of 0. This lets the loop fetch Code[IP]
	//     directly and drop the per-opcode `IP >= len(Code)` bounds check that
	//     used to be the single hottest line in the interpreter (~13% of CPU).
	//     Guarded by TestFallOffEndReturnsZero, TestEmptyProgramReturnsZero, and
	//     TestEveryCompiledProgramEndsWithTerminator.
	for vm.frame != nil {
		// Fetch the cached current frame once and dispatch the next opcode
		// directly, avoiding repeated CurrentFrame lookups on the hottest path.
		cur := vm.frame
		op := bytecode.OpCode(cur.Program.Code[cur.IP])
		cur.IP++
		if bytecode.CountsTick(op) {
			vm.Ticks++
			vm.syncContextTicks()
		}
		err := vm.Execute(op)
		if err != nil {
			// Verb debug flag check: when the current frame's VerbDebug is false,
			// push the error as a value instead of propagating it as an exception.
			// This applies to ALL errors including explicit raise().
			// Matches Toast's PUSH_ERROR/RAISE_ERROR macro behavior in execute.cc.
			frame := vm.CurrentFrame()
			if frame != nil && !frame.VerbDebug {
				var errCode types.ErrorCode
				if vmErr, ok := err.(VMException); ok {
					errCode = vmErr.Code
				} else if mooErr, ok := err.(MooError); ok {
					errCode = mooErr.Code
				} else {
					errCode = extractErrorCode(err)
					if errCode == types.E_NONE {
						errCode = types.E_EXEC
					}
				}
				vm.Push(types.NewErr(errCode))
				continue
			}

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

	return types.Result{Flow: types.FlowReturn, Val: types.NewInt(0)}
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

	// VM frames map 1:1 to task CallStack entries (the initial frame pushed
	// by the scheduler is both VM frame 0 and CallStack entry 0).
	var lineNumbers []int
	for _, frame := range vm.Frames {
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

// Step executes a single instruction
func (vm *VM) Step() error {
	frame := vm.CurrentFrame()
	if frame == nil {
		return fmt.Errorf("no active frame")
	}

	if frame.IP >= len(frame.Program.Code) {
		// End of program - implicit return 0
		vm.Return(types.NewInt(0))
		return nil
	}

	op := bytecode.OpCode(frame.Program.Code[frame.IP])
	frame.IP++

	// Count ticks for expensive operations
	if bytecode.CountsTick(op) {
		vm.Ticks++
		vm.syncContextTicks()
	}

	return vm.Execute(op)
}

// Execute dispatches an opcode
func (vm *VM) Execute(op bytecode.OpCode) error {
	// Check for immediate integer
	if bytecode.IsImmediateInt(op) {
		val := bytecode.GetImmediateValue(op)
		vm.Push(types.NewInt(int64(val)))
		return nil
	}

	switch op {
	// Stack operations
	case bytecode.OP_PUSH:
		idx := vm.ReadByte()
		vm.Push(vm.CurrentFrame().Program.Constants[idx])

	case bytecode.OP_POP:
		vm.Pop()

	case bytecode.OP_DUP:
		vm.Push(vm.Peek(0))

	// Variable operations
	case bytecode.OP_GET_VAR:
		idx := vm.ReadByte()
		val := vm.CurrentFrame().Locals[idx]
		if val.IsUnbound() {
			return MooError{Code: types.E_VARNF}
		}
		vm.Push(val)

	case bytecode.OP_SET_VAR:
		idx := vm.ReadByte()
		vm.CurrentFrame().Locals[idx] = vm.Pop()

	// Property operations
	case bytecode.OP_GET_PROP:
		return vm.executeGetProp()
	case bytecode.OP_SET_PROP:
		return vm.executeSetProp()

	// Arithmetic operations
	case bytecode.OP_ADD:
		return vm.executeAdd()
	case bytecode.OP_STRING_APPEND:
		return vm.executeStringAppend()
	case bytecode.OP_SUB:
		return vm.executeSub()
	case bytecode.OP_MUL:
		return vm.executeMul()
	case bytecode.OP_DIV:
		return vm.executeDiv()
	case bytecode.OP_MOD:
		return vm.executeMod()
	case bytecode.OP_POW:
		return vm.executePow()
	case bytecode.OP_NEG:
		return vm.executeNeg()

	// Comparison operations
	case bytecode.OP_EQ:
		return vm.executeEq()
	case bytecode.OP_NE:
		return vm.executeNe()
	case bytecode.OP_LT:
		return vm.executeLt()
	case bytecode.OP_LE:
		return vm.executeLe()
	case bytecode.OP_GT:
		return vm.executeGt()
	case bytecode.OP_GE:
		return vm.executeGe()
	case bytecode.OP_IN:
		return vm.executeIn()

	// Logical operations
	case bytecode.OP_NOT:
		return vm.executeNot()
	case bytecode.OP_AND:
		return vm.executeAnd()
	case bytecode.OP_OR:
		return vm.executeOr()

	// Bitwise operations
	case bytecode.OP_BITOR:
		return vm.executeBitOr()
	case bytecode.OP_BITAND:
		return vm.executeBitAnd()
	case bytecode.OP_BITXOR:
		return vm.executeBitXor()
	case bytecode.OP_BITNOT:
		return vm.executeBitNot()
	case bytecode.OP_SHL:
		return vm.executeShl()
	case bytecode.OP_SHR:
		return vm.executeShr()

	// Control flow
	case bytecode.OP_JUMP:
		offset := vm.ReadShort()
		vm.CurrentFrame().IP += int(offset)

	case bytecode.OP_JUMP_IF_FALSE:
		offset := vm.ReadShort()
		if !vm.Pop().Truthy() {
			vm.CurrentFrame().IP += int(offset)
		}

	case bytecode.OP_JUMP_IF_TRUE:
		offset := vm.ReadShort()
		if vm.Pop().Truthy() {
			vm.CurrentFrame().IP += int(offset)
		}

	case bytecode.OP_RETURN:
		val := vm.Pop()
		vm.Return(val)

	case bytecode.OP_LOOP:
		offset := vm.ReadShort()
		vm.CurrentFrame().IP -= int(offset)

	case bytecode.OP_FOR_RANGE_CHECK:
		// Range-for condition, fused: if Locals[valueVar] > Locals[endVar], jump to exit.
		// Replicates GET_VAR/GET_VAR/LE/JUMP_IF_FALSE using the same compare semantics.
		valueIdx := vm.ReadByte()
		endIdx := vm.ReadByte()
		offset := vm.ReadShort()
		frame := vm.CurrentFrame()
		cmp, err := compareValues(frame.Locals[valueIdx], frame.Locals[endIdx], vm.promoting())
		if err != nil {
			return err
		}
		if cmp > 0 {
			frame.IP += int(offset)
		}

	case bytecode.OP_FOR_RANGE_NEXT:
		// Range-for increment + loop back, fused: Locals[valueVar] += 1; IP -= offset.
		// Replicates GET_VAR/IMM(1)/ADD/SET_VAR/LOOP; integer fast path, E_TYPE otherwise
		// (matching OP_ADD: only int+int is valid for a +1 increment).
		valueIdx := vm.ReadByte()
		offset := vm.ReadShort()
		frame := vm.CurrentFrame()
		cur := frame.Locals[valueIdx]
		if cur.Type() != types.TYPE_INT {
			return fmt.Errorf("E_TYPE: invalid operands for +")
		}
		frame.Locals[valueIdx] = types.NewInt(cur.Int() + 1)
		frame.IP -= int(offset)

	case bytecode.OP_FOR_LIST_LOAD:
		// for-in element load, fused: value = normalizedList[idx], unwrapping the
		// {value,key} pair when the iteration is over pairs. idx is provably in
		// [1..len] (FOR_RANGE_CHECK gates it), so no bounds-check dispatch needed.
		listIdx := vm.ReadByte()
		elemIdx := vm.ReadByte()
		valueIdx := vm.ReadByte()
		isPairsIdx := vm.ReadByte()
		frame := vm.CurrentFrame()
		list := frame.Locals[listIdx]
		if list.Type() != types.TYPE_LIST {
			return fmt.Errorf("E_TYPE: for loop iterator is not a list")
		}
		elem := list.Get(int(frame.Locals[elemIdx].Int()))
		if frame.Locals[isPairsIdx].Truthy() {
			if elem.Type() != types.TYPE_LIST {
				return MooError{Code: types.E_TYPE}
			}
			elem = elem.Get(1)
		}
		frame.Locals[valueIdx] = elem

	case bytecode.OP_FOR_LIST_LOAD_KV:
		// for-in k,v element load, fused: elem={value,key}=normalizedList[idx];
		// value=elem[1]; index=elem[2]. Elements are always pairs here.
		listIdx := vm.ReadByte()
		elemIdx := vm.ReadByte()
		valueIdx := vm.ReadByte()
		indexIdx := vm.ReadByte()
		frame := vm.CurrentFrame()
		list := frame.Locals[listIdx]
		if list.Type() != types.TYPE_LIST {
			return fmt.Errorf("E_TYPE: for loop iterator is not a list")
		}
		pair := list.Get(int(frame.Locals[elemIdx].Int()))
		if pair.Type() != types.TYPE_LIST {
			return MooError{Code: types.E_TYPE}
		}
		frame.Locals[valueIdx] = pair.Get(1)
		frame.Locals[indexIdx] = pair.Get(2)

	case bytecode.OP_RETURN_NONE:
		vm.Return(types.NewInt(0))

	// Collection operations
	case bytecode.OP_INDEX:
		return vm.executeIndex()
	case bytecode.OP_INDEX_SET:
		return vm.executeIndexSet()
	case bytecode.OP_RANGE:
		return vm.executeRange()
	case bytecode.OP_RANGE_SET:
		return vm.executeRangeSet()
	case bytecode.OP_MAKE_LIST:
		return vm.executeMakeList()
	case bytecode.OP_MAKE_MAP:
		return vm.executeMakeMap()
	case bytecode.OP_LENGTH:
		return vm.executeLength()
	case bytecode.OP_INDEX_MARKER:
		return vm.executeIndexMarker()
	case bytecode.OP_LIST_RANGE:
		return vm.executeListRange()
	case bytecode.OP_LIST_APPEND:
		return vm.executeListAppend()
	case bytecode.OP_LIST_EXTEND:
		return vm.executeListExtend()
	case bytecode.OP_SPLICE:
		return vm.executeSplice()

	// Scatter assignment
	case bytecode.OP_SCATTER:
		return vm.executeScatter()

	// Iteration preparation
	case bytecode.OP_ITER_PREP:
		return vm.executeIterPrep()

	// Builtin calls
	case bytecode.OP_CALL_BUILTIN:
		return vm.executeCallBuiltin()

	// Verb calls
	case bytecode.OP_CALL_VERB:
		return vm.executeCallVerb()

	// Fork
	case bytecode.OP_FORK:
		return vm.executeFork()

	// Pass (parent verb call)
	case bytecode.OP_PASS:
		return vm.executePass()

	// Exception handling
	case bytecode.OP_TRY_EXCEPT:
		return vm.executeTryExcept()
	case bytecode.OP_END_EXCEPT:
		return vm.executeEndExcept()
	case bytecode.OP_TRY_FINALLY:
		return vm.executeTryFinally()
	case bytecode.OP_END_FINALLY:
		return vm.executeEndFinally()

	default:
		return fmt.Errorf("unknown opcode: %s (%d)", op.String(), op)
	}

	return nil
}

// CurrentFrame returns the current stack frame
func (vm *VM) CurrentFrame() *StackFrame {
	return vm.frame
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

// HandleError handles an error by looking for exception handlers.
// Searches the current frame's ExceptStack first, then unwinds through caller
// frames if no handler is found. This supports cross-frame exception propagation
// for native verb calls.
func (vm *VM) HandleError(err error) bool {
	// Extract error code
	errCode := types.E_NONE
	exceptionValue := types.None
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
	// Toast includes the eval'd-code activation in the traceback when the error
	// is caught at (or unwinds to) the eval frame, but not when a verb above the
	// eval frame catches it first. Decide which case we're in before unwinding.
	traceback := vm.buildTraceback(!vm.matchingExceptAboveEvalFrame(errCode))

	// Build or augment the 4-element exception value: {code, message, value, traceback}
	if exceptionValue.IsNone() {
		exceptionValue = types.NewList([]types.Value{
			types.NewErr(errCode),
			types.NewStr(errCode.Message()),
			types.NewInt(0),
			traceback,
		})
	} else if exceptionValue.Type() == types.TYPE_LIST {
		// raise() produces a 3-element list; append traceback as 4th element.
		elems := make([]types.Value, 0, 4)
		for i := 1; i <= exceptionValue.Len() && i <= 3; i++ {
			elems = append(elems, exceptionValue.Get(i))
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

			if handler.Type == bytecode.HandlerFinally {
				// Finally handler: run the finally block, then re-raise the error.
				// Pop this handler and everything above it.
				frame.ExceptStack = frame.ExceptStack[:i]
				// Save the pending error so after finally runs, we re-raise it
				frame.PendingError = err
				frame.IP = handler.HandlerIP
				return true
			}

			if handler.Type == bytecode.HandlerExcept && handler.Matches(errCode) {
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

		// Eval frame boundary: catch the error and wrap as {0, error}.
		// This matches Toast's bf_eval_callback which catches all errors
		// from eval'd code and returns them as the eval() result.
		// Exception: E_QUOTA/E_MAXREC propagate through to kill the task.
		if frame.IsEvalFrame && errCode != types.E_QUOTA && errCode != types.E_MAXREC {
			// Restore context, pop the eval frame, and keep unwinding so the
			// runtime error propagates to eval()'s caller.
			if vm.Context != nil {
				vm.Context.ThisObj = frame.SavedThisObj
				vm.Context.ThisValue = frame.SavedThisValue
				vm.Context.Verb = frame.SavedVerb
				vm.Context.Programmer = frame.SavedProgrammer
				vm.Context.IsWizard = frame.SavedIsWizard
			}
			if vm.Context != nil && vm.Context.Task != nil {
				if t, ok := vm.Context.Task.(*task.Task); ok {
					t.PopFrame()
				}
			}
			vm.SP = frame.BasePointer
			vm.popFrame()
			continue
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
		vm.popFrame()
		// Continue searching in the caller frame
	}

	// No frames left
	return false
}
