package store

import (
	"testing"

	"barn/types"
)

func TestReadOnlyTransactionSeesStableSnapshot(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.SetObjectName(0, "before"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName before failed: %v", errCode)
	}
	if errCode := store.DefineProperty(0, "scratch", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject child failed: %v", errCode)
	}

	readTS := store.ReadTimestamp()
	tx := store.BeginReadOnly(0)
	if tx.ReadTimestamp() != readTS {
		t.Fatalf("txn timestamp = %d, want %d", tx.ReadTimestamp(), readTS)
	}

	if errCode := store.SetObjectName(0, "after"); errCode != types.E_NONE {
		t.Fatalf("SetObjectName after failed: %v", errCode)
	}
	if errCode := store.SetPropertyValue(0, "scratch", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue after failed: %v", errCode)
	}
	if errCode := store.SetVerbCode(0, "look", []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode after failed: %v", errCode)
	}
	if _, errCode := store.CreateObject([]types.ObjID{0}, 0, false); errCode != types.E_NONE {
		t.Fatalf("CreateObject second child failed: %v", errCode)
	}

	if got, errCode := tx.ObjectName(0); errCode != types.E_NONE || got != "before" {
		t.Fatalf("txn ObjectName = %q err=%v, want before", got, errCode)
	}
	prop, errCode := tx.FindProperty(0, "scratch")
	if errCode != types.E_NONE {
		t.Fatalf("txn FindProperty failed: %v", errCode)
	}
	if got := prop.Value.Int(); got != 1 {
		t.Fatalf("txn property value = %d, want 1", got)
	}
	verb, _, err := tx.FindVerb(0, "look")
	if err != nil {
		t.Fatalf("txn FindVerb failed: %v", err)
	}
	if len(verb.Code) != 1 || verb.Code[0] != "return 1;" {
		t.Fatalf("txn verb code = %#v, want original code", verb.Code)
	}
	children, errCode := tx.Children(0)
	if errCode != types.E_NONE {
		t.Fatalf("txn Children failed: %v", errCode)
	}
	if len(children) != 1 || children[0] != child {
		t.Fatalf("txn children = %#v, want only child #%d", children, child)
	}

	if got, errCode := store.ObjectName(0); errCode != types.E_NONE || got != "after" {
		t.Fatalf("live ObjectName = %q err=%v, want after", got, errCode)
	}
	if nextTS := store.ReadTimestamp(); nextTS <= readTS {
		t.Fatalf("store timestamp after writes = %d, want > %d", nextTS, readTS)
	}
}

func TestReadOnlyTransactionClonesReturnedContainers(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject child failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	children, errCode := tx.Children(0)
	if errCode != types.E_NONE {
		t.Fatalf("txn Children failed: %v", errCode)
	}
	children[0] = 99

	again, errCode := tx.Children(0)
	if errCode != types.E_NONE {
		t.Fatalf("txn Children second read failed: %v", errCode)
	}
	if len(again) != 1 || again[0] != child {
		t.Fatalf("mutating returned children changed txn snapshot: %#v", again)
	}
}

func TestReadOnlyTransactionLoadsObjectsLazily(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.CreateObject([]types.ObjID{0}, 0, false); errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if got := len(tx.objects); got != 0 {
		t.Fatalf("BeginReadOnly cached %d objects, want 0", got)
	}
	if _, errCode := tx.ObjectName(0); errCode != types.E_NONE {
		t.Fatalf("ObjectName failed: %v", errCode)
	}
	if got := len(tx.objects); got != 1 {
		t.Fatalf("after one object read cached %d objects, want 1", got)
	}
	if _, errCode := tx.Children(0); errCode != types.E_NONE {
		t.Fatalf("Children failed: %v", errCode)
	}
	if got := len(tx.objects); got != 1 {
		t.Fatalf("relationship read cached %d objects, want still 1", got)
	}
}

func TestTransactionChildrenTracksRelationshipRead(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.CreateObject([]types.ObjID{0}, 0, false); errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if _, errCode := tx.Children(0); errCode != types.E_NONE {
		t.Fatalf("Children failed: %v", errCode)
	}

	live := store.load(0)
	if got := tx.relationshipReads[0]; got != live.relationshipVersion {
		t.Fatalf("relationship read version = %d, want %d", got, live.relationshipVersion)
	}
}

func TestTransactionRelationshipReadInvalidatesCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if _, errCode := tx.Children(0); errCode != types.E_NONE {
		t.Fatalf("Children failed: %v", errCode)
	}
	if errCode := tx.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	if _, errCode := store.CreateObject([]types.ObjID{0}, 0, false); errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	if errCode := tx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("Commit = %v, want E_INVARG conflict", errCode)
	}
	if !tx.ValidationFailed() {
		t.Fatalf("transaction did not record validation failure")
	}
	value, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := value.Int(); got != 1 {
		t.Fatalf("property a = %d, want unchanged value 1", got)
	}
}

func TestTransactionAdoptLiveRelationshipsSeesMove(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject obj failed: %v", errCode)
	}
	oldLocation, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject old location failed: %v", errCode)
	}
	newLocation, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject new location failed: %v", errCode)
	}
	if errCode := store.MoveObject(obj, oldLocation, 0); errCode != types.E_NONE {
		t.Fatalf("initial MoveObject failed: %v", errCode)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if _, errCode := tx.Location(obj); errCode != types.E_NONE {
		t.Fatalf("tx Location obj failed: %v", errCode)
	}
	if _, errCode := tx.Contents(oldLocation); errCode != types.E_NONE {
		t.Fatalf("tx Contents old location failed: %v", errCode)
	}
	if errCode := store.MoveObject(obj, newLocation, 0); errCode != types.E_NONE {
		t.Fatalf("live MoveObject failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveRelationships(obj, oldLocation, newLocation); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveRelationships failed: %v", errCode)
	}
	location, errCode := tx.Location(obj)
	if errCode != types.E_NONE {
		t.Fatalf("tx Location after adopt failed: %v", errCode)
	}
	if location != newLocation {
		t.Fatalf("tx Location after adopt = #%d, want #%d", location, newLocation)
	}
	if errCode := tx.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
}

func TestTransactionAdoptLiveRelationshipsSeesCreatedChild(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	children, errCode := tx.Children(0)
	if errCode != types.E_NONE {
		t.Fatalf("tx Children before create failed: %v", errCode)
	}
	if len(children) != 0 {
		t.Fatalf("tx Children before create = %#v, want empty", children)
	}

	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject child failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveObject(child); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveObject failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveRelationships(child, 0); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveRelationships failed: %v", errCode)
	}

	children, errCode = tx.Children(0)
	if errCode != types.E_NONE {
		t.Fatalf("tx Children after create failed: %v", errCode)
	}
	if len(children) != 1 || children[0] != child {
		t.Fatalf("tx Children after create = %#v, want child #%d", children, child)
	}
	if errCode := tx.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
}

