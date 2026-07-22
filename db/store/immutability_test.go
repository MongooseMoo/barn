package store

import (
	"testing"

	"barn/types"
)

// The published-image immutability invariant (store_core.go objectSlot contract):
// once an *Object pointer is Stored into a slot, that object's fields and the
// *Property/*Verb nodes it owns are never written again. Every runtime mutation
// must publish a NEW image and retain the old one immutably, so a read
// transaction that has ALIASED the published image (Phase 2) never observes it
// change underfoot. These tests capture a published image pointer + a snapshot of
// its mutated field BEFORE an operation and assert, AFTER the operation, that the
// captured pointer's content is unchanged (not mutated in place) and that the slot
// now holds a DIFFERENT pointer carrying the new value.

func objIDsEqual(a, b []types.ObjID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// immutFixture builds a tiny store with a wizard root and n plain objects.
func immutFixture(t *testing.T, n int) (*Store, []types.ObjID) {
	t.Helper()
	s := NewStore()
	root := NewObjectBuilder(0)
	root.SetFlags(FlagWizard | FlagRead | FlagWrite)
	if err := s.Add(root.Build()); err != nil {
		t.Fatalf("Add root: %v", err)
	}
	ids := make([]types.ObjID, n)
	for i := range ids {
		id, ec := s.CreateObject(nil, 0, false)
		if ec != types.E_NONE {
			t.Fatalf("CreateObject: %v", ec)
		}
		ids[i] = id
	}
	return s, ids
}

func TestMovePublishesNewImageNotInPlace(t *testing.T) {
	s, ids := immutFixture(t, 3)
	roomA, roomB, thing := ids[0], ids[1], ids[2]
	if ec := s.MoveObject(thing, roomA, 0); ec != types.E_NONE {
		t.Fatalf("initial move: %v", ec)
	}

	oldThing := s.load(thing)
	oldA := s.load(roomA)
	thingLocBefore := oldThing.location
	aContentsBefore := append([]types.ObjID(nil), oldA.contents...)

	if ec := s.MoveObject(thing, roomB, 0); ec != types.E_NONE {
		t.Fatalf("move to B: %v", ec)
	}

	// The old published images must NOT have been mutated in place.
	if oldThing.location != thingLocBefore {
		t.Errorf("published `thing` image mutated in place: location %v -> %v", thingLocBefore, oldThing.location)
	}
	if !objIDsEqual(oldA.contents, aContentsBefore) {
		t.Errorf("published `roomA` image mutated in place: contents %v -> %v", aContentsBefore, oldA.contents)
	}
	// The slot must now hold a fresh image carrying the new state.
	if s.load(thing) == oldThing {
		t.Errorf("`thing` image was not republished (same pointer)")
	}
	if got := s.load(thing).location; got != roomB {
		t.Errorf("new `thing` image has wrong location: got %v want %v", got, roomB)
	}
}

// TestReadAliasSurvivesConcurrentMutation is the load-bearing Phase 2 safety test:
// a read transaction that has ALIASED a published image must observe its snapshot
// unchanged when another path mutates the same object, because the mutation
// republishes a fresh image and never writes the aliased (old) one in place.
func TestReadAliasSurvivesConcurrentMutation(t *testing.T) {
	s, ids := immutFixture(t, 1)
	id := ids[0]
	if ec := s.DefineProperty(id, "foo", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}

	tx := s.BeginReadOnly(0)
	defer tx.Release()
	obj := tx.object(id) // aliases the published image at this txn's snapshot
	_, propBefore, ok := propertyByName(obj.properties, "foo")
	if !ok {
		t.Fatal("foo not visible to reader")
	}

	// Mutate the same object through the store; this republishes a fresh image.
	if ec := s.SetPropertyValue(id, "foo", types.NewInt(999)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}

	_, propAfter, _ := propertyByName(obj.properties, "foo")
	if !propBefore.value.Equal(propAfter.value) {
		t.Errorf("aliased image changed under concurrent mutation: foo %v -> %v", propBefore.value, propAfter.value)
	}
	if !obj.properties["foo"].value.Equal(types.NewInt(1)) {
		t.Errorf("reader snapshot corrupted: foo = %v, want 1", obj.properties["foo"].value)
	}
	// The reader must also be aliasing (not cloning) the live image: same pointer
	// as the store's published image at alias time. After the mutation the store's
	// live pointer has moved on, but the reader's cached pointer is the old image.
	if s.load(id) == obj {
		t.Errorf("store still points at the reader's aliased image after mutation (not republished)")
	}
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assertRepublished fails if the slot for id still holds oldPtr (i.e. the image
// was NOT republished) — the second half of the invariant (the first half, that
// oldPtr's content is unchanged, is checked per-field by each caller).
func assertRepublished(t *testing.T, s *Store, id types.ObjID, oldPtr *Object, op string) {
	t.Helper()
	if s.load(id) == oldPtr {
		t.Errorf("%s: image #%d was not republished (same pointer)", op, id)
	}
}

func TestSetObjectNamePublishesNewImage(t *testing.T) {
	s, ids := immutFixture(t, 1)
	id := ids[0]
	old := s.load(id)
	nameBefore := old.name
	if ec := s.SetObjectName(id, "renamed"); ec != types.E_NONE {
		t.Fatalf("SetObjectName: %v", ec)
	}
	if old.name != nameBefore {
		t.Errorf("SetObjectName mutated published image in place: %q -> %q", nameBefore, old.name)
	}
	assertRepublished(t, s, id, old, "SetObjectName")
	if s.load(id).name != "renamed" {
		t.Errorf("new image has wrong name: %q", s.load(id).name)
	}
}

func TestCreateObjectPublishesNewParentImage(t *testing.T) {
	s, ids := immutFixture(t, 1)
	parent := ids[0]
	oldParent := s.load(parent)
	childrenBefore := append([]types.ObjID(nil), oldParent.children...)
	child, ec := s.CreateObject([]types.ObjID{parent}, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject: %v", ec)
	}
	if !objIDsEqual(oldParent.children, childrenBefore) {
		t.Errorf("CreateObject mutated parent's published children in place: %v -> %v", childrenBefore, oldParent.children)
	}
	assertRepublished(t, s, parent, oldParent, "CreateObject(child attach)")
	found := false
	for _, c := range s.load(parent).children {
		if c == child {
			found = true
		}
	}
	if !found {
		t.Errorf("new parent image missing child %v", child)
	}
}

func TestChangeParentsPublishesNewImages(t *testing.T) {
	s, ids := immutFixture(t, 3)
	obj, oldP, newP := ids[0], ids[1], ids[2]
	if ec := s.ChangeParents(obj, []types.ObjID{oldP}); ec != types.E_NONE {
		t.Fatalf("initial ChangeParents: %v", ec)
	}
	oldObj := s.load(obj)
	oldOldParent := s.load(oldP)
	objParentsBefore := append([]types.ObjID(nil), oldObj.parents...)
	oldParentChildrenBefore := append([]types.ObjID(nil), oldOldParent.children...)

	if ec := s.ChangeParents(obj, []types.ObjID{newP}); ec != types.E_NONE {
		t.Fatalf("ChangeParents: %v", ec)
	}
	if !objIDsEqual(oldObj.parents, objParentsBefore) {
		t.Errorf("ChangeParents mutated obj's published parents in place: %v -> %v", objParentsBefore, oldObj.parents)
	}
	if !objIDsEqual(oldOldParent.children, oldParentChildrenBefore) {
		t.Errorf("ChangeParents mutated old parent's published children in place: %v -> %v", oldParentChildrenBefore, oldOldParent.children)
	}
	assertRepublished(t, s, obj, oldObj, "ChangeParents(obj)")
}

func TestDefinePropertyPublishesNewImage(t *testing.T) {
	s, ids := immutFixture(t, 1)
	id := ids[0]
	old := s.load(id)
	propsBefore := len(old.properties)
	orderBefore := append([]string(nil), old.propOrder...)
	if ec := s.DefineProperty(id, "foo", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	if len(old.properties) != propsBefore {
		t.Errorf("DefineProperty mutated published properties map in place (len %d -> %d)", propsBefore, len(old.properties))
	}
	if !stringsEqual(old.propOrder, orderBefore) {
		t.Errorf("DefineProperty mutated published propOrder in place")
	}
	assertRepublished(t, s, id, old, "DefineProperty")
}

func TestSetPropertyValuePublishesNewImage(t *testing.T) {
	s, ids := immutFixture(t, 1)
	id := ids[0]
	if ec := s.DefineProperty(id, "foo", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	old := s.load(id)
	_, propBefore, ok := propertyByName(old.properties, "foo")
	if !ok {
		t.Fatal("foo not found after define")
	}
	if ec := s.SetPropertyValue(id, "foo", types.NewInt(2)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	_, propAfter, _ := propertyByName(old.properties, "foo")
	if !propBefore.value.Equal(propAfter.value) {
		t.Errorf("SetPropertyValue mutated published property value in place: %v -> %v", propBefore.value, propAfter.value)
	}
	assertRepublished(t, s, id, old, "SetPropertyValue")
}

func TestDeleteDefinedPropertyPublishesNewImage(t *testing.T) {
	s, ids := immutFixture(t, 1)
	id := ids[0]
	if ec := s.DefineProperty(id, "foo", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	old := s.load(id)
	propsBefore := len(old.properties)
	if ec := s.DeleteDefinedProperty(id, "foo"); ec != types.E_NONE {
		t.Fatalf("DeleteDefinedProperty: %v", ec)
	}
	if len(old.properties) != propsBefore {
		t.Errorf("DeleteDefinedProperty mutated published properties map in place (len %d -> %d)", propsBefore, len(old.properties))
	}
	assertRepublished(t, s, id, old, "DeleteDefinedProperty")
}

func TestAddVerbPublishesNewImage(t *testing.T) {
	s, ids := immutFixture(t, 1)
	id := ids[0]
	old := s.load(id)
	verbsBefore := len(old.verbList)
	v := NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "this", Prep: "none", That: "none"}, []string{"return 1;"})
	if _, ec := s.AddVerb(id, v); ec != types.E_NONE {
		t.Fatalf("AddVerb: %v", ec)
	}
	if len(old.verbList) != verbsBefore {
		t.Errorf("AddVerb mutated published verbList in place (len %d -> %d)", verbsBefore, len(old.verbList))
	}
	assertRepublished(t, s, id, old, "AddVerb")
}

func TestSetVerbCodePublishesNewImage(t *testing.T) {
	s, ids := immutFixture(t, 1)
	id := ids[0]
	v := NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "this", Prep: "none", That: "none"}, []string{"return 1;"})
	if _, ec := s.AddVerb(id, v); ec != types.E_NONE {
		t.Fatalf("AddVerb: %v", ec)
	}
	old := s.load(id)
	oldVerb := old.verbs["look"]
	codeBefore := append([]string(nil), oldVerb.code...)
	if ec := s.SetVerbCode(id, "look", []string{"return 42;"}); ec != types.E_NONE {
		t.Fatalf("SetVerbCode: %v", ec)
	}
	if !stringsEqual(oldVerb.code, codeBefore) {
		t.Errorf("SetVerbCode mutated published verb node in place: %v -> %v", codeBefore, oldVerb.code)
	}
	assertRepublished(t, s, id, old, "SetVerbCode")
}

func TestDeleteVerbPublishesNewImage(t *testing.T) {
	s, ids := immutFixture(t, 1)
	id := ids[0]
	v := NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "this", Prep: "none", That: "none"}, []string{"return 1;"})
	if _, ec := s.AddVerb(id, v); ec != types.E_NONE {
		t.Fatalf("AddVerb: %v", ec)
	}
	old := s.load(id)
	verbsBefore := len(old.verbList)
	if ec := s.DeleteVerb(id, "look"); ec != types.E_NONE {
		t.Fatalf("DeleteVerb: %v", ec)
	}
	if len(old.verbList) != verbsBefore {
		t.Errorf("DeleteVerb mutated published verbList in place (len %d -> %d)", verbsBefore, len(old.verbList))
	}
	assertRepublished(t, s, id, old, "DeleteVerb")
}
