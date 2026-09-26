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

// Thread mode belongs to each activation, and Toast persists it per activation.
// A task suspended inside eval() under an outer activation that disabled thread
// mode must come back from a checkpoint with the eval activation enabled and
// the outer activation still disabled once eval() returns.
func TestSuspendedActivationThreadModesSurviveCheckpointRoundTrip(t *testing.T) {
	store := dbstore.NewStore()
	for _, builder := range []*dbstore.ObjectBuilder{
		dbstore.NewObjectBuilder(0),
		dbstore.NewObjectBuilder(2),
	} {
		builder.SetOwner(2)
		builder.SetLocation(types.ObjNothing)
		builder.SetFlags(dbstore.FlagWizard | dbstore.FlagUser | dbstore.FlagProgrammer)
		if err := store.Add(builder.Build()); err != nil {
			t.Fatalf("add object #%d: %v", builder.ID(), err)
		}
	}
	registry := BuildVMRegistry()
	session := newTestSessionWithTaskManager(registry)
	newContext := func() *kernel.TaskContext {
		ctx := kernel.NewTaskContext()
		ctx.Store = store
		ctx.Player = 2
		ctx.Programmer = 2
		ctx.IsWizard = true
		return ctx
	}
	ctx := newContext()
	taskValue := task.NewTask(1, 2, ctx.TicksRemaining, 1)

	program, diagnostics := registry.Compiler().CompileMOO([]string{
		"set_thread_mode(0);",
		`inner = eval("suspend(); return set_thread_mode();");`,
		"return {inner, set_thread_mode()};",
	})
	if len(diagnostics) > 0 {
		t.Fatalf("compile failed: %v", diagnostics)
	}

	machine := NewVM(store, session)
	machine.Context = ctx
	machine.Task = taskValue
	if result := machine.Run(program); result.Flow != types.FlowSuspend {
		t.Fatalf("initial result = %#v, want suspend inside eval()", result)
	}

	snapshot := machine.PersistenceVMSnapshot()
	if got := len(snapshot.Frames); got != 2 {
		t.Fatalf("snapshot frames = %d, want outer and eval activations", got)
	}
	if snapshot.Frames[0].ThreadMode || !snapshot.Frames[1].ThreadMode {
		t.Fatalf("snapshot thread modes = {%t, %t}, want {false, true}", snapshot.Frames[0].ThreadMode, snapshot.Frames[1].ThreadMode)
	}

	path := filepath.Join(t.TempDir(), "thread-mode.db")
	if err := dbformat.WriteCheckpoint(path, store, nil, []task.Snapshot{{
		ID:            1,
		Owner:         2,
		State:         task.TaskSuspended,
		WakeValue:     types.NewInt(0),
		TaskLocal:     types.NewEmptyMap(),
		Programmer:    2,
		This:          0,
		ReadingPlayer: types.ObjNothing,
		VM:            snapshot,
	}}, nil); err != nil {
		t.Fatalf("WriteCheckpoint failed: %v", err)
	}
	reloaded, err := dbformat.LoadDatabase(path + ".new")
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}
	if got := len(reloaded.SuspendedTasks); got != 1 {
		t.Fatalf("suspended tasks = %d, want 1", got)
	}

	restoredCtx := newContext()
	restored, err := RestoreVMSnapshot(reloaded.SuspendedTasks[0].Snapshot.VM, store, session, restoredCtx)
	if err != nil {
		t.Fatalf("RestoreVMSnapshot after checkpoint reload: %v", err)
	}
	if !restoredCtx.ThreadMode {
		t.Fatalf("restored running eval activation has thread mode off, want on")
	}
	restored.Task = taskValue
	result := restored.Resume()
	if result.Flow != types.FlowReturn || result.Val.String() != "{{1, 1}, 0}" {
		t.Fatalf("restored result = %#v, want {{1, 1}, 0}", result)
	}
}