func TestTransactionAdoptLiveRelationshipsSeesChangedParents(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject obj failed: %v", errCode)
	}
	newParent, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject new parent failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if _, errCode := tx.Parents(obj); errCode != types.E_NONE {
		t.Fatalf("tx Parents before change failed: %v", errCode)
	}
	if _, errCode := tx.Children(0); errCode != types.E_NONE {
		t.Fatalf("tx Children old parent before change failed: %v", errCode)
	}
	if errCode := store.ChangeParents(obj, []types.ObjID{newParent}); errCode != types.E_NONE {
		t.Fatalf("ChangeParents failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveRelationships(obj, 0, newParent); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveRelationships failed: %v", errCode)
	}

	parents, errCode := tx.Parents(obj)
	if errCode != types.E_NONE {
		t.Fatalf("tx Parents after change failed: %v", errCode)
	}
	if len(parents) != 1 || parents[0] != newParent {
		t.Fatalf("tx Parents after change = %#v, want #%d", parents, newParent)
	}
	oldChildren, errCode := tx.Children(0)
	if errCode != types.E_NONE {
		t.Fatalf("tx Children old parent after change failed: %v", errCode)
	}
	if len(oldChildren) != 1 || oldChildren[0] != newParent {
		t.Fatalf("tx old parent children = %#v, want only new parent #%d", oldChildren, newParent)
	}
	newChildren, errCode := tx.Children(newParent)
	if errCode != types.E_NONE {
		t.Fatalf("tx Children new parent after change failed: %v", errCode)
	}
	if len(newChildren) != 1 || newChildren[0] != obj {
		t.Fatalf("tx new parent children = %#v, want object #%d", newChildren, obj)
	}
}

func TestTransactionAdoptLiveRelationshipsRefreshesAnonymousChildAfterRenumber(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	freeID, errCode := store.CreateObject(nil, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject free slot failed: %v", errCode)
	}
	if err := store.Recycle(freeID); err != nil {
		t.Fatalf("Recycle free slot failed: %v", err)
	}
	parent, errCode := store.CreateObject(nil, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject parent failed: %v", errCode)
	}
	anon, errCode := store.CreateObject([]types.ObjID{parent}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject anonymous child failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DefineProperty(parent, "xyz", NewProperty(types.NewInt(1), 0, PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty parent failed: %v", errCode)
	}
	children, errCode := tx.AnonymousChildren(parent)
	if errCode != types.E_NONE {
		t.Fatalf("AnonymousChildren before renumber failed: %v", errCode)
	}
	if len(children) != 1 || children[0] != anon {
		t.Fatalf("AnonymousChildren before renumber = %#v, want anonymous child #%d", children, anon)
	}
	if _, errCode := tx.Parents(anon); errCode != types.E_NONE {
		t.Fatalf("Parents anonymous before renumber failed: %v", errCode)
	}

	if err := store.Renumber(parent, freeID); err != nil {
		t.Fatalf("Renumber failed: %v", err)
	}
	tx.MoveStagedProperties(parent, freeID)
	tx.ForgetObject(parent)
	if errCode := tx.AdoptLiveObject(freeID); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveObject renumbered parent failed: %v", errCode)
	}
	tx.ApplyStagedProperties(freeID)
	if errCode := tx.AdoptLiveRelationships(freeID, anon); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveRelationships failed: %v", errCode)
	}

	parents, errCode := tx.Parents(anon)
	if errCode != types.E_NONE {
		t.Fatalf("Parents anonymous after renumber failed: %v", errCode)
	}
	if len(parents) != 1 || parents[0] != freeID {
		t.Fatalf("Parents anonymous after renumber = %#v, want #%d", parents, freeID)
	}
	value, errCode := tx.PropertyValue(anon, "xyz")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue anonymous after renumber failed: %v", errCode)
	}
	if got := value.Int(); got != 1 {
		t.Fatalf("PropertyValue anonymous after renumber = %d, want 1", got)
	}
}

func TestTransactionRenumberLeavesOldObjectIDInvalid(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	freeID, errCode := store.CreateObject(nil, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject free slot failed: %v", errCode)
	}
	if err := store.Recycle(freeID); err != nil {
		t.Fatalf("Recycle free slot failed: %v", err)
	}
	oldID, errCode := store.CreateObject(nil, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject renumber source failed: %v", errCode)
	}
	if _, errCode := store.AddVerb(oldID, NewVerb("test", []string{"test"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return \"test\";"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	before := store.BeginReadOnly(0)
	if !before.Valid(oldID) {
		t.Fatalf("pre-renumber transaction Valid(%d) = false, want true", oldID)
	}
	if before.Valid(freeID) {
		t.Fatalf("pre-renumber transaction Valid(%d) = true, want false for recycled target", freeID)
	}

	if err := store.Renumber(oldID, freeID); err != nil {
		t.Fatalf("Renumber failed: %v", err)
	}

	if !before.Valid(oldID) {
		t.Fatalf("pre-renumber transaction Valid(%d) after renumber = false, want snapshot true", oldID)
	}
	if before.Valid(freeID) {
		t.Fatalf("pre-renumber transaction Valid(%d) after renumber = true, want snapshot false", freeID)
	}
	if _, _, err := before.FindVerb(oldID, "test"); err != nil {
		t.Fatalf("pre-renumber transaction FindVerb old id failed: %v", err)
	}

	after := store.BeginReadOnly(0)
	if after.Valid(oldID) {
		t.Fatalf("post-renumber transaction Valid(%d) = true, want false", oldID)
	}
	if !after.Valid(freeID) {
		t.Fatalf("post-renumber transaction Valid(%d) = false, want true", freeID)
	}
	if _, _, err := after.FindVerb(oldID, "test"); err == nil {
		t.Fatalf("post-renumber transaction FindVerb old id succeeded, want invalid object")
	}
	if _, _, err := after.FindVerb(freeID, "test"); err != nil {
		t.Fatalf("post-renumber transaction FindVerb new id failed: %v", err)
	}
}

func TestTransactionDisjointPropertyWritesBothCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty a failed: %v", errCode)
	}
	if errCode := store.DefineProperty(0, "b", NewProperty(types.NewInt(10), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty b failed: %v", errCode)
	}

	txA := store.BeginReadOnly(0)
	txB := store.BeginReadOnly(0)
	if errCode := txA.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("txA SetPropertyValue failed: %v", errCode)
	}
	if errCode := txB.SetPropertyValue(0, "b", types.NewInt(20)); errCode != types.E_NONE {
		t.Fatalf("txB SetPropertyValue failed: %v", errCode)
	}

	if errCode := txA.Commit(); errCode != types.E_NONE {
		t.Fatalf("txA Commit failed: %v", errCode)
	}
	if errCode := txB.Commit(); errCode != types.E_NONE {
		t.Fatalf("txB Commit failed: %v", errCode)
	}

	a, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue a failed: %v", errCode)
	}
	if got := a.Int(); got != 2 {
		t.Fatalf("a = %d, want 2", got)
	}
	b, errCode := store.PropertyValue(0, "b")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue b failed: %v", errCode)
	}
	if got := b.Int(); got != 20 {
		t.Fatalf("b = %d, want 20", got)
	}
}

