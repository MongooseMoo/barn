package vm

import (
	"reflect"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A verb call fills its frame in place. Every StackFrame field must hold what
// the call sets or be zero, whether the struct is new or comes from the pool;
// a field added to StackFrame needs a stated value here.
func TestVerbCallFrameFieldsFromNewAndPooledFrames(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		// Every value the call sets differs from its zero value, so a dropped
		// assignment shows: the receiver #1 inherits nothing, the definer is
		// #1, the verb is debug, and the caller runs in thread mode.
		store := newBytecodeVerbStore()
		builder := dbstore.NewObjectBuilder(1)
		builder.SetOwner(0)
		builder.SetParents([]types.ObjID{0})
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add #1: %v", err)
		}
		debug := dbstore.VerbRead | dbstore.VerbExecute | dbstore.VerbDebug
		if _, ec := store.AddVerb(1, dbstore.NewVerb([]string{"probe"}, 0, debug, dbstore.VerbArgs{}, []string{"return 1;"})); ec != types.E_NONE {
			t.Fatalf("add #1:probe: %v", ec)
		}
		registry := BuildVMRegistry()
		caller, diagnostics := registry.Compiler().CompileMOO([]string{"return 0;"})
		if len(diagnostics) != 0 {
			t.Fatalf("compile caller: %v", diagnostics)
		}
		machine := NewVM(store, newTestSession(registry))
		machine.Context = task.NewContext()
		machine.Context.ThisObj, machine.Context.Verb, machine.Context.Programmer = 7, "outer", 8
		machine.Context.IsWizard, machine.Context.ThreadMode = true, true
		machine.Context.ThisValue = types.NewInt(9)
		machine.Task = task.NewState(1, 0, 30_000, 1)
		machine.beginProgram(caller)
		outer := machine.CurrentFrame()
		outer.This, outer.Player = 4, 5
		if pooled {
			spare := machine.frameFrom(StackFrame{Program: caller, Locals: machine.allocLocals(caller.NumLocals), localsOnStack: true})
			machine.pushFrame(spare)
			spare.LoopStack = append(spare.LoopStack, bytecode.LoopState{})
			spare.ExceptStack = append(spare.ExceptStack, bytecode.Handler{})
			machine.popFrame()
		}
		machine.Push(types.NewInt(2)) // so the callee's BasePointer is not zero
		args := []types.Value{types.NewInt(3)}
		if err := machine.startVerbCall(types.NewObj(1), "probe", args); err != nil {
			t.Fatalf("pooled=%v: startVerbCall: %v", pooled, err)
		}
		frame := machine.CurrentFrame()
		if frame == outer {
			t.Fatalf("pooled=%v: no frame pushed", pooled)
		}
		if pooled && (cap(frame.LoopStack) == 0 || cap(frame.ExceptStack) == 0) {
			t.Fatalf("pooled=%v: the call did not reuse the pooled stacks", pooled)
		}
		if len(frame.LoopStack) != 0 || len(frame.ExceptStack) != 0 {
			t.Fatalf("pooled=%v: loop/handler stacks not empty: %d %d", pooled, len(frame.LoopStack), len(frame.ExceptStack))
		}
		if machine.SP == 0 {
			t.Fatal("caller stack is empty")
		}
		set := map[string]any{
			"BasePointer": machine.SP, "This": types.ObjID(1), "ThisValue": types.None,
			"Player": types.ObjID(5), "Verb": "probe", "StoredVerbNames": []string{"probe"},
			"Caller": types.ObjID(4), "VerbLoc": types.ObjID(1), "Args": args, "IsVerbCall": true,
			"VerbDebug": true, "SavedThisObj": types.ObjID(7), "SavedThisValue": types.NewInt(9),
			"SavedVerb": "outer", "SavedProgrammer": types.ObjID(8), "SavedIsWizard": true,
			"SavedThreadMode": true, "localsOnStack": true,
		}
		checked := map[string]bool{"Program": true, "Locals": true, "LoopStack": true, "ExceptStack": true, "CommandEnvironment": true}
		if frame.Program == nil || len(frame.Locals) != frame.Program.NumLocals {
			t.Fatalf("pooled=%v: program/locals not set: %v %d", pooled, frame.Program, len(frame.Locals))
		}
		// The callee references no command variable: all six come from the caller.
		if frame.CommandEnvironment != outer.commandEnvironment() {
			t.Fatalf("pooled=%v: CommandEnvironment = %v, want %v", pooled, frame.CommandEnvironment, outer.commandEnvironment())
		}
		value := reflect.ValueOf(frame).Elem()
		for _, field := range reflect.VisibleFields(value.Type()) {
			if checked[field.Name] {
				continue
			}
			got := value.FieldByIndex(field.Index)
			if want, ok := set[field.Name]; ok {
				if !reflect.DeepEqual(reflect.NewAt(got.Type(), got.Addr().UnsafePointer()).Elem().Interface(), want) {
					t.Errorf("pooled=%v: %s = %v, want %v", pooled, field.Name, got, want)
				}
				continue
			}
			if !got.IsZero() {
				t.Errorf("pooled=%v: %s = %v, want zero", pooled, field.Name, got)
			}
		}
	}
}
