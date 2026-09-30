package vm

import (
	"fmt"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func retryTestVM(t testing.TB, code string) *VM {
	t.Helper()
	registry := BuildVMRegistry()
	program, diagnostics := registry.Compiler().CompileMOO([]string{code})
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	store := dbstore.NewStore()
	machine := NewVM(store, newTestSessionWithTaskManager(registry))
	machine.Context = kernel.NewTaskContext()
	machine.Context.Store = store
	machine.Task = task.NewTask(1, 0, 30000, 3)
	if result := machine.Run(program); result.Flow != types.FlowSuspend {
		t.Fatalf("flow=%v, want suspended fixture", result.Flow)
	}
	if machine.CurrentFrame() == nil {
		t.Fatal("no active frame")
	}
	return machine
}

func BenchmarkContinuationCheckpoint(b *testing.B) {
	for _, depth := range []int{1, 8, 32} {
		for _, locals := range []int{8, 256} {
			machine := retryTestVM(b, `suspend(0); return 1;`)
			initial := *machine.CurrentFrame()
			machine.Frames = nil
			for i := 0; i < depth; i++ {
				frame := initial
				frame.Locals = make([]types.Value, locals)
				frame.LoopStack = []bytecode.LoopState{{Iterator: types.NewList([]types.Value{types.NewAnon(12345)})}}
				machine.pushFrame(&frame)
			}
			b.Run(fmt.Sprintf("depth=%d/locals=%d/capture", depth, locals), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					machine.CheckpointForRetry()
					machine.ReleaseRetryCheckpoint()
				}
			})
			checkpoint := machine.CheckpointForRetry()
			b.Run(fmt.Sprintf("depth=%d/locals=%d/restore", depth, locals), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					checkpoint.Restore(machine)
				}
			})
		}
	}
}

func finishRetryVM(machine *VM) types.Result {
	result := machine.Resume()
	for result.Flow == types.FlowSuspend {
		result = machine.Resume()
	}
	return result
}

func TestRetryCheckpointReplaysLoopsAndFinally(t *testing.T) {
	for _, code := range []string{
		`r = {}; for x in ({1, 2, 3}) r = {@r, x}; suspend(0); endfor return r;`,
		`r = {}; for x, k in (["a" -> 1, "b" -> 2]) suspend(0); r = {@r, {k, x}}; endfor return r;`,
		`try return 17; finally suspend(0); endtry`,
		"try raise(E_INVARG); finally suspend(0); endtry",
	} {
		machine := retryTestVM(t, code)
		checkpoint := machine.CheckpointForRetry()
		program := machine.CurrentFrame().Program
		first := finishRetryVM(machine)
		for attempt := 0; attempt < 2; attempt++ {
			checkpoint.Restore(machine)
			if machine.CurrentFrame() == nil || machine.CurrentFrame().Program != program {
				t.Fatal("restore did not install the frame with shared immutable bytecode")
			}
			second := finishRetryVM(machine)
			if first.Flow != second.Flow || first.Error != second.Error || !first.Val.Equal(second.Val) {
				t.Fatalf("code=%s first=%+v replay=%+v", code, first, second)
			}
		}
	}
}

func TestRetryCheckpointDetachesMutableFrameStorage(t *testing.T) {
	machine := retryTestVM(t, `x = 4; suspend(0); return x;`)
	frame := machine.CurrentFrame()
	frame.LoopStack = []bytecode.LoopState{{Iterator: types.NewInt(7)}}
	frame.ExceptStack = []bytecode.Handler{{Codes: []types.ErrorCode{types.E_TYPE}}}
	frame.PendingError = &VMException{Code: types.E_INVARG, Value: types.NewInt(9)}
	frame.MoveContinuation = &task.MoveContinuationSnapshot{Stage: 3, What: types.NewObj(7)}
	frame.RecycleContinuation = &recycleContinuation{request: builtins.RecycleLifecycleRequest{OldParents: []types.ObjID{1}}}
	checkpoint := machine.CheckpointForRetry()
	frame.LoopStack[0].Iterator = types.NewInt(8)
	frame.ExceptStack[0].Codes[0] = types.E_ARGS
	frame.PendingError.(*VMException).Value = types.NewInt(10)
	frame.MoveContinuation.Stage = 99
	frame.RecycleContinuation.request.OldParents[0] = 99
	machine.popFrame() // recycle and clear the original frame's backing storage
	checkpoint.Restore(machine)
	frame = machine.CurrentFrame()
	if frame == nil {
		t.Fatal("restore did not install an active frame")
	}
	if frame.LoopStack[0].Iterator.(types.Value).Int() != 7 || frame.ExceptStack[0].Codes[0] != types.E_TYPE || frame.PendingError.(*VMException).Value.Int() != 9 || frame.MoveContinuation.Stage != 3 || frame.RecycleContinuation.request.OldParents[0] != 1 {
		t.Fatal("checkpoint aliased the abandoned attempt")
	}
}

func TestRetryCheckpointRootsOutliveOverwrittenLocals(t *testing.T) {
	machine := retryTestVM(t, `suspend(0); return 1;`)
	frame := machine.CurrentFrame()
	waif := types.NewWaif(12346, 0)
	frame.Locals = append(frame.Locals, types.NewList([]types.Value{types.NewAnon(12345), waif}))
	machine.CheckpointForRetry()
	frame.Locals[len(frame.Locals)-1] = types.NewInt(0)
	refs := make(map[types.ObjID]struct{})
	CollectAnonymousRefsFromVM(machine, refs)
	if _, ok := refs[12345]; !ok {
		t.Fatal("lost checkpoint-only anonymous root")
	}
	var waifs []types.Value
	CollectWaifsFromVM(machine, &waifs)
	if !waifValueInList(waif, waifs) {
		t.Fatal("lost checkpoint-only WAIF root")
	}
	machine.ReleaseRetryCheckpoint()
	clear(refs)
	CollectAnonymousRefsFromVM(machine, refs)
	if _, ok := refs[12345]; ok {
		t.Fatal("retained a released checkpoint")
	}
	waifs = nil
	CollectWaifsFromVM(machine, &waifs)
	if waifValueInList(waif, waifs) {
		t.Fatal("retained a released checkpoint WAIF")
	}
}
