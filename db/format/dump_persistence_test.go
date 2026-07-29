package format

import (
	"barn/db/store"
	"barn/types"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTripPreservesLastMove(t *testing.T) {
	loaded, err := LoadDatabase(filepath.Join("..", "..", "Test_fresh2.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	objectStore := loaded.NewStoreFromDatabase()
	const movedObject = types.ObjID(0)
	source, locationErr := objectStore.Location(movedObject)
	if locationErr != types.E_NONE {
		t.Fatalf("Location failed: %v", locationErr)
	}
	const destination = types.ObjID(1)
	before := time.Now().Unix()
	if errCode := objectStore.MoveObject(movedObject, destination, 0); errCode != types.E_NONE {
		t.Fatalf("MoveObject failed: %v", errCode)
	}
	after := time.Now().Unix()

	tmpFile, err := os.CreateTemp(t.TempDir(), "last-move-*.db")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer tmpFile.Close()

	if err := NewWriter(tmpFile, objectStore.Snapshot()).WriteDatabase(); err != nil {
		t.Fatalf("WriteDatabase failed: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reloaded, err := LoadDatabase(tmpFile.Name())
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	lastMove, errCode := reloaded.NewStoreFromDatabase().LastMove(movedObject)
	if errCode != types.E_NONE {
		t.Fatalf("LastMove failed: %v", errCode)
	}
	gotSource, ok := lastMove.MapGet(types.NewStr("source"))
	if !ok || gotSource.Type() != types.TYPE_OBJ || gotSource.Obj() != source {
		t.Fatalf("last_move source = %v, want #%d", gotSource, source)
	}
	gotTime, ok := lastMove.MapGet(types.NewStr("time"))
	if !ok || gotTime.Type() != types.TYPE_INT || gotTime.Int() < before || gotTime.Int() > after {
		t.Fatalf("last_move time = %v, want in [%d, %d]", gotTime, before, after)
	}
}

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
	if errCode := objectStore.DefineProperty(1, propName, store.NewProperty(types.NewStr("base"), 3, 0, false, true)); errCode != types.E_NONE {
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
	if got := childProp.Value.Str(); childProp.Value.Type() != types.TYPE_STR || got != "child-override" {
		t.Fatalf("child override = %q, want child-override", got)
	}
}

func TestRoundTripPreservesAnonymousInheritedOverride(t *testing.T) {
	loaded, err := LoadDatabase(filepath.Join("..", "..", "Test_fresh2.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	objectStore := loaded.NewStoreFromDatabase()
	const classID = types.ObjID(0)
	if ec := objectStore.DefineProperty(classID, "anon_marker", store.NewProperty(types.NewStr("class-marker"), 3, 0, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty anon_marker: %v", ec)
	}
	if ec := objectStore.DefineProperty(classID, "saved_anon", store.NewProperty(types.NewInt(0), 3, 0, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty saved_anon: %v", ec)
	}

	anonID, ec := objectStore.CreateObject([]types.ObjID{classID}, 3, true)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject anonymous: %v", ec)
	}
	if ec := objectStore.SetPropertyValue(anonID, "anon_marker", types.NewStr("anonymous-marker")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue anonymous override: %v", ec)
	}
	if ec := objectStore.SetPropertyValue(classID, "saved_anon", types.NewAnon(anonID)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue saved_anon: %v", ec)
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "anonymous-override-*.db")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer tmpFile.Close()

	if err := NewWriter(tmpFile, objectStore.Snapshot()).WriteDatabase(); err != nil {
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
	anonRef, ok, propErr := reloadedStore.LocalProperty(classID, "saved_anon")
	if propErr != types.E_NONE || !ok {
		t.Fatalf("reloaded saved_anon missing: ok=%v err=%v", ok, propErr)
	}
	if anonRef.Value.Type() != types.TYPE_ANON || !reloadedStore.Valid(anonRef.Value.ID()) {
		t.Fatalf("reloaded saved_anon = %v, want valid anonymous object", anonRef.Value)
	}

	override, ok, propErr := reloadedStore.LocalProperty(anonRef.Value.ID(), "anon_marker")
	if propErr != types.E_NONE || !ok {
		t.Fatalf("reloaded anonymous override missing: ok=%v err=%v", ok, propErr)
	}
	if override.Clear || override.Value.Type() != types.TYPE_STR || override.Value.Str() != "anonymous-marker" {
		t.Fatalf("anonymous override = %#v, want anonymous-marker", override)
	}
}

// TestRoundTripPreservesSiblingAfterClear checks that clearing an inherited
// property override on an object does not corrupt sibling property values in
// the checkpoint. Before the fix, ClearPropertyOverride removed the slot from
// obj.properties but left it in obj.propOrder; snapshotPropertyNames then
// skipped the orphaned propOrder entry, shifting all subsequent property values
// and corrupting unrelated properties (e.g. password) on reload.
//
// The test uses two checkpoint cycles: the first cycle establishes propOrder on
// the child (matching what happens at server load from a real DB), and the
// second cycle exercises the clear+checkpoint path.
func TestRoundTripPreservesSiblingAfterClear(t *testing.T) {
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

	// Define two properties on the parent; both propagate cleared inherited
	// slots to the child's properties map (but not yet to its propOrder).
	if ec := objectStore.DefineProperty(1, "conn_state", store.NewProperty(types.NewStr("active"), 3, 0, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty conn_state: %v", ec)
	}
	if ec := objectStore.DefineProperty(1, "sentinel", store.NewProperty(types.NewStr("base"), 3, 0, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty sentinel: %v", ec)
	}

	// Give the child local overrides for both, then checkpoint+reload so that
	// propOrder is populated on the child (as it would be in a real DB load).
	if ec := objectStore.SetPropertyValue(0, "conn_state", types.NewStr("connected")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue conn_state: %v", ec)
	}
	if ec := objectStore.SetPropertyValue(0, "sentinel", types.NewStr("expected-value")); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue sentinel: %v", ec)
	}

	round1, err := os.CreateTemp(t.TempDir(), "clear-persist-round1-*.db")
	if err != nil {
		t.Fatalf("CreateTemp round1: %v", err)
	}
	defer round1.Close()
	if err := NewWriter(round1, objectStore.Snapshot()).WriteDatabase(); err != nil {
		t.Fatalf("WriteDatabase round1: %v", err)
	}
	if err := round1.Close(); err != nil {
		t.Fatalf("Close round1: %v", err)
	}

	// Reload: now child's propOrder includes both properties as loaded from DB.
	reloaded1, err := LoadDatabase(round1.Name())
	if err != nil {
		t.Fatalf("Reload round1: %v", err)
	}
	objectStore = reloaded1.NewStoreFromDatabase()

	// Simulate disconnect: clear conn_state. This removes it from obj.properties
	// but leaves it in obj.propOrder.
	if ec := objectStore.ClearPropertyOverride(0, "conn_state"); ec != types.E_NONE {
		t.Fatalf("ClearPropertyOverride: %v", ec)
	}

	// Second checkpoint: this is where the bug triggered (9 missing properties
	// caused all subsequent property values to shift on reload).
	round2, err := os.CreateTemp(t.TempDir(), "clear-persist-round2-*.db")
	if err != nil {
		t.Fatalf("CreateTemp round2: %v", err)
	}
	defer round2.Close()
	if err := NewWriter(round2, objectStore.Snapshot()).WriteDatabase(); err != nil {
		t.Fatalf("WriteDatabase round2: %v", err)
	}
	if err := round2.Close(); err != nil {
		t.Fatalf("Close round2: %v", err)
	}

	reloaded2, err := LoadDatabase(round2.Name())
	if err != nil {
		t.Fatalf("Reload round2: %v", err)
	}

	finalStore := reloaded2.NewStoreFromDatabase()
	prop, ok, _ := finalStore.LocalProperty(0, "sentinel")
	if !ok {
		t.Fatalf("reloaded child missing sentinel after clear+checkpoint")
	}
	if got := prop.Value.Str(); got != "expected-value" {
		t.Fatalf("sentinel = %q after clear+roundtrip, want expected-value (property values shifted by missing cleared slot)", got)
	}
}