func TestTransactionSamePropertyWriteConflicts(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	first := store.BeginReadOnly(0)
	second := store.BeginReadOnly(0)
	if errCode := first.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("first SetPropertyValue failed: %v", errCode)
	}
	if errCode := second.SetPropertyValue(0, "a", types.NewInt(3)); errCode != types.E_NONE {
		t.Fatalf("second SetPropertyValue failed: %v", errCode)
	}

	if errCode := first.Commit(); errCode != types.E_NONE {
		t.Fatalf("first Commit failed: %v", errCode)
	}
	if errCode := second.Commit(); errCode != types.E_INVARG {
		t.Fatalf("second Commit = %v, want E_INVARG conflict", errCode)
	}
	if !second.ValidationFailed() {
		t.Fatalf("second transaction did not record validation failure")
	}

	a, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := a.Int(); got != 2 {
		t.Fatalf("a = %d, want first writer value 2", got)
	}
}

func TestTransactionSetPropertyInfoStagesUntilCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	newOwner := types.ObjID(7)
	newPerms := PropRead
	if errCode := tx.SetPropertyInfo(0, "a", &newOwner, &newPerms); errCode != types.E_NONE {
		t.Fatalf("SetPropertyInfo failed: %v", errCode)
	}

	txProp, ok, errCode := tx.LocalProperty(0, "a")
	if errCode != types.E_NONE || !ok {
		t.Fatalf("tx LocalProperty ok=%v err=%v, want local property", ok, errCode)
	}
	if txProp.Owner != newOwner || txProp.Perms != newPerms {
		t.Fatalf("tx property info owner=%d perms=%v, want owner=%d perms=%v", txProp.Owner, txProp.Perms, newOwner, newPerms)
	}
	liveProp, ok, errCode := store.LocalProperty(0, "a")
	if errCode != types.E_NONE || !ok {
		t.Fatalf("live LocalProperty ok=%v err=%v, want local property", ok, errCode)
	}
	if liveProp.Owner == newOwner || liveProp.Perms == newPerms {
		t.Fatalf("live property info changed before commit: owner=%d perms=%v", liveProp.Owner, liveProp.Perms)
	}

	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	liveProp, ok, errCode = store.LocalProperty(0, "a")
	if errCode != types.E_NONE || !ok {
		t.Fatalf("live LocalProperty after commit ok=%v err=%v, want local property", ok, errCode)
	}
	if liveProp.Owner != newOwner || liveProp.Perms != newPerms {
		t.Fatalf("live property info owner=%d perms=%v, want owner=%d perms=%v", liveProp.Owner, liveProp.Perms, newOwner, newPerms)
	}
}

func TestTransactionPropertyInfoConflictsWithValueWrite(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	infoTx := store.BeginReadOnly(0)
	valueTx := store.BeginReadOnly(0)
	newPerms := PropRead
	if errCode := infoTx.SetPropertyInfo(0, "a", nil, &newPerms); errCode != types.E_NONE {
		t.Fatalf("infoTx SetPropertyInfo failed: %v", errCode)
	}
	if errCode := valueTx.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("valueTx SetPropertyValue failed: %v", errCode)
	}

	if errCode := infoTx.Commit(); errCode != types.E_NONE {
		t.Fatalf("infoTx Commit failed: %v", errCode)
	}
	if errCode := valueTx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("valueTx Commit = %v, want E_INVARG conflict", errCode)
	}
	if !valueTx.ValidationFailed() {
		t.Fatalf("valueTx did not record validation failure")
	}

	value, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := value.Int(); got != 1 {
		t.Fatalf("property a = %d, want unchanged value 1", got)
	}
	prop, ok, errCode := store.LocalProperty(0, "a")
	if errCode != types.E_NONE || !ok {
		t.Fatalf("LocalProperty ok=%v err=%v, want local property", ok, errCode)
	}
	if prop.Perms != newPerms {
		t.Fatalf("property perms = %v, want %v", prop.Perms, newPerms)
	}
}

func TestTransactionClearPropertyOverrideStagesUntilCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	if errCode := store.SetPropertyValue(child, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue override failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.ClearPropertyOverride(child, "a"); errCode != types.E_NONE {
		t.Fatalf("ClearPropertyOverride failed: %v", errCode)
	}
	txValue, errCode := tx.PropertyValue(child, "a")
	if errCode != types.E_NONE {
		t.Fatalf("tx PropertyValue failed: %v", errCode)
	}
	if got := txValue.Int(); got != 1 {
		t.Fatalf("tx property value = %d, want inherited value 1", got)
	}
	liveValue, errCode := store.PropertyValue(child, "a")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue failed: %v", errCode)
	}
	if got := liveValue.Int(); got != 2 {
		t.Fatalf("live property value before commit = %d, want local override 2", got)
	}

	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	liveValue, errCode = store.PropertyValue(child, "a")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue after commit failed: %v", errCode)
	}
	if got := liveValue.Int(); got != 1 {
		t.Fatalf("live property value after commit = %d, want inherited value 1", got)
	}
}

func TestTransactionClearPropertyOverrideConflictsWithValueWrite(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	if errCode := store.SetPropertyValue(child, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue override failed: %v", errCode)
	}

	clearTx := store.BeginReadOnly(0)
	valueTx := store.BeginReadOnly(0)
	if errCode := clearTx.ClearPropertyOverride(child, "a"); errCode != types.E_NONE {
		t.Fatalf("clearTx ClearPropertyOverride failed: %v", errCode)
	}
	if errCode := valueTx.SetPropertyValue(child, "a", types.NewInt(3)); errCode != types.E_NONE {
		t.Fatalf("valueTx SetPropertyValue failed: %v", errCode)
	}

	if errCode := clearTx.Commit(); errCode != types.E_NONE {
		t.Fatalf("clearTx Commit failed: %v", errCode)
	}
	if errCode := valueTx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("valueTx Commit = %v, want E_INVARG conflict", errCode)
	}
	if !valueTx.ValidationFailed() {
		t.Fatalf("valueTx did not record validation failure")
	}
	liveValue, errCode := store.PropertyValue(child, "a")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue failed: %v", errCode)
	}
	if got := liveValue.Int(); got != 1 {
		t.Fatalf("live property value = %d, want inherited value 1", got)
	}
}

