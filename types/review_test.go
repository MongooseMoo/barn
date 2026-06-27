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

// TestReview_WaifSetPropertyMutatesOriginal asserts true Toast REFERENCE
// semantics for waifs (F4). The original review framed this as a broken
// copy-on-write, but waifs are reference types in ToastStunt: a TYPE_WAIF Var
// holds a `Waif *` (structures.h:174), aliasing only addref's that pointer
// (utils.cc:282-284), var_dup refuses to copy a waif (utils.cc:340-341 ->
// waif.cc:612 "can't dup waif yet"), and waif_put_prop mutates the one shared
// waif in place (waif.cc:742). So `w2 = w1; w2.x = 42` MUST make `w1.x == 42`,
// while two SEPARATELY created waifs are independent.
func TestReview_WaifSetPropertyMutatesOriginal(t *testing.T) {
	w1 := NewWaif(1, 2)
	w2 := w1 // alias — same underlying waif (reference type)

	w2 = w2.SetProperty("x", NewInt(42))

	// w1 MUST see the property set through its alias w2.
	got, ok := w1.GetProperty("x")
	if !ok {
		t.Fatal("waif reference semantics broken: w1 does not see property set via its alias w2 (Toast waif_put_prop mutates the shared waif, waif.cc:742)")
	}
	if got.Type() != TYPE_INT || got.Int() != 42 {
		t.Fatalf("w1.x = %v, want 42 via aliased mutation", got)
	}

	// Two independently created waifs are independent references.
	other := NewWaif(1, 2)
	if _, ok := other.GetProperty("x"); ok {
		t.Fatal("two separately created waifs must be independent references, but a separate waif saw w2's property")
	}
}

// TestReview_WaifEqualUsesDeepequalNotIdentity confirms a secondary waif
// equality problem: WaifValue.Equal uses structural map comparison, so two
// distinct waif instances with the same class and identical properties are
// considered equal, even though waif identity in MOO is reference-based.
func TestReview_WaifEqualUsesDeepequalNotIdentity(t *testing.T) {
	// Toast waif equality is REFERENCE IDENTITY, not structural: utils.cc:478
	// (`equality`: `return lhs.v.waif == rhs.v.waif;`) and utils.cc:431 (the
	// `compare` path: `return lhs.v.waif == rhs.v.waif ? 0 : 1;`). Both halves of
	// that contract are pinned here.
	w1 := NewWaif(1, 2)
	w2 := NewWaif(1, 2)
	// Half 1: two independently created waifs with same class/owner/props are
	// distinct references and must NOT be equal.
	if w1.Equal(w2) {
		t.Fatal("BUG: two independently created WaifValues with same class/owner compare Equal — waif equality must use reference identity, not structural comparison")
	}
	// Even with identical property contents they remain unequal (no deep compare).
	w1.SetProperty("x", NewInt(7))
	w2.SetProperty("x", NewInt(7))
	if w1.Equal(w2) {
		t.Fatal("BUG: two distinct waifs with identical properties compare Equal — Equal must not deep-compare property contents")
	}
	// Half 2: two handles sharing the SAME underlying *waifData ARE equal.
	w1alias := w1
	if !w1.Equal(w1alias) {
		t.Fatal("BUG: two WaifValue handles referencing the same underlying waif must compare Equal under reference identity")
	}
}
