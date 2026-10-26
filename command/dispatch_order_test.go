package command

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func addDispatchTestVerb(t *testing.T, store *dbstore.Store, objID types.ObjID, names string, args dbstore.VerbArgs, code string) {
	t.Helper()
	_, err := store.AddVerb(objID, dbstore.NewVerb([]string{names}, 2,
		dbstore.VerbRead|dbstore.VerbExecute, args, []string{code}))
	if err != types.E_NONE {
		t.Fatalf("add verb #%d:%s: %v", objID, names, err)
	}
}

// Two verbs on one object may share their first name and differ in argument
// specs. The command must reach the one whose specs match.
func TestFindVerbReachesASecondVerbWithTheSameName(t *testing.T) {
	store := dbstore.NewStore()
	addCommandTestObject(t, store, 2, "player", types.ObjNothing, nil)
	addDispatchTestVerb(t, store, 2, "put", dbstore.VerbArgs{This: "this", Prep: "none", That: "none"}, "return 1;")
	addDispatchTestVerb(t, store, 2, "put", dbstore.VerbArgs{This: "none", Prep: "none", That: "none"}, "return 2;")

	cmd := NewParsedCommand()
	cmd.Verb = "put"
	match := FindVerb(store, 2, types.ObjNothing, cmd)
	if match == nil {
		t.Fatal("`put` with no object found no verb; the second `put` takes no object")
	}
	if got := match.Verb.Code[0]; got != "return 2;" {
		t.Fatalf("matched the verb with code %q, want the second `put`", got)
	}
}

// When two verbs on one object both match, the one defined first wins.
func TestFindVerbPrefersTheVerbDefinedFirst(t *testing.T) {
	store := dbstore.NewStore()
	addCommandTestObject(t, store, 2, "player", types.ObjNothing, nil)
	none := dbstore.VerbArgs{This: "none", Prep: "none", That: "none"}
	addDispatchTestVerb(t, store, 2, "a*bc", none, "return 1;")
	for _, name := range []string{"ab", "abc", "ab*c", "a*b", "*"} {
		addDispatchTestVerb(t, store, 2, name, none, "return 2;")
	}

	cmd := NewParsedCommand()
	cmd.Verb = "ab"
	for attempt := 0; attempt < 200; attempt++ {
		match := FindVerb(store, 2, types.ObjNothing, cmd)
		if match == nil {
			t.Fatal("`ab` found no verb")
		}
		if got := match.Verb.Names[0]; got != "a*bc" {
			t.Fatalf("attempt %d matched %q, want the first verb a*bc", attempt, got)
		}
	}
}

// An ancestor's verb is found, and the child's verb comes before it.
func TestFindVerbSearchesTheChildBeforeItsParent(t *testing.T) {
	store := dbstore.NewStore()
	addCommandTestObject(t, store, 1, "parent", types.ObjNothing, nil)
	builder := dbstore.NewObjectBuilder(2)
	builder.SetName("player")
	builder.SetOwner(2)
	builder.SetParents([]types.ObjID{1})
	if err := store.Add(builder.Build()); err != nil {
		t.Fatalf("add object #2: %v", err)
	}
	none := dbstore.VerbArgs{This: "none", Prep: "none", That: "none"}
	addDispatchTestVerb(t, store, 1, "wave", none, "return 1;")
	addDispatchTestVerb(t, store, 1, "bow", none, "return 1;")
	addDispatchTestVerb(t, store, 2, "wave", none, "return 2;")

	cmd := NewParsedCommand()
	cmd.Verb = "wave"
	match := FindVerb(store, 2, types.ObjNothing, cmd)
	if match == nil || match.VerbLoc != 2 || match.This != 2 {
		t.Fatalf("`wave` matched %+v, want the verb defined on #2", match)
	}
	cmd.Verb = "bow"
	match = FindVerb(store, 2, types.ObjNothing, cmd)
	if match == nil || match.VerbLoc != 1 || match.This != 2 {
		t.Fatalf("`bow` matched %+v, want the verb defined on #1 with this #2", match)
	}
}
