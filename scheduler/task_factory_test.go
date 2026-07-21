package scheduler

import (
	"testing"

	"barn/builtins"
	dbstore "barn/db/store"
	"barn/types"
	"barn/vm"
)

func TestConfigureVMStackLimitReadsLiveServerOption(t *testing.T) {
	builtins.LoadServerOptionsFromStore(nil)
	t.Cleanup(func() { builtins.LoadServerOptionsFromStore(nil) })

	store := dbstore.NewStore()
	system := dbstore.NewObjectBuilder(0)
	system.SetProperty("server_options", dbstore.NewProperty(
		types.NewObj(1), 0, dbstore.PropRead|dbstore.PropWrite, false, true,
	))
	if err := store.Add(system.Build()); err != nil {
		t.Fatalf("add system object: %v", err)
	}
	options := dbstore.NewObjectBuilder(1)
	options.SetProperty("max_stack_depth", dbstore.NewProperty(
		types.NewInt(60), 0, dbstore.PropRead, false, true,
	))
	if err := store.Add(options.Build()); err != nil {
		t.Fatalf("add server options object: %v", err)
	}

	machine := vm.NewVM(store, builtins.NewRegistry())
	configureVMStackLimit(machine)
	if machine.MaxStackDepth != 60 {
		t.Fatalf("VM max stack depth = %d, want live $server_options value 60", machine.MaxStackDepth)
	}
}
