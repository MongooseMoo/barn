package server

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	dbformat "barn/db/format"
	dbstore "barn/db/store"
	"barn/types"
	"barn/vm"
)

func pendingFinalizationVM(store *dbstore.Store, value types.Value) *vm.VM {
	exec := vm.NewVM(store, nil)
	exec.Frames = []*vm.StackFrame{{Locals: []types.Value{value}}}
	return exec
}

func TestPendingFinalizationSinkReducesRootsAcrossShutdownVMs(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetOwner(0)
	root.SetLocation(types.ObjNothing)
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("add root: %v", err)
	}

	anonA, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous A: %v", errCode)
	}
	anonB, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create anonymous B: %v", errCode)
	}
	separate, errCode := store.CreateObject(nil, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("create separate anonymous object: %v", errCode)
	}
	if errCode := store.DefineProperty(anonA, "next", dbstore.NewProperty(types.NewAnon(anonB), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define A.next: %v", errCode)
	}
	if errCode := store.DefineProperty(anonB, "next", dbstore.NewProperty(types.NewAnon(anonA), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define B.next: %v", errCode)
	}

	server := &Server{store: store}
	var pendingByVM [][]types.Value
	for _, exec := range []*vm.VM{
		pendingFinalizationVM(store, types.NewAnon(anonA)),
		pendingFinalizationVM(store, types.NewAnon(anonB)),
		pendingFinalizationVM(store, types.NewAnon(separate)),
	} {
		roots := vm.CollectPendingFinalizationValues(store, exec)
		if len(roots) != 1 {
			t.Fatalf("per-VM pending roots = %v, want exactly one", roots)
		}
		pendingByVM = append(pendingByVM, roots)
	}

	start := make(chan struct{})
	var writers sync.WaitGroup
	for _, roots := range pendingByVM {
		roots := roots
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			server.store.AppendPendingFinalizations(roots)
		}()
	}
	close(start)
	writers.Wait()

	snapshot := server.store.Snapshot()
	if got := len(snapshot.PendingFinalizations); got != 2 {
		t.Fatalf("pending roots = %v, want one cycle root plus one distinct root", snapshot.PendingFinalizations)
	}
	if got := len(snapshot.AnonymousObjects); got != 3 {
		t.Fatalf("serialized anonymous objects = %d, want complete cycle plus distinct object", got)
	}

	path := filepath.Join(t.TempDir(), "pending-finalizations.db")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if err := dbformat.NewWriter(file, snapshot).WriteDatabase(); err != nil {
		file.Close()
		t.Fatalf("write checkpoint: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close checkpoint: %v", err)
	}
	reloaded, err := dbformat.LoadDatabase(path)
	if err != nil {
		t.Fatalf("reload checkpoint: %v", err)
	}
	if got := len(reloaded.PendingFinalizations); got != 2 {
		t.Fatalf("reloaded pending roots = %v, want one cycle root plus one distinct root", reloaded.PendingFinalizations)
	}
	if got := len(reloaded.AnonymousObjs); got != 3 {
		t.Fatalf("reloaded anonymous objects = %d, want complete cycle plus distinct object", got)
	}
}
