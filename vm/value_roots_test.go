package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestVMRootCollectorsObserveEveryValueBearingField(t *testing.T) {
	type fixture func(*VM, types.Value)
	cases := []struct {
		name string
		set  fixture
	}{
		{"program constant", func(vm *VM, v types.Value) { vm.Frames[0].Program.Constants = []types.Value{v} }},
		{"local", func(vm *VM, v types.Value) { vm.Frames[0].Locals = []types.Value{v} }},
		{"argument", func(vm *VM, v types.Value) { vm.Frames[0].Args = []types.Value{v} }},
		{"receiver", func(vm *VM, v types.Value) { vm.Frames[0].ThisValue = v }},
		{"saved receiver", func(vm *VM, v types.Value) { vm.Frames[0].SavedThisValue = v }},
		{"pending return", func(vm *VM, v types.Value) { vm.Frames[0].PendingReturn, vm.Frames[0].HasPendingReturn = v, true }},
		{"pending exception", func(vm *VM, v types.Value) { vm.Frames[0].PendingError = VMException{Code: types.E_INVARG, Value: v} }},
		{"loop iterator", func(vm *VM, v types.Value) { vm.Frames[0].LoopStack = []bytecode.LoopState{{Iterator: v}} }},
		{"loop end", func(vm *VM, v types.Value) { vm.Frames[0].LoopStack = []bytecode.LoopState{{End: v}} }},
		{"move what", func(vm *VM, v types.Value) { vm.Frames[0].MoveContinuation = &task.MoveContinuationSnapshot{What: v} }},
		{"move where", func(vm *VM, v types.Value) { vm.Frames[0].MoveContinuation = &task.MoveContinuationSnapshot{Where: v} }},
		{"move old location", func(vm *VM, v types.Value) {
			vm.Frames[0].MoveContinuation = &task.MoveContinuationSnapshot{OldLocation: v}
		}},
		{"recycle request", func(vm *VM, v types.Value) {
			vm.Frames[0].RecycleContinuation = &recycleContinuation{request: builtins.RecycleLifecycleRequest{Object: v}}
		}},
		{"operand stack", func(vm *VM, v types.Value) { vm.Stack, vm.SP = []types.Value{v}, 1 }},
		{"yield result", func(vm *VM, v types.Value) { vm.yieldResult.Val = v }},
		{"yield traceback receiver", func(vm *VM, v types.Value) {
			vm.yieldResult.CallStack = []types.ActivationFrame{{ThisValue: v}}
		}},
		{"yield traceback argument", func(vm *VM, v types.Value) {
			vm.yieldResult.CallStack = []types.ActivationFrame{{Args: []types.Value{v}}}
		}},
		{"yield traceback variables", func(vm *VM, v types.Value) {
			vm.yieldResult.CallStack = []types.ActivationFrame{{RuntimeVariables: v}}
		}},
		{"fork receiver", func(vm *VM, v types.Value) { vm.yieldResult.ForkInfo = &types.ForkInfo{ThisValue: v} }},
		{"fork variable", func(vm *VM, v types.Value) {
			vm.yieldResult.ForkInfo = &types.ForkInfo{Variables: map[string]types.Value{"x": v}}
		}},
		{"context receiver", func(vm *VM, v types.Value) { vm.Context.ThisValue = v }},
		{"context map first", func(vm *VM, v types.Value) { vm.Context.MapFirstKey = v }},
		{"context map last", func(vm *VM, v types.Value) { vm.Context.MapLastKey = v }},
		{"context task local", func(vm *VM, v types.Value) { vm.Context.TaskLocal = v }},
		{"task local", func(vm *VM, v types.Value) {
			vm.Context = nil
			vm.Task = task.NewTask(1, 1, 100, 1)
			vm.Task.SetTaskLocal(v)
		}},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			anon := types.NewAnon(types.ObjID(1000 + i))
			waif := types.NewWaif(types.ObjID(2000+i), 1)
			value := types.NewList([]types.Value{anon, waif})
			machine := &VM{Frames: []*StackFrame{{Program: &bytecode.Program{}}}, Context: &kernel.TaskContext{}}
			tc.set(machine, value)

			var waifs []types.Value
			CollectWaifsFromVM(machine, &waifs)
			if !waifValueInList(waif, waifs) {
				t.Errorf("WAIF collector missed %s", tc.name)
			}
			anons := make(map[types.ObjID]struct{})
			CollectAnonymousRefsFromVM(machine, anons)
			if _, ok := anons[anon.ID()]; !ok {
				t.Errorf("anonymous collector missed %s", tc.name)
			}
			direct := CollectDirectFinalizationRoots(machine)
			if _, ok := direct.AnonRefs[anon.ID()]; !ok || !waifValueInList(waif, direct.Waifs) {
				t.Errorf("direct finalization collector missed %s: %#v", tc.name, direct)
			}
		})
	}
}

