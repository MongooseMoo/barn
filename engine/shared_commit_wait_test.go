package engine

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/metrics"
	"github.com/MongooseMoo/barn/types"
)

func TestTaskRowCommitWaitReachesRuntimeDiagnostics(t *testing.T) {
	rt := NewRuntime(newConflictTestStore(t))
	defer rt.Stop()
	queued := NewTaskFull(82001, 0, compileTestProgram(t, rt.registry, "return 7;"), 10000, 3)
	queued.This, queued.VerbName = 0, "row_wait"
	rt.QueueTask(queued)
	phase, known := metrics.ParsePhase("shared_commit_gate")
	var before metrics.PhaseStats
	if known {
		before = metrics.RuntimePhaseStats(phase)
	}
	grant, err := rt.store.AcquireExclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer grant.Release()
	done := make(chan error, 1)
	go func() { done <- rt.runTask(queued) }()
	deadline := time.Now().Add(3 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(stack, "[select]") && strings.Contains(stack, "(*StoreTxn).commitTaskRowsOnly(") && strings.Contains(stack, "(*Gate).Acquire(") {
				blocked = true
				break
			}
		}
		if blocked {
			break
		}
		runtime.Gosched()
	}
	if blocked {
		// The waiter is already proven queued; span Windows clock resolution.
		time.Sleep(5 * time.Millisecond)
	}
	grant.Release()
	if !blocked {
		t.Fatal("task did not wait in its task-row commit behind the exclusive owner")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("task did not complete after the gate was released")
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 7 {
		t.Fatalf("task result = %+v", queued.Result)
	}
	if !known {
		t.Fatal("task-row commit wait has no runtime diagnostic phase")
	}
	got := metrics.RuntimePhaseStats(phase).Sub(before)
	if got.Count != 1 || got.TotalNS <= 0 {
		t.Fatalf("shared commit wait = %+v, want the one real task-row commit", got)
	}
	if rt.AdmissionStats().GateWait <= 0 {
		t.Fatal("task-row commit wait was charged as execution service")
	}
}

func TestStandaloneHookSharedCommitWaitIsVisible(t *testing.T) {
	for _, sweep := range []bool{false, true} {
		name := "admitted"
		if sweep {
			name = "sweep_without_admission_scope"
		}
		t.Run(name, func(t *testing.T) {
			store := newConflictTestStore(t)
			admissionVerb(t, store, "row_wait", "#0.v = 9; return 7;")
			rt := NewRuntime(store)
			defer rt.Stop()
			phase, ok := metrics.ParsePhase("shared_commit_gate")
			if !ok {
				t.Fatal("shared commit wait phase is missing")
			}
			before := metrics.RuntimePhaseStats(phase)
			var result types.Result
			if sweep {
				// The production sweep owns these barriers before invoking a hook.
				rt.lifecycle.SweepMu.Lock()
				rt.lifecycle.VMStartMu.Lock()
				result = rt.callVerbWithArgstr(types.NewObj(0), "row_wait", nil, types.ObjNothing, "", vmOwnershipSweep, 0, nil)
				rt.lifecycle.VMStartMu.Unlock()
				rt.lifecycle.SweepMu.Unlock()
			} else {
				result = rt.CallVerb(0, "row_wait", nil, 0)
			}
			if result.Flow != types.FlowReturn || result.Val.Int() != 7 {
				t.Fatalf("hook result = %+v", result)
			}
			value, code := store.DirectTxn().FindProperty(0, "v")
			if code != types.E_NONE || value.Value.Int() != 9 {
				t.Fatalf("hook write = %v/%v", value, code)
			}
			if got := metrics.RuntimePhaseStats(phase).Sub(before); got.Count != 1 {
				t.Fatalf("hook shared waits = %+v, want one committed hook observation", got)
			}
		})
	}
}
