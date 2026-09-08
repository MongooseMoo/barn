package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestLineSyncDoesNotMaterializeRuntimeVariables(t *testing.T) {
	taskValue := &task.Task{}
	taskValue.PushFrame(types.ActivationFrame{Verb: "raise"})
	machine := &VM{
		Context: &kernel.TaskContext{},
		Task:    taskValue,
		Frames: []*StackFrame{{
			Program: &bytecode.Program{VarNames: []string{"local"}},
			Locals:  []types.Value{types.NewInt(42)},
		}},
	}

	machine.syncTaskLineNumbers()
	stack := taskValue.GetCallStack()
	if stack[0].LineNumber != 1 {
		t.Fatalf("line = %d, want 1", stack[0].LineNumber)
	}
	if stack[0].RuntimeVariables.Type() == types.TYPE_MAP {
		t.Fatal("line synchronization materialized a runtime-variable map")
	}
}

func TestUnobservedCaughtErrorDoesNotBuildExceptionValue(t *testing.T) {
	for _, handlerType := range []bytecode.HandlerType{bytecode.HandlerExcept, bytecode.HandlerFinally} {
		machine := &VM{}
		frame := &StackFrame{
			ExceptStack: []bytecode.Handler{{
				Type: handlerType, VarIndex: -1, HandlerIP: 10,
				Codes: []types.ErrorCode{types.E_DIV},
			}},
		}
		machine.pushFrame(frame)
		if machine.CurrentFrame() != frame {
			t.Fatal("fixture has no active frame")
		}
		handled, value := machine.HandleError(VMException{Code: types.E_DIV, Value: types.None})
		if !handled || machine.Frames[0].IP != 10 {
			t.Fatalf("handler %v did not receive control", handlerType)
		}
		if !value.IsNone() {
			t.Errorf("handler %v materialized an unobserved exception value", handlerType)
		}
	}
}

func TestTracebackRuntimeVariablesFollowServerOption(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		ctx := &kernel.TaskContext{PendingEffects: []kernel.PendingEffect{{
			Kind:          kernel.PendingEffectServerOptions,
			ServerOptions: kernel.PendingServerOptions{IncludeRTVars: enabled},
		}}}
		taskValue := &task.Task{}
		taskValue.PushFrame(types.ActivationFrame{Verb: "boom"})
		machine := &VM{Context: ctx, Task: taskValue,
			Builtins: newTestSessionWithTaskManager(BuildVMRegistry())}
		machine.pushFrame(&StackFrame{
			Program: &bytecode.Program{VarNames: []string{"local", "unbound"}},
			Locals:  []types.Value{types.NewInt(42)},
		})
		traceback := machine.buildTraceback(false)
		frame := traceback.Get(1)
		wantLength := 6
		if enabled {
			wantLength = 7
		}
		if frame.Len() != wantLength {
			t.Fatalf("include_rt_vars=%v: frame length=%d, want %d", enabled, frame.Len(), wantLength)
		}
		if enabled {
			variables := frame.Get(7)
			value, found := variables.MapGet(types.NewStr("local"))
			if !found || value.Int() != 42 || variables.Len() != 1 {
				t.Fatal("traceback did not preserve bound local variables")
			}
		}
	}
}

func TestRuntimeVariableSnapshotSurvivesResume(t *testing.T) {
	taskValue := &task.Task{}
	taskValue.PushFrame(types.ActivationFrame{Verb: "suspended"})
	machine := &VM{Task: taskValue}
	machine.pushFrame(&StackFrame{
		Program: &bytecode.Program{VarNames: []string{"local", "unbound"}},
		Locals:  []types.Value{types.NewInt(42)},
	})
	machine.snapshotTaskRuntimeVariables()
	before := taskValue.GetCallStack()[0]
	if before.RuntimeVariables.Type() == types.TYPE_MAP {
		t.Fatal("snapshot eagerly constructed a map")
	}
	machine.CurrentFrame().Locals[0] = types.NewInt(99)
	machine.snapshotTaskRuntimeVariables()
	after := taskValue.GetCallStack()[0]
	for i, frame := range []types.ActivationFrame{before, after} {
		variables := frame.RuntimeVariableMap()
		value, found := variables.MapGet(types.NewStr("local"))
		want := int64(42)
		if i == 1 {
			want = 99
		}
		if !found || value.Int() != want || variables.Len() != 1 {
			t.Fatalf("snapshot %d did not retain its bound locals", i)
		}
	}
}

func TestUncaughtStackCapturesOptionalRuntimeVariables(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		ctx := &kernel.TaskContext{PendingEffects: []kernel.PendingEffect{{
			Kind:          kernel.PendingEffectServerOptions,
			ServerOptions: kernel.PendingServerOptions{IncludeRTVars: enabled},
		}}}
		machine := &VM{Context: ctx,
			Builtins: newTestSessionWithTaskManager(BuildVMRegistry())}
		machine.pushFrame(&StackFrame{
			Program: &bytecode.Program{VarNames: []string{"local"}},
			Locals:  []types.Value{types.NewInt(42)},
		})
		stack := machine.snapshotActivationFrames(1)
		if (stack[0].RuntimeVariableSnapshot != nil) != enabled {
			t.Fatalf("include_rt_vars=%v: uncaught snapshot variable presence differs", enabled)
		}
		if enabled {
			value, found := stack[0].RuntimeVariableMap().MapGet(types.NewStr("local"))
			if !found || value.Int() != 42 {
				t.Fatal("uncaught snapshot lost the local")
			}
		}
	}
}
