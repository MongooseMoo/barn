package engine

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// A suspended task's row names the programmer of the verb it is suspended in,
// which kill_task() and resume() check (Toast tasks.cc progr_of_cur_verb):
// here a task the wizard forked suspends inside a verb owned by p.
func TestSuspendedRowOwnerIsSuspendedVerbProgrammer(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(3)
	wizard.SetOwner(3)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}
	p, errCode := store.DirectTxn().CreateObject(nil, 3, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject p failed: %v", errCode)
	}
	if errCode := store.DirectTxn().SetObjectFlag(p, dbstore.FlagProgrammer, true); errCode != types.E_NONE {
		t.Fatalf("SetObjectFlag failed: %v", errCode)
	}
	obj, errCode := store.DirectTxn().CreateObject(nil, 3, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	park := dbstore.NewVerb([]string{"park"}, p, dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"}, []string{"suspend();"})
	if _, errCode := store.AddVerb(obj, park); errCode != types.E_NONE {
		t.Fatalf("AddVerb park failed: %v", errCode)
	}

	s := NewRuntime(store)
	defer s.Stop()
	line := s.EvalCommandOutput(3, fmt.Sprintf(
		"fork t (0) #%d:park(); endfork for i in [1..50] if (typeof(`task_stack(t) ! ANY') == LIST) break; endif suspend(0); endfor return t;", obj))
	value := strings.TrimSuffix(strings.TrimPrefix(line, "{1, "), "}")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("eval result = %q, want {1, task id}", line)
	}
	row, ok := store.TaskRow(id)
	if !ok || row.Status != dbstore.TaskRowSuspended {
		t.Fatalf("child row = %+v present %v, want suspended", row, ok)
	}
	if row.Owner != p {
		t.Fatalf("child row owner = %v, want #%d, the programmer of the verb it is suspended in", row.Owner, p)
	}
}
