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
	_, parentOK := objectStore.Get(1)
	_, childOK := objectStore.Get(0)
	if !parentOK || !childOK {
		t.Fatalf("missing parent=%v child=%v", parentOK, childOK)
	}

	propName := "persist_prop"
	// Add the property on the parent at runtime (propagates a clear inherited slot
	// to the child), then give the child a non-clear local override.
	if errCode := objectStore.DefineProperty(1, store.NewProperty(propName, types.NewStr("base"), 3, 0, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty(parent) failed: %v", errCode)
	}
	if errCode := objectStore.SetPropertyValue(0, propName, types.NewStr("child-override")); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue(child) failed: %v", errCode)
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
	if _, ok := reloadedStore.Get(1); !ok {
		t.Fatalf("reloaded parent missing")
	}
	if _, ok := reloadedStore.Get(0); !ok {
		t.Fatalf("reloaded child missing")
	}

	parentProp, ok, _ := reloadedStore.LocalProperty(1, propName)
	if !ok {
		t.Fatalf("reloaded parent missing %q", propName)
	}
	if parentProp.Clear {
		t.Fatalf("reloaded parent %q unexpectedly clear", propName)
	}

	childProp, ok, _ := reloadedStore.LocalProperty(0, propName)
	if !ok {
		t.Fatalf("reloaded child missing %q", propName)
	}
	if childProp.Clear {
		t.Fatalf("reloaded child %q unexpectedly clear", propName)
	}
	if got := childProp.Value.(types.StrValue).Value(); got != "child-override" {
		t.Fatalf("child override = %q, want child-override", got)
	}
}
