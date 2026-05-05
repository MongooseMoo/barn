package vm

import "barn/types"

func collectWaifsForGC(v types.Value, out *[]types.WaifValue) {
	switch val := v.(type) {
	case types.WaifValue:
		for _, existing := range *out {
			if existing.Equal(val) {
				return
			}
		}
		*out = append(*out, val)
	case types.ListValue:
		for _, elem := range val.Elements() {
			collectWaifsForGC(elem, out)
		}
	case types.MapValue:
		for _, pair := range val.Pairs() {
			collectWaifsForGC(pair[0], out)
			collectWaifsForGC(pair[1], out)
		}
	}
}

// CollectWaifsFromValue adds all waifs referenced by a value tree.
func CollectWaifsFromValue(v types.Value, out *[]types.WaifValue) {
	collectWaifsForGC(v, out)
}

func (vm *VM) collectPendingWaifsFromFrame(frame *StackFrame) {
	if frame == nil {
		return
	}
	for _, value := range frame.Locals {
		collectWaifsForGC(value, &vm.PendingWaifs)
	}
}

// TakePendingWaifs returns waifs whose frame references have gone out of scope.
func (vm *VM) TakePendingWaifs() []types.WaifValue {
	if len(vm.PendingWaifs) == 0 {
		return nil
	}
	pending := append([]types.WaifValue(nil), vm.PendingWaifs...)
	vm.PendingWaifs = nil
	return pending
}

// CollectWaifsFromVM adds all waifs currently referenced by VM frames or stack.
func CollectWaifsFromVM(exec *VM, out *[]types.WaifValue) {
	if exec == nil {
		return
	}
	for _, frame := range exec.Frames {
		if frame == nil {
			continue
		}
		for _, value := range frame.Locals {
			collectWaifsForGC(value, out)
		}
	}
	for i := 0; i < exec.SP && i < len(exec.Stack); i++ {
		collectWaifsForGC(exec.Stack[i], out)
	}
}
