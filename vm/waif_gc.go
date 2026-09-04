package vm

import (
	"github.com/MongooseMoo/barn/types"
)

// collectWaifsForGC adds every waif reachable from v to out, deduplicated by
// identity, preserving first-seen order. Prefer collectWaifsInto with a shared
// WaifSet when walking many values: seeding a set from out costs O(len(out)).
func collectWaifsForGC(v types.Value, out *[]types.Value) {
	if !v.MayHoldFinalizable() {
		return
	}
	set := types.NewWaifSet(*out)
	collectWaifsInto(v, set)
	*out = set.Values
}

func collectDirectWaifsForGC(v types.Value, out *[]types.Value) {
	if !v.MayHoldFinalizable() {
		return
	}
	switch v.Type() {
	case types.TYPE_WAIF:
		for _, existing := range *out {
			if existing.Equal(v) {
				return
			}
		}
		*out = append(*out, v)
	case types.TYPE_LIST:
		for _, elem := range v.Elements() {
			collectDirectWaifsForGC(elem, out)
		}
	case types.TYPE_MAP:
		for _, pair := range v.Pairs() {
			collectDirectWaifsForGC(pair[0], out)
			collectDirectWaifsForGC(pair[1], out)
		}
	}
}

// collectWaifsInto records every waif reachable from v (through lists, maps,
// and waif properties) in set. A waif already present is not re-expanded, which
// also terminates on cyclic waif graphs.
func collectWaifsInto(v types.Value, set *types.WaifSet) {
	switch v.Type() {
	case types.TYPE_WAIF:
		if !set.Add(v) {
			return
		}
		for _, name := range v.PropertyNames() {
			if prop, ok := v.GetProperty(name); ok {
				collectWaifsInto(prop, set)
			}
		}
	case types.TYPE_LIST:
		if !v.MayHoldFinalizable() {
			return
		}
		for _, elem := range v.Elements() {
			collectWaifsInto(elem, set)
		}
	case types.TYPE_MAP:
		if !v.MayHoldFinalizable() {
			return
		}
		for _, pair := range v.Pairs() {
			collectWaifsInto(pair[0], set)
			collectWaifsInto(pair[1], set)
		}
	}
}

// CollectWaifsFromValue adds all waifs referenced by a value tree.
func CollectWaifsFromValue(v types.Value, out *[]types.Value) {
	collectWaifsForGC(v, out)
}

func (vm *VM) collectPendingWaifsFromFrame(frame *StackFrame) {
	frame.visitValues(func(value types.Value, _ valueRootKind) {
		collectDirectWaifsForGC(value, &vm.PendingWaifs)
	})
}

// TakePendingWaifs returns waifs whose frame references have gone out of scope.
func (vm *VM) TakePendingWaifs() []types.Value {
	if len(vm.PendingWaifs) == 0 {
		return nil
	}
	pending := append([]types.Value(nil), vm.PendingWaifs...)
	vm.PendingWaifs = nil
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
			collectWaifsInto(value, set)
		}
	})
}