func TestTransactionDefinePropertyStagesAndPropagatesOnCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	prop := NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)
	if errCode := tx.DefineProperty(0, "a", prop); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	txRoot, ok, errCode := tx.LocalProperty(0, "a")
	if errCode != types.E_NONE || !ok {
		t.Fatalf("tx root LocalProperty ok=%v err=%v, want local property", ok, errCode)
	}
	if !txRoot.Defined || txRoot.Clear {
		t.Fatalf("tx root property defined=%v clear=%v, want defined local value", txRoot.Defined, txRoot.Clear)
	}
	txChild, ok, errCode := tx.LocalProperty(child, "a")
	if errCode != types.E_NONE || !ok {
		t.Fatalf("tx child LocalProperty ok=%v err=%v, want inherited slot", ok, errCode)
	}
	if txChild.Defined || !txChild.Clear {
		t.Fatalf("tx child property defined=%v clear=%v, want inherited clear slot", txChild.Defined, txChild.Clear)
	}
	if _, errCode := store.FindProperty(0, "a"); errCode != types.E_PROPNF {
		t.Fatalf("live FindProperty before commit = %v, want E_PROPNF", errCode)
	}

	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	rootProp, ok, errCode := store.LocalProperty(0, "a")
	if errCode != types.E_NONE || !ok {
		t.Fatalf("live root LocalProperty ok=%v err=%v, want local property", ok, errCode)
	}
	if !rootProp.Defined || rootProp.Clear {
		t.Fatalf("live root property defined=%v clear=%v, want defined local value", rootProp.Defined, rootProp.Clear)
	}
	childProp, ok, errCode := store.LocalProperty(child, "a")
	if errCode != types.E_NONE || !ok {
		t.Fatalf("live child LocalProperty ok=%v err=%v, want inherited slot", ok, errCode)
	}
	if childProp.Defined || !childProp.Clear {
		t.Fatalf("live child property defined=%v clear=%v, want inherited clear slot", childProp.Defined, childProp.Clear)
	}
}

func TestTransactionDuplicateDefinedPropertySeesStagedDefinitions(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	left, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject left failed: %v", errCode)
	}
	right, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject right failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DefineProperty(left, "foo", NewProperty(types.NewInt(1), left, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty left failed: %v", errCode)
	}
	if errCode := tx.DefineProperty(right, "FOO", NewProperty(types.NewInt(2), right, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty right failed: %v", errCode)
	}

	duplicate, errCode := tx.HasDuplicateDefinedPropertyAmong([]types.ObjID{left, right})
	if errCode != types.E_NONE {
		t.Fatalf("HasDuplicateDefinedPropertyAmong failed: %v", errCode)
	}
	if !duplicate {
		t.Fatalf("HasDuplicateDefinedPropertyAmong = false, want true")
	}
}

func TestTransactionTruthyPropertiesWithPrefixSeesStagedDefinitions(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DefineProperty(0, "protect_length", NewProperty(types.NewInt(1), 0, PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty protect_length failed: %v", errCode)
	}
	if errCode := tx.DefineProperty(0, "protect_tostr", NewProperty(types.NewInt(0), 0, PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty protect_tostr failed: %v", errCode)
	}

	flags, errCode := tx.TruthyPropertiesWithPrefixInAncestry(0, "protect_")
	if errCode != types.E_NONE {
		t.Fatalf("TruthyPropertiesWithPrefixInAncestry failed: %v", errCode)
	}
	if !flags["length"] {
		t.Fatalf("protect_length not reported truthy: %#v", flags)
	}
	if flags["tostr"] {
		t.Fatalf("protect_tostr reported truthy despite false value: %#v", flags)
	}
}

func TestTransactionDefinedPropertyConflictSeesStagedDefinitions(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject obj failed: %v", errCode)
	}
	parent, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject parent failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DefineProperty(obj, "foo", NewProperty(types.NewInt(1), obj, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty obj failed: %v", errCode)
	}
	if errCode := tx.DefineProperty(parent, "FOO", NewProperty(types.NewInt(2), parent, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty parent failed: %v", errCode)
	}

	conflict, errCode := tx.HasDefinedPropertyConflictWithAncestry(obj, []types.ObjID{parent})
	if errCode != types.E_NONE {
		t.Fatalf("HasDefinedPropertyConflictWithAncestry failed: %v", errCode)
	}
	if !conflict {
		t.Fatalf("HasDefinedPropertyConflictWithAncestry = false, want true")
	}
}

func TestTransactionChparentDescendantConflictSeesStagedDefinitions(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject child failed: %v", errCode)
	}
	parent, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject parent failed: %v", errCode)
	}
	newParent, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject newParent failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DefineProperty(child, "foo", NewProperty(types.NewInt(1), child, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty child failed: %v", errCode)
	}
	if errCode := tx.DefineProperty(newParent, "FOO", NewProperty(types.NewInt(2), newParent, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty newParent failed: %v", errCode)
	}
	if errCode := store.ChangeParents(child, []types.ObjID{parent}); errCode != types.E_NONE {
		t.Fatalf("ChangeParents child failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveRelationships(child, 0, parent); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveRelationships failed: %v", errCode)
	}

	names, errCode := tx.DefinedPropertyNamesInAncestry(newParent)
	if errCode != types.E_NONE {
		t.Fatalf("DefinedPropertyNamesInAncestry failed: %v", errCode)
	}
	conflict, errCode := tx.HasChparentDescendantPropertyConflict(parent, names)
	if errCode != types.E_NONE {
		t.Fatalf("HasChparentDescendantPropertyConflict failed: %v", errCode)
	}
	if !conflict {
		t.Fatalf("HasChparentDescendantPropertyConflict = false, want true")
	}
}

func TestTransactionReseedInheritedPropertiesUsesStagedParents(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	left, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject left failed: %v", errCode)
	}
	right, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject right failed: %v", errCode)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject child failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DefineProperty(left, "foo", NewProperty(types.NewStr("left"), left, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty left failed: %v", errCode)
	}
	if errCode := tx.DefineProperty(right, "foo", NewProperty(types.NewStr("right"), right, PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty right failed: %v", errCode)
	}

	if errCode := store.ChangeParents(child, []types.ObjID{left}); errCode != types.E_NONE {
		t.Fatalf("ChangeParents left failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveRelationships(child, 0, left); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveRelationships left failed: %v", errCode)
	}
	if errCode := tx.ReseedInheritedProperties(child); errCode != types.E_NONE {
		t.Fatalf("ReseedInheritedProperties left failed: %v", errCode)
	}
	if errCode := tx.SetPropertyValue(child, "foo", types.NewStr("override")); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue child failed: %v", errCode)
	}

	if errCode := store.ChangeParents(child, []types.ObjID{right}); errCode != types.E_NONE {
		t.Fatalf("ChangeParents right failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveRelationships(child, left, right); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveRelationships right failed: %v", errCode)
	}
	if errCode := tx.ReseedInheritedProperties(child); errCode != types.E_NONE {
		t.Fatalf("ReseedInheritedProperties right failed: %v", errCode)
	}

	prop, errCode := tx.FindProperty(child, "foo")
	if errCode != types.E_NONE {
		t.Fatalf("FindProperty child failed: %v", errCode)
	}
	if got := prop.Value.Str(); got != "right" {
		t.Fatalf("child foo = %q, want right", got)
	}
	if prop.Owner != right || prop.Perms != PropRead {
		t.Fatalf("child foo info owner=%d perms=%v, want owner=%d perms=%v", prop.Owner, prop.Perms, right, PropRead)
	}
}

func TestTransactionDefinePropertyConflictsWithConcurrentDefinition(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("tx DefineProperty failed: %v", errCode)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(2), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("live DefineProperty failed: %v", errCode)
	}

	if errCode := tx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("Commit = %v, want E_INVARG conflict", errCode)
	}
	if !tx.ValidationFailed() {
		t.Fatalf("transaction did not record validation failure")
	}
	value, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := value.Int(); got != 2 {
		t.Fatalf("property a = %d, want concurrent value 2", got)
	}
}

