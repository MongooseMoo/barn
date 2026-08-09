package vm

import (
	"unsafe"

	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func collectWaifsForGC(v types.Value, out *[]types.Value) {
	collectWaifsForGCVisited(v, out, nil)
}

func collectDirectWaifsForGC(v types.Value, out *[]types.Value) {
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

func collectWaifsForGCVisited(v types.Value, out *[]types.Value, visited map[unsafe.Pointer]struct{}) {
	switch v.Type() {
	case types.TYPE_WAIF:
		for _, existing := range *out {
			if existing.Equal(v) {
				return
			}
		}
		*out = append(*out, v)
		identity := v.WaifIdentity()
		if _, seen := visited[identity]; seen {
			return
		}
		if visited == nil {
			visited = make(map[unsafe.Pointer]struct{})
		}
		visited[identity] = struct{}{}
		for _, name := range v.PropertyNames() {
			if prop, ok := v.GetProperty(name); ok {
				collectWaifsForGCVisited(prop, out, visited)
			}
		}
	case types.TYPE_LIST:
		for _, elem := range v.Elements() {
			collectWaifsForGCVisited(elem, out, visited)
		}
	case types.TYPE_MAP:
		for _, pair := range v.Pairs() {
			collectWaifsForGCVisited(pair[0], out, visited)
			collectWaifsForGCVisited(pair[1], out, visited)
		}
	}
}

// CollectWaifsFromValue adds all waifs referenced by a value tree.
func CollectWaifsFromValue(v types.Value, out *[]types.Value) {
	collectWaifsForGC(v, out)
}

func (vm *VM) collectPendingWaifsFromFrame(frame *StackFrame) {
	if frame == nil {
		return
	}
	for _, value := range frame.Locals {
		collectDirectWaifsForGC(value, &vm.PendingWaifs)
	}
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
	for _, frame := range exec.Frames {
		if frame == nil {
			continue
		}
		for _, value := range frame.Locals {
			collectWaifsForGC(value, out)
		}
		collectWaifsForGC(frame.ThisValue, out)
		for _, value := range frame.Args {
			collectWaifsForGC(value, out)
		}
		collectWaifsForGC(frame.SavedThisValue, out)
		collectWaifsFromPendingError(frame.PendingError, out)
	}
	for i := 0; i < exec.SP && i < len(exec.Stack); i++ {
		collectWaifsForGC(exec.Stack[i], out)
	}
	collectWaifsForGC(exec.yieldResult.Val, out)
	if fork := exec.yieldResult.ForkInfo; fork != nil {
		collectWaifsForGC(fork.ThisValue, out)
		for _, value := range fork.Variables {
			collectWaifsForGC(value, out)
		}
	}
	if exec.Context != nil {
		collectWaifsForGC(exec.Context.ThisValue, out)
		collectWaifsForGC(exec.Context.MapFirstKey, out)
		collectWaifsForGC(exec.Context.MapLastKey, out)
		collectWaifsForGC(exec.Context.TaskLocal, out)
		if owner, ok := exec.Context.Task.(*task.Task); ok && owner != nil {
			collectWaifsForGC(owner.GetTaskLocal(), out)
		}
	}
}

func collectWaifsFromPendingError(err error, out *[]types.Value) {
	for err != nil {
		switch pending := err.(type) {
		case VMException:
			collectWaifsForGC(pending.Value, out)
			return
		case *VMException:
			collectWaifsForGC(pending.Value, out)
			return
		case interface{ Unwrap() error }:
			err = pending.Unwrap()
		default:
			return
		}
	}
}
