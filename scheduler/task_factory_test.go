package scheduler

import (
	"testing"

	"barn/builtins"
	"barn/bytecode"
	dbstore "barn/db/store"
	"barn/task"
	"barn/types"
	"barn/vm"
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
	s := NewScheduler(store)
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
	defer task.GetManager().RemoveTask(forkID)

	forked := task.GetManager().GetTask(forkID)
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

func TestCreateForkedTaskReportsParentSourceLine(t *testing.T) {
	store := dbstore.NewStore()
	s := NewScheduler(store)
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
	defer task.GetManager().RemoveTask(forkID)

	forked := task.GetManager().GetTask(forkID)
	if forked == nil {
		t.Fatalf("forked task %d was not registered", forkID)
	}
	if got, want := forked.ToQueuedTaskInfo(false).Get(8).Int(), int64(6); got != want {
		t.Fatalf("queued task line = %d, want fork body source line %d", got, want)
	}
}
