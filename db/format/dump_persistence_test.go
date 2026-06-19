package format

import (
	"barn/db/store"
	"barn/types"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripPreservesRuntimeAddedInheritedOverride(t *testing.T) {
	loaded, err := LoadDatabase(filepath.Join("..", "..", "Test_fresh2.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	objectStore := loaded.NewStoreFromDatabase()
	parent := objectStore.Get(1)
	child := objectStore.Get(0)
	if parent == nil || child == nil {
		t.Fatalf("missing parent=%v child=%v", parent, child)
	}

	propName := "persist_prop"
	parentPropVal := store.NewProperty(propName, types.NewStr("base"), 3, 0, false, true)
	parent.Properties[propName] = &parentPropVal
	pos := parent.PropDefsCount
	if pos > len(parent.PropOrder) {
		pos = len(parent.PropOrder)
	}
	parent.PropOrder = append(parent.PropOrder, "")
	copy(parent.PropOrder[pos+1:], parent.PropOrder[pos:])
	parent.PropOrder[pos] = propName
	parent.PropDefsCount++

	parentView := parentPropVal.View()
	childPropVal := store.NewProperty(propName, types.NewStr("child-override"), parentView.Owner, parentView.Perms, false, false)
	child.Properties[propName] = &childPropVal

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

	parentProp, ok := reloadedParent.Properties[propName]
	if !ok {
		t.Fatalf("reloaded parent missing %q; propdefs=%d order=%v", propName, reloadedParent.PropDefsCount, reloadedParent.PropOrder)
	}
	if parentProp.View().Clear {
		t.Fatalf("reloaded parent %q unexpectedly clear", propName)
	}

	childProp, ok := reloadedChild.Properties[propName]
	if !ok {
		t.Fatalf("reloaded child missing %q; propdefs=%d order=%v", propName, reloadedChild.PropDefsCount, reloadedChild.PropOrder)
	}
	childView := childProp.View()
	if childView.Clear {
		t.Fatalf("reloaded child %q unexpectedly clear", propName)
	}
	if got := childView.Value.(types.StrValue).Value(); got != "child-override" {
		t.Fatalf("child override = %q, want child-override", got)
	}
}
