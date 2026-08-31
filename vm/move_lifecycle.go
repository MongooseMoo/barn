package vm

import (
	"fmt"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

const (
	moveAwaitAccept = iota + 1
	moveAwaitExit
	moveAwaitEnter
)

func (vm *VM) startMoveLifecycle(request builtins.MoveLifecycleRequest) types.Result {
	state := &task.MoveContinuationSnapshot{
		Stage:         moveAwaitAccept,
		What:          request.What,
		Where:         request.Where,
		OldLocation:   types.None,
		Position:      request.Position,
		Decentralized: request.Decentralized,
	}
	if request.Where.ID() == types.ObjNothing {
		return vm.resumeMoveLifecycle(state, types.Err(types.E_VERBNF))
	}
	return vm.startOrSkipMoveLifecycleVerb(state, request.Where, "accept")
}

func (vm *VM) startOrSkipMoveLifecycleVerb(state *task.MoveContinuationSnapshot, target types.Value, verbName string) types.Result {
	started, err := vm.pushMoveLifecycleVerb(state, target, verbName)
	if err != nil {
		return moveLifecycleErrorResult(err)
	}
	if started {
		return types.Result{Flow: types.FlowBuiltinPush}
	}
	return vm.resumeMoveLifecycle(state, types.Err(types.E_VERBNF))
}

func (vm *VM) pushMoveLifecycleVerb(state *task.MoveContinuationSnapshot, target types.Value, verbName string) (bool, error) {
	if vm.Context == nil || vm.Context.StoreTxn == nil {
		return false, fmt.Errorf("E_INVARG: move lifecycle has no transaction")
	}
	if _, _, err := findCallableVerbForRead(vm.Context.StoreTxn, target.ID(), verbName); err != nil {
		return false, nil
	}
	if err := vm.startVerbCall(target, verbName, []types.Value{state.What}); err != nil {
		if extractErrorCode(err) == types.E_VERBNF {
			return false, nil
		}
		return false, err
	}
	frame := vm.CurrentFrame()
	if frame == nil {
		return false, fmt.Errorf("E_INVARG: move lifecycle verb created no frame")
	}
	frame.DiscardReturn = true
	frame.MoveContinuation = cloneMoveContinuation(state)
	return true, nil
}

func (vm *VM) resumeMoveLifecycle(state *task.MoveContinuationSnapshot, hookResult types.Result) types.Result {
	switch state.Stage {
	case moveAwaitAccept:
		if hookResult.Flow == types.FlowException {
			if hookResult.Error != types.E_VERBNF {
				return hookResult
			}
			if vm.Context != nil && !vm.Context.IsWizard && state.Where.ID() != types.ObjNothing {
				return types.Err(types.E_NACC)
			}
		} else if !hookResult.Val.Truthy() && vm.Context != nil && !vm.Context.IsWizard {
			return types.Err(types.E_NACC)
		}
		if vm.Context == nil || !vm.Context.StoreTxn.Valid(state.What.ID()) {
			return types.Ok(types.NewInt(0))
		}

		execution := vm.Builtins.NewExecution(vm.Context, vm.Task)
		oldLocation, errCode := builtins.ApplyMoveLifecycle(
			execution,
			state.What,
			state.Where,
			state.Position,
			state.Decentralized,
		)
		if errCode != types.E_NONE {
			return types.Err(errCode)
		}
		state.OldLocation = oldLocation
		if oldLocation.ID() == state.Where.ID() {
			return types.Ok(types.NewInt(0))
		}
		if oldLocation.ID() != types.ObjNothing && vm.Context.StoreTxn.Valid(oldLocation.ID()) {
			state.Stage = moveAwaitExit
			return vm.startOrSkipMoveLifecycleVerb(state, oldLocation, "exitfunc")
		}
		return vm.continueMoveLifecycleAfterExit(state)

	case moveAwaitExit:
		if hookResult.Flow == types.FlowException && hookResult.Error != types.E_VERBNF {
			return hookResult
		}
		return vm.continueMoveLifecycleAfterExit(state)

	case moveAwaitEnter:
		if hookResult.Flow == types.FlowException && hookResult.Error != types.E_VERBNF {
			return hookResult
		}
		return types.Ok(types.NewInt(0))

	default:
		return types.Err(types.E_INVARG)
	}
}

func (vm *VM) continueMoveLifecycleAfterExit(state *task.MoveContinuationSnapshot) types.Result {
	execution := vm.Builtins.NewExecution(vm.Context, vm.Task)
	if !builtins.MoveLifecycleAtDestination(execution, state.What, state.Where) {
		return types.Ok(types.NewInt(0))
	}
	state.Stage = moveAwaitEnter
	return vm.startOrSkipMoveLifecycleVerb(state, state.Where, "enterfunc")
}

func moveLifecycleErrorResult(err error) types.Result {
	errCode := extractErrorCode(err)
	if errCode == types.E_NONE {
		errCode = types.E_INVARG
	}
	return types.Err(errCode)
}
