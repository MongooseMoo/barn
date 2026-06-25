package types

// TestReview_* tests written by the analyst role.
// These are RED tests — they expose confirmed bugs.

import "testing"

// TestReview_ObjEqualIgnoresAnonFlag confirms that ObjValue.Equal() does not
// distinguish regular objects from anonymous objects with the same numeric ID.
// A regular #5 and an anonymous *#5 are different values (different Type()),
// but Equal returns true.
func TestReview_ObjEqualIgnoresAnonFlag(t *testing.T) {
	regular := NewObj(5)
	anon := NewAnon(5)

	// Sanity: they have different types.
	if regular.Type() == anon.Type() {
		t.Fatal("precondition failed: Type() should differ between regular and anonymous object")
	}

	// The BUG: Equal ignores the anonymous flag.
	// NewObj(5).Equal(NewAnon(5)) returns true; it should return false.
	if regular.Equal(anon) {
		t.Fatal("BUG: NewObj(5).Equal(NewAnon(5)) returned true — Equal ignores the anonymous flag; regular and anonymous objects with the same ID must not be equal")
	}
}

// TestReview_WaifSetPropertyMutatesOriginal confirms that WaifValue.SetProperty
// mutates the shared underlying properties map, corrupting the original value.
// WaifValue is a value-receiver struct containing a reference-type map, so
// copies share the same backing map and SetProperty's in-place write is visible
// through any alias.
func TestReview_WaifSetPropertyMutatesOriginal(t *testing.T) {
	w1 := NewWaif(1, 2)
	w2 := w1 // struct copy — should be independent

	w2 = w2.SetProperty("x", NewInt(42))

	// w1 must not see the property set on w2.
	if _, ok := w1.GetProperty("x"); ok {
		t.Fatal("BUG: SetProperty on a WaifValue copy mutated the original's property map — the map is shared between struct copies, not COW")
	}
}

// TestReview_WaifEqualUsesDeepequalNotIdentity confirms a secondary waif
// equality problem: WaifValue.Equal uses structural map comparison, so two
// distinct waif instances with the same class and identical properties are
// considered equal, even though waif identity in MOO is reference-based.
func TestReview_WaifEqualUsesDeepequalNotIdentity(t *testing.T) {
	w1 := NewWaif(1, 2)
	w2 := NewWaif(1, 2)
	// Two independently created waifs with same class/owner should NOT be equal
	// (MOO waifs use reference identity), but Equal uses structural comparison.
	if w1.Equal(w2) {
		t.Fatal("BUG: two independently created WaifValues with same class/owner compare Equal — waif equality must use reference identity, not structural comparison")
	}
}