func TestTransactionDefinePropertyConflictsWithTopologyChange(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("tx DefineProperty failed: %v", errCode)
	}
	if _, errCode := store.CreateObject([]types.ObjID{0}, 0, false); errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	if errCode := tx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("Commit = %v, want E_INVARG conflict", errCode)
	}
	if !tx.ValidationFailed() {
		t.Fatalf("transaction did not record validation failure")
	}
	if _, errCode := store.FindProperty(0, "a"); errCode != types.E_PROPNF {
		t.Fatalf("live FindProperty after failed commit = %v, want E_PROPNF", errCode)
	}
}

func TestTransactionDeleteDefinedPropertyStagesAndRemovesInheritedOnCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DeleteDefinedProperty(0, "a"); errCode != types.E_NONE {
		t.Fatalf("DeleteDefinedProperty failed: %v", errCode)
	}
	if _, ok, errCode := tx.LocalProperty(0, "a"); errCode != types.E_NONE || ok {
		t.Fatalf("tx root LocalProperty ok=%v err=%v, want no local property", ok, errCode)
	}
	if _, errCode := tx.FindProperty(child, "a"); errCode != types.E_PROPNF {
		t.Fatalf("tx child FindProperty = %v, want E_PROPNF", errCode)
	}
	if _, errCode := store.FindProperty(child, "a"); errCode != types.E_NONE {
		t.Fatalf("live child FindProperty before commit = %v, want inherited property", errCode)
	}

	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	if _, ok, errCode := store.LocalProperty(0, "a"); errCode != types.E_NONE || ok {
		t.Fatalf("live root LocalProperty ok=%v err=%v, want no local property", ok, errCode)
	}
	if _, errCode := store.FindProperty(child, "a"); errCode != types.E_PROPNF {
		t.Fatalf("live child FindProperty after commit = %v, want E_PROPNF", errCode)
	}
}

func TestTransactionDeleteThenRedefinePropertyCommitsReplacement(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	child, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.DeleteDefinedProperty(0, "a"); errCode != types.E_NONE {
		t.Fatalf("DeleteDefinedProperty failed: %v", errCode)
	}
	if errCode := tx.DefineProperty(0, "a", NewProperty(types.NewInt(2), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("replacement DefineProperty failed: %v", errCode)
	}
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}

	for _, id := range []types.ObjID{0, child} {
		value, errCode := store.PropertyValue(id, "a")
		if errCode != types.E_NONE {
			t.Fatalf("PropertyValue #%d.a failed: %v", id, errCode)
		}
		if got := value.Int(); got != 2 {
			t.Fatalf("PropertyValue #%d.a = %d, want replacement 2", id, got)
		}
	}
}

func TestTransactionDeleteThenRedefinePropertyCommitsReplacementOnCoarsePath(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.DeleteDefinedProperty(0, "a"); errCode != types.E_NONE {
		t.Fatalf("DeleteDefinedProperty failed: %v", errCode)
	}
	if errCode := tx.DefineProperty(0, "a", NewProperty(types.NewInt(2), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("replacement DefineProperty failed: %v", errCode)
	}
	tx.MarkLiveMutated()
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	value, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := value.Int(); got != 2 {
		t.Fatalf("PropertyValue = %d, want replacement 2", got)
	}
}

func TestTransactionDeleteDefinedPropertyConflictsWithConcurrentPropertyWrite(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.DeleteDefinedProperty(0, "a"); errCode != types.E_NONE {
		t.Fatalf("DeleteDefinedProperty failed: %v", errCode)
	}
	if errCode := store.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}

	if errCode := tx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("Commit = %v, want E_INVARG conflict", errCode)
	}
	if !tx.ValidationFailed() {
		t.Fatalf("transaction did not record validation failure")
	}
	value, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := value.Int(); got != 2 {
		t.Fatalf("property a = %d, want concurrent value 2", got)
	}
}

func TestTransactionCommitPreservesHistoricalReads(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	reader := store.BeginReadOnly(0)
	writer := store.BeginReadOnly(0)
	if errCode := writer.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("writer SetPropertyValue failed: %v", errCode)
	}
	if errCode := writer.Commit(); errCode != types.E_NONE {
		t.Fatalf("writer Commit failed: %v", errCode)
	}

	prop, errCode := reader.FindProperty(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("reader FindProperty failed: %v", errCode)
	}
	if got := prop.Value.Int(); got != 1 {
		t.Fatalf("reader property value after commit = %d, want historical value 1", got)
	}

	live, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue failed: %v", errCode)
	}
	if got := live.Int(); got != 2 {
		t.Fatalf("live property value after commit = %d, want 2", got)
	}
}

