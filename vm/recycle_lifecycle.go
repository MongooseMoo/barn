package vm

import (
	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/types"
)

type recycleContinuation struct {
	request builtins.RecycleLifecycleRequest
}

func (vm *VM) startRecycleLifecycle(request builtins.RecycleLifecycleRequest) types.Result {
	state := &recycleContinuation{request: request}
	if vm.Context == nil || vm.Context.StoreTxn == nil {
		return vm.resumeRecycleLifecycle(state, types.Err(types.E_VERBNF))
	}
	if _, _, err := findCallableVerbForRead(vm.Context.StoreTxn, request.Object.ID(), "recycle"); err != nil {
		return vm.resumeRecycleLifecycle(state, types.Err(types.E_VERBNF))
	}
	if err := vm.startVerbCall(request.Object, "recycle", nil); err != nil {
		if extractErrorCode(err) == types.E_VERBNF {
			return vm.resumeRecycleLifecycle(state, types.Err(types.E_VERBNF))
		}
		return moveLifecycleErrorResult(err)
	}
	frame := vm.CurrentFrame()
	frame.DiscardReturn = true
	frame.RecycleContinuation = state
	return types.Result{Flow: types.FlowBuiltinPush}
}

func (vm *VM) resumeRecycleLifecycle(state *recycleContinuation, hookResult types.Result) types.Result {
	execution := vm.Builtins.NewExecution(vm.Context, vm.Task)
	return builtins.FinishRecycleLifecycle(execution, state.request, hookResult)
}
