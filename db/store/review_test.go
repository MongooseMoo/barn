package store

// TestReview_* tests written by the analyst subagent to confirm bugs found
// during architectural review of db/store and db/format.
//
// Every test here is expected to FAIL (red) against the current codebase; they
// document real defects. Do NOT change the assertions to make them pass without
// fixing the underlying bug.

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestReview_DeleteVerbInheritedSilentSuccess confirms that DeleteVerb returns
// E_NONE (success) when called on a verb that exists only on an ancestor, not
// on the object itself. The correct behaviour is E_VERBNF (verb not found on
// this object). The silent success means callers believe the verb was deleted
// when nothing happened.
func TestReview_DeleteVerbInheritedSilentSuccess(t *testing.T) {
	s := NewStore()

	// #0: parent with verb "look"
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	if _, ec := s.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb look on #0: %v", ec)
	}

	// #1: child inheriting from #0
	childID, ec := s.CreateObject([]types.ObjID{0}, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject child: %v", ec)
	}

	// Precondition: child does NOT have "look" defined locally.
	if s.HasLocalVerb(childID, "look") {
		t.Fatal("precondition failed: child should not have look locally")
	}

	// DeleteVerb on the child for an inherited verb must return E_VERBNF,
	// because the verb is not defined on the child itself.
	ec = s.DeleteVerb(childID, "look")
	if ec == types.E_NONE {
		t.Errorf("DeleteVerb on inherited verb returned E_NONE (silent success); want E_VERBNF")
	}

	// Additionally confirm the parent's verb was not touched.
	_, definer, err := s.FindVerb(0, "look")
	if err != nil {
		t.Errorf("parent verb 'look' missing after DeleteVerb on child: %v", err)
	} else if definer != 0 {
		t.Errorf("parent verb 'look' found on #%d, want #0", definer)
	}
}

// TestReview_SetVerbCodeMutatesAncestor confirms that SetVerbCode, when called
// on an object that only inherits the named verb from an ancestor, mutates the
// ancestor's verb in-place instead of returning E_VERBNF.  The mutation
// silently corrupts the shared verb for every other inheritor.
func TestReview_SetVerbCodeMutatesAncestor(t *testing.T) {
	s := NewStore()

	// #0: parent with verb "look" having specific code
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	originalCode := []string{"return 1;"}
	if _, ec := s.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, originalCode)); ec != types.E_NONE {
		t.Fatalf("AddVerb look on #0: %v", ec)
	}

	// #1: child inheriting from #0
	childID, ec := s.CreateObject([]types.ObjID{0}, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject child: %v", ec)
	}

	// SetVerbCode on the child for an inherited verb should return E_VERBNF.
	// Instead it mutates the ancestor's verb.
	newCode := []string{"return 2;"}
	ec = s.SetVerbCode(childID, "look", newCode)
	if ec == types.E_NONE {
		t.Errorf("SetVerbCode on inherited verb returned E_NONE; want E_VERBNF")
	}

	// Even if SetVerbCode returns E_NONE, verify the parent's code is untouched.
	parentVerb, err := s.FindVerbOnObject(0, "look")
	if err != nil {
		t.Fatalf("parent verb 'look' not found: %v", err)
	}
	if len(parentVerb.Code) > 0 && parentVerb.Code[0] == newCode[0] {
		t.Errorf("SetVerbCode on child mutated parent verb: parent code[0] = %q, want %q",
			parentVerb.Code[0], originalCode[0])
	}
}

// TestReview_SetVerbInfoMutatesAncestor confirms that SetVerbInfo, when called
// on an object that only inherits the named verb from an ancestor, mutates the
// ancestor's verb permissions in-place instead of returning E_VERBNF.
func TestReview_SetVerbInfoMutatesAncestor(t *testing.T) {
	s := NewStore()

	// #0: parent with verb "look" that is readable AND executable
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	originalPerms := VerbRead | VerbExecute
	if _, ec := s.AddVerb(0, NewVerb("look", []string{"look"}, 0, originalPerms, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb look on #0: %v", ec)
	}

	// #1: child inheriting from #0
	childID, ec := s.CreateObject([]types.ObjID{0}, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject child: %v", ec)
	}

	// SetVerbInfo stripping the read permission via the child
	newPerms := VerbExecute // no VerbRead
	ec = s.SetVerbInfo(childID, "look", 0, newPerms, []string{"look"})
	if ec == types.E_NONE {
		t.Errorf("SetVerbInfo on inherited verb returned E_NONE; want E_VERBNF")
	}

	// Verify the parent verb is untouched.
	parentVerb, err := s.FindVerbOnObject(0, "look")
	if err != nil {
		t.Fatalf("parent verb 'look' not found: %v", err)
	}
	if !parentVerb.Perms.Has(VerbRead) {
		t.Errorf("SetVerbInfo on child stripped VerbRead from parent's verb (ancestor mutation confirmed)")
	}
}

// TestReview_RuntimeAnonLostAtSnapshot confirms that an anonymous object created
// at runtime via CreateObject is not included in the store's Snapshot (and would
// therefore be serialized as NOTHING at checkpoint, losing the object).
//
// Root cause: CreateObject inserts anonymous objects into s.objects but
// planAnonymousSerializationLocked only expands s.anonObjects when building the
// serialisation plan. The load path correctly uses AddAnonymous -> s.anonObjects,
// so the bug is specific to runtime-created anonymous objects.
func TestReview_RuntimeAnonLostAtSnapshot(t *testing.T) {
	s := NewStore()

	// #0: a regular object that will hold a reference to the anonymous object
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}

	// Create an anonymous object at runtime (as the VM would do)
	anonID, ec := s.CreateObject([]types.ObjID{0}, 0, true /*anonymous*/)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject anonymous: %v", ec)
	}

	// Store a reference to the anon object in #0's property so it is
	// reference-reachable from a non-anonymous object (required for serialisation).
	anonRef := types.NewAnon(anonID)
	if ec := s.DefineProperty(0, "anon_ref", NewProperty(anonRef, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty anon_ref: %v", ec)
	}

	snap := s.Snapshot()

	// The snapshot must include the runtime-created anonymous object.
	// If it is missing, WriteCheckpoint will serialise the reference as NOTHING
	// (dangling) and the object is permanently lost.
	if len(snap.AnonymousObjects) == 0 {
		t.Errorf("runtime-created anonymous object #%d is absent from snapshot (data loss at checkpoint); AnonymousObjects=%v",
			anonID, snap.AnonymousObjects)
	}

	// Also verify the property value is not rewritten to NOTHING in the snapshot.
	so, ok := snap.Objects[0]
	if !ok {
		t.Fatal("snapshot missing #0")
	}
	refProp, hasProp := so.Properties["anon_ref"]
	if !hasProp {
		t.Fatal("snapshot #0 missing anon_ref property")
	}
	rv := refProp.Value
	if (rv.Type() != types.TYPE_OBJ && rv.Type() != types.TYPE_ANON) || rv.ID() == types.ObjNothing {
		t.Errorf("snapshot rewrote anon_ref to NOTHING; got %v", refProp.Value)
	}
}

