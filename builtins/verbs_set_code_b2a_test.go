package builtins

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

// B2a: set_verb_code must reject a call to an UNKNOWN builtin at compile time,
// returning Toast's exact error list and leaving the verb code UNCHANGED.
//
// Toast (ToastStunt, captured live against toastcore.db):
//
//	; set_verb_code(player,"scratch",{"x = 1;"})           => {}
//	; set_verb_code(player,"scratch",{"x = foo(1,2,3);"})  => {"Line 1:  Unknown built-in function: foo"}
//	; verb_code(player,"scratch")                          => {"x = 1;"}   (UNCHANGED)
func b2aTestContext(t *testing.T) (*kernel.TaskContext, *dbstore.Store, types.ObjID) {
	t.Helper()

	store := dbstore.NewStore()
	objID := types.ObjID(1)
	obj := dbstore.NewObject(objID, objID)
	if err := store.Add(obj); err != nil {
		t.Fatalf("store.Add failed: %v", err)
	}

	verb := dbstore.NewVerb(
		"scratch",
		[]string{"scratch"},
		objID,
		dbstore.VerbPerms(0),
		dbstore.VerbArgs{This: "this", Prep: "none", That: "this"},
		[]string{"x = 1;"},
	)
	if _, code := store.AddVerb(objID, verb); code != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", code)
	}

	ctx := kernel.NewTaskContext()
	ctx.IsWizard = true
	ctx.Player = objID
	ctx.Programmer = objID
	ctx.Store = store
	ctx.Registry = NewRegistry()
	return ctx, store, objID
}

func b2aVerbCode(t *testing.T, store *dbstore.Store, objID types.ObjID) []string {
	t.Helper()
	view, _, err := store.FindVerb(objID, "scratch")
	if err != nil {
		t.Fatalf("FindVerb failed: %v", err)
	}
	return view.Code
}

func TestSetVerbCodeRejectsUnknownBuiltin_B2a(t *testing.T) {
	ctx, store, objID := b2aTestContext(t)

	// Sanity: a known builtin compiles fine (length() is a real builtin).
	res := builtinSetVerbCode(ctx, []types.Value{
		types.NewObj(objID),
		types.NewStr("scratch"),
		types.NewList([]types.Value{types.NewStr("x = length({1, 2});")}),
	})
	if res.IsError() {
		t.Fatalf("known builtin: unexpected error result %v", res.Error)
	}
	if list, ok := res.Val.(types.ListValue); !ok || list.Len() != 0 {
		t.Fatalf("known builtin: expected empty list (success), got %v", res.Val)
	}

	// Reset the verb to a known starting body so we can prove "unchanged".
	if code := store.SetVerbCode(objID, "scratch", []string{"x = 1;"}); code != types.E_NONE {
		t.Fatalf("reset SetVerbCode failed: %v", code)
	}

	// Unknown builtin foo() must be rejected with Toast's exact message.
	res = builtinSetVerbCode(ctx, []types.Value{
		types.NewObj(objID),
		types.NewStr("scratch"),
		types.NewList([]types.Value{types.NewStr("x = foo(1,2,3);")}),
	})
	if res.IsError() {
		t.Fatalf("unknown builtin: unexpected error result %v", res.Error)
	}
	list, ok := res.Val.(types.ListValue)
	if !ok {
		t.Fatalf("unknown builtin: expected list result, got %T", res.Val)
	}
	if list.Len() != 1 {
		t.Fatalf("unknown builtin: expected 1 error string, got %d: %v", list.Len(), res.Val)
	}
	got := list.Get(1).(types.StrValue).Value()
	const want = "Line 1:  Unknown built-in function: foo"
	if got != want {
		t.Fatalf("unknown builtin error mismatch:\n got: %q\nwant: %q", got, want)
	}

	// The verb code must be UNCHANGED after the rejected set.
	if code := b2aVerbCode(t, store, objID); len(code) != 1 || code[0] != "x = 1;" {
		t.Fatalf("verb code changed after rejected set_verb_code: %v", code)
	}
}

// The unknown-builtin line number must reflect the offending line, matching
// Toast's "Line N:" prefix.
func TestSetVerbCodeUnknownBuiltinReportsLine_B2a(t *testing.T) {
	ctx, store, objID := b2aTestContext(t)

	res := builtinSetVerbCode(ctx, []types.Value{
		types.NewObj(objID),
		types.NewStr("scratch"),
		types.NewList([]types.Value{
			types.NewStr("x = 1;"),
			types.NewStr("y = nosuchbuiltin(x);"),
		}),
	})
	list, ok := res.Val.(types.ListValue)
	if !ok || list.Len() != 1 {
		t.Fatalf("expected 1 error string, got %v", res.Val)
	}
	got := list.Get(1).(types.StrValue).Value()
	const want = "Line 2:  Unknown built-in function: nosuchbuiltin"
	if got != want {
		t.Fatalf("error mismatch:\n got: %q\nwant: %q", got, want)
	}
	if code := b2aVerbCode(t, store, objID); len(code) != 1 || code[0] != "x = 1;" {
		t.Fatalf("verb code changed after rejected set_verb_code: %v", code)
	}
}
