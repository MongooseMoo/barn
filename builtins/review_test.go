package builtins

// TestReview_* tests for the builtins analyst review.
// These tests are expected to FAIL (red) demonstrating confirmed bugs.

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

// newReviewCtx builds a minimal TaskContext backed by a real Store and Registry.
func newReviewCtx(t *testing.T) (*kernel.TaskContext, *dbstore.Store) {
	t.Helper()
	store := dbstore.NewStore()
	registry := NewRegistry()
	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.Registry = registry
	ctx.IsWizard = true
	ctx.Programmer = 0
	ctx.Player = 0
	return ctx, store
}

// mustCreate creates a live object and fails the test on error.
func mustCreate(t *testing.T, store *dbstore.Store, parents []types.ObjID, owner types.ObjID) types.ObjID {
	t.Helper()
	id, ec := store.CreateObject(parents, owner, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject: %s", ec)
	}
	return id
}

// mustAddVerb adds a verb to an object and fails the test on error.
func mustAddVerb(t *testing.T, store *dbstore.Store, objID types.ObjID, verbName string, owner types.ObjID, perms dbstore.VerbPerms) {
	t.Helper()
	verb := dbstore.NewVerb(verbName, []string{verbName}, owner, perms, dbstore.VerbArgs{
		This: "none", Prep: "none", That: "none",
	}, []string{`return 1;`})
	if _, ec := store.AddVerb(objID, verb); ec != types.E_NONE {
		t.Fatalf("AddVerb %q on #%d: %s", verbName, objID, ec)
	}
}

// ============================================================================
// BUG: delete_verb on inherited verb silently succeeds (should be E_VERBNF).
//
// The store's DeleteVerb calls findVerbLocked (BFS ancestry) to locate the
// verb pointer, then scans obj.verbs of the CHILD object looking for that
// pointer. Because the pointer lives in the parent's verb table, it is never
// found in the child's map; nothing is deleted; E_NONE is returned. The
// builtin layer propagates that success back to the caller.
//
// Toast: delete_verb(child, inherited_verb) → E_VERBNF.
// ============================================================================
func TestReview_DeleteVerbOnInheritedVerbReturnsEVERBNF(t *testing.T) {
	ctx, store := newReviewCtx(t)

	// #0: parent with verb "look"
	parent := mustCreate(t, store, []types.ObjID{types.ObjNothing}, types.ObjNothing)
	mustAddVerb(t, store, parent, "look", parent, dbstore.VerbRead|dbstore.VerbExecute)

	// #1: child that inherits "look" from parent
	child := mustCreate(t, store, []types.ObjID{parent}, types.ObjNothing)

	// delete_verb(child, "look") should return E_VERBNF — verb is inherited.
	result := builtinDeleteVerb(ctx, []types.Value{
		types.NewObj(child),
		types.NewStr("look"),
	})

	// BUG: currently returns E_NONE (silent success, verb not actually deleted).
	if result.IsNormal() {
		t.Fatalf("delete_verb on inherited verb returned success (E_NONE); want E_VERBNF")
	}
	if !result.IsError() || result.Error != types.E_VERBNF {
		t.Fatalf("delete_verb on inherited verb returned %v; want E_VERBNF", result)
	}
}

// ============================================================================
// BUG: set_verb_info mutates the ancestor verb in-place when called on a child.
//
// store.SetVerbInfo calls findVerbLocked (BFS) obtaining the ancestor's *Verb
// pointer, then writes directly to verb.owner / verb.perms / verb.names.
// This corrupts the ancestor's verb for every inheritor.
//
// Toast: set_verb_info(child, inherited_verb, info) modifies the defining
// ancestor's verb (which changes it for all inheritors), but the permission
// check should be against the definer's owner, not the child's owner.
// The observable bug here is that the PARENT's verb is modified when the
// builtin is directed at the CHILD.
// ============================================================================
func TestReview_SetVerbInfoMutatesAncestorVerb(t *testing.T) {
	ctx, store := newReviewCtx(t)

	// parent owns verb "look" with no write perm on purpose
	parent := mustCreate(t, store, []types.ObjID{types.ObjNothing}, types.ObjNothing)
	// owner = 0 (wizard); perms = rx
	mustAddVerb(t, store, parent, "look", 0, dbstore.VerbRead|dbstore.VerbExecute)

	child := mustCreate(t, store, []types.ObjID{parent}, types.ObjNothing)

	// Inspect parent's verb before the call
	verbBefore, err := store.FindVerbOnObject(parent, "look")
	if err != nil {
		t.Fatalf("parent verb 'look' not found: %v", err)
	}
	ownerBefore := verbBefore.Owner

	// set_verb_info(child, "look", {#99, "rx", "look"})
	// Directed at child — but child doesn't define "look".
	// Toast would find the verb on parent and modify it there.
	// The test verifies the parent verb IS modified (showing ancestor mutation).
	newOwner := types.ObjID(99)
	_ = builtinSetVerbInfo(ctx, []types.Value{
		types.NewObj(child),
		types.NewStr("look"),
		types.NewList([]types.Value{
			types.NewObj(newOwner),
			types.NewStr("rx"),
			types.NewStr("look"),
		}),
	})

	verbAfter, err := store.FindVerbOnObject(parent, "look")
	if err != nil {
		t.Fatalf("parent verb 'look' vanished after set_verb_info: %v", err)
	}

	// The parent's verb owner should NOT have changed — the call was on child.
	// BUG: it IS changed because the store mutated the ancestor verb in-place.
	if verbAfter.Owner != ownerBefore {
		t.Fatalf("set_verb_info(child, inherited, ...) mutated parent verb owner: was #%d, now #%d (BUG: ancestor corrupted)",
			ownerBefore, verbAfter.Owner)
	}
}

