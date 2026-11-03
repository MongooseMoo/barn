package engine

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// Calls from one static call site follow every change to what they resolve to,
// whether the change is staged by the calling task itself or committed by
// another task, and whatever receiver each call names.

const siteSetup = `a = create(#-1); b = create(a); c = create(a);` +
	`add_verb(a, {player, "rxd", "v"}, {"this", "none", "this"});` +
	`set_verb_code(a, "v", {"return {1, this};"});` +
	`add_verb(#1, {player, "rxd", "drive"}, {"this", "none", "this"});` +
	`set_verb_code(#1, "drive", {"return args[1]:v();"});` +
	`add_property(#1, "oa", a, {player, "rw"}); add_property(#1, "ob", b, {player, "rw"}); add_property(#1, "oc", c, {player, "rw"});`

func TestVerbCallSiteSeesOwnChangesInOneTask(t *testing.T) {
	s := NewRuntime(protectedRedirectStore(t))
	defer s.Stop()
	if got := s.EvalCommandOutput(2, siteSetup+`return 1;`); got != "{1, 1}" {
		t.Fatalf("setup: %s", got)
	}
	code := `a = #1.oa; b = #1.ob; c = #1.oc; r = {};` +
		`for step in [1..7]` +
		`  if (step == 2) set_verb_code(a, "v", {"return {2, this};"}); endif` +
		`  if (step == 3) add_verb(b, {player, "rxd", "v"}, {"this", "none", "this"}); set_verb_code(b, "v", {"return {3, this};"}); endif` +
		`  if (step == 4) set_verb_info(b, "v", {player, "rxd", "w"}); endif` +
		`  if (step == 5) chparent(c, #1); endif` +
		`  if (step == 6) delete_verb(a, "v"); endif` +
		`  for o in ({b, c})` +
		`    try r = {@r, o:v()}; except e (ANY) r = {@r, e[1]}; endtry` +
		`  endfor` +
		` endfor return {r, a, b, c};`
	got := s.EvalCommandOutput(2, code)
	// b and c are the 2nd..4th objects created in setup; compare by position.
	want := "{1, {{{1, #4}, {1, #5}, {2, #4}, {2, #5}, {3, #4}, {2, #5}, {2, #4}, {2, #5}, {2, #4}, E_VERBNF, E_VERBNF, E_VERBNF, E_VERBNF, E_VERBNF}, #3, #4, #5}}"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestVerbCallSiteSeesOtherTasksCommits(t *testing.T) {
	store := protectedRedirectStore(t)
	s := NewRuntime(store)
	defer s.Stop()
	if got := s.EvalCommandOutput(2, siteSetup+`return 1;`); got != "{1, 1}" {
		t.Fatalf("setup: %s", got)
	}
	drive := func(which string) string {
		t.Helper()
		return s.EvalCommandOutput(2, `return #1:drive(#1.o`+which+`);`)
	}
	steps := []struct{ change, which, want string }{
		{``, "b", "{1, {1, #4}}"},
		{``, "c", "{1, {1, #5}}"},
		{`set_verb_code(#1.oa, "v", {"return {2, this};"});`, "b", "{1, {2, #4}}"},
		{`add_verb(#1.ob, {player, "rxd", "v"}, {"this", "none", "this"}); set_verb_code(#1.ob, "v", {"return {3, this};"});`, "b", "{1, {3, #4}}"},
		{``, "c", "{1, {2, #5}}"},
		{`set_verb_info(#1.ob, "v", {player, "rxd", "w"});`, "b", "{1, {2, #4}}"},
		{`set_verb_code(#1.ob, "w", {"return {0, this};"}); set_verb_info(#1.ob, "w", {player, "rxd", "v"});`, "b", "{1, {0, #4}}"},
		{`chparent(#1.oc, #1);`, "c", "{2, {E_VERBNF, \"Verb not found\", 0}}"},
		{`chparent(#1.oc, #1.oa);`, "c", "{1, {2, #5}}"},
		{`recycle(#1.oc);`, "c", "{2, {E_INVIND, \"Invalid indirection\", 0}}"},
		{`delete_verb(#1.oa, "v");`, "b", "{1, {0, #4}}"},
		{`delete_verb(#1.ob, "v");`, "b", "{2, {E_VERBNF, \"Verb not found\", 0}}"},
	}
	for i, step := range steps {
		if step.change != "" {
			if got := s.EvalCommandOutput(2, step.change+`return 1;`); got != "{1, 1}" {
				t.Fatalf("step %d change: %s", i, got)
			}
		}
		if got := drive(step.which); got != step.want {
			t.Fatalf("step %d (%s then %s): got %s, want %s", i, step.change, step.which, got, step.want)
		}
	}
}

// A task's snapshot is renewed after it suspends; the same call site then
// resolves against what other tasks committed meanwhile.
func TestVerbCallSiteFollowsRenewedSnapshot(t *testing.T) {
	store := protectedRedirectStore(t)
	s := NewRuntime(store)
	defer s.Stop()
	setup := `add_verb(#1, {player, "rxd", "v"}, {"this", "none", "this"}); set_verb_code(#1, "v", {"return 1;"});` +
		`add_property(#1, "result", 0, {player, "rw"});` +
		`add_verb(#1, {player, "rxd", "loop"}, {"this", "none", "this"});` +
		`set_verb_code(#1, "loop", {"r = {this:v(), this:v()}; suspend(0); r = {@r, this:v()}; this.result = r; return 1;"}); return 1;`
	if got := s.EvalCommandOutput(2, setup); got != "{1, 1}" {
		t.Fatalf("setup: %s", got)
	}
	result, err := s.RunServerVerbTaskWithArgstr(1, "loop", nil, 2, "", nil)
	if err != nil || result.Flow != types.FlowSuspend {
		t.Fatalf("result=%+v, err=%v; want suspend", result, err)
	}
	if got := s.EvalCommandOutput(2, `set_verb_code(#1, "v", {"return 2;"}); return 1;`); got != "{1, 1}" {
		t.Fatalf("edit: %s", got)
	}
	for i := 0; i < 10 && s.ProcessReadyTasks() > 0; i++ {
	}
	got, code := store.DirectTxn().PropertyValue(1, "result")
	if code != types.E_NONE || got.String() != "{1, 1, 2}" {
		t.Fatalf("result=%s, error=%s", got.String(), code)
	}
}
