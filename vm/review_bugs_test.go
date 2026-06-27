package vm

// TestReview_* tests written by analyst to confirm suspected bugs.
// Tests are written to FAIL on buggy behaviour and turn GREEN only after a fix.

import (
	"testing"

	"barn/types"
)

// ---------------------------------------------------------------------------
// B1 (F27): `in` operator on a map searches VALUES — and that is CORRECT.
//
// The review (reports/review-vm.md F27) claimed `x in map` should search the
// map's KEYS. That is FALSE. ToastStunt source is the authority:
//
//   execute.cc:1403  OP_IN  → ans.v.num = ismember(lhs, rhs, 0)
//   collection.cc:31-43 do_map_iteration / 46-69 ismember:
//       the map branch calls mapforeach (map.cc:809, key-sorted rbtree order)
//       and compares the iterated *value* against lhs:
//           equality(value, ismember_data->value, case_matters)
//       returning the running 1-based index `i` on the first VALUE match, else 0.
//
// So `x in map` returns the 1-based position (in key-sorted order) of the first
// pair whose VALUE equals x, or 0. case_matters=0 → case-INSENSITIVE.
//
// Conformance corroboration (moo-conformance-tests builtins/map.yaml, which
// drives the identical ismember helper via is_member):
//   is_member("FOO", ["FOO" -> "BAR"]) == 0  (map.yaml:126-129) — "FOO" is a
//       KEY, not a value, so it is NOT found. Proves key-search is wrong.
//   is_member("5", ["3"->"3","1"->"1","4"->"4","5"->"5","9"->"9","2"->"2"]) == 5
//       (map.yaml:116-119) — value "5" sits at key-sorted position 5.
//
// Barn's executeIn (op_compare.go) and the dead inOp (operators.go) already
// implement this value-search. The tests below assert Toast's TRUE returns.
// ---------------------------------------------------------------------------

func TestReview_MapInSearchesValues(t *testing.T) {
	// "a" is a KEY in ["a" -> 1], not a value. Toast searches values, so it is
	// not found → 0. (collection.cc:36 compares the value slot, not the key.)
	result := runBytecodeExpr(t, `"a" in ["a" -> 1]`)
	if result.Flow == types.FlowException {
		t.Fatalf("unexpected exception %s: %v", result.Error, result.Val)
	}
	if result.Val.Type() != types.TYPE_INT {
		t.Fatalf("result is not an int: %v", result.Val)
	}
	got := result.Val.Int()
	if got != 0 {
		t.Errorf(`"a" in ["a" -> 1] = %d, want 0 ("a" is a key, not a value; `+
			`Toast ismember searches values — collection.cc:36)`, got)
	}
}

func TestReview_MapInValueFoundReturnsKeySortedPosition(t *testing.T) {
	// 1 IS the value of key "a" in ["a" -> 1]. Toast searches values and
	// returns the 1-based key-sorted position of the matching pair → 1.
	result := runBytecodeExpr(t, `1 in ["a" -> 1]`)
	if result.Flow == types.FlowException {
		t.Fatalf("unexpected exception %s: %v", result.Error, result.Val)
	}
	if result.Val.Type() != types.TYPE_INT {
		t.Fatalf("result is not an int: %v", result.Val)
	}
	got := result.Val.Int()
	if got != 1 {
		t.Errorf(`1 in ["a" -> 1] = %d, want 1 (value 1 at key-sorted position 1; `+
			`Toast ismember — collection.cc:36,65)`, got)
	}
}

// A value at a later key-sorted position returns that position, mirroring
// is_member("5", ...) == 5 in conformance map.yaml:116-119.
func TestReview_MapInValueAtSortedPosition(t *testing.T) {
	requireInt(t, runBytecodeExpr(t, `30 in ["b" -> 20, "a" -> 10, "c" -> 30]`), 3)
}

// Sanity: a token that is neither key nor value returns 0.
func TestReview_MapInNotPresent(t *testing.T) {
	requireInt(t, runBytecodeExpr(t, `"z" in ["a" -> 1]`), 0)
}

// ---------------------------------------------------------------------------
// B2 (F4): Waifs are REFERENCE types — aliases share one underlying waif.
//
// The original review framed WaifValue's shared mutable state as a broken
// copy-on-write. It is not a bug: ToastStunt waifs are reference types. A
// TYPE_WAIF Var holds a `Waif *` pointer (structures.h:174); aliasing only
// addref's that pointer (utils.cc:282-284); var_dup refuses to copy a waif
// (utils.cc:340-341 -> waif.cc:612 "can't dup waif yet"); and waif_put_prop
// mutates the one shared waif in place (waif.cc:742). So `b = a; b.foo = 99`
// MUST make `a.foo == 99`. These tests now assert that reference behavior.
//
// This is also why the VM path (op_property.go:249) can discard the returned
// WaifValue: the underlying waif is shared, so the in-place mutation is visible
// to all holders — exactly Toast's semantics.
// ---------------------------------------------------------------------------

