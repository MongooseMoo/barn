package vm

import (
	"fmt"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func (vm *VM) executeCallBuiltin() error {
	funcID := vm.FetchByte()
	argc := vm.FetchByte()

	var args []types.Value
	if argc == 0xFF {
		// Splice mode: args list is on top of stack
		listVal := vm.Pop()
		if listVal.Type() != types.TYPE_LIST {
			return fmt.Errorf("E_TYPE: expected list for spliced builtin args")
		}
		args = make([]types.Value, listVal.Len())
		for i := 1; i <= listVal.Len(); i++ {
			args[i-1] = listVal.Get(i)
		}
	} else {
		n := int(argc)
		if vm.SP < n {
			panic("stack underflow")
		}
		if n > 0 {
			args = vm.Stack[vm.SP-n : vm.SP]
			vm.SP -= n
		}
	}

	// Sync task call-stack line numbers only for builtins that expose them.
	if vm.Builtins.NeedsLineSyncByID(int(funcID)) {
		vm.syncTaskLineNumbers()
	}

	// Supply runtime services explicitly for this builtin invocation.
	execution := vm.Builtins.NewExecution(vm.Context, vm.Task)
	execution.PushEval = vm.pushEval
	execution.CollectAnonymousRefs = func(out map[types.ObjID]struct{}) {
		CollectAnonymousRefsFromVM(vm, out)
	}
	if vm.Context == nil || !vm.Context.DeferredGC {
		execution.PendingFinalizations = func() []types.Value {
			return CollectPendingFinalizationValues(vm.Store, vm)
		}
	}
	result := vm.Builtins.CallByIDWithExecution(int(funcID), execution, args)
	if vm.Context != nil && vm.Context.BuiltinTicksConsumed != 0 {
		vm.Ticks += vm.Context.BuiltinTicksConsumed
		vm.Context.BuiltinTicksConsumed = 0
		vm.syncContextTicks()
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

	// Handle FlowSuspend: yield control back to the execution engine.
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
func getPrimitivePrototypeFromStore(store *dbstore.Store, txn *dbstore.StoreTxn, val types.Value) types.ObjID {
	var propName string
	switch val.Type() {
	case types.TYPE_INT:
		propName = "int_proto"
	case types.TYPE_FLOAT:
		propName = "float_proto"
	case types.TYPE_STR:
		propName = "str_proto"
	case types.TYPE_LIST:
		propName = "list_proto"
	case types.TYPE_MAP:
		propName = "map_proto"
	case types.TYPE_ERR:
		propName = "err_proto"
	case types.TYPE_BOOL:
		propName = "bool_proto"
	default:
		return types.ObjNothing
	}

	var (
		propValue types.Value
		errCode   types.ErrorCode
	)
	if txn != nil {
		propValue, errCode = txn.PropertyValue(0, propName)
	} else {
		propValue, errCode = store.PropertyValue(0, propName)
	}
	if errCode != types.E_NONE {
		return types.ObjNothing
	}

	if isObjLike(propValue) {
		protoID := propValue.ID()
		if txn != nil && txn.Valid(protoID) {
			return protoID
		}
		if txn == nil && store.Valid(protoID) {
			return protoID
		}
	}

	return types.ObjNothing
}
