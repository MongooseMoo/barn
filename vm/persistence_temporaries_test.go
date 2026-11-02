package vm

import (
	"path/filepath"
	"testing"

	dbformat "github.com/MongooseMoo/barn/db/format"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A task suspended inside a loop holds its cursor in compiler temporaries above
// the named variables. Writing it to a checkpoint and loading that checkpoint
// back must yield a snapshot that verifies and resumes to the same result as
// the never-persisted task; this is the restart-from-checkpoint path the
// conformance suite exercises with resume()d suspended VMs.
func TestSuspendedLoopSurvivesCheckpointRoundTrip(t *testing.T) {
	for _, tc := range []struct{ loop, keyExpr, want string }{
		{"for x in ({1, 2, 3})", "0", "{6, {0, 0, 0}}"},
		{"for x, key in ({1, 2, 3})", "key", "{6, {1, 2, 3}}"},
		{`for x in (["c" -> 3, "a" -> 1, "b" -> 2])`, "0", "{6, {0, 0, 0}}"},
		{`for x, key in (["c" -> 3, "a" -> 1, "b" -> 2])`, "key", `{6, {"a", "b", "c"}}`},
		{`for x, key in ("123")`, "key", "{6, {1, 2, 3}}"},
	} {
		t.Run(tc.loop, func(t *testing.T) { testSuspendedLoopCheckpoint(t, tc.loop, tc.keyExpr, tc.want) })
	}
}

func testSuspendedLoopCheckpoint(t *testing.T, loop, keyExpr, want string) {
	result := resumeThroughCheckpoint(t, []string{
		"total = 0; keys = {};",
		loop,
		"  total = total + toint(x);",
		"  suspend();",
		"  keys = {@keys, " + keyExpr + "};",
		"endfor",
		"return {total, keys};",
	})
	if result.Flow != types.FlowReturn || result.Val.String() != want {
		t.Fatalf("restored result = %#v, want %s", result, want)
	}
}

// An except arm whose codes are expressions keeps its evaluated code list in a
// compiler temporary. A task suspended inside that try must still match the
// arm after a checkpoint reload.
func TestSuspendedExpressionCodeHandlerSurvivesCheckpointRoundTrip(t *testing.T) {
	result := resumeThroughCheckpoint(t, []string{
		`c = "E_CUSTOM";`,
		"try",
		"  suspend();",
		`  raise("e_custom");`,
		`except e ("E_OTHER", c)`,
		"  return e[1];",
		"endtry",
	})
	if result.Flow != types.FlowReturn || result.Val.String() != `"e_custom"` {
		t.Fatalf("restored result = %#v, want \"e_custom\"", result)
	}
}

// resumeThroughCheckpoint runs a program to its first suspension, writes the
// suspended task to a checkpoint, reloads it, and resumes it to completion. The
// program must use compiler temporaries, or the round trip proves nothing about
// them.
func resumeThroughCheckpoint(t *testing.T, source []string) types.Result {
	t.Helper()
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
	ctx := task.NewContext()
	ctx.Store = store
	taskValue := task.NewState(1, 2, ctx.TicksRemaining, 1)

	program, diagnostics := registry.Compiler().CompileMOO(source)
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

	restored, err := RestoreVMSnapshot(reloaded.SuspendedTasks[0].Snapshot.VM, store, session, task.NewContext())
	if err != nil {
		t.Fatalf("RestoreVMSnapshot after checkpoint reload: %v", err)
	}
	restored.Task = taskValue
	result := restored.Resume()
	for suspends := 0; result.Flow == types.FlowSuspend; suspends++ {
		if suspends > 3 {
			t.Fatalf("restored task keeps suspending; compiler temporaries were not restored")
		}
		result = restored.Resume()
	}
	return result
}
