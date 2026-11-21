package engine

import (
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// A verb called straight from intrinsic-eval code sees the eval'd user-code
// frame followed by the two synthetic eval wrapper frames. Called from code
// eval() ran inside that eval, it sees eval()'s activation and its builtin
// entry above those (Toast's make_stack_list lists every activation; the
// conformance test callers_below_direct_eval_activation checks it on Toast).
func TestEvalNestedVerbSeesEvalFrameInCallers(t *testing.T) {
	store := dbstore.NewStore()
	wizard := dbstore.NewObjectBuilder(3)
	wizard.SetOwner(3)
	wizard.SetLocation(types.ObjNothing)
	wizard.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wizard.Build()); err != nil {
		t.Fatalf("add wizard: %v", err)
	}
	s := NewRuntime(store)
	defer s.Stop()

	const probe = `o = create(#-1);` +
		`add_verb(o, {player, "rxd", "c"}, {"this", "none", "this"});` +
		`set_verb_code(o, "c", {"return callers();"});` +
		`r = o:c(); recycle(o); return r;`
	direct := s.EvalCommandOutput(3, probe)
	want := `{1, {{#-1, "", #3, #-1, #3}, {#-1, "eval", #-1, #-1, #3}, {#-1, "eval", #3, #-1, #3}}}`
	if direct != want {
		t.Fatalf("direct call: %s\nwant:        %s", direct, want)
	}
	viaEval := s.EvalCommandOutput(3, `return eval("`+strings.ReplaceAll(probe, `"`, `\"`)+`")[2];`)
	wantViaEval := `{1, {{#-1, "", #3, #-1, #3}, {#-1, "eval", #-1, #-1, #3}, {#-1, "", #3, #-1, #3}, {#-1, "eval", #-1, #-1, #3}, {#-1, "eval", #3, #-1, #3}}}`
	if viaEval != wantViaEval {
		t.Fatalf("via eval(): %s\nwant:       %s", viaEval, wantViaEval)
	}
	if top := s.EvalCommandOutput(3, "return callers();"); top != `{1, {{#-1, "eval", #-1, #-1, #3}, {#-1, "eval", #3, #-1, #3}}}` {
		t.Fatalf("top level: %s", top)
	}
}
