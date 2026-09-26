package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// A threaded sort() suspends the task on its root VM, and the task resumes
// with the sorted list on a fresh background budget.
func TestThreadedSortResumesOnBackgroundBudget(t *testing.T) {
	store := newBoundaryTestStore(t)
	s := NewRuntime(store)
	defer s.Stop()

	prog, diagnostics := s.registry.Compiler().CompileMOO(strings.Split(`
before = ticks_left();
sorted = sort({3, 1, 2});
#0.write_value = {sorted, before, ticks_left()};
`, "\n"))
	if len(diagnostics) > 0 {
		t.Fatalf("CompileMOO failed: %v", diagnostics[0])
	}
	s.CreateForegroundTask(0, prog)

	fgTicks, _ := foregroundTaskLimits(s.session)
	bgTicks, _ := backgroundTaskLimits(s.session)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.ProcessReadyTasks() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		value, errCode := store.DirectTxn().PropertyValue(0, "write_value")
		if errCode != types.E_NONE {
			t.Fatalf("PropertyValue failed: %v", errCode)
		}
		if value.Type() != types.TYPE_LIST {
			continue
		}
		if got := value.Get(1).String(); got != "{1, 2, 3}" {
			t.Fatalf("sorted = %s, want {1, 2, 3}", got)
		}
		if before, after := value.Get(2).Int(), value.Get(3).Int(); before <= bgTicks || before > fgTicks || after > bgTicks {
			t.Fatalf("ticks before/after sort = %d/%d, want foreground (%d) then background (%d)", before, after, fgTicks, bgTicks)
		}
		return
	}
	t.Fatal("task never finished")
}

// create() runs :initialize on a nested VM that cannot suspend, so a sort()
// there runs inline and the task finishes in one slice.
func TestThreadedSortInsideNestedVerbRunsInline(t *testing.T) {
	store := newBoundaryTestStore(t)
	initialize := dbstore.NewVerb("initialize", []string{"initialize"}, 0, dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{"#0.write_value = sort({3, 1, 2});"})
	if _, errCode := store.AddVerb(0, initialize); errCode != types.E_NONE {
		t.Fatalf("AddVerb initialize: %v", errCode)
	}
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()

	ticks, seconds := foregroundTaskLimits(newTestRegistry())
	queued := task.NewTaskFull(3106, 0, compileTestProgram(t, s.registry, `
c = create(#0);
return {valid(c), #0.write_value};
`), ticks, seconds)
	queued.Context.IsWizard = true

	if err := s.runTask(queued); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}
	if queued.Result.Flow != types.FlowReturn || queued.Result.Val.String() != "{1, {1, 2, 3}}" {
		t.Fatalf("result = flow %v val %v err %v, want return {1, {1, 2, 3}}", queued.Result.Flow, queued.Result.Val, queued.Result.Error)
	}
}
