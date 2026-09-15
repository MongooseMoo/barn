package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// protectedRedirectStore builds #0 (system object, $server_options -> #1),
// #1 (the server options object) and #2 (a wizard player).
func protectedRedirectStore(t *testing.T) *dbstore.Store {
	t.Helper()
	store := dbstore.NewStore()
	for id, flags := range map[types.ObjID]dbstore.ObjectFlags{
		0: dbstore.FlagRead,
		1: dbstore.FlagRead,
		2: dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser | dbstore.FlagRead,
	} {
		b := dbstore.NewObjectBuilder(id)
		b.SetOwner(2)
		b.SetLocation(types.ObjNothing)
		b.SetFlags(flags)
		if id == 0 {
			b.SetProperty("server_options", dbstore.NewProperty(types.NewObj(1), 2, dbstore.PropRead, false, true))
		}
		if err := store.Add(b.Build()); err != nil {
			t.Fatalf("add #%d: %v", id, err)
		}
	}
	return store
}

const protectValidSetup = `add_property(#1, "protect_valid", 1, {player, "r"});` +
	`add_verb(#0, {player, "rxd", "bf_valid"}, {"this", "none", "this"});` +
	`set_verb_code(#0, "bf_valid", {"return valid(args[1]) + 10;"});` +
	`load_server_options();`

const protectValidProbe = `o = create(#-1);` +
	`add_verb(o, {player, "rxd", "probe"}, {"this", "none", "this"});` +
	`set_verb_code(o, "probe", {"return valid(args[1]);"});` +
	`return {o:probe(o), o:probe(#-1), valid(o)};`

func TestProtectedBuiltinRedirectsVerbFrameSameTask(t *testing.T) {
	s := NewRuntime(protectedRedirectStore(t))
	defer s.Stop()
	line := s.EvalCommandOutput(2, protectValidSetup+protectValidProbe)
	if want := "{1, {11, 10, 11}}"; line != want {
		t.Fatalf("same task: line = %q, want %q", line, want)
	}
}

// The conformance harness's ";" runs through Test.db's #2:eval command verb, so
// the eval is a real committing task whose $server_options writes are staged.
// Toast's load_server_options() still takes effect at once for that task
// (protected_builtin_redirect.yaml::protected_valid_verb_on_ordinary_object_uses_wrapper).
func TestProtectedBuiltinRedirectsVerbFrameInCommittingTask(t *testing.T) {
	store := protectedRedirectStore(t)
	verb := dbstore.NewVerb(
		"go",
		[]string{"go"},
		2,
		dbstore.VerbRead|dbstore.VerbExecute,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{protectValidSetup + protectValidProbe},
	)
	if _, errCode := store.AddVerb(1, verb); errCode != types.E_NONE {
		t.Fatalf("add verb: %s", errCode)
	}
	s := NewRuntime(store)
	defer s.Stop()

	result := s.CallVerb(1, "go", nil, 2)
	if result.Flow == types.FlowException {
		t.Fatalf("go raised %s", result.Error)
	}
	if got, want := result.Val.String(), "{11, 10, 11}"; got != want {
		t.Fatalf("committing task: got %s, want %s", got, want)
	}
	// After the commit the reload is published session-wide.
	if line := s.EvalCommandOutput(2, "return valid(#0);"); line != "{1, 11}" {
		t.Fatalf("after commit: %q, want {1, 11}", line)
	}
}

func TestProtectedBuiltinRedirectsVerbFrameLaterTask(t *testing.T) {
	s := NewRuntime(protectedRedirectStore(t))
	defer s.Stop()
	if line := s.EvalCommandOutput(2, protectValidSetup+"return 1;"); line != "{1, 1}" {
		t.Fatalf("setup: %q", line)
	}
	line := s.EvalCommandOutput(2, protectValidProbe)
	if want := "{1, {11, 10, 11}}"; line != want {
		t.Fatalf("later task: line = %q, want %q", line, want)
	}
}
