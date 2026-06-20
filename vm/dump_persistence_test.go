package vm

import (
	"os"
	"path/filepath"
	"testing"

	dbformat "barn/db/format"
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
	if got := result.Val.(types.StrValue).Value(); got != "child-override" {
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
	if got := prop.Value.(types.StrValue).Value(); got != "child-override" {
		t.Fatalf("reloaded child override = %q, want child-override", got)
	}
}
