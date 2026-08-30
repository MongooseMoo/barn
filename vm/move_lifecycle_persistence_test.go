package vm

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
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
	restored, err := RestoreVMSnapshot(snapshot, store, session, kernel.NewTaskContext())
	if err != nil {
		t.Fatalf("RestoreVMSnapshot: %v", err)
	}
	state := restored.CurrentFrame().MoveContinuation
	if state == nil || state.Stage != moveAwaitExit || state.What.ID() != 12 || state.Where.ID() != 11 ||
		state.OldLocation.ID() != 10 || state.Position != 2 || !state.Decentralized {
		t.Fatalf("restored move continuation = %#v", state)
	}
}
