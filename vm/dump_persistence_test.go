package vm

import (
	"os"
	"path/filepath"
	"testing"

	dbformat "barn/db/format"
	dbstore "barn/db/store"
	"barn/kernel"
	"barn/types"
)

func TestEvalRoundTripPreservesRuntimeAddedInheritedOverride(t *testing.T) {
	loaded, err := dbformat.LoadDatabase(filepath.Join("..", "Test_fresh2.db"))
	if err != nil {
		t.Fatalf("LoadDatabase failed: %v", err)
	}

	store := loaded.NewStoreFromDatabase()
	ctx := kernel.NewTaskContext()
	ctx.Player = 3
	ctx.Programmer = 3
	ctx.IsWizard = true

	setup := `try delete_property(#1, "persist_prop"); except (ANY) endtry; add_property(#1, "persist_prop", "base", {#3, ""}); #0.persist_prop = "child-override"; return #0.persist_prop;`
	result := runBytecodeProgram(t, setup, store, ctx)
	if result.Flow == types.FlowException {
		t.Fatalf("setup failed: %v", result.Error)
	}
	if got := result.Val.Str(); got != "child-override" {
		t.Fatalf("setup returned %q, want child-override", got)
	}

	tmpFile, err := os.CreateTemp(t.TempDir(), "eval-dump-persist-*.db")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	defer tmpFile.Close()

	writer := dbformat.NewWriter(tmpFile, store.Snapshot())
	if err := writer.WriteDatabase(); err != nil {
		t.Fatalf("WriteDatabase failed: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reloaded, err := dbformat.LoadDatabase(tmpFile.Name())
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	reloadedStore := reloaded.NewStoreFromDatabase()
	if _, ok := reloadedStore.Get(0); !ok {
		t.Fatal("reloaded child missing")
	}

	prop, ok, _ := reloadedStore.LocalProperty(0, "persist_prop")
	if !ok {
		t.Fatalf("reloaded child missing persist_prop")
	}
	if prop.Clear {
		t.Fatalf("reloaded child persist_prop unexpectedly clear")
	}
	if got := prop.Value.Str(); got != "child-override" {
		t.Fatalf("reloaded child override = %q, want child-override", got)
	}
}

func TestRuntimePendingWaifCollectorRoundTripsNestedAnonymousObject(t *testing.T) {
	store := dbstore.NewStore()
	class := dbstore.NewObjectBuilder(0)
	class.SetOwner(0)
	if err := store.Add(class.Build()); err != nil {
		t.Fatalf("add WAIF class: %v", err)
	}
	if errCode := store.DefineProperty(0, ":anon", dbstore.NewProperty(types.None, 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define WAIF property: %v", errCode)
	}
	anonID, errCode := store.CreateObject([]types.ObjID{0}, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create nested anonymous object: %v", errCode)
	}
	waif := types.NewWaif(0, 0)
	waif.SetProperty("anon", types.NewAnon(anonID))
	exec := NewVM(store, nil)
	exec.PendingFinalizations = []types.Value{waif}
	pending := CollectPendingFinalizationValues(store, exec)
	if len(pending) != 1 || !pending[0].Equal(waif) {
		t.Fatalf("runtime pending roots = %v, want exactly WAIF identity %p", pending, waif.WaifIdentity())
	}
	store.AppendPendingFinalizations(pending)

	path := filepath.Join(t.TempDir(), "runtime-pending-waif.db")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if err := dbformat.NewWriter(file, store.Snapshot()).WriteDatabase(); err != nil {
		_ = file.Close()
		t.Fatalf("write checkpoint: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close checkpoint: %v", err)
	}
	reloaded, err := dbformat.LoadDatabase(path)
	if err != nil {
		t.Fatalf("reload checkpoint: %v", err)
	}
	if len(reloaded.PendingFinalizations) != 1 || reloaded.PendingFinalizations[0].Type() != types.TYPE_WAIF {
		t.Fatalf("reloaded pending roots = %v, want one WAIF", reloaded.PendingFinalizations)
	}
	nested, ok := reloaded.PendingFinalizations[0].GetProperty("anon")
	if !ok || nested.Type() != types.TYPE_ANON || !reloaded.NewStoreFromDatabase().Valid(nested.ID()) {
		t.Fatalf("reloaded nested anonymous ref = %v, ok=%v, want valid anonymous object", nested, ok)
	}
}
