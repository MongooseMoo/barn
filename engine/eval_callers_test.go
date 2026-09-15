package engine

import (
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// A verb called straight from intrinsic-eval code sees the eval activation in
// callers() exactly as it does when eval() ran the code: the eval'd user-code
// frame followed by the two synthetic eval wrapper frames.
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
	if viaEval != want {
		t.Fatalf("via eval(): %s\nwant:       %s", viaEval, want)
	}
	if top := s.EvalCommandOutput(3, "return callers();"); top != `{1, {{#-1, "eval", #-1, #-1, #3}, {#-1, "eval", #3, #-1, #3}}}` {
		t.Fatalf("top level: %s", top)
	}
}
