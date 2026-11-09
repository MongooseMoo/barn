package store

import (
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func TestRenumberParentPreservesMultipleParentTopology(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	create := func(name string) types.ObjID {
		t.Helper()
		id, errCode := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject %s failed: %v", name, errCode)
		}
		return id
	}

	hole := create("hole")
	left := create("left parent")
	right := create("right parent")
	child, errCode := store.DirectTxn().CreateObject([]types.ObjID{left, right}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject child failed: %v", errCode)
	}

	if err := store.Recycle(hole); err != nil {
		t.Fatalf("Recycle hole failed: %v", err)
	}
	if err := store.Renumber(right, hole); err != nil {
		t.Fatalf("Renumber right parent failed: %v", err)
	}

	parents, errCode := store.DirectTxn().Parents(child)
	if errCode != types.E_NONE {
		t.Fatalf("Parents child failed: %v", errCode)
	}
	if want := []types.ObjID{left, hole}; !slices.Equal(parents, want) {
		t.Fatalf("Parents child = %v, want %v", parents, want)
	}

	children, errCode := store.DirectTxn().Children(hole)
	if errCode != types.E_NONE {
		t.Fatalf("Children renumbered parent failed: %v", errCode)
	}
	if !slices.Contains(children, child) {
		t.Fatalf("Children renumbered parent = %v, want child %d", children, child)
	}
	if store.DirectTxn().Valid(right) {
		t.Fatalf("old parent id %d remains valid after renumber", right)
	}
}

// TestRenumberRewritesVerbAndPropertyOwners checks that Renumber rewrites
// ownership references that point at the old id, not just the renumbered
// object's own .owner. ToastStunt's db_renumber_object walks every object and
// rewrites the object's own .owner, each verbdef's .owner, and each propval's
// .owner when they equal the old id (db_objects.cc:653-705, plus anonymous
// objects 686-705).
func TestRenumberRewritesVerbAndPropertyOwners(t *testing.T) {
	s := NewStore()

	// #0: holds a verb and a property both OWNED BY #1.
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	// #1: the object we will renumber to #2. It owns the verb/prop on #0.
	if err := s.Add(NewObject(1, 0)); err != nil {
		t.Fatalf("Add #1: %v", err)
	}

	// A verb on #0 owned by #1.
	if _, ec := s.AddVerb(0, NewVerb([]string{"greet"}, 1, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb greet on #0: %v", ec)
	}
	// A property on #0 owned by #1.
	if ec := s.DirectTxn().DefineProperty(0, "color", NewProperty(types.NewStr("red"), 1, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty color: %v", ec)
	}

	// Renumber #1 -> #2.
	if err := s.Renumber(1, 2); err != nil {
		t.Fatalf("Renumber(1,2): %v", err)
	}

	// The renumbered object's own owner is unchanged here (#0), but the verb and
	// property owners that pointed at #1 MUST now point at #2.
	verb, err := s.DirectTxn().FindVerbOnObject(0, "greet")
	if err != nil {
		t.Fatalf("FindVerbOnObject greet: %v", err)
	}
	if verb.Owner != 2 {
		t.Errorf("verb owner = #%d after renumber, want #2 (Toast rewrites verbdef owners)", verb.Owner)
	}

	prop, _, ec := s.DirectTxn().LocalProperty(0, "color")
	if ec != types.E_NONE {
		t.Fatalf("LocalProperty color: %v", ec)
	}
	if prop.Owner != 2 {
		t.Errorf("property owner = #%d after renumber, want #2 (Toast rewrites propval owners)", prop.Owner)
	}
}

// TestRenumberLeavesPropertyValueReferences checks the ToastStunt/LambdaMOO
// behaviour that renumber() rewrites only structural references (parents,
// children, location, contents) and owner fields. It does NOT walk property
// VALUES, so an ObjValue(oldID) stored in a property keeps the stale id.
//
// Authority: db_renumber_object in toaststunt/src/db_objects.cc lines 569-714.
// The FIX macro (591-619) only touches parents/children and location/contents;
// lines 624-641 fix anonymous children's parent slots; lines 643-652 fix
// all_users; lines 653-705 rewrite only the .owner fields of objects, verbdefs
// and propvals. The propval VALUES (p[i].var) are never scanned for TYPE_OBJ
// refs.
func TestRenumberLeavesPropertyValueReferences(t *testing.T) {
	s := NewStore()

	// #0: will hold a reference to #1 in a property
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	// #1: the object we will renumber
	if err := s.Add(NewObject(1, 0)); err != nil {
		t.Fatalf("Add #1: %v", err)
	}

	// Store a reference to #1 in a property of #0.
	refTo1 := types.NewObj(1)
	if ec := s.DirectTxn().DefineProperty(0, "ref", NewProperty(refTo1, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty ref: %v", ec)
	}

	// Also create a STRUCTURAL reference to #1: move #0 inside #1 so #0.location
	// == #1. Renumber must rewrite this built-in slot (the positive control that
	// keeps the property-value negative below honest).
	if ec := s.DirectTxn().MoveObject(0, 1, 0); ec != types.E_NONE {
		t.Fatalf("MoveObject #0 into #1: %v", ec)
	}

	// Renumber #1 -> #2
	if err := s.Renumber(1, 2); err != nil {
		t.Fatalf("Renumber(1,2): %v", err)
	}

	// Positive control: the structural location slot DID update to #2.
	loc, ec := s.DirectTxn().Location(0)
	if ec != types.E_NONE {
		t.Fatalf("Location(0) after renumber: %v", ec)
	}
	if loc != 2 {
		t.Errorf("location ref = #%d after renumber, want #2 (Toast rewrites structural refs)", loc)
	}

	// #0's "ref" property value still holds the stale id #1.
	prop, ec := s.DirectTxn().FindProperty(0, "ref")
	if ec != types.E_NONE {
		t.Fatalf("FindProperty ref after renumber: %v", ec)
	}
	rv := prop.Value
	if rv.Type() != types.TYPE_OBJ && rv.Type() != types.TYPE_ANON {
		t.Fatalf("ref property value is not an object: %v", prop.Value)
	}
	if rv.ID() != 1 {
		t.Errorf("ref property value = #%d, want stale #1 (Toast does not rewrite property-value refs)", rv.ID())
	}
}
