package vm

// TestReview_* tests written by analyst to confirm suspected bugs.
// Tests are written to FAIL on buggy behaviour and turn GREEN only after a fix.

import (
	"testing"

	"barn/types"
)

// ---------------------------------------------------------------------------
// B1: `in` operator on a map checks VALUES instead of KEYS
//
// ToastStunt semantics: `x in map` returns the 1-based position of x among
// the map's *keys* (sorted in MOO canonical order), or 0 if x is not a key.
// Both executeIn (op_compare.go) and inOp (operators.go) search pair[1]
// (the value slot) not pair[0] (the key slot).
//
// Evidence: "a" in ["a" -> 1]
//   Keys:   {"a"} → "a" is at position 1 → correct answer: 1
//   Values: {1}   → "a" is not a value  → actual answer:  0  (bug)
// ---------------------------------------------------------------------------

func TestReview_MapInChecksValuesNotKeys(t *testing.T) {
	result := runBytecodeExpr(t, `"a" in ["a" -> 1]`)
	if result.Flow == types.FlowException {
		t.Fatalf("unexpected exception %s: %v", result.Error, result.Val)
	}
	got, ok := result.Val.(types.IntValue)
	if !ok {
		t.Fatalf("result is not an int: %T %v", result.Val, result.Val)
	}
	// Correct: 1 (key found at sorted-position 1).  Current: 0 (value scan).
	if got.Val != 1 {
		t.Errorf(`"a" in ["a" -> 1] = %d, want 1 (key lookup); `+
			`op_compare.go executeIn and operators.go inOp both scan pair[1] (values)`, got.Val)
	}
}

func TestReview_MapInValueFoundAsKey_ReturnsZero(t *testing.T) {
	// 1 is a VALUE, not a KEY, in ["a" -> 1].
	// Correct (key-based): 0 — 1 is not a key.
	// Buggy  (value-based): 1 — value 1 found at sorted-position 1.
	result := runBytecodeExpr(t, `1 in ["a" -> 1]`)
	if result.Flow == types.FlowException {
		t.Fatalf("unexpected exception %s: %v", result.Error, result.Val)
	}
	got, ok := result.Val.(types.IntValue)
	if !ok {
		t.Fatalf("result is not an int: %T %v", result.Val, result.Val)
	}
	if got.Val != 0 {
		t.Errorf(`1 in ["a" -> 1] = %d, want 0; current impl finds the value instead of a key`, got.Val)
	}
}

// Sanity: a key that is genuinely absent returns 0 regardless of impl.
func TestReview_MapInKeyNotPresent(t *testing.T) {
	requireInt(t, runBytecodeExpr(t, `"z" in ["a" -> 1]`), 0)
}

// ---------------------------------------------------------------------------
// B2: WaifValue has aliased shared mutable state, not copy-on-write semantics.
//
// WaifValue is a Go struct (value type) whose `properties` field is a
// map[string]Value (reference type).  Copying a WaifValue copies the struct
// header but NOT the underlying map — all copies share the same map.
// SetProperty mutates that shared map in place, so every copy of the waif
// sees every mutation.  This violates the expected value semantics and
// enables ghost aliasing in the VM when the same waif is stored in multiple
// local variables.
//
// The VM path (op_property.go:249) also discards the returned WaifValue:
//   _ = waif.SetProperty(propName, value)
// but because of the shared map the mutation happens anyway.  If WaifValue is
// ever fixed to be truly copy-on-write the discard will then silently break
// all property writes on locally-held waifs.
// ---------------------------------------------------------------------------

func TestReview_WaifPropertyMutationAliasesAcrossStructCopies(t *testing.T) {
	// Simulate two MOO locals pointing to "the same waif":
	//   a = some_waif; b = a
	// Both a and b are struct copies of the same WaifValue.
	waif := types.NewWaif(1, 0)
	localA := waif
	localB := waif

	// Mutate through localA (as the VM does in setWaifProp).
	localA.SetProperty("foo", types.NewInt(99))

	// Correct (value semantics): localB should NOT see the mutation.
	// Buggy  (aliased map):      localB.foo == 99.
	val, ok := localB.GetProperty("foo")
	if ok {
		t.Errorf("localB.foo = %v after mutating localA.foo; "+
			"WaifValue.properties map is shared across struct copies — "+
			"all aliases see every mutation (types/waif.go)", val)
	}
}

func TestReview_WaifSetPropertyMutatesOriginalNotCopy(t *testing.T) {
	// SetProperty is advertised as returning a new copy; original should be pristine.
	waif := types.NewWaif(1, 0)
	_ = waif.SetProperty("hp", types.NewInt(42))

	_, ok := waif.GetProperty("hp")
	if ok {
		t.Error("WaifValue.SetProperty mutated the original struct via shared map; " +
			"copy-on-write semantics are broken (types/waif.go:68-75)")
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
