package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// A callee's `args` variable is filled only when its program has one; the
// arguments themselves stay on the frame for pass(), suspension and everything
// else that reads them.

const argsSlotSetup = `p = create(#-1); k = create(p);` +
	`add_property(#1, "op", p, {player, "rw"}); add_property(#1, "ok", k, {player, "rw"});` +
	`add_property(#1, "result", 0, {player, "rw"});` +
	`add_verb(p, {player, "rxd", "show"}, {"this", "none", "this"}); set_verb_code(p, "show", {"return args;"});` +
	`add_verb(k, {player, "rxd", "show"}, {"this", "none", "this"}); set_verb_code(k, "show", {"return pass();"});` +
	`add_verb(p, {player, "rxd", "late"}, {"this", "none", "this"}); set_verb_code(p, "late", {"return args;"});` +
	`add_verb(k, {player, "rxd", "late"}, {"this", "none", "this"}); set_verb_code(k, "late", {"suspend(0); return pass();"});` +
	`add_verb(k, {player, "rxd", "drive"}, {"this", "none", "this"}); set_verb_code(k, "drive", {"#1.result = this:late(5, \"x\", {6});"});` +
	`add_verb(k, {player, "rxd", "none"}, {"this", "none", "this"}); set_verb_code(k, "none", {"return 0;"});` +
	`add_verb(k, {player, "rxd", "some"}, {"this", "none", "this"}); set_verb_code(k, "some", {"return args;"});` +
	`return 1;`

func argsSlotRuntime(t *testing.T) *Runtime {
	t.Helper()
	s := NewRuntime(protectedRedirectStore(t))
	t.Cleanup(s.Stop)
	if got := s.EvalCommandOutput(2, argsSlotSetup); got != "{1, 1}" {
		t.Fatalf("setup: %s", got)
	}
	return s
}

func TestVerbArgsReachPassWithoutArgsVariable(t *testing.T) {
	s := argsSlotRuntime(t)
	for code, want := range map[string]string{
		`return #1.ok:show(1, "two", {3});`:                  `{1, {1, "two", {3}}}`,
		`return #1.ok:show(@{1, 2});`:                        `{1, {1, 2}}`,
		`return #1.ok:show();`:                               `{1, {}}`,
		`return #1.op:show(7);`:                              `{1, {7}}`,
		`a = create(#-1, 1); return #1.ok:show(a) == {a};`:   `{1, 1}`,
		`return #1.ok:some(1, 2);`:                           `{1, {1, 2}}`,
		`return #1.ok:none(1, 2);`:                           `{1, 0}`,
		`return {#1.ok:none(@{3, 4}), #1.ok:some(@{3, 4})};`: `{1, {0, {3, 4}}}`,
	} {
		if got := s.EvalCommandOutput(2, code); got != want {
			t.Fatalf("%s: got %s, want %s", code, got, want)
		}
	}
}

func TestVerbArgsSurviveSuspendWithoutArgsVariable(t *testing.T) {
	store := protectedRedirectStore(t)
	s := NewRuntime(store)
	defer s.Stop()
	if got := s.EvalCommandOutput(2, argsSlotSetup); got != "{1, 1}" {
		t.Fatalf("setup: %s", got)
	}
	k, _ := store.DirectTxn().PropertyValue(1, "ok")
	result, err := s.RunServerVerbTaskWithArgstr(k.ID(), "drive", nil, 2, "", nil)
	if err != nil || result.Flow != types.FlowSuspend {
		t.Fatalf("result=%+v, err=%v; want suspend", result, err)
	}
	for i := 0; i < 10 && s.ProcessReadyTasks() > 0; i++ {
	}
	got, code := store.DirectTxn().PropertyValue(1, "result")
	if code != types.E_NONE || got.String() != `{5, "x", {6}}` {
		t.Fatalf("resumed result=%s, error=%s", got.String(), code)
	}
}

// A call whose callee never names `args` builds no argument list for it.
func TestVerbCallWithoutArgsVariableBuildsNoArgsList(t *testing.T) {
	s := argsSlotRuntime(t)
	loop := func(verb string) float64 {
		code := `k = #1.ok; for i in [1..200] k:` + verb + `(i, i); endfor return 1;`
		s.EvalCommandOutput(2, code)
		return testing.AllocsPerRun(5, func() { s.EvalCommandOutput(2, code) })
	}
	none, some := loop("none"), loop("some")
	if none+150 > some {
		t.Fatalf("200 calls without an args variable allocated %.0f, with one %.0f; want at least 150 fewer", none, some)
	}
}
