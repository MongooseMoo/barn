package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// serverOptionsVerbStore is protectedRedirectStore plus #1:go holding code, so
// the code runs as a committing task whose $server_options writes are staged,
// the shape the conformance harness's ";" (Test.db #2:eval) produces.
func serverOptionsVerbStore(t *testing.T, code string) *dbstore.Store {
	t.Helper()
	store := protectedRedirectStore(t)
	verb := dbstore.NewVerb(
		"go",
		[]string{"go"},
		2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug, // d: errors raise instead of becoming values
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{code},
	)
	if _, errCode := store.AddVerb(1, verb); errCode != types.E_NONE {
		t.Fatalf("add verb: %s", errCode)
	}
	return store
}

// server_options_same_task.yaml::max_string_concat_reload_applies_in_same_task:
// lowering max_string_concat and reloading makes the next concat in the same
// task raise E_QUOTA; the commit then publishes the limit session-wide.
func TestLoadServerOptionsLowersStringLimitInSameTask(t *testing.T) {
	const code = `add_property(#1, "max_concat_catchable", 1, {player, "r"});` +
		`add_property(#1, "max_string_concat", 1021, {player, "r"});` +
		`load_server_options();` +
		`try s = ""; for i in [1..60]; s = s + "xxxxxxxxxxxxxxxxxxxx"; endfor; return length(s);` +
		`except error (E_QUOTA) return error[1]; endtry`
	s := NewRuntime(serverOptionsVerbStore(t, code))
	defer s.Stop()

	result := s.CallVerb(1, "go", nil, 2)
	if result.Flow == types.FlowException {
		t.Fatalf("go raised %s", result.Error)
	}
	if got := result.Val.String(); got != "E_QUOTA" {
		t.Fatalf("same task: got %s, want E_QUOTA", got)
	}
	if limit := s.Session().GetMaxStringConcat(); limit != 1021 {
		t.Fatalf("after commit: session max_string_concat = %d, want 1021", limit)
	}
}

// server_options_same_task.yaml::yin_reads_raised_fg_ticks_in_same_task:
// yin() validates against the fg_ticks this task reloaded, not the stale
// session-wide value, so a minimum the default 60000 rejects is accepted.
func TestLoadServerOptionsRaisesTickLimitForYinInSameTask(t *testing.T) {
	const code = `add_property(#1, "fg_ticks", 100000, {player, "r"});` +
		`load_server_options();` +
		`return yin(0, 60000);`
	s := NewRuntime(serverOptionsVerbStore(t, code))
	defer s.Stop()

	result := s.CallVerb(1, "go", nil, 2)
	if result.Flow == types.FlowException {
		t.Fatalf("yin raised %s after the reload; want it to accept min_ticks 60000 < 100000", result.Error)
	}
}

// MOO has no rollback: a task that reloads and then dies of an uncaught error
// still commits its $server_options writes, so the reload is published when
// the pending effects flush, exactly as Toast's already-applied reload stays.
func TestLoadServerOptionsPublishesWhenTaskRaises(t *testing.T) {
	const code = `add_property(#1, "max_string_concat", 1021, {player, "r"});` +
		`load_server_options();` +
		`raise(E_INVARG);`
	s := NewRuntime(serverOptionsVerbStore(t, code))
	defer s.Stop()

	result := s.CallVerb(1, "go", nil, 2)
	if result.Flow != types.FlowException || result.Error != types.E_INVARG {
		t.Fatalf("go = %+v, want E_INVARG", result)
	}
	if limit := s.Session().GetMaxStringConcat(); limit != 1021 {
		t.Fatalf("after the task: session max_string_concat = %d, want 1021", limit)
	}
}
