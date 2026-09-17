package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A task that recycles an object created by an earlier task (a staged,
// decentralized recycle) and then runs a coarse builtin naming that object must
// see it as invalid after the flush. The flush used to drop the recycled
// object's cache entry, and the next read re-resolved through the snapshot,
// which still held the pre-recycle image: chparent then passed its validity
// check and died reading the phantom parent's properties with E_INVIND where
// Toast (objects.cc bf_chparent_chparents) returns E_INVARG.
func TestRecycleThenChparentOnCommittedObjectsReturnsInvarg(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(root.Build()); err != nil {
		t.Fatal(err)
	}
	s := NewRuntime(store)
	defer s.Stop()
	run := func(code string) types.Result {
		t.Helper()
		program := compileTestProgram(t, s.registry, code)
		taskID := s.CreateBackgroundTask(0, program, 0)
		running := s.GetTask(taskID)
		running.Context.IsWizard = true
		for pass := 0; pass < 8 && running.GetState() != task.TaskCompleted && running.GetState() != task.TaskKilled; pass++ {
			s.ProcessReadyTasks()
		}
		return running.Result
	}
	if got := run("return create(#-1);"); got.Flow != types.FlowReturn || got.Val.String() != "#1" {
		t.Fatalf("first create = %+v, want #1", got)
	}
	if got := run("return create(#-1);"); got.Flow != types.FlowReturn || got.Val.String() != "#2" {
		t.Fatalf("second create = %+v, want #2", got)
	}
	got := run("recycle(#2); return {`chparent(#1, #2) ! ANY', valid(#2), `chparents(#1, {#2}) ! ANY'};")
	if got.Flow != types.FlowReturn || got.Val.String() != "{E_INVARG, 0, E_INVARG}" {
		t.Fatalf("recycle then chparent = flow %v val %v err %v, want {E_INVARG, 0, E_INVARG}", got.Flow, got.Val, got.Error)
	}
}
