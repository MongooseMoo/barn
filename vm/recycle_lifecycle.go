package vm

import (
	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/types"
)

type recycleContinuation struct {
	request      builtins.RecycleLifecycleRequest
	exitCallback bool
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
		if errorCode(err) == types.E_VERBNF {
			return vm.resumeRecycleLifecycle(state, types.Err(types.E_VERBNF))
		}
		return moveLifecycleErrorResult(err)
	}
	frame := vm.CurrentFrame()
	frame.DiscardReturn = true
	frame.RecycleContinuation = state
	vm.markBuiltinCaller("recycle", 2)
	return types.Result{Flow: types.FlowBuiltinPush}
}

// abortAttempt yields FlowAbortAttempt: the runtime is abandoning this attempt
// at its irreversible-effect boundary and re-runs the task. No handler runs
// and nothing the attempt did is kept.
func (vm *VM) abortAttempt() {
	vm.yielded = true
	vm.yieldResult = types.Result{Flow: types.FlowAbortAttempt}
}

func (vm *VM) resumeRecycleLifecycle(state *recycleContinuation, hookResult types.Result) types.Result {
	execution := vm.Builtins.NewExecution(vm.Context, vm.Task)
	if hookResult.IsError() && hookResult.Error != types.E_VERBNF {
		if state.exitCallback {
			vm.Builtins.EndRecycle(state.request.Object.ID())
			return hookResult
		}
		return builtins.FinishRecycleLifecycle(execution, state.request, hookResult)
	}
	for {
		target, content, result := builtins.NextRecycleExit(execution, state.request.Object)
		if result.Flow == types.FlowAbortAttempt {
			return result
		}
		if result.IsError() {
			vm.Builtins.EndRecycle(state.request.Object.ID())
			return result
		}
		if target.IsNone() {
			request := state.request
			request.OldContents = nil
			request.OldLocation = types.ObjNothing
			return builtins.FinishRecycleLifecycle(execution, request, hookResult)
		}
		if _, _, err := findCallableVerbForRead(vm.Context.StoreTxn, target.ID(), "exitfunc"); err != nil {
			continue
		}
		if err := vm.startVerbCall(target, "exitfunc", []types.Value{content}); err != nil {
			vm.Builtins.EndRecycle(state.request.Object.ID())
			return moveLifecycleErrorResult(err)
		}
		state.exitCallback = true
		frame := vm.CurrentFrame()
		frame.DiscardReturn = true
		frame.RecycleContinuation = state
		vm.markBuiltinCaller("recycle", 2)
		return types.Result{Flow: types.FlowBuiltinPush}
	}
}