func TestTransactionDisjointObjectScalarWritesBothCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	childA, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject childA failed: %v", errCode)
	}
	childB, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject childB failed: %v", errCode)
	}

	txA := store.BeginReadOnly(0)
	txB := store.BeginReadOnly(0)
	if errCode := txA.SetObjectName(childA, "alpha"); errCode != types.E_NONE {
		t.Fatalf("txA SetObjectName failed: %v", errCode)
	}
	if errCode := txB.SetObjectFlag(childB, FlagRead, true); errCode != types.E_NONE {
		t.Fatalf("txB SetObjectFlag failed: %v", errCode)
	}

	if errCode := txA.Commit(); errCode != types.E_NONE {
		t.Fatalf("txA Commit failed: %v", errCode)
	}
	if errCode := txB.Commit(); errCode != types.E_NONE {
		t.Fatalf("txB Commit failed: %v", errCode)
	}

	name, errCode := store.ObjectName(childA)
	if errCode != types.E_NONE {
		t.Fatalf("ObjectName childA failed: %v", errCode)
	}
	if name != "alpha" {
		t.Fatalf("childA name = %q, want alpha", name)
	}
	hasRead, errCode := store.HasObjectFlag(childB, FlagRead)
	if errCode != types.E_NONE {
		t.Fatalf("HasObjectFlag childB failed: %v", errCode)
	}
	if !hasRead {
		t.Fatalf("childB read flag = false, want true")
	}
}

func TestTransactionSameObjectScalarWriteConflicts(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	first := store.BeginReadOnly(0)
	second := store.BeginReadOnly(0)
	if errCode := first.SetObjectName(0, "first"); errCode != types.E_NONE {
		t.Fatalf("first SetObjectName failed: %v", errCode)
	}
	if errCode := second.SetObjectFlag(0, FlagRead, true); errCode != types.E_NONE {
		t.Fatalf("second SetObjectFlag failed: %v", errCode)
	}

	if errCode := first.Commit(); errCode != types.E_NONE {
		t.Fatalf("first Commit failed: %v", errCode)
	}
	if errCode := second.Commit(); errCode != types.E_INVARG {
		t.Fatalf("second Commit = %v, want E_INVARG conflict", errCode)
	}
	if !second.ValidationFailed() {
		t.Fatalf("second transaction did not record validation failure")
	}

	name, errCode := store.ObjectName(0)
	if errCode != types.E_NONE {
		t.Fatalf("ObjectName failed: %v", errCode)
	}
	if name != "first" {
		t.Fatalf("name = %q, want first", name)
	}
	hasRead, errCode := store.HasObjectFlag(0, FlagRead)
	if errCode != types.E_NONE {
		t.Fatalf("HasObjectFlag failed: %v", errCode)
	}
	if hasRead {
		t.Fatalf("read flag = true, want false after conflicting write")
	}
}

func TestTransactionAdoptLiveObjectSeesCreatedObject(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	tx := store.BeginReadOnly(0)
	obj, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveObject(obj); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveObject failed: %v", errCode)
	}
	if errCode := tx.SetObjectOwner(obj, obj); errCode != types.E_NONE {
		t.Fatalf("SetObjectOwner on adopted object failed: %v", errCode)
	}
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	owner, errCode := store.ObjectOwner(obj)
	if errCode != types.E_NONE {
		t.Fatalf("ObjectOwner failed: %v", errCode)
	}
	if owner != obj {
		t.Fatalf("owner = #%d, want #%d", owner, obj)
	}
}

func TestTransactionObjectLocationStagesUntilCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.SetObjectLocationRaw(obj, 0); errCode != types.E_NONE {
		t.Fatalf("SetObjectLocationRaw failed: %v", errCode)
	}
	txLocation, errCode := tx.Location(obj)
	if errCode != types.E_NONE {
		t.Fatalf("tx Location failed: %v", errCode)
	}
	if txLocation != 0 {
		t.Fatalf("tx location = %d, want 0", txLocation)
	}
	liveLocation, errCode := store.Location(obj)
	if errCode != types.E_NONE {
		t.Fatalf("live Location failed: %v", errCode)
	}
	if liveLocation != types.ObjNothing {
		t.Fatalf("live location before commit = %d, want #-1", liveLocation)
	}

	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	liveLocation, errCode = store.Location(obj)
	if errCode != types.E_NONE {
		t.Fatalf("live Location after commit failed: %v", errCode)
	}
	if liveLocation != 0 {
		t.Fatalf("live location after commit = %d, want 0", liveLocation)
	}
}

func TestTransactionObjectLocationConflicts(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject obj failed: %v", errCode)
	}
	other, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject other failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.SetObjectLocationRaw(obj, 0); errCode != types.E_NONE {
		t.Fatalf("tx SetObjectLocationRaw failed: %v", errCode)
	}
	if errCode := store.SetObjectLocationRaw(obj, other); errCode != types.E_NONE {
		t.Fatalf("live SetObjectLocationRaw failed: %v", errCode)
	}

	if errCode := tx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("Commit = %v, want E_INVARG conflict", errCode)
	}
	if !tx.ValidationFailed() {
		t.Fatalf("transaction did not record validation failure")
	}
	liveLocation, errCode := store.Location(obj)
	if errCode != types.E_NONE {
		t.Fatalf("live Location failed: %v", errCode)
	}
	if liveLocation != other {
		t.Fatalf("live location = %d, want concurrent location %d", liveLocation, other)
	}
}

func TestTransactionScalarAndPropertyWritesSameObjectBothCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	scalarTx := store.BeginReadOnly(0)
	propertyTx := store.BeginReadOnly(0)
	if errCode := scalarTx.SetObjectName(0, "renamed"); errCode != types.E_NONE {
		t.Fatalf("scalarTx SetObjectName failed: %v", errCode)
	}
	if errCode := propertyTx.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("propertyTx SetPropertyValue failed: %v", errCode)
	}

	if errCode := scalarTx.Commit(); errCode != types.E_NONE {
		t.Fatalf("scalarTx Commit failed: %v", errCode)
	}
	if errCode := propertyTx.Commit(); errCode != types.E_NONE {
		t.Fatalf("propertyTx Commit failed: %v", errCode)
	}
	if propertyTx.ValidationFailed() {
		t.Fatalf("successful property transaction recorded validation failure")
	}

	name, errCode := store.ObjectName(0)
	if errCode != types.E_NONE {
		t.Fatalf("ObjectName failed: %v", errCode)
	}
	if name != "renamed" {
		t.Fatalf("name = %q, want renamed", name)
	}
	value, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := value.Int(); got != 2 {
		t.Fatalf("property a = %d, want 2", got)
	}
}

