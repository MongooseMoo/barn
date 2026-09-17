package builtins

import (
	"strings"
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func TestLoadServerOptionsAfterElidedWrites(t *testing.T) {
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetProperty("server_options", dbstore.NewProperty(types.NewObj(1), 0, dbstore.PropRead, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatal(err)
	}
	options := dbstore.NewObjectBuilder(1)
	options.SetProperty("max_list_value_bytes", dbstore.NewProperty(types.NewInt(3000), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	options.SetProperty("protect_create", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	if err := store.Add(options.Build()); err != nil {
		t.Fatal(err)
	}
	ctx := newTestExecution()
	ctx.Store = store
	ctx.StoreTxn = store.BeginReadOnly(0)
	ctx.IsWizard = true
	ctx.Session.LoadServerOptionsFromStore(store)
	ctx.Session.LoadProtectedBuiltinsFromStore(store)

	set := func(name string, value int64) {
		t.Helper()
		if err := ctx.StoreTxn.SetPropertyValue(1, name, types.NewInt(value)); err != types.E_NONE {
			t.Fatalf("set %s: %s", name, err)
		}
	}
	reload := func() {
		t.Helper()
		if result := builtinLoadServerOptions(ctx, nil); result.Flow != types.FlowNormal {
			t.Fatalf("reload: %s", result.Error)
		}
	}
	list := types.NewList([]types.Value{types.NewStr(strings.Repeat("x", 3500))})
	set("max_list_value_bytes", 1000000)
	set("protect_create", 1)
	reload()
	if err := ctx.Session.CheckListLimitForTask(ctx.TaskContext, list); err != types.E_NONE {
		t.Fatalf("first reload did not raise the task limit: %s", err)
	}
	if ctx.Session.GetMaxListValueBytes() != 3000 || ctx.Session.IsProtectedBuiltin("create") {
		t.Fatal("uncommitted options leaked into the session")
	}

	// Returning to the committed values removes every staged write. The last
	// reload must still supersede the first, both in this task and at commit.
	set("max_list_value_bytes", 3000)
	set("protect_create", 0)
	if ctx.StoreTxn.HasWrites() {
		t.Fatal("round-trip writes were not elided")
	}
	reload()
	if err := ctx.Session.CheckListLimitForTask(ctx.TaskContext, list); err != types.E_QUOTA {
		t.Errorf("restored task limit: got %s, want E_QUOTA", err)
	}
	if err := ctx.StoreTxn.Commit(); err != types.E_NONE {
		t.Fatalf("commit: %s", err)
	}
	FlushPendingEffects(ctx)
	if got := ctx.Session.GetMaxListValueBytes(); got != 3000 {
		t.Errorf("committed list limit = %d, want 3000", got)
	}
	if ctx.Session.IsProtectedBuiltin("create") {
		t.Error("an earlier reload overwrote the restored protected flag")
	}
}
