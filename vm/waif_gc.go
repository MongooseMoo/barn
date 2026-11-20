package vm

import (
	"github.com/MongooseMoo/barn/types"
)

func (vm *VM) recordPendingWaif(v types.Value) {
	if vm.pendingWaifIDs == nil {
		vm.pendingWaifIDs = make(map[types.WaifIdentity]struct{}, len(vm.PendingWaifs))
		for _, existing := range vm.PendingWaifs {
			vm.pendingWaifIDs[existing.WaifIdentity()] = struct{}{}
		}
	}
	id := v.WaifIdentity()
	if _, exists := vm.pendingWaifIDs[id]; exists {
		return
	}
	vm.pendingWaifIDs[id] = struct{}{}
	vm.PendingWaifs = append(vm.PendingWaifs, v)
}

// TakePendingWaifs returns waifs whose frame references have gone out of scope.
func (vm *VM) TakePendingWaifs() []types.Value {
	if len(vm.PendingWaifs) == 0 {
		return nil
	}
	pending := append([]types.Value(nil), vm.PendingWaifs...)
	vm.PendingWaifs = nil
	vm.pendingWaifIDs = nil
	return pending
}

// CollectWaifsFromVM adds all waifs currently referenced by VM frames or stack.
func CollectWaifsFromVM(exec *VM, out *[]types.Value) {
	if exec == nil {
		return
	}
	set := types.NewWaifSet(*out)
	CollectWaifsFromVMInto(exec, set)
	*out = set.Values
}

// CollectWaifsFromVMInto records all waifs currently referenced by VM frames or
// stack in set. Callers walking several VMs share one set so the whole capture
// stays linear in the number of references.
func CollectWaifsFromVMInto(exec *VM, set *types.WaifSet) {
	if exec == nil {
		return
	}
	exec.visitValues(func(value types.Value, kind valueRootKind) {
		if kind == valueRootLive {
			set.AddReachable(value, true)
		}
	})
}
