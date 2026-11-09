package engine

import (
	"fmt"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// conformance fork/fork_timing.yaml suspend_zero_queued_tasks_lists_child_suspended_first:
// a parent that forked a child, suspend(0)s, lists queued_tasks() and kills the
// child must return normally. In the full statement recycle() is an
// irreversible boundary that publishes the kill; the child then settles and
// its row is removed, which the parent's final commit must not lose to.
func TestSuspendZeroThenKillChildCommits(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(3)
	wizard.SetOwner(3)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}
	obj, errCode := store.DirectTxn().CreateObject(nil, 3, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	park := dbstore.NewVerb([]string{"park"}, 3, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"for i in [1..3000]", "endfor", "suspend();"})
	if _, errCode := store.AddVerb(obj, park); errCode != types.E_NONE {
		t.Fatalf("AddVerb park failed: %v", errCode)
	}
	run := dbstore.NewVerb([]string{"run"}, 3, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"fork child (0)", "  this:park();", "endfork", "suspend(0);", "found = 0;",
			"for entry in (queued_tasks())", "  if (entry[1] == child)", "    found = 1;", "  endif", "endfor",
			"kill_task(child);", "return found;"})
	if _, errCode := store.AddVerb(obj, run); errCode != types.E_NONE {
		t.Fatalf("AddVerb run failed: %v", errCode)
	}

	s := NewRuntime(store)
	defer s.Stop()
	line := s.EvalCommandOutput(3, fmt.Sprintf("return #%d:run();", obj))
	if line != "{1, 1}" {
		t.Fatalf("eval result = %q, want {1, 1}", line)
	}
	full := `o = create(#-1);
add_verb(o, {player, "xd", "park"}, {"this", "none", "this"});
set_verb_code(o, "park", {"for i in [1..3000]", "endfor", "suspend();"});
add_verb(o, {player, "xd", "run"}, {"this", "none", "this"});
set_verb_code(o, "run", {"fork child (0)", "  this:park();", "endfork", "suspend(0);", "found = 0;", "for entry in (queued_tasks())", "  if (entry[1] == child)", "    found = 1;", "  endif", "endfor", "kill_task(child);", "return found;"});
result = o:run();
recycle(o);
return result;`
	if line := s.EvalCommandOutput(3, full); line != "{1, 1}" {
		t.Fatalf("full eval result = %q, want {1, 1}", line)
	}
}
