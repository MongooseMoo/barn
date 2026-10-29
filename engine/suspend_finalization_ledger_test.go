package engine

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A task that keeps suspending must not carry every WAIF and anonymous object
// it ever released in its finalization ledger: the suspend hand-off queues the
// WAIFs for the deferred sweep, and recycled anonymous objects need no root.
func TestSuspendPrunesPendingFinalizationLedger(t *testing.T) {
	store := newConflictTestStore(t)
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()
	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	setup := NewTaskFull(5401, 0, compileTestProgram(t, s.registry, `c = create(-1);
add_verb(c, {#0, "xd", "new"}, {"this", "none", "this"});
set_verb_code(c, "new", {"return new_waif();"});
add_property(#0, "maker", c, {#0, "rw"});
return 0;`), ticks, seconds)
	setup.Context.IsWizard = true
	if err := s.runTask(setup); err != nil {
		t.Fatal(err)
	}
	runUntilTerminal(t, s, setup)

	const iterations = 50
	loop := NewTaskFull(5402, 0, compileTestProgram(t, s.registry, `for i in [1..50]
  w = #0.maker:new();
  a = create(#0, 1);
  suspend(0);
  #0.v = i;
endfor
suspend();`), ticks, seconds)
	loop.Context.IsWizard = true
	defer removeTasksForOwner(s, 0)
	if err := s.runTask(loop); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for readRootV(t, store) != iterations || loop.GetState() != task.TaskSuspended {
		if time.Now().After(deadline) {
			t.Fatalf("loop did not reach its final suspend: v=%d state=%v", readRootV(t, store), loop.GetState())
		}
		s.ProcessReadyTasks()
	}
	s.flushDeferredGC()

	ledger := loop.VM().PendingFinalizations
	waifs := 0
	for _, value := range ledger {
		if value.Type() == types.TYPE_WAIF {
			waifs++
		}
	}
	if waifs != 0 || len(ledger) > 2 {
		t.Fatalf("ledger after %d suspends holds %d entries (%d WAIFs), want it pruned", iterations, len(ledger), waifs)
	}
}
