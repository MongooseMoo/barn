package format

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A suspended frame whose program uses compiler temporaries carries
// NumLocals == bytecode.MaxLocals with only a few named variables. The
// Toast-shaped rt_env only lists named variables, so the temporaries travel in
// Barn's frame metadata; both the slot count and the bound temporary values
// must come back, or the task cannot resume after a restart.
func TestSuspendedFrameRoundTripsCompilerTemporarySlots(t *testing.T) {
	objectStore := dbstore.NewStore()
	for _, builder := range []*dbstore.ObjectBuilder{
		dbstore.NewObjectBuilder(0),
		dbstore.NewObjectBuilder(2),
	} {
		builder.SetOwner(2)
		builder.SetLocation(types.ObjNothing)
		builder.SetFlags(dbstore.FlagWizard | dbstore.FlagUser)
		if err := objectStore.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", builder.ID(), err)
		}
	}

	locals := make([]types.Value, bytecode.MaxLocals)
	for i := range locals {
		locals[i] = types.Unbound
	}
	locals[0] = types.NewInt(7)
	locals[2] = types.NewStr("named")
	locals[254] = types.NewList([]types.Value{types.NewInt(1), types.NewInt(2)})
	locals[255] = types.NewInt(3)

	suspended := []task.Snapshot{{
		ID:            1,
		Owner:         2,
		State:         task.TaskSuspended,
		WakeValue:     types.NewInt(0),
		TaskLocal:     types.NewEmptyMap(),
		Programmer:    2,
		This:          0,
		ReadingPlayer: types.ObjNothing,
		VM: &task.VMSnapshot{
			MaxStackDepth: 50,
			Frames: []task.VMFrameSnapshot{{
				Program: bytecode.Program{
					Code:          []byte{byte(bytecode.OP_RETURN_NONE)},
					Source:        []string{"return;"},
					VarNames:      []string{"a", "b", "c"},
					NumLocals:     bytecode.MaxLocals,
					LineInfo:      []bytecode.LineEntry{{StartIP: 0, Line: 1}},
					BuiltinLayout: [32]byte{1, 2, 3},
				},
				Locals:    locals,
				This:      0,
				ThisValue: types.NewObj(0),
				Player:    2,
			}},
		},
	}}

	path := filepath.Join(t.TempDir(), "temporaries.db")
	if err := WriteCheckpoint(path, objectStore, nil, suspended, nil); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}
	reloaded, err := LoadDatabase(path + ".new")
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	if got := len(reloaded.SuspendedTasks); got != 1 {
		t.Fatalf("suspended tasks = %d, want 1", got)
	}
	frame := reloaded.SuspendedTasks[0].Snapshot.VM.Frames[0]
	if frame.Program.BuiltinLayout != [32]byte{1, 2, 3} {
		t.Fatal("checkpoint lost builtin layout")
	}
	// The program is rebuilt at restore (vm.RestoreVMSnapshot places these
	// in its slots); the reader holds bound named locals by name, in saved
	// order, and the temporaries by slot.
	if frame.Rebuild == nil || frame.Rebuild.Legacy {
		t.Fatalf("rebuild = %+v, want v2", frame.Rebuild)
	}
	if !slices.Equal(frame.Program.VarNames, []string{"a", "c"}) || len(frame.Locals) != 2 ||
		!frame.Locals[0].Equal(types.NewInt(7)) || !frame.Locals[1].Equal(types.NewStr("named")) {
		t.Fatalf("named locals = %v %v", frame.Program.VarNames, frame.Locals)
	}
	want := []task.LocalSlot{
		{Slot: 254, Value: types.NewList([]types.Value{types.NewInt(1), types.NewInt(2)})},
		{Slot: 255, Value: types.NewInt(3)},
	}
	if len(frame.Rebuild.Internal) != len(want) {
		t.Fatalf("temporaries = %+v, want %+v", frame.Rebuild.Internal, want)
	}
	for i, slot := range want {
		if got := frame.Rebuild.Internal[i]; got.Slot != slot.Slot || !got.Value.Equal(slot.Value) {
			t.Errorf("temporary %d = %+v, want %+v", i, got, slot)
		}
	}
}
