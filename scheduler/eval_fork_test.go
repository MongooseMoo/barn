package scheduler

import (
	"fmt"
	"testing"
	"time"

	dbstore "barn/db/store"
	"barn/parser"
	"barn/types"
)

func TestEvalForkedSuspenderCanBeInspectedWithTaskStack(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(3)
	wizard.SetOwner(3)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}
	obj, errCode := store.CreateObject(nil, 3, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	verb := dbstore.NewVerb(
		"suspender",
		[]string{"suspender"},
		3,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"suspend(100);", "return 42;"},
	)
	if _, errCode := store.AddVerb(obj, verb); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	s := NewScheduler(store)
	defer s.Stop()

	lines := s.EvalCommandOutput(3, fmt.Sprintf(
		"fork id (0) "+
			"#%d:suspender(); "+
			"endfork "+
			"suspend(0); "+
			"s = task_stack(id);\n"+
			"kill_task(id);\n"+
			"return typeof(s);\n",
		obj,
	), "-=!-^-!=-", "-=!-v-!=-")

	if len(lines) != 3 {
		t.Fatalf("lines = %#v, want prefix/result/suffix", lines)
	}
	if lines[1] != "{1, 4}" {
		t.Fatalf("eval result = %q, want {1, 4}", lines[1])
	}
}

func TestForkedZeroDelayResumeCommitsPostSuspendWrites(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(0)
	wizard.SetOwner(0)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser | dbstore.FlagRead | dbstore.FlagWrite)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}
	if errCode := store.DefineProperty(0, dbstore.NewProperty("fork_value", types.NewList(nil), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	p := parser.NewParser(`
set_task_local({"parent", 7});
fork (0)
  suspend(0);
  #0.fork_value = task_local();
endfork
suspend(0);
suspend(0);
return #0.fork_value;
`)
	stmts, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("ParseProgram failed: %v", err)
	}

	s := NewScheduler(store)
	defer s.Stop()
	s.CreateForegroundTask(0, stmts)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.ProcessReadyTasks() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		value, errCode := store.PropertyValue(0, "fork_value")
		if errCode != types.E_NONE {
			t.Fatalf("PropertyValue failed: %v", errCode)
		}
		if _, ok := value.(types.MapValue); ok {
			return
		}
	}
	value, _ := store.PropertyValue(0, "fork_value")
	t.Fatalf("fork_value = %T %v, want empty map from fork task_local()", value, value)
}