func TestReview_WaifPropertyMutationAliasesAcrossStructCopies(t *testing.T) {
	// Two MOO locals pointing to "the same waif":  a = some_waif; b = a
	// Both alias one underlying reference-typed waif.
	waif := types.NewWaif(1, 0)
	localA := waif
	localB := waif

	// Mutate through localA (as the VM does in setWaifProp).
	localA.SetProperty("foo", types.NewInt(99))

	// Reference semantics: localB MUST see the mutation through the shared waif.
	val, ok := localB.GetProperty("foo")
	if !ok {
		t.Fatalf("localB.foo not set after mutating localA.foo; waifs are reference " +
			"types and aliases must share one underlying waif (waif.cc:742)")
	}
	if val.Type() != types.TYPE_INT || val.Int() != 99 {
		t.Errorf("localB.foo = %v, want 99 via aliased mutation", val)
	}
}

func TestReview_WaifSetPropertyMutatesOriginalNotCopy(t *testing.T) {
	// Waifs are reference types: SetProperty mutates the shared underlying waif
	// in place (Toast waif_put_prop, waif.cc:742), visible through the original
	// handle even when the returned value is discarded.
	waif := types.NewWaif(1, 0)
	_ = waif.SetProperty("hp", types.NewInt(42))

	val, ok := waif.GetProperty("hp")
	if !ok {
		t.Fatal("WaifValue.SetProperty did not mutate the shared waif; reference " +
			"semantics require the original handle to observe the write (types/waif.go)")
	}
	if val.Type() != types.TYPE_INT || val.Int() != 42 {
		t.Errorf("waif.hp = %v, want 42", val)
	}
}

// ---------------------------------------------------------------------------
// B3: containsWaif compares class+owner, not instance identity.
//
// Two distinct WaifValue instances that share class and owner are treated as
// equal, causing false-positive E_RECMOVE errors on legitimate property
// assignments (collection_helpers.go:62-63).
// ---------------------------------------------------------------------------

func TestReview_ContainsWaifFalsePositive_SameClassOwnerDistinctInstances(t *testing.T) {
	waifA := types.NewWaif(1, 0)
	waifB := types.NewWaif(1, 0) // distinct instance, same class+owner

	// waifA and waifB are different objects; assigning waifB as a property
	// of waifA is not circular.  containsWaif falsely returns true.
	if containsWaif(waifB, waifA) {
		t.Error("containsWaif(waifB, waifA) = true for distinct instances with same class+owner; " +
			"should compare by instance identity, not class+owner " +
			"(collection_helpers.go:62)")
	}

	// True-positive half: genuine containment by the SAME instance MUST be
	// detected (matches Toast refers_to, waif.cc:250 pointer identity).
	// 1) The target waif IS itself.
	if !containsWaif(waifA, waifA) {
		t.Error("containsWaif(waifA, waifA) = false; a waif must contain itself (instance identity)")
	}
	// 2) The target nested inside a list.
	if !containsWaif(types.NewList([]types.Value{waifA}), waifA) {
		t.Error("containsWaif({waifA}, waifA) = false; should detect target inside a list")
	}
	// 3) The target held in another waif's property (Toast recurses into
	//    waif propvals, waif.cc:252-256). waifB holds waifA on .child.
	container := types.NewWaif(2, 0)
	container.SetProperty("child", waifA)
	if !containsWaif(container, waifA) {
		t.Error("containsWaif(container, waifA) = false; should detect target stored in another waif's property")
	}
	// Distinct same-class instance in that property must still NOT match waifB.
	if containsWaif(container, waifB) {
		t.Error("containsWaif(container, waifB) = true; distinct instance must not match via class+owner")
	}
}

// ---------------------------------------------------------------------------
// B4: FOR_LIST_LOAD / FOR_LIST_LOAD_KV happy-path smoke tests.
//     These pass today.  They are here to catch future regressions in the
//     fused loop opcodes that contain unchecked type assertions.
// ---------------------------------------------------------------------------

func TestReview_ForListLoadHappyPath(t *testing.T) {
	result := runBytecodeProgram(t,
		`acc = 0; for x in ({10, 20, 30}); acc = acc + x; endfor; return acc;`, nil, nil)
	requireInt(t, result, 60)
}

// For `for k, v in (list)` the first variable (k) receives the element
// and the second (v) receives the 1-based index (matching ToastStunt list-kv
// semantics where "key" is position).  Confirm the index is 1-based by summing.
func TestReview_ForListKVSecondVarIsOneBased(t *testing.T) {
	// k = element (string), v = 1-based index (int): sum of v over {"a","b","c"} = 1+2+3 = 6.
	result := runBytecodeProgram(t,
		`acc = 0; for k, v in ({"a","b","c"}); acc = acc + v; endfor; return acc;`,
		nil, nil)
	requireInt(t, result, 6)
}