func TestTransactionFindVerbTracksReadAndScan(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if _, _, err := tx.FindVerb(0, "look"); err != nil {
		t.Fatalf("FindVerb failed: %v", err)
	}

	live := store.load(0)
	verb := live.verbs["look"]
	if got := tx.verbReads[verbReadKey{objID: 0, name: "look"}]; got != verb.version {
		t.Fatalf("verb read version = %d, want %d", got, verb.version)
	}
	if got := tx.verbScans[0]; got != live.verbVersion {
		t.Fatalf("verb scan version = %d, want %d", got, live.verbVersion)
	}
}

func TestTransactionVerbByIndexTracksReadAndScan(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if _, errCode := tx.VerbByIndex(0, 0); errCode != types.E_NONE {
		t.Fatalf("VerbByIndex failed: %v", errCode)
	}

	live := store.load(0)
	verb := live.verbs["look"]
	if got := tx.verbReads[verbReadKey{objID: 0, name: "look"}]; got != verb.version {
		t.Fatalf("verb read version = %d, want %d", got, verb.version)
	}
	if got := tx.verbScans[0]; got != live.verbVersion {
		t.Fatalf("verb scan version = %d, want %d", got, live.verbVersion)
	}
}

func TestTransactionVerbReadInvalidatesCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if _, _, err := tx.FindVerb(0, "look"); err != nil {
		t.Fatalf("FindVerb failed: %v", err)
	}
	if errCode := store.SetVerbCode(0, "look", []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("SetVerbCode failed: %v", errCode)
	}
	if errCode := tx.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("tx SetPropertyValue failed: %v", errCode)
	}

	if errCode := tx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("tx Commit = %v, want E_INVARG conflict", errCode)
	}
	if !tx.ValidationFailed() {
		t.Fatalf("transaction did not record validation failure")
	}
	value, errCode := store.PropertyValue(0, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue failed: %v", errCode)
	}
	if got := value.Int(); got != 1 {
		t.Fatalf("property a = %d, want unchanged value 1", got)
	}
}

func TestTransactionSetVerbCodeStagesUntilCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.SetVerbCode(0, "look", []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("tx SetVerbCode failed: %v", errCode)
	}

	txVerb, _, err := tx.FindVerb(0, "look")
	if err != nil {
		t.Fatalf("tx FindVerb failed: %v", err)
	}
	if len(txVerb.Code) != 1 || txVerb.Code[0] != "return 2;" {
		t.Fatalf("tx verb code = %#v, want staged code", txVerb.Code)
	}
	liveVerb, _, err := store.FindVerb(0, "look")
	if err != nil {
		t.Fatalf("live FindVerb failed: %v", err)
	}
	if len(liveVerb.Code) != 1 || liveVerb.Code[0] != "return 1;" {
		t.Fatalf("live verb code before commit = %#v, want original code", liveVerb.Code)
	}

	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("tx Commit failed: %v", errCode)
	}
	liveVerb, _, err = store.FindVerb(0, "look")
	if err != nil {
		t.Fatalf("live FindVerb after commit failed: %v", err)
	}
	if len(liveVerb.Code) != 1 || liveVerb.Code[0] != "return 2;" {
		t.Fatalf("live verb code after commit = %#v, want staged code", liveVerb.Code)
	}
}

func TestTransactionAdoptLiveVerbsSeesAddedVerb(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	tx := store.BeginReadOnly(0)
	names, errCode := tx.VerbNames(0)
	if errCode != types.E_NONE {
		t.Fatalf("tx VerbNames failed: %v", errCode)
	}
	if len(names) != 0 {
		t.Fatalf("tx VerbNames before add = %#v, want none", names)
	}
	verb := NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)
	if _, errCode := store.AddVerb(0, verb); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveVerbs(0); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs failed: %v", errCode)
	}
	if errCode := tx.SetVerbCode(0, "look", []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("tx SetVerbCode failed: %v", errCode)
	}
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit failed: %v", errCode)
	}
	verbView, _, err := store.FindVerb(0, "look")
	if err != nil {
		t.Fatalf("FindVerb failed: %v", err)
	}
	if len(verbView.Code) != 1 || verbView.Code[0] != "return 2;" {
		t.Fatalf("verb code = %#v, want return 2", verbView.Code)
	}
}