// ============================================================================
// BUG: verb_code() denies the verb owner if the 'r' bit is not set.
//
// Barn's check: `if !verb.Perms.Has(VerbRead) && !ctx.IsWizard → E_PERM`
// Missing: owner should bypass the 'r' bit check (as in Toast).
// Toast: owner can always read their own verb code regardless of 'r' perm.
// ============================================================================
func TestReview_VerbCodeAllowsOwnerWithoutReadBit(t *testing.T) {
	ctx, store := newReviewCtx(t)
	ctx.IsWizard = false

	// Object #0 with programmer #0
	obj := mustCreate(t, store, []types.ObjID{types.ObjNothing}, 0)

	// Add verb owned by programmer #0 but with NO read bit ("wx" not "rwx")
	mustAddVerb(t, store, obj, "secret", 0, dbstore.VerbWrite|dbstore.VerbExecute)

	ctx.Programmer = 0 // caller owns the verb

	result := builtinVerbCode(ctx, []types.Value{
		types.NewObj(obj),
		types.NewStr("secret"),
	})

	// BUG: currently returns E_PERM even though programmer owns the verb.
	if result.IsError() && result.Error == types.E_PERM {
		t.Fatalf("verb_code denied owner without 'r' bit — want success, got E_PERM (BUG)")
	}
	if !result.IsNormal() {
		t.Fatalf("verb_code returned unexpected error %v; want a list of code lines", result)
	}
}

// ============================================================================
// BUG: add_verb() permission check uses ctx.Player instead of ctx.Programmer.
//
// When a verb's task_perms are lowered via set_task_perms, Programmer != Player.
// The ownership check uses ctx.Player (the connected player object) instead of
// ctx.Programmer (the effective permission identity). A programmer who owns the
// target object via Programmer but not via Player gets E_PERM wrongly.
// ============================================================================
func TestReview_AddVerbUsesProgNotPlayerForPerm(t *testing.T) {
	ctx, store := newReviewCtx(t)
	ctx.IsWizard = false

	// Object owned by programmer #0
	obj := mustCreate(t, store, []types.ObjID{types.ObjNothing}, 0)

	// Simulate lowered task perms: programmer is #0 (owns the object),
	// but player (connected player) is #5 (does NOT own the object).
	ctx.Programmer = 0
	ctx.Player = 5

	result := builtinAddVerb(ctx, []types.Value{
		types.NewObj(obj),
		types.NewList([]types.Value{ // {owner, perms, names}
			types.NewObj(0),          // owner = programmer
			types.NewStr("rx"),
			types.NewStr("myfunc"),
		}),
		types.NewList([]types.Value{ // {dobj, prep, iobj}
			types.NewStr("none"),
			types.NewStr("none"),
			types.NewStr("none"),
		}),
	})

	// BUG: returns E_PERM because objectOwner (#0) != ctx.Player (#5).
	// Should succeed because objectOwner (#0) == ctx.Programmer (#0).
	if result.IsError() && result.Error == types.E_PERM {
		t.Fatalf("add_verb denied programmer (#0) who owns object; ctx.Player=#5 wrongly used (BUG): got E_PERM")
	}
	if !result.IsNormal() {
		t.Fatalf("add_verb returned unexpected error %v", result)
	}
}
