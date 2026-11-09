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
	ctx.StoreTxn = store.BeginSnapshot(0)
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
		if result := admitTestBuiltin("load_server_options", ctx, nil); result.Flow != types.FlowNormal {
			t.Fatalf("reload: %s", result.Error)
		}
	}
	list := types.NewList([]types.Value{types.NewStr(strings.Repeat("x", 3500))})
	set("max_list_value_bytes", 1000000)
	set("protect_create", 1)
	reload()
	if err := ctx.Session.CheckListLimitForTask(ctx.Context, list); err != types.E_NONE {
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
	if err := ctx.Session.CheckListLimitForTask(ctx.Context, list); err != types.E_QUOTA {
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

func newOutputCapacityExecution(t *testing.T, value types.Value) *Execution {
	t.Helper()
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetProperty("server_options", dbstore.NewProperty(types.NewObj(1), 0, dbstore.PropRead, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatal(err)
	}
	options := dbstore.NewObjectBuilder(1)
	if !value.IsNone() {
		options.SetProperty("max_queued_output", dbstore.NewProperty(value, 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	}
	if err := store.Add(options.Build()); err != nil {
		t.Fatal(err)
	}
	ctx := newTestExecution()
	ctx.Store = store
	ctx.StoreTxn = store.DirectTxn()
	ctx.IsWizard = true
	return ctx
}

func assertOutputCapacity(t *testing.T, ctx *Execution, want int64) {
	t.Helper()
	result := admitTestBuiltin("buffered_output_length", ctx, nil)
	if !result.IsNormal() || result.Val.Type() != types.TYPE_INT || result.Val.Int() != want {
		t.Fatalf("buffered_output_length() = %+v, want %d", result, want)
	}
}

func TestBufferedOutputCapacityCanonicalization(t *testing.T) {
	assertOutputCapacity(t, newTestExecution(), 65536)
	for _, test := range []struct {
		name  string
		value types.Value
		want  int64
	}{
		{"missing", types.None, 65536},
		{"wrong type", types.NewStr("invalid"), 65536},
		{"small positive", types.NewInt(1), 2048},
		{"minimum", types.NewInt(2048), 2048},
		{"configured", types.NewInt(4096), 4096},
		{"zero", types.NewInt(0), 0},
		{"negative", types.NewInt(-1), -1},
		{"signed maximum", types.NewInt(2147483647), 2147483647},
		{"signed wrap", types.NewInt(2147483648), -2147483648},
		{"wrap then minimum", types.NewInt(4294967297), 2048},
		{"wrap to zero", types.NewInt(4294967296), 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := newOutputCapacityExecution(t, test.value)
			ctx.Session.LoadServerOptionsFromStore(ctx.Store)
			assertOutputCapacity(t, ctx, test.want)
		})
	}
}

func TestBufferedOutputCapacityReloadIsTaskLocalUntilCommit(t *testing.T) {
	ctx := newOutputCapacityExecution(t, types.NewInt(3000))
	ctx.Session.LoadServerOptionsFromStore(ctx.Store)
	ctx.StoreTxn = ctx.Store.BeginSnapshot(0)
	t.Cleanup(ctx.StoreTxn.Release)
	peer := newTestExecutionForSession(ctx.Session)
	peer.Store = ctx.Store
	peer.StoreTxn = ctx.Store.DirectTxn()
	assertOutputCapacity(t, ctx, 3000)
	assertOutputCapacity(t, peer, 3000)
	for _, value := range []int64{4096, 8192, 3000, 7000} {
		if err := ctx.StoreTxn.SetPropertyValue(1, "max_queued_output", types.NewInt(value)); err != types.E_NONE {
			t.Fatalf("set capacity: %s", err)
		}
		if value == 3000 && ctx.StoreTxn.HasWrites() {
			t.Fatal("restoring original capacity did not elide the write")
		}
		if result := admitTestBuiltin("load_server_options", ctx, nil); !result.IsNormal() {
			t.Fatalf("reload: %+v", result)
		}
		assertOutputCapacity(t, ctx, value)
		assertOutputCapacity(t, peer, 3000)
	}
	if err := ctx.StoreTxn.Commit(); err != types.E_NONE {
		t.Fatalf("commit capacity: %s", err)
	}
	FlushPendingEffects(ctx)
	assertOutputCapacity(t, ctx, 7000)
	assertOutputCapacity(t, peer, 7000)
}
