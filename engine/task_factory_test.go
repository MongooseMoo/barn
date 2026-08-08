package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/bytecode"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
	"github.com/MongooseMoo/barn/vm"
)

func TestConfigureVMStackLimitReadsLiveServerOption(t *testing.T) {
	builtins.LoadServerOptionsFromStore(nil)
	t.Cleanup(func() { builtins.LoadServerOptionsFromStore(nil) })

	store := dbstore.NewStore()
	system := dbstore.NewObjectBuilder(0)
	system.SetProperty("server_options", dbstore.NewProperty(
		types.NewObj(1), 0, dbstore.PropRead|dbstore.PropWrite, false, true,
	))
	if err := store.Add(system.Build()); err != nil {
		t.Fatalf("add system object: %v", err)
	}
	options := dbstore.NewObjectBuilder(1)
	options.SetProperty("max_stack_depth", dbstore.NewProperty(
		types.NewInt(60), 0, dbstore.PropRead, false, true,
	))
	if err := store.Add(options.Build()); err != nil {
		t.Fatalf("add server options object: %v", err)
	}

	machine := vm.NewVM(store, builtins.NewRegistry())
	configureVMStackLimit(machine)
	if machine.MaxStackDepth != 60 {
		t.Fatalf("VM max stack depth = %d, want live $server_options value 60", machine.MaxStackDepth)
	}
}

func TestCreateForkedTaskUsesCurrentProgrammer(t *testing.T) {
	store := dbstore.NewStore()
	s := NewRuntime(store)
	defer s.Stop()

	program := compileTestProgram(t, s.registry, "return 1;")
	parent := task.NewTaskFull(6101, 3, program, 1000, 1)
	parent.Programmer = 3
	parent.Context.Programmer = 2

	forkID := s.CreateForkedTask(parent, &types.ForkInfo{
		Body:      [3]interface{}{program, 0, len(program.Code)},
		ThisObj:   0,
		ThisValue: types.NewObj(0),
		Player:    3,
		Caller:    3,
		Verb:      "delayed",
		VerbLoc:   0,
		Variables: map[string]types.Value{
			"marker": types.NewStr("current-programmer"),
		},
	})
	defer s.taskManager.RemoveTask(forkID)

	forked := s.taskManager.GetTask(forkID)
	if forked == nil {
		t.Fatalf("forked task %d was not registered", forkID)
	}
	if got, want := forked.Programmer, types.ObjID(2); got != want {
		t.Fatalf("forked task programmer = #%d, want current task programmer #%d", got, want)
	}
	if got, want := forked.ToQueuedTaskInfo(false).Get(5).Obj(), types.ObjID(2); got != want {
		t.Fatalf("queued task programmer = #%d, want current task programmer #%d", got, want)
	}
}

func TestTaskSnapshotsExcludeKilledSuspendedVMTask(t *testing.T) {
	store := dbstore.NewStore()
	s := NewRuntime(store)
	defer s.Stop()

	ticks, seconds := backgroundTaskLimits()
	killed := task.NewTaskFull(
		6200,
		3,
		compileTestProgram(t, s.registry, "suspend(100);"),
		ticks,
		seconds,
	)
	s.populateTaskContextDependencies(killed.Context)
	killed.IsForked = true
	killed.ForkCreator = s
	s.QueueTask(killed)
	defer s.taskManager.RemoveTask(killed.ID)

	if got := s.ProcessReadyTasks(); got != 1 {
		t.Fatalf("scheduler pass ran %d tasks, want 1", got)
	}
	if got := killed.GetState(); got != task.TaskSuspended {
		t.Fatalf("task state = %s, want suspended", got)
	}
	if killed.BytecodeVMValue() == nil {
		t.Fatal("suspended task has no saved VM")
	}

	killed.Kill()
	queued, suspended := s.TaskSnapshots()
	if len(queued) != 0 || len(suspended) != 0 {
		t.Fatalf("checkpoint captured killed task: queued=%d suspended=%d", len(queued), len(suspended))
	}
}

func TestCreateForkedTaskReportsParentSourceLine(t *testing.T) {
	store := dbstore.NewStore()
	s := NewRuntime(store)
	defer s.Stop()

	program := &bytecode.Program{
		Code: []byte{byte(bytecode.OP_RETURN_NONE)},
		LineInfo: []bytecode.LineEntry{{
			StartIP: 0,
			Line:    6,
		}},
	}
	parent := task.NewTaskFull(6102, 3, program, 1000, 1)

	forkID := s.CreateForkedTask(parent, &types.ForkInfo{
		Body:      [3]interface{}{program, 0, len(program.Code)},
		ThisObj:   0,
		ThisValue: types.NewObj(0),
		Player:    3,
		Caller:    3,
		Verb:      "delayed",
		VerbLoc:   0,
	})
	defer s.taskManager.RemoveTask(forkID)

	forked := s.taskManager.GetTask(forkID)
	if forked == nil {
		t.Fatalf("forked task %d was not registered", forkID)
	}
	if got, want := forked.ToQueuedTaskInfo(false).Get(8).Int(), int64(6); got != want {
		t.Fatalf("queued task line = %d, want fork body source line %d", got, want)
	}
}
