package store

import (
	"barn/types"
	"testing"
)

func TestStoreBasics(t *testing.T) {
	store := NewStore()

	// Test initial state
	if store.MaxObject() != -1 {
		t.Errorf("MaxObject() = %d, want -1", store.MaxObject())
	}

	if store.NextID() != 0 {
		t.Errorf("NextID() = %d, want 0", store.NextID())
	}

	// Add an object
	obj := NewObject(0, 0)
	if err := store.Add(obj); err != nil {
		t.Fatalf("Add() failed: %v", err)
	}

	// Check max object updated
	if store.MaxObject() != 0 {
		t.Errorf("MaxObject() = %d, want 0", store.MaxObject())
	}

	if store.NextID() != 1 {
		t.Errorf("NextID() = %d, want 1", store.NextID())
	}

	// Get object
	retrieved, ok := store.Get(0)
	if !ok {
		t.Fatal("Get(0) returned nil")
	}
	if retrieved.ID != 0 {
		t.Errorf("Retrieved object ID = %d, want 0", retrieved.ID)
	}
}

func TestStoreValid(t *testing.T) {
	store := NewStore()

	// Negative IDs are sentinels
	if store.Valid(-1) {
		t.Error("Valid(-1) = true, want false (sentinel)")
	}

	if store.Valid(-2) {
		t.Error("Valid(-2) = true, want false (sentinel)")
	}

	// Non-existent object
	if store.Valid(99) {
		t.Error("Valid(99) = true, want false (doesn't exist)")
	}

	// Add object
	obj := NewObject(0, 0)
	store.Add(obj)

	if !store.Valid(0) {
		t.Error("Valid(0) = false, want true (exists)")
	}

	// Recycle object
	store.Recycle(0)

	if store.Valid(0) {
		t.Error("Valid(0) = true, want false (recycled)")
	}
}

func TestStoreRecycle(t *testing.T) {
	store := NewStore()

	obj := NewObject(0, 0)
	store.Add(obj)

	// Recycle
	if err := store.Recycle(0); err != nil {
		t.Fatalf("Recycle() failed: %v", err)
	}

	// Check recycled
	if _, ok := store.Get(0); ok {
		t.Error("Get(0) returned object after recycle, want nil")
	}

	// Check flags set
	unsafe, ok := store.GetUnsafe(0)
	if !ok {
		t.Fatal("GetUnsafe(0) returned no object after recycle")
	}
	if !unsafe.Flags.Has(FlagRecycled) {
		t.Error("FlagRecycled not set")
	}
	if !unsafe.Flags.Has(FlagInvalid) {
		t.Error("FlagInvalid not set")
	}

	// Can't recycle twice
	if err := store.Recycle(0); err == nil {
		t.Error("Recycle() succeeded on already recycled object, want error")
	}
}

func TestStoreMaxObjectAfterRecycle(t *testing.T) {
	store := NewStore()

	// Create objects #0, #1, #2
	store.Add(NewObject(0, 0))
	store.Add(NewObject(1, 0))
	store.Add(NewObject(2, 0))

	if store.MaxObject() != 2 {
		t.Errorf("MaxObject() = %d, want 2", store.MaxObject())
	}

	// Recycle #1
	store.Recycle(1)

	// MaxObject should still be 2 (high-water mark)
	if store.MaxObject() != 2 {
		t.Errorf("MaxObject() = %d, want 2 (high-water mark)", store.MaxObject())
	}

	// NextID should be 3 (sequential allocation)
	if store.NextID() != 3 {
		t.Errorf("NextID() = %d, want 3", store.NextID())
	}
}

func TestRecreateWithNothingParentReusesRecycledSlot(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, -1)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	obj, errCode := store.CreateObject([]types.ObjID{types.ObjNothing}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	if err := store.Recycle(obj); err != nil {
		t.Fatalf("Recycle failed: %v", err)
	}
	if err := store.Recreate(obj, types.ObjNothing, 0); err != nil {
		t.Fatalf("Recreate failed: %v", err)
	}
	if !store.Valid(obj) {
		t.Fatalf("recreated object is not valid")
	}
	if parent, errCode := store.Parent(obj); errCode != types.E_NONE || parent != types.ObjNothing {
		t.Fatalf("Parent = %d, %v; want #-1, E_NONE", parent, errCode)
	}
	if next := store.LowestFreeID(); next == obj {
		t.Fatalf("LowestFreeID still returns recreated object %d", obj)
	}
}

func TestStorePendingFinalizationsSnapshot(t *testing.T) {
	store := NewStore()

	loaded := []types.Value{types.NewAnon(10)}
	store.SetPendingFinalizations(loaded)
	loaded[0] = types.NewAnon(11)

	store.AppendPendingFinalizations([]types.Value{
		types.NewAnon(10),
		types.NewAnon(12),
	})

	snapshot := store.Snapshot()
	if got, want := len(snapshot.PendingFinalizations), 2; got != want {
		t.Fatalf("len(PendingFinalizations) = %d, want %d", got, want)
	}
	if got, want := snapshot.PendingFinalizations[0].String(), types.NewAnon(10).String(); got != want {
		t.Errorf("PendingFinalizations[0] = %s, want %s", got, want)
	}
	if got, want := snapshot.PendingFinalizations[1].String(), types.NewAnon(12).String(); got != want {
		t.Errorf("PendingFinalizations[1] = %s, want %s", got, want)
	}

	snapshot.PendingFinalizations[0] = types.NewAnon(99)
	nextSnapshot := store.Snapshot()
	if got, want := nextSnapshot.PendingFinalizations[0].String(), types.NewAnon(10).String(); got != want {
		t.Errorf("mutating snapshot changed store: got %s, want %s", got, want)
	}
}

