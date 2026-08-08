package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

func TestRunTaskDoesNotRecommitAfterTerminalCommitFailure(t *testing.T) {
	store := dbstore.NewStore()
	for _, id := range []types.ObjID{0, 1, 2} {
		if err := store.Add(dbstore.NewObject(id, 0)); err != nil {
			t.Fatalf("Add(#%d): %v", id, err)
		}
	}

	s := NewRuntime(store)
	defer s.Stop()
	s.registry.Register("force_terminal_commit", func(ctx *kernel.TaskContext, _ []types.Value) types.Result {
		if errCode := ctx.StoreTxn.SetObjectName(0, "private"); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		if errCode := ctx.StoreTxn.MoveObject(1, 2, 0); errCode != types.E_NONE {
			return types.Err(errCode)
		}
		// Simulate a concurrent coarse mutation after MoveObject staged a blind
		// contents add. The missing write target is a terminal preflight failure,
		// not a retryable read-set conflict.
		if err := store.Recycle(2); err != nil {
			t.Errorf("Recycle(#2): %v", err)
			return types.Err(types.E_INVARG)
		}
		return types.Ok(types.None)
	})

	ticks, seconds := foregroundTaskLimits()
	running := task.NewTaskFull(94002, 0, compileTestProgram(t, s.registry, `
force_terminal_commit();
return 1;
`), ticks, seconds)
	running.Context.IsWizard = true
	beforeAttempts := store.CommitAttempts()
	if err := s.runTask(running); err != nil {
		t.Fatalf("runTask: %v", err)
	}

	if got := store.CommitAttempts() - beforeAttempts; got != 1 {
		t.Errorf("terminal transaction commit attempts = %d, want 1", got)
	}
	if running.Result.Flow != types.FlowException || running.Result.Error != types.E_INVIND {
		t.Errorf("terminal task result = %+v, want E_INVIND exception", running.Result)
	}
	if got, errCode := store.ObjectName(0); errCode != types.E_NONE || got != "" {
		t.Errorf("terminal commit published name = %q, %v; want empty, E_NONE", got, errCode)
	}
}
