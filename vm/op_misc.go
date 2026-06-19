package vm

import (
	"barn/db"
	"barn/types"
	"fmt"
)

func (vm *VM) executeCallBuiltin() error {
	funcID := vm.ReadByte()
	argc := vm.ReadByte()

	var args []types.Value
	if argc == 0xFF {
		// Splice mode: args list is on top of stack
		listVal := vm.Pop()
		list, ok := listVal.(types.ListValue)
		if !ok {
			return fmt.Errorf("E_TYPE: expected list for spliced builtin args")
		}
		args = make([]types.Value, list.Len())
		for i := 1; i <= list.Len(); i++ {
			args[i-1] = list.Get(i)
		}
	} else {
		args = vm.PopN(int(argc))
	}

	// Sync task call-stack line numbers so builtins like callers() see
	// accurate values.
	vm.syncTaskLineNumbers()

	// Set CallerVM so builtins like eval() can push frames on this VM
	if vm.Context != nil {
		vm.Context.CallerVM = vm
	}

	// Call builtin
	result := vm.Builtins.CallByID(int(funcID), vm.Context, args)

	// Clear CallerVM after the call
	if vm.Context != nil {
		vm.Context.CallerVM = nil
	}

	if result.Flow == types.FlowException {
		// Propagate builtin exceptions to executeLoop() so traceback capture
		// happens before any stack unwinding.
		return VMException{Code: result.Error, Value: result.Val}
	}

	// Handle FlowEvalPush: eval() pushed a frame on this VM.
	// The new frame is already on vm.Frames — just continue execution.
	if result.Flow == types.FlowEvalPush {
		return nil
	}

	// Handle FlowSuspend: yield control back to the caller (scheduler).
	// Push 0 onto the stack first as the return value of suspend() — when
	// Resume() is called, execution continues after the builtin call with
	// this value already on the stack.
	if result.Flow == types.FlowSuspend {
		vm.Push(types.NewInt(0)) // suspend() returns 0 in MOO
		vm.yielded = true
		vm.yieldResult = result
		return nil
	}

	vm.Push(result.Val)
	return nil
}

// getPrimitivePrototypeFromStore returns the prototype object ID for a primitive value
// by reading the appropriate property (str_proto, int_proto, etc.) from #0.
// Returns ObjNothing if no prototype is configured for this type.
// Primitive prototypes are configured through #0's *_proto properties.
func getPrimitivePrototypeFromStore(store *db.Store, val types.Value) types.ObjID {
	var propName string
	switch val.(type) {
	case types.IntValue:
		propName = "int_proto"
	case types.FloatValue:
		propName = "float_proto"
	case types.StrValue:
		propName = "str_proto"
	case types.ListValue:
		propName = "list_proto"
	case types.MapValue:
		propName = "map_proto"
	case types.ErrValue:
		propName = "err_proto"
	case types.BoolValue:
		propName = "bool_proto"
	default:
		return types.ObjNothing
	}

	propValue, errCode := store.PropertyValue(0, propName)
	if errCode != types.E_NONE {
		return types.ObjNothing
	}

	if objVal, ok := propValue.(types.ObjValue); ok {
		protoID := objVal.ID()
		if store.Valid(protoID) {
			return protoID
		}
	}

	return types.ObjNothing
}
