package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
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

func TestVerbCodeReturnsCanonicalSource(t *testing.T) {
	ctx, store := newReviewCtx(t)
	obj := mustCreate(t, store, []types.ObjID{types.ObjNothing}, 0)
	mustAddVerb(t, store, obj, "canonical", 0, dbstore.VerbRead)
	if errCode := store.SetVerbCode(obj, "canonical", []string{"1   ;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode failed: %v", errCode)
	}

	result := builtinVerbCode(ctx, []types.Value{types.NewObj(obj), types.NewStr("canonical")})
	if result.IsError() {
		t.Fatalf("verb_code failed: %v", result.Error)
	}
	want := types.NewList([]types.Value{types.NewStr("1;")})
	if !result.Val.Equal(want) {
		t.Fatalf("verb_code = %s, want %s", result.Val.String(), want.String())
	}
}

func TestVerbCodeReturnsCompoundStatementAsSeparateLines(t *testing.T) {
	ctx, store := newReviewCtx(t)
	obj := mustCreate(t, store, []types.ObjID{types.ObjNothing}, 0)
	mustAddVerb(t, store, obj, "loop", 0, dbstore.VerbRead)
	source := []string{"for i, i in ({})", "  break i;", "endfor"}
	if errCode := store.SetVerbCode(obj, "loop", source); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode failed: %v", errCode)
	}

	result := builtinVerbCode(ctx, []types.Value{types.NewObj(obj), types.NewStr("loop")})
	if result.IsError() {
		t.Fatalf("verb_code failed: %v", result.Error)
	}
	want := types.NewList([]types.Value{
		types.NewStr("for i, i in ({})"),
		types.NewStr("  break i;"),
		types.NewStr("endfor"),
	})
	if !result.Val.Equal(want) {
		t.Fatalf("verb_code = %s, want %s", result.Val.String(), want.String())
	}
}
