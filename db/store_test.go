package db

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
	retrieved := store.Get(0)
	if retrieved == nil {
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
	retrieved := store.Get(0)
	if retrieved != nil {
		t.Error("Get(0) returned object after recycle, want nil")
	}

	// Check flags set
	unsafe := store.GetUnsafe(0)
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

func TestNewObject(t *testing.T) {
	obj := NewObject(5, 10)

	if obj.ID != 5 {
		t.Errorf("ID = %d, want 5", obj.ID)
	}

	if obj.Owner != 10 {
		t.Errorf("Owner = %d, want 10", obj.Owner)
	}

	if obj.Location != types.ObjNothing {
		t.Errorf("Location = %d, want %d (nothing)", obj.Location, types.ObjNothing)
	}

	if len(obj.Properties) != 0 {
		t.Errorf("Properties len = %d, want 0", len(obj.Properties))
	}

	if len(obj.Verbs) != 0 {
		t.Errorf("Verbs len = %d, want 0", len(obj.Verbs))
	}

	// Check default flags (readable + writable)
	if !obj.Flags.Has(FlagRead) {
		t.Error("FlagRead not set by default")
	}
	if !obj.Flags.Has(FlagWrite) {
		t.Error("FlagWrite not set by default")
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
