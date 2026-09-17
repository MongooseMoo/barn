package engine

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Expectations are checked against the managed Toast oracle. A test-owned
// driver supplies the caller instead of relying on the eval receiver.
func TestProtectedBuiltinDispatchSemantics(t *testing.T) {
	for _, tc := range []struct{ name, perms, wrapper, driver, want string }{
		{"same_task_and_caller", "rxd", "return {task_id() == args[1], caller == args[2]};", "return valid(task_id(), this);", "{1, {1, 1}}"},
		{"exception_value", "rxd", `raise(E_INVARG, \"wrapped\", 77);`, "try valid(#0); except e (E_INVARG) return e[3]; endtry", "{1, 77}"},
		{"nonexecutable_wrapper", "rd", "return 99;", "return valid(#0);", "{1, 1}"},
		{"owned_arguments", "rxd", "x = {3, 4, 5}; return args;", "return valid(7, 9);", "{1, {7, 9}}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewRuntime(protectedRedirectStore(t))
			defer s.Stop()
			code := `add_property(#1, "protect_valid", 1, {player, "r"});` +
				`add_verb(#0, {player, "` + tc.perms + `", "bf_valid"}, {"this", "none", "this"});` +
				`set_verb_code(#0, "bf_valid", {"` + tc.wrapper + `"});` +
				`o = create(#-1); add_verb(o, {player, "rxd", "driver"}, {"this", "none", "this"});` +
				`set_verb_code(o, "driver", {"` + tc.driver + `"}); load_server_options(); return o:driver();`
			if got := s.EvalCommandOutput(2, code); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestProtectedBuiltinDispatchSuspendsAndResumes(t *testing.T) {
	store := protectedRedirectStore(t)
	s := NewRuntime(store)
	defer s.Stop()
	setup := protectValidSetup +
		`add_property(#1, "result", 0, {player, "rw"});` +
		`set_verb_code(#0, "bf_valid", {"suspend(0); return args;"});` +
		`add_verb(#1, {player, "rxd", "driver"}, {"this", "none", "this"});` +
		`set_verb_code(#1, "driver", {"this.result = valid(7, 9); return 1;"}); return 1;`
	if got := s.EvalCommandOutput(2, setup); got != "{1, 1}" {
		t.Fatal(got)
	}
	result, err := s.RunServerVerbTaskWithArgstr(1, "driver", nil, 2, "", nil)
	if err != nil || result.Flow != types.FlowSuspend {
		t.Fatalf("result=%+v, err=%v; want suspend", result, err)
	}
	for i := 0; i < 10 && s.ProcessReadyTasks() > 0; i++ {
	}
	got, code := store.DirectTxn().PropertyValue(1, "result")
	if code != types.E_NONE || got.String() != "{7, 9}" {
		t.Fatalf("resumed result=%s, error=%s", got.String(), code)
	}
}

func TestProtectedBuiltinDispatchIdentityInCommittingTask(t *testing.T) {
	store := protectedRedirectStore(t)
	s := NewRuntime(store)
	defer s.Stop()
	setup := protectValidSetup +
		`set_verb_code(#0, "bf_valid", {"return {task_id() == args[1], caller == args[2]};"}); return 1;`
	if got := s.EvalCommandOutput(2, setup); got != "{1, 1}" {
		t.Fatal(got)
	}
	verb := dbstore.NewVerb("driver", []string{"driver"}, 2,
		dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"return valid(task_id(), this);"})
	if _, code := store.AddVerb(1, verb); code != types.E_NONE {
		t.Fatal(code)
	}
	result := s.CallVerb(1, "driver", nil, 2)
	if result.Flow != types.FlowReturn || result.Val.String() != "{1, 1}" {
		t.Fatalf("result=%+v, want {1, 1}", result)
	}
}