func TestNewObject(t *testing.T) {
	obj := NewObject(5, 10)

	if obj.id != 5 {
		t.Errorf("ID = %d, want 5", obj.id)
	}

	if obj.owner != 10 {
		t.Errorf("Owner = %d, want 10", obj.owner)
	}

	if obj.location != types.ObjNothing {
		t.Errorf("Location = %d, want %d (nothing)", obj.location, types.ObjNothing)
	}

	if len(obj.properties) != 0 {
		t.Errorf("Properties len = %d, want 0", len(obj.properties))
	}

	if len(obj.verbs) != 0 {
		t.Errorf("Verbs len = %d, want 0", len(obj.verbs))
	}

	// Check default flags (not readable or writable per MOO semantics)
	if obj.flags.Has(FlagRead) {
		t.Error("FlagRead should not be set by default")
	}
	if obj.flags.Has(FlagWrite) {
		t.Error("FlagWrite should not be set by default")
	}
}

func TestObjectFlags(t *testing.T) {
	var flags ObjectFlags = 0

	// Set flags
	flags = flags.Set(FlagUser)
	if !flags.Has(FlagUser) {
		t.Error("FlagUser not set")
	}

	flags = flags.Set(FlagProgrammer)
	if !flags.Has(FlagProgrammer) {
		t.Error("FlagProgrammer not set")
	}

	// Clear flag
	flags = flags.Clear(FlagUser)
	if flags.Has(FlagUser) {
		t.Error("FlagUser still set after clear")
	}

	// Programmer should still be set
	if !flags.Has(FlagProgrammer) {
		t.Error("FlagProgrammer cleared incorrectly")
	}
}

func TestPropertyPermsString(t *testing.T) {
	tests := []struct {
		perms PropertyPerms
		want  string
	}{
		{0, ""},
		{PropRead, "r"},
		{PropWrite, "w"},
		{PropChown, "c"},
		{PropRead | PropWrite, "rw"},
		{PropRead | PropWrite | PropChown, "rwc"},
		{PropWrite | PropChown, "wc"},
	}

	for _, tt := range tests {
		got := tt.perms.String()
		if got != tt.want {
			t.Errorf("PropertyPerms(%d).String() = %q, want %q", tt.perms, got, tt.want)
		}
	}
}

func TestPropertyNamesAreCaseInsensitiveForLookup(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add() failed: %v", err)
	}

	if errCode := store.DefineProperty(0, NewProperty("CaseProbe", types.NewInt(42), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty() failed: %v", errCode)
	}

	prop, errCode := store.FindProperty(0, "caseprobe")
	if errCode != types.E_NONE {
		t.Fatalf("FindProperty() failed: %v", errCode)
	}
	if prop.Name != "CaseProbe" {
		t.Fatalf("FindProperty() name = %q, want CaseProbe", prop.Name)
	}
	if got := prop.Value.Int(); prop.Value.Type() != types.TYPE_INT || got != 42 {
		t.Fatalf("FindProperty() value = %d, want 42", got)
	}

	if _, ok, errCode := store.LocalProperty(0, "CASEPROBE"); errCode != types.E_NONE || !ok {
		t.Fatalf("LocalProperty() ok=%v err=%v, want ok", ok, errCode)
	}

	if errCode := store.SetPropertyValue(0, "CASEPROBE", types.NewInt(99)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue() failed: %v", errCode)
	}
	prop, errCode = store.FindProperty(0, "caseprobe")
	if errCode != types.E_NONE {
		t.Fatalf("FindProperty() after set failed: %v", errCode)
	}
	if got := prop.Value.Int(); prop.Value.Type() != types.TYPE_INT || got != 99 {
		t.Fatalf("SetPropertyValue() value = %d, want 99", got)
	}

	perms := PropRead
	if errCode := store.SetPropertyInfo(0, "caseprobe", nil, &perms); errCode != types.E_NONE {
		t.Fatalf("SetPropertyInfo() failed: %v", errCode)
	}
	prop, errCode = store.FindProperty(0, "CASEPROBE")
	if errCode != types.E_NONE {
		t.Fatalf("FindProperty() after info failed: %v", errCode)
	}
	if prop.Perms != PropRead {
		t.Fatalf("SetPropertyInfo() perms = %v, want %v", prop.Perms, PropRead)
	}

	if errCode := store.DefineProperty(0, NewProperty("caseprobe", types.NewInt(1), 0, PropRead, false, true)); errCode != types.E_INVARG {
		t.Fatalf("DefineProperty() duplicate = %v, want E_INVARG", errCode)
	}

	if errCode := store.DeleteDefinedProperty(0, "CASEPROBE"); errCode != types.E_NONE {
		t.Fatalf("DeleteDefinedProperty() failed: %v", errCode)
	}
	if _, errCode := store.FindProperty(0, "caseprobe"); errCode != types.E_PROPNF {
		t.Fatalf("FindProperty() after delete = %v, want E_PROPNF", errCode)
	}
}
