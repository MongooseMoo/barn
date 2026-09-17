package vm

import (
	"path/filepath"
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A task suspended inside a loop holds its cursor in compiler temporaries above
// the named variables. Writing it to a checkpoint and loading that checkpoint
// back must yield a snapshot that verifies and resumes to the same result as
// the never-persisted task; this is the restart-from-checkpoint path the
// conformance suite exercises with resume()d suspended VMs.
func TestSuspendedLoopSurvivesCheckpointRoundTrip(t *testing.T) {
	store := dbstore.NewStore()
	for _, builder := range []*dbstore.ObjectBuilder{
		dbstore.NewObjectBuilder(0),
		dbstore.NewObjectBuilder(2),
	} {
		builder.SetOwner(2)
		builder.SetLocation(types.ObjNothing)
		builder.SetFlags(dbstore.FlagWizard | dbstore.FlagUser)
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", builder.ID(), err)
		}
	}
	registry := BuildVMRegistry()
	session := newTestSessionWithTaskManager(registry)
	ctx := kernel.NewTaskContext()
	ctx.Store = store
	taskValue := task.NewTask(1, 2, ctx.TicksRemaining, 1)

	program, diagnostics := registry.Compiler().CompileMOO([]string{
		"total = 0;",
		"for x in ({1, 2, 3})",
		"  total = total + x;",
		"  suspend();",
		"endfor",
		"return total;",
	})
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}
	if program.NumLocals <= len(program.VarNames) {
		t.Fatalf("NumLocals = %d, VarNames = %d; program does not use compiler temporaries so the test proves nothing", program.NumLocals, len(program.VarNames))
	}

	machine := NewVM(store, session)
	machine.Context = ctx
	machine.Task = taskValue
	if result := machine.Run(program); result.Flow != types.FlowSuspend {
		t.Fatalf("initial result = %#v, want suspend inside the loop", result)
	}

	snapshot := machine.PersistenceVMSnapshot()
	suspended := []task.Snapshot{{
		ID:            1,
		Owner:         2,
		State:         task.TaskSuspended,
		WakeValue:     types.NewInt(0),
		TaskLocal:     types.NewEmptyMap(),
		Programmer:    2,
		This:          0,
		ReadingPlayer: types.ObjNothing,
		VM:            snapshot,
	}}
	path := filepath.Join(t.TempDir(), "loop.db")
	if err := dbformat.WriteCheckpoint(path, store, nil, suspended, nil); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	if got := len(reloaded.SuspendedTasks); got != 1 {
		t.Fatalf("suspended tasks = %d, want 1", got)
	}

	restored, err := RestoreVMSnapshot(reloaded.SuspendedTasks[0].Snapshot.VM, store, session, kernel.NewTaskContext())
	if err != nil {
		t.Fatalf("RestoreVMSnapshot after checkpoint reload: %v", err)
	}
	restored.Task = taskValue
	result := restored.Resume()
	for suspends := 0; result.Flow == types.FlowSuspend; suspends++ {
		if suspends > 3 {
			t.Fatalf("restored task keeps suspending; loop cursor was not restored")
		}
		result = restored.Resume()
	}
	if result.Flow != types.FlowReturn || !result.Val.Equal(types.NewInt(6)) {
		t.Fatalf("restored result = %#v, want return 6", result)
	}
}