// TestReview_RenumberRewritesVerbAndPropOwners confirms that Renumber rewrites
// ownership references that point at the old id, not just the renumbered object's
// own .owner. ToastStunt's db_renumber_object walks every object and rewrites the
// object's own .owner, each verbdef's .owner, and each propval's .owner when they
// equal the old id (db_objects.cc:653-705, plus anonymous objects 686-705).
//
// Barn previously only rewrote the object .owner field, leaving verb owners and
// property owners pointing at the stale (now-recycled) old id.
func TestReview_RenumberRewritesVerbAndPropOwners(t *testing.T) {
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
	if _, ec := s.AddVerb(0, NewVerb("greet", []string{"greet"}, 1, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); ec != types.E_NONE {
		t.Fatalf("AddVerb greet on #0: %v", ec)
	}
	// A property on #0 owned by #1.
	if ec := s.DefineProperty(0, "color", NewProperty(types.NewStr("red"), 1, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty color: %v", ec)
	}

	// Renumber #1 -> #2.
	if err := s.Renumber(1, 2); err != nil {
		t.Fatalf("Renumber(1,2): %v", err)
	}

	// The renumbered object's own owner is unchanged here (#0), but the verb and
	// property owners that pointed at #1 MUST now point at #2.
	verb, err := s.FindVerbOnObject(0, "greet")
	if err != nil {
		t.Fatalf("FindVerbOnObject greet: %v", err)
	}
	if verb.Owner != 2 {
		t.Errorf("verb owner = #%d after renumber, want #2 (Toast rewrites verbdef owners)", verb.Owner)
	}

	prop, _, ec := s.LocalProperty(0, "color")
	if ec != types.E_NONE {
		t.Fatalf("LocalProperty color: %v", ec)
	}
	if prop.Owner != 2 {
		t.Errorf("property owner = #%d after renumber, want #2 (Toast rewrites propval owners)", prop.Owner)
	}
}

// TestReview_RenumberDoesNotUpdatePropertyValues documents the DELIBERATE
// ToastStunt/LambdaMOO behaviour: renumber() rewrites only structural/built-in
// references (parents, children, location, contents) and owner fields. It does
// NOT walk arbitrary property VALUES to rewrite object references buried inside
// them. An ObjValue(oldID) stored in a property therefore retains the stale id
// after renumber.
//
// Authority: db_renumber_object in C:/Users/Q/src/toaststunt/src/db_objects.cc
// lines 569-714. The FIX macro (591-619) only touches parents/children and
// location/contents; lines 624-641 fix anonymous children's parent slots; lines
// 643-652 fix all_users; lines 653-705 rewrite only the .owner fields of
// objects, verbdefs and propvals. The property propval VALUES (p[i].var) are
// never scanned for TYPE_OBJ refs. Barn's Renumber matches this exactly, so the
// stale reference below is correct, not a bug.
func TestReview_RenumberDoesNotUpdatePropertyValues(t *testing.T) {
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
	if ec := s.DefineProperty(0, "ref", NewProperty(refTo1, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty ref: %v", ec)
	}

	// Also create a STRUCTURAL reference to #1: move #0 inside #1 so #0.location
	// == #1. Renumber must rewrite this built-in slot (the positive control that
	// keeps the property-value negative below honest).
	if ec := s.MoveObject(0, 1, 0); ec != types.E_NONE {
		t.Fatalf("MoveObject #0 into #1: %v", ec)
	}

	// Renumber #1 -> #2
	if err := s.Renumber(1, 2); err != nil {
		t.Fatalf("Renumber(1,2): %v", err)
	}

	// Positive control: the structural location slot DID update to #2.
	loc, ec := s.Location(0)
	if ec != types.E_NONE {
		t.Fatalf("Location(0) after renumber: %v", ec)
	}
	if loc != 2 {
		t.Errorf("location ref = #%d after renumber, want #2 (Toast rewrites structural refs)", loc)
	}

	// After renumber, #0's "ref" property value is NOT rewritten: it still holds
	// the stale id #1, matching ToastStunt's db_renumber_object (see citation
	// above). The MOO programmer is responsible for fixing property-value refs.
	prop, ec := s.FindProperty(0, "ref")
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
