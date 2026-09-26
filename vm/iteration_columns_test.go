package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestValueOnlyMapIterationRetainsKeyRoots(t *testing.T) {
	key := types.NewWaif(0, 0)
	input := types.NewMap([][2]types.Value{{key, types.NewInt(7)}})
	registry := BuildVMRegistry()
	program, diagnostics := registry.Compiler().CompileMOO([]string{
		"source = args[1]; args = {}; for value in (source) source = []; suspend(); endfor return 0;",
	})
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	store := dbstore.NewStore()
	ctx := kernel.NewTaskContext()
	ctx.Store = store
	m := NewVM(store, newTestSessionWithTaskManager(registry))
	m.Context = ctx
	m.Task = task.NewTask(1, 0, ctx.TicksRemaining, 1)
	result := m.RunWithVerbContext(program, 0, 0, 0, "roots", 0, []types.Value{input})
	if result.Flow != types.FlowSuspend {
		t.Fatalf("result = %+v", result)
	}
	frame := m.CurrentFrame()
	if frame == nil {
		t.Fatal("no active frame")
	}
	var roots []types.Value
	// Inspect the hidden iteration state itself, excluding the original args.
	for _, value := range frame.Locals[len(program.VarNames):] {
		collectWaifsForGC(value, &roots)
	}
	if len(roots) != 1 || !roots[0].Equal(key) {
		t.Fatalf("hidden iteration roots = %v, want map key", roots)
	}
}

// Saved programs keep the earlier pair representation and instruction widths.
func TestPairIterationBytecodeStillExecutes(t *testing.T) {
	for _, input := range []types.Value{
		types.NewList([]types.Value{types.NewInt(7)}),
		types.NewMap([][2]types.Value{{types.NewInt(1), types.NewInt(7)}}),
	} {
		p := &bytecode.Program{NumLocals: 5, Constants: []types.Value{input}, Code: []byte{
			byte(bytecode.OP_PUSH), 0, byte(bytecode.OP_ITER_PREP), 1,
			byte(bytecode.OP_SET_LOCAL), 1, byte(bytecode.OP_SET_LOCAL), 0,
			byte(bytecode.OP_IMM_BASE) + byte(1-bytecode.OP_IMM_MIN), byte(bytecode.OP_SET_LOCAL), 2,
			byte(bytecode.OP_FOR_LIST_LOAD_KV), 0, 2, 3, 4,
			byte(bytecode.OP_GET_VAR), 3, byte(bytecode.OP_GET_VAR), 4,
			byte(bytecode.OP_MAKE_LIST), 2, byte(bytecode.OP_RETURN),
		}}
		if err := bytecode.VerifyProgram(p); err != nil {
			t.Fatal(err)
		}
		m := NewVM(dbstore.NewStore(), newTestSession(BuildVMRegistry()))
		result := m.Run(p)
		if result.Flow != types.FlowReturn || result.Val.String() != "{7, 1}" {
			t.Fatalf("saved bytecode result = %+v", result)
		}
	}
}

func TestColumnLoadReleasesBothPreviousLoopValues(t *testing.T) {
	store := dbstore.NewStore()
	for _, id := range []types.ObjID{4, 5} {
		if err := store.Add(testObject(id, true)); err != nil {
			t.Fatal(err)
		}
	}
	frame := &StackFrame{Program: &bytecode.Program{Code: []byte{byte(bytecode.OP_FOR_LIST_LOAD_COLUMNS), 0, 1, 2, 3, 4}}, Locals: []types.Value{
		types.NewList([]types.Value{types.NewInt(7)}), types.NewInt(1), types.NewAnon(4), types.NewAnon(5), types.NewInt(0),
	}, IP: 1}
	m := NewVM(store, newTestSession(BuildVMRegistry()))
	m.pushFrame(frame)
	if m.CurrentFrame() != frame {
		t.Fatal("frame installation failed")
	}
	if err := m.Execute(bytecode.OP_FOR_LIST_LOAD_COLUMNS); err != nil {
		t.Fatal(err)
	}
	if !frame.Locals[2].Equal(types.NewInt(7)) || !frame.Locals[3].Equal(types.NewInt(1)) {
		t.Fatal("incorrect loop bindings")
	}
	if len(m.PendingFinalizations) != 2 {
		t.Fatalf("pending finalizations=%v", m.PendingFinalizations)
	}
}