func TestPendingQueuesAreFinalizationCandidatesNotLiveRoots(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*VM, types.Value)
	}{
		{"pending WAIF", func(vm *VM, value types.Value) { vm.PendingWaifs = []types.Value{value} }},
		{"pending finalization", func(vm *VM, value types.Value) { vm.PendingFinalizations = []types.Value{value} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			waif := types.NewWaif(1, 1)
			anon := types.NewAnon(2)
			machine := &VM{}
			tc.set(machine, types.NewList([]types.Value{waif, anon}))

			var liveWaifs []types.Value
			CollectWaifsFromVM(machine, &liveWaifs)
			liveAnons := make(map[types.ObjID]struct{})
			CollectAnonymousRefsFromVM(machine, liveAnons)
			if len(liveWaifs) != 0 || len(liveAnons) != 0 {
				t.Fatalf("pending candidate reported live: waifs=%v anons=%v", liveWaifs, liveAnons)
			}
			direct := CollectDirectFinalizationRoots(machine)
			if !waifValueInList(waif, direct.Waifs) {
				t.Error("direct finalization roots missed pending WAIF")
			}
			if _, ok := direct.AnonRefs[anon.ID()]; !ok {
				t.Error("direct finalization roots missed pending anonymous object")
			}
		})
	}
}

func TestPopFrameRetainsEveryFrameValueAndDiscardedOperand(t *testing.T) {
	fields := []struct {
		name string
		set  func(*StackFrame, types.Value)
	}{
		{"argument", func(f *StackFrame, v types.Value) { f.Args = []types.Value{v} }},
		{"pending return", func(f *StackFrame, v types.Value) { f.PendingReturn, f.HasPendingReturn = v, true }},
		{"move continuation", func(f *StackFrame, v types.Value) { f.MoveContinuation = &task.MoveContinuationSnapshot{What: v} }},
		{"recycle continuation", func(f *StackFrame, v types.Value) {
			f.RecycleContinuation = &recycleContinuation{request: builtins.RecycleLifecycleRequest{Object: v}}
		}},
	}
	for i, tc := range fields {
		t.Run(tc.name, func(t *testing.T) {
			waif := types.NewWaif(types.ObjID(i+1), 1)
			frame := &StackFrame{Program: &bytecode.Program{}}
			tc.set(frame, waif)
			machine := &VM{Frames: []*StackFrame{frame}, frame: frame}
			machine.popFrame()
			if !waifValueInList(waif, machine.PendingWaifs) {
				t.Errorf("pending WAIFs missed popped-frame %s", tc.name)
			}
		})
	}

	t.Run("discarded operand", func(t *testing.T) {
		waif := types.NewWaif(99, 1)
		frame := &StackFrame{Program: &bytecode.Program{}, BasePointer: 0}
		machine := &VM{Frames: []*StackFrame{frame}, frame: frame, Stack: []types.Value{waif}, SP: 1}
		machine.popFrame()
		if !waifValueInList(waif, machine.PendingWaifs) {
			t.Fatalf("pending WAIFs missed discarded operand: %v", machine.PendingWaifs)
		}
	})
}
