package vm

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestMoveLifecycleContinuationSurvivesVMSnapshot(t *testing.T) {
	store := dbstore.NewStore()
	session := builtins.NewSession(builtins.NewRegistry(), builtins.NoHost())
	machine := NewVM(store, session)
	machine.pushFrame(&StackFrame{
		Program: &bytecode.Program{Code: []byte{byte(bytecode.OP_RETURN_NONE)}},
		MoveContinuation: &task.MoveContinuationSnapshot{
			Stage:         moveAwaitExit,
			What:          types.NewObj(12),
			Where:         types.NewObj(11),
			OldLocation:   types.NewObj(10),
			Position:      2,
			Decentralized: true,
		},
	})
	machine.yielded = true

	snapshot := machine.PersistenceVMSnapshot()
	machine.CurrentFrame().MoveContinuation.Stage = moveAwaitEnter
	restored, err := RestoreVMSnapshot(snapshot, store, session, task.NewContext())
	if err != nil {
		t.Fatalf("RestoreVMSnapshot: %v", err)
	}
	state := restored.CurrentFrame().MoveContinuation
	if state == nil || state.Stage != moveAwaitExit || state.What.ID() != 12 || state.Where.ID() != 11 ||
		state.OldLocation.ID() != 10 || state.Position != 2 || !state.Decentralized {
		t.Fatalf("restored move continuation = %#v", state)
	}
}

func TestControlContinuationsSurviveVMSnapshot(t *testing.T) {
	machine := retryTestVM(t, `try return 73; finally suspend(); endtry`)
	store, session := machine.Store, machine.Builtins
	machine.CurrentFrame().RecycleContinuation = &recycleContinuation{request: builtins.RecycleLifecycleRequest{
		Object:      types.NewAnon(12),
		OldParents:  []types.ObjID{1, 2},
		OldChildren: []types.ObjID{3},
		OldContents: []types.ObjID{4, 5},
		OldLocation: 6,
	}}
	machine.yielded = true

	snapshot := machine.PersistenceVMSnapshot()
	machine.CurrentFrame().PendingReasons[0].Value = types.NewInt(99)
	machine.CurrentFrame().RecycleContinuation.request.OldParents[0] = 99
	restored, err := RestoreVMSnapshot(snapshot, store, session, task.NewContext())
	if err != nil {
		t.Fatalf("RestoreVMSnapshot: %v", err)
	}
	frame := restored.CurrentFrame()
	if len(frame.PendingReasons) != 1 || !frame.PendingReasons[0].Value.Equal(types.NewInt(73)) {
		t.Fatalf("restored pending return = %#v, want 73", frame.PendingReasons)
	}
	if frame.RecycleContinuation == nil {
		t.Fatal("restored recycle continuation is nil")
	}
	request := frame.RecycleContinuation.request
	if !request.Object.Equal(types.NewAnon(12)) || !slices.Equal(request.OldParents, []types.ObjID{1, 2}) ||
		!slices.Equal(request.OldChildren, []types.ObjID{3}) || !slices.Equal(request.OldContents, []types.ObjID{4, 5}) ||
		request.OldLocation != 6 {
		t.Fatalf("restored recycle request = %#v", request)
	}
	if _, err := RestoreVMSnapshot(snapshot, store, session, task.NewContext()); err == nil {
		t.Fatal("second restore of in-flight recycle succeeded, want recycle guard conflict")
	}
}

func TestNumberedRecycleContinuationDoesNotReserveObject(t *testing.T) {
	machine := retryTestVM(t, `try return 73; finally suspend(); endtry`)
	machine.CurrentFrame().RecycleContinuation = &recycleContinuation{
		request: builtins.RecycleLifecycleRequest{Object: types.NewObj(12)},
	}
	machine.yielded = true
	snapshot := machine.PersistenceVMSnapshot()
	for range 2 {
		restored, err := RestoreVMSnapshot(snapshot, machine.Store, machine.Builtins, task.NewContext())
		if err != nil {
			t.Fatalf("numbered recycle restore: %v", err)
		}
		frame := restored.CurrentFrame()
		if frame == nil || frame.RecycleContinuation == nil || !frame.RecycleContinuation.request.Object.Equal(types.NewObj(12)) {
			t.Fatal("numbered recycle continuation was not restored")
		}
	}
}
