package vm

import (
	"errors"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

// errorObservation inspects handlers before unwinding destroys the stack.
// Finally blocks retain the original error; unnamed except handlers discard it.
func (vm *VM) errorObservation(err error) (caught, observe bool) {
	code := raisedCodeOf(err)
	for frameIndex := len(vm.Frames) - 1; frameIndex >= 0; frameIndex-- {
		frame := vm.Frames[frameIndex]
		for i := len(frame.ExceptStack) - 1; i >= 0; i-- {
			handler := &frame.ExceptStack[i]
			if handler.Type == bytecode.HandlerFinally {
				// Preserve the originating activation before finally unwinds it.
				return true, true
			}
			if vm.handlerMatches(frame, handler, code) {
				return true, handler.VarIndex >= 0
			}
		}
		// Recycling may consume or replace the exception during unwinding.
		if frame.RecycleContinuation != nil {
			return false, true
		}
	}
	return false, true
}

// raisedCode is the code an except arm compares against: the error itself,
// or, for raise() of a non-error value (carried as E_NONE), that value, which
// is the first element of the exception tuple.
func raisedCode(code types.ErrorCode, exception types.Value) types.Value {
	if code == types.E_NONE && exception.Type() == types.TYPE_LIST && exception.Len() >= 1 {
		return exception.Get(1)
	}
	return types.NewErr(code)
}

func raisedCodeOf(err error) types.Value {
	code := errorCode(err)
	var vmErr VMException
	if code == types.E_NONE && errors.As(err, &vmErr) {
		return raisedCode(code, vmErr.Value)
	}
	return types.NewErr(code)
}

// handlerMatches reports whether an except handler in frame catches code, as
// Toast's find_handler_activ (execute.cc) decides: ANY catches everything,
// and a code list catches its members (ismember, case-insensitive).
func (vm *VM) handlerMatches(frame *StackFrame, handler *bytecode.Handler, code types.Value) bool {
	if handler.Type != bytecode.HandlerExcept {
		return false
	}
	if len(handler.Codes) == 0 {
		return true
	}
	if slot, ok := handler.CodesLocal(); ok {
		if slot >= len(frame.Locals) {
			return false
		}
		codes := frame.Locals[slot]
		return codes.Type() == types.TYPE_LIST && vm.listIndex(code, codes) != 0
	}
	if code.Type() != types.TYPE_ERR {
		return false
	}
	for _, errorCode := range handler.Codes {
		if errorCode == code.Code() {
			return true
		}
	}
	return false
}
