package store

// TestReview_* tests written by the analyst subagent to confirm bugs found
// during architectural review of db/store and db/format.
//
// Every test here is expected to FAIL (red) against the current codebase; they
// document real defects. Do NOT change the assertions to make them pass without
// fixing the underlying bug.

import (
	"testing"

	"barn/types"
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
	if ec := s.DefineProperty(0, NewProperty("anon_ref", anonRef, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
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
	if objVal, isObj := refProp.Value.(types.ObjValue); !isObj || objVal.ID() == types.ObjNothing {
		t.Errorf("snapshot rewrote anon_ref to NOTHING; got %v", refProp.Value)
	}
}

// TestReview_RenumberDoesNotUpdatePropertyValues confirms that Renumber fails to
// update ObjValue references stored in property values. After Renumber(oldID,
// newID), any property whose value is ObjValue(oldID) still holds the stale id.
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
	if ec := s.DefineProperty(0, NewProperty("ref", refTo1, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty ref: %v", ec)
	}

	// Renumber #1 -> #2
	if err := s.Renumber(1, 2); err != nil {
		t.Fatalf("Renumber(1,2): %v", err)
	}

	// After renumber, #0's "ref" property should point to #2, not stale #1.
	prop, ec := s.FindProperty(0, "ref")
	if ec != types.E_NONE {
		t.Fatalf("FindProperty ref after renumber: %v", ec)
	}
	objVal, ok := prop.Value.(types.ObjValue)
	if !ok {
		t.Fatalf("ref property value is not ObjValue: %T", prop.Value)
	}
	if objVal.ID() == 1 {
		t.Errorf("Renumber did not update property value: ref still points to old id #1; want #2")
	}
	if objVal.ID() != 2 {
		t.Errorf("ref property value = #%d, want #2", objVal.ID())
	}
}