func TestTransactionAdoptLiveVerbsPreservesStagedCode(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	tx := store.BeginReadOnly(0)
	if _, errCode := store.AddVerb(0, NewVerb("first", []string{"first"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb first failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveVerbs(0); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs first failed: %v", errCode)
	}
	if errCode := tx.SetVerbCode(0, "first", []string{"return 1;"}); errCode != types.E_NONE {
		t.Fatalf("tx SetVerbCode first failed: %v", errCode)
	}
	if _, errCode := store.AddVerb(0, NewVerb("second", []string{"second"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb second failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveVerbs(0); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs second failed: %v", errCode)
	}

	verbView, _, err := tx.FindVerb(0, "first")
	if err != nil {
		t.Fatalf("FindVerb first failed: %v", err)
	}
	if len(verbView.Code) != 1 || verbView.Code[0] != "return 1;" {
		t.Fatalf("first staged code = %#v, want return 1", verbView.Code)
	}
}

func TestTransactionLiveMutationDoesNotRebaseUnrelatedReads(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	other, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	if errCode := store.DefineProperty(0, "read", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty read failed: %v", errCode)
	}
	if errCode := store.DefineProperty(0, "write", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty write failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if _, errCode := tx.PropertyValue(0, "read"); errCode != types.E_NONE {
		t.Fatalf("PropertyValue read failed: %v", errCode)
	}

	if _, errCode := store.AddVerb(other, NewVerb("live", []string{"live"}, 0, VerbRead|VerbExecute, VerbArgs{}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}
	tx.MarkLiveMutated()
	if errCode := tx.AdoptLiveVerbs(other); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs failed: %v", errCode)
	}

	concurrent := store.BeginReadOnly(0)
	if errCode := concurrent.SetPropertyValue(0, "read", types.NewInt(1)); errCode != types.E_NONE {
		t.Fatalf("concurrent SetPropertyValue failed: %v", errCode)
	}
	if errCode := concurrent.Commit(); errCode != types.E_NONE {
		t.Fatalf("concurrent Commit failed: %v", errCode)
	}
	concurrent.Release()

	if errCode := tx.SetPropertyValue(0, "write", types.NewInt(1)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue write failed: %v", errCode)
	}
	if errCode := tx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("Commit = %v, want E_INVARG conflict", errCode)
	}
	if !tx.ValidationFailed() {
		t.Fatal("transaction did not record validation failure")
	}
	value, errCode := store.PropertyValue(0, "write")
	if errCode != types.E_NONE {
		t.Fatalf("live PropertyValue write failed: %v", errCode)
	}
	if got := value.Int(); got != 0 {
		t.Fatalf("write property = %d, want unchanged 0", got)
	}
}

func TestTransactionForgetObjectDropsStagedVerbCode(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	tx := store.BeginReadOnly(0)
	if _, errCode := store.AddVerb(0, NewVerb("scratch", []string{"scratch"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, nil)); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}
	if errCode := tx.AdoptLiveVerbs(0); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs failed: %v", errCode)
	}
	if errCode := tx.SetVerbCode(0, "scratch", []string{"return 1;"}); errCode != types.E_NONE {
		t.Fatalf("tx SetVerbCode failed: %v", errCode)
	}
	if err := store.Recycle(0); err != nil {
		t.Fatalf("Recycle failed: %v", err)
	}
	tx.ForgetObject(0)
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit after ForgetObject failed: %v", errCode)
	}
}

func TestTransactionSetVerbCodeByIndexStagesUntilCommit(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.SetVerbCodeByIndex(0, 0, []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("tx SetVerbCodeByIndex failed: %v", errCode)
	}

	txVerb, errCode := tx.VerbByIndex(0, 0)
	if errCode != types.E_NONE {
		t.Fatalf("tx VerbByIndex failed: %v", errCode)
	}
	if len(txVerb.Code) != 1 || txVerb.Code[0] != "return 2;" {
		t.Fatalf("tx verb code = %#v, want staged code", txVerb.Code)
	}
	liveVerb, errCode := store.VerbByIndex(0, 0)
	if errCode != types.E_NONE {
		t.Fatalf("live VerbByIndex failed: %v", errCode)
	}
	if len(liveVerb.Code) != 1 || liveVerb.Code[0] != "return 1;" {
		t.Fatalf("live verb code before commit = %#v, want original code", liveVerb.Code)
	}

	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("tx Commit failed: %v", errCode)
	}
	liveVerb, errCode = store.VerbByIndex(0, 0)
	if errCode != types.E_NONE {
		t.Fatalf("live VerbByIndex after commit failed: %v", errCode)
	}
	if len(liveVerb.Code) != 1 || liveVerb.Code[0] != "return 2;" {
		t.Fatalf("live verb code after commit = %#v, want staged code", liveVerb.Code)
	}
}

func TestTransactionSetVerbCodeConflicts(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if _, errCode := store.AddVerb(0, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})); errCode != types.E_NONE {
		t.Fatalf("AddVerb failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.SetVerbCode(0, "look", []string{"return 2;"}); errCode != types.E_NONE {
		t.Fatalf("tx SetVerbCode failed: %v", errCode)
	}
	if errCode := store.SetVerbCode(0, "look", []string{"return 3;"}); errCode != types.E_NONE {
		t.Fatalf("live SetVerbCode failed: %v", errCode)
	}

	if errCode := tx.Commit(); errCode != types.E_INVARG {
		t.Fatalf("tx Commit = %v, want E_INVARG conflict", errCode)
	}
	if !tx.ValidationFailed() {
		t.Fatalf("transaction did not record validation failure")
	}
	liveVerb, _, err := store.FindVerb(0, "look")
	if err != nil {
		t.Fatalf("live FindVerb failed: %v", err)
	}
	if len(liveVerb.Code) != 1 || liveVerb.Code[0] != "return 3;" {
		t.Fatalf("live verb code after conflict = %#v, want concurrent code", liveVerb.Code)
	}
}

func TestTransactionPropertyValuesSeeStagedWrites(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	tx := store.BeginReadOnly(0)
	if errCode := tx.SetPropertyValue(0, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue failed: %v", errCode)
	}
	values, errCode := tx.PropertyValues(0)
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValues failed: %v", errCode)
	}
	if len(values) != 1 {
		t.Fatalf("len(PropertyValues) = %d, want 1", len(values))
	}
	if got := values[0].Int(); got != 2 {
		t.Fatalf("PropertyValues[0] = %d, want 2", got)
	}
}

// TestTransactionAdoptAndCommitAnonymousObject is the regression guard for the F2
// merge gap: a runtime-created anonymous object lives out-of-band in s.anonObjects
// (no numbered slot, no history). The MVCC read-transaction resolvers were only
// numbered-aware, so create($object, 1) (-> tx.AdoptLiveObject on the anon id) and
// any commit whose write footprint targets the anon returned E_INVIND, cascading to
// 95 conformance failures. This test exercises both the adoption resolver and the
// commit apply path (coarse routing via writeFootprintHasAnon).
func TestTransactionAdoptAndCommitAnonymousObject(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add #0: %v", err)
	}
	// Inheritable property the anon will write through its parent #0.
	if errCode := store.DefineProperty(0, "a", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty #0.a: %v", errCode)
	}

	// Runtime-created anonymous object: lands in s.anonObjects, not the numbered map.
	anon, ec := store.CreateObject([]types.ObjID{0}, 0, true /*anonymous*/)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject anonymous: %v", ec)
	}

	tx := store.BeginReadOnly(0)

	// (1) PRIMARY: adoption of a freshly-created anon must succeed (was E_INVIND).
	if errCode := tx.AdoptLiveObject(anon); errCode != types.E_NONE {
		t.Fatalf("AdoptLiveObject(anon) = %v, want E_NONE", errCode)
	}

	// (2) The anon round-trips valid + anonymous through the txn.
	if !tx.Valid(anon) {
		t.Fatalf("tx.Valid(anon) = false, want true")
	}
	isAnon, errCode := tx.ObjectIsAnonymous(anon)
	if errCode != types.E_NONE {
		t.Fatalf("tx.ObjectIsAnonymous(anon) err = %v", errCode)
	}
	if !isAnon {
		t.Fatalf("tx.ObjectIsAnonymous(anon) = false, want true")
	}

	// (3) SIBLING COVERAGE: stage a property-value write on the anon, then commit.
	// This exercises the commit apply path; the anon write footprint must route to
	// the coarse exclusive path (writeFootprintHasAnon) instead of commitDecentralized,
	// which has no slot for the anon id and previously returned E_INVIND.
	if errCode := tx.SetPropertyValue(anon, "a", types.NewInt(2)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue(anon, a) = %v, want E_NONE", errCode)
	}
	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit() = %v, want E_NONE", errCode)
	}

	// (4) The committed value is readable from a fresh read transaction.
	tx2 := store.BeginReadOnly(0)
	value, errCode := tx2.PropertyValue(anon, "a")
	if errCode != types.E_NONE {
		t.Fatalf("PropertyValue(anon, a) after commit = %v, want E_NONE", errCode)
	}
	if got := value.Int(); got != 2 {
		t.Fatalf("PropertyValue(anon, a) after commit = %d, want 2", got)
	}
}
