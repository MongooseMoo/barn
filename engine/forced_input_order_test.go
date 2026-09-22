package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

type deferredInput struct{ complete func() }

func (f *deferredInput) ForceInput(_ types.ObjID, _ string, _ bool, complete func()) {
	f.complete = complete
}

func TestForcedInputPrecedesSourceContinuationWithoutBlockingOthers(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(0)
	wizard.SetOwner(0)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime(store)
	defer rt.Stop()
	forcer := &deferredInput{}
	host := rt.Session().Host()
	host.InputForcer = forcer
	rt.Session().ConfigureHost(host)
	done := make(chan EvalOutcome, 1)
	source := rt.StartEval(0, []string{`force_input(#0, "injected"); suspend(0); return 9;`}, func(out EvalOutcome) { done <- out })
	if forcer.complete == nil || source.GetState() != task.TaskQueued {
		t.Fatal("source did not inject input and yield")
	}
	other := rt.CreateBackgroundTask(0, compileTestProgram(t, rt.registry, "return 1;"), 0)
	if n := rt.ProcessReadyBatch(); n != 1 || rt.GetTask(other).GetState() != task.TaskCompleted {
		t.Fatalf("unrelated work blocked: ran %d", n)
	}
	select {
	case <-done:
		t.Fatal("source resumed before its target's first slice completed")
	default:
	}
	forcer.complete()
	if n := rt.ProcessReadyBatch(); n != 1 {
		t.Fatalf("source not scheduled after input completion: ran %d", n)
	}
	select {
	case out := <-done:
		if out.Result.Val != types.NewInt(9) {
			t.Fatalf("source result = %+v", out)
		}
	default:
		t.Fatal("source result missing")
	}
}
