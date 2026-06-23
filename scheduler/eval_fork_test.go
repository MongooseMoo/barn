package scheduler

import (
	"fmt"
	"testing"

	dbstore "barn/db/store"
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
		"fork id (0)\n"+
			"  #%d:suspender();\n"+
			"endfork\n"+
			"suspend(0);\n"+
			"s = task_stack(id);\n"+
			"kill_task(id);\n"+
			"return typeof(s);\n",
		obj,
	), "", "")

	if len(lines) != 1 {
		t.Fatalf("lines = %#v, want one result", lines)
	}
	if lines[0] != "{1, 4}" {
		t.Fatalf("eval result = %q, want {1, 4}", lines[0])
	}
}
