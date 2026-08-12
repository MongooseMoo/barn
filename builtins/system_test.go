package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestLoadServerOptionsDoesNotPublishStagedValuesAfterFailedCommit(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetName("Root")
	root.SetOwner(0)
	root.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	root.SetProperty("server_options", dbstore.NewProperty(types.NewObj(1), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	root.SetProperty("conflict_marker", dbstore.NewProperty(types.NewStr("old"), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatalf("store.Add root failed: %v", err)
	}
	options := dbstore.NewObjectBuilder(1)
	options.SetName("Server Options")
	options.SetOwner(0)
	options.SetFlags(dbstore.FlagRead | dbstore.FlagWrite | dbstore.FlagWizard)
	options.SetProperty("max_string_concat", dbstore.NewProperty(types.NewInt(3000), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(options.Build()); err != nil {
		t.Fatalf("store.Add options failed: %v", err)
	}

	ctx := newTestExecution()
	ctx.Registry.LoadServerOptionsFromStore(store)
	ctx.Registry.LoadProtectedBuiltinsFromStore(store)
	if got := ctx.Registry.GetMaxStringConcat(); got != 3000 {
		t.Fatalf("initial max_string_concat cache = %d, want 3000", got)
	}
	if ctx.Registry.IsProtectedBuiltin("create") {
		t.Fatalf("create should not start protected")
	}

	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	ctx.IsWizard = true
	if _, errCode := ctx.StoreTxn.PropertyValue(0, "conflict_marker"); errCode != types.E_NONE {
		t.Fatalf("PropertyValue conflict_marker failed: %s", errCode)
	}
	if errCode := ctx.StoreTxn.SetPropertyValue(1, "max_string_concat", types.NewInt(4000)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue max_string_concat failed: %s", errCode)
	}
	if errCode := ctx.StoreTxn.DefineProperty(1, "protect_create", dbstore.NewProperty(types.NewInt(1), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty protect_create failed: %s", errCode)
	}

	result := builtinLoadServerOptions(ctx, nil)
	if result.Flow != types.FlowNormal {
		t.Fatalf("load_server_options result = flow %v err %v, want return", result.Flow, result.Error)
	}

	if errCode := store.SetPropertyValue(0, "conflict_marker", types.NewStr("live")); errCode != types.E_NONE {
		t.Fatalf("live SetPropertyValue conflict_marker failed: %s", errCode)
	}
	if errCode := ctx.StoreTxn.SetPropertyValue(0, "conflict_marker", types.NewStr("task")); errCode != types.E_NONE {
		t.Fatalf("txn SetPropertyValue conflict_marker failed: %s", errCode)
	}
	if errCode := ctx.StoreTxn.Commit(); errCode != types.E_INVARG {
		t.Fatalf("Commit = %s, want E_INVARG conflict", errCode)
	}

	if got := ctx.Registry.GetMaxStringConcat(); got != 3000 {
		t.Fatalf("max_string_concat cache after failed commit = %d, want committed 3000", got)
	}
	if ctx.Registry.IsProtectedBuiltin("create") {
		t.Fatalf("create protected flag leaked from failed transaction")
	}
}
