package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Issue 423: the fork's committed write must not let its yielding parent run
// before the uncaught-error handler publishes its own write. Hold the real
// handler before that write, so the scheduler sees this window every run.
func TestUncaughtHandlerBeforeYieldedParent(t *testing.T) {
	store := newConflictTestStore(t)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	var pause builtins.BuiltinFunc = func(_ *builtins.Execution, _ []types.Value) types.Result {
		close(entered)
		<-release
		return types.Ok(types.NewInt(1))
	}
	rt := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 2,
		testBuiltinSlot("hold_error_handler", 0, 0, nil, &pause))
	defer rt.Stop()
	defer unblock()
	admissionVerb(t, store, "handle_uncaught_error",
		`hold_error_handler(); #0.v = {#0.v, args[1] == E_TYPE}; return 1;`)
	admissionVerb(t, store, "parent",
		`fork (0) #0.v = 194; raise(E_TYPE); endfork`,
		`suspend(0); p = #0.v; return typeof(p) == LIST ? p | {p, 0};`)
	var parentID int64
	result, err := rt.RunServerVerbTaskWithArgstr(0, "parent", nil, 0, "", func(id int64) { parentID = id })
	if err != nil || result.Flow != types.FlowSuspend {
		t.Fatalf("parent did not yield: result=%+v err=%v", result, err)
	}
	done := make(chan int, 1)
	go func() { done <- rt.ProcessReadyBatch() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("fork did not reach the uncaught-error handler")
	}
	var child *task.Task
	for _, candidate := range rt.taskManager.Snapshot() {
		if candidate.IsForked {
			child = candidate
			break
		}
	}
	if child == nil || child.GetState() != task.TaskKilled || child.Result.Error != types.E_TYPE {
		t.Fatal("handler was not entered for a fork killed by E_TYPE")
	}
	if got := readRootV(t, store); got != 194 {
		t.Fatalf("fork's committed probe = %d, want 194", got)
	}
	// Use the same unfinished-task catalog as the production dispatch loop.
	// No sleeps or repeated attempts determine whether the parent is eligible.
	resumed := rt.ProcessReadyBatch()
	t.Logf("handler held: fork killed by E_TYPE, committed probe=194, parent dispatched=%d", resumed)
	unblock()
	select {
	case count := <-done:
		if count != 1 {
			t.Fatalf("fork batch ran %d tasks, want 1", count)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("uncaught-error handler did not finish after release")
	}
	rt.ProcessReadyBatch()
	parent := rt.GetTask(parentID)
	want := types.NewList([]types.Value{types.NewInt(194), types.NewInt(1)})
	if parent.GetState() != task.TaskCompleted || !parent.Result.Val.Equal(want) {
		t.Fatalf("parent observed %s, want {194, 1}; dispatched while handler held=%d", parent.Result.Val.String(), resumed)
	}
	if resumed != 0 {
		t.Fatalf("parent dispatched before handler publication: %d", resumed)
	}
	probe, code := store.DirectTxn().PropertyValue(0, "v")
	if code != types.E_NONE || !probe.Equal(want) {
		t.Fatalf("handler's final publication = %s (%v), want %s", probe.String(), code, want.String())
	}
	t.Logf("handler committed %s; parent observed %s", probe.String(), parent.Result.Val.String())
}
