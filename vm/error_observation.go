package vm

import (
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/types"
)

// errorObservation inspects handlers before unwinding destroys the stack.
// Finally blocks retain the original error; unnamed except handlers discard it.
func (vm *VM) errorObservation(err error) (caught, observe bool) {
	var code types.ErrorCode
	switch err := err.(type) {
	case VMException:
		code = err.Code
	case MooError:
		code = err.Code
	default:
		code = extractErrorCode(err)
	}
	for frameIndex := len(vm.Frames) - 1; frameIndex >= 0; frameIndex-- {
		frame := vm.Frames[frameIndex]
		for i := len(frame.ExceptStack) - 1; i >= 0; i-- {
			handler := frame.ExceptStack[i]
			if handler.Type == bytecode.HandlerFinally {
				return true, false
			}
			if handler.Type == bytecode.HandlerExcept && handler.Matches(code) {
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
