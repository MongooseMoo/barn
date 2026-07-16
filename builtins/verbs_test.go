package builtins

import (
	"testing"

	dbstore "barn/db/store"
	"barn/types"
)

func TestVerbInfoNumericIndexPastEndReturnsEVERBNF(t *testing.T) {
	ctx, store := newReviewCtx(t)
	obj := mustCreate(t, store, []types.ObjID{types.ObjNothing}, 0)
	mustAddVerb(t, store, obj, "only_verb", 0, dbstore.VerbRead)

	result := builtinVerbInfo(ctx, []types.Value{
		types.NewObj(obj),
		types.NewInt(2),
	})
	if !result.IsError() || result.Error != types.E_VERBNF {
		t.Fatalf("verb_info past final numeric index returned %v; want E_VERBNF", result)
	}
}
