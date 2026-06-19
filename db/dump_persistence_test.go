package db

import (
	"barn/db/store"
	"barn/types"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripPreservesRuntimeAddedInheritedOverride(t *testing.T) {
	loaded, err := LoadDatabase(filepath.Join("..", "Test_fresh2.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	objectStore := loaded.NewStoreFromDatabase()
	parent := objectStore.Get(1)
	child := objectStore.Get(0)
	if parent == nil || child == nil {
		t.Fatalf("missing parent=%v child=%v", parent, child)
	}

	prop := &store.Property{
		Name:    "persist_prop",
		Value:   types.NewStr("base"),
		Owner:   3,
		Perms:   0,
		Clear:   false,
		Defined: true,
	}
	parent.Properties[prop.Name] = prop
	pos := parent.PropDefsCount
	if pos > len(parent.PropOrder) {
		pos = len(parent.PropOrder)
	}
	parent.PropOrder = append(parent.PropOrder, "")
	copy(parent.PropOrder[pos+1:], parent.PropOrder[pos:])
	parent.PropOrder[pos] = prop.Name
	parent.PropDefsCount++

	child.Properties[prop.Name] = &store.Property{
		Name:    prop.Name,
		Value:   types.NewStr("child-override"),
		Owner:   prop.Owner,
		Perms:   prop.Perms,
		Clear:   false,
		Defined: false,
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "dump-persist-*.db")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer tmpFile.Close()

	writer := NewWriter(tmpFile, objectStore.Snapshot())
	if err := writer.WriteDatabase(); err != nil {
		t.Fatalf("WriteDatabase failed: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reloaded, err := LoadDatabase(tmpFile.Name())
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	reloadedStore := reloaded.NewStoreFromDatabase()
	reloadedParent := reloadedStore.Get(1)
	reloadedChild := reloadedStore.Get(0)
	if reloadedParent == nil || reloadedChild == nil {
		t.Fatalf("reloaded parent=%v child=%v", reloadedParent, reloadedChild)
	}

	parentProp, ok := reloadedParent.Properties[prop.Name]
	if !ok {
		t.Fatalf("reloaded parent missing %q; propdefs=%d order=%v", prop.Name, reloadedParent.PropDefsCount, reloadedParent.PropOrder)
	}
	if parentProp.Clear {
		t.Fatalf("reloaded parent %q unexpectedly clear", prop.Name)
	}

	childProp, ok := reloadedChild.Properties[prop.Name]
	if !ok {
		t.Fatalf("reloaded child missing %q; propdefs=%d order=%v", prop.Name, reloadedChild.PropDefsCount, reloadedChild.PropOrder)
	}
	if childProp.Clear {
		t.Fatalf("reloaded child %q unexpectedly clear", prop.Name)
	}
	if got := childProp.Value.(types.StrValue).Value(); got != "child-override" {
		t.Fatalf("child override = %q, want child-override", got)
	}
}
