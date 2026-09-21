package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestIntrinsicEvalHandsSuspendedVMToScheduler(t *testing.T) {
	rt := NewRuntime(dbstore.NewStore())
	defer rt.Stop()
	completed := make(chan EvalOutcome, 1)
	rt.StartEval(types.ObjNothing, []string{"suspend(0); return 7;"}, func(out EvalOutcome) { completed <- out })
	select {
	case <-completed:
		t.Fatal("input lane resumed its own suspended eval")
	default:
	}
	if n := rt.ProcessReadyBatch(); n != 1 {
		t.Fatalf("scheduler could not acquire suspended eval: ran %d", n)
	}
	select {
	case out := <-completed:
		if out.Panic != nil || out.Result.Val != types.NewInt(7) {
			t.Fatalf("eval completion = %+v", out)
		}
	default:
		t.Fatal("terminal eval did not deliver its result")
	}
}

func TestEvalSettlesWhenForkKillsItsSuspendedParent(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(0)
	wizard.SetOwner(0)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime(store)
	defer rt.Stop()
	done := make(chan EvalOutcome, 1)
	go func() { done <- rt.Eval(0, []string{"id=task_id(); fork (0) kill_task(id); endfork suspend();"}) }()
	select {
	case out := <-done:
		if out.Result.Error != types.E_INVARG {
			t.Fatalf("cancelled eval = %+v", out)
		}
	case <-time.After(time.Second):
		t.Fatal("killed synchronous eval never settled")
	}
}

func TestEvalCompileCompletionCanReenterAdmission(t *testing.T) {
	rt := newRuntimeWithWorkerCount(dbstore.NewStore(), config.Options{AdmissionLimit: 1}, 1)
	defer rt.Stop()
	done := make(chan string, 1)
	go rt.StartEval(types.ObjNothing, []string{"if ("}, func(out EvalOutcome) {
		if len(out.Diagnostics) == 0 {
			done <- "missing diagnostics"
			return
		}
		done <- rt.EvalCommandOutput(types.ObjNothing, "return 42;")
	})
	select {
	case got := <-done:
		if got != "{1, 42}" {
			t.Fatalf("reentrant completion = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("compile completion retained its admission")
	}
}
