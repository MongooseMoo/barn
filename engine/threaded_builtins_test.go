package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
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

// Synchronous host calls do not retain a suspended VM. A threaded builtin in
// a nested verb on that VM must finish inline, including inside :initialize.
func TestThreadedSortInsideSynchronousHostCallRunsInline(t *testing.T) {
	store := newBoundaryTestStore(t)
	initialize := dbstore.NewVerb([]string{"initialize"}, 0, dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{"#0.write_value = sort({3, 1, 2});"})
	if _, errCode := store.AddVerb(0, initialize); errCode != types.E_NONE {
		t.Fatalf("AddVerb initialize: %v", errCode)
	}
	driver := dbstore.NewVerb([]string{"drive"}, 0, dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{
			"c = create(#0); return {valid(c), #0.write_value};",
		})
	if _, errCode := store.AddVerb(0, driver); errCode != types.E_NONE {
		t.Fatalf("AddVerb drive: %v", errCode)
	}
	s := newTestRuntimeWithWorkersAndBuiltins(t, store, config.Options{}, 1)
	defer s.Stop()

	result := s.CallVerb(0, "drive", nil, 0)
	if result.Flow != types.FlowReturn || result.Val.String() != "{1, {1, 2, 3}}" {
		t.Fatalf("result = flow %v val %v err %v, want return {1, {1, 2, 3}}", result.Flow, result.Val, result.Error)
	}
}
