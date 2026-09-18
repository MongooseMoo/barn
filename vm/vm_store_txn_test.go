package vm

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/kernel"
)

func TestEnsureContextDependenciesUsesDirectTransaction(t *testing.T) {
	store := dbstore.NewStore()
	machine := NewVM(store, nil)
	machine.Context = kernel.NewTaskContext()

	machine.ensureContextDependencies()

	if machine.Context.StoreTxn == nil {
		t.Fatal("VM context has a nil StoreTxn")
	}
	if machine.Context.StoreTxn != store.DirectTxn() {
		t.Fatal("VM context did not receive the store's direct transaction")
	}
}
