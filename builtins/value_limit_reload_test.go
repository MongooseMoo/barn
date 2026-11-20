package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// newValueLimitReloadExecution returns a wizard execution whose session has
// cached generous committed value limits, and whose task has then lowered
// max_list_value_bytes and max_map_value_bytes to 1100 and called
// load_server_options() without committing.
func newValueLimitReloadExecution(t *testing.T) *Execution {
	t.Helper()
	store := dbstore.NewStore()
	root := dbstore.NewObjectBuilder(0)
	root.SetProperty("server_options", dbstore.NewProperty(types.NewObj(1), 0, dbstore.PropRead, false, true))
	if err := store.Add(root.Build()); err != nil {
		t.Fatal(err)
	}
	options := dbstore.NewObjectBuilder(1)
	for _, name := range []string{"max_list_value_bytes", "max_map_value_bytes"} {
		options.SetProperty(name, dbstore.NewProperty(types.NewInt(1000000), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
	}
	if err := store.Add(options.Build()); err != nil {
		t.Fatal(err)
	}
	ctx := newTestExecution()
	ctx.Store = store
	ctx.IsWizard = true
	ctx.Session.LoadServerOptionsFromStore(store)
	ctx.StoreTxn = store.BeginSnapshot(0)
	t.Cleanup(ctx.StoreTxn.Release)
	for _, name := range []string{"max_list_value_bytes", "max_map_value_bytes"} {
		if err := ctx.StoreTxn.SetPropertyValue(1, name, types.NewInt(1100)); err != types.E_NONE {
			t.Fatalf("set %s: %s", name, err)
		}
	}
	if result := admitTestBuiltin("load_server_options", ctx, nil); !result.IsNormal() {
		t.Fatalf("load_server_options: %+v", result)
	}
	return ctx
}

func intRange(n int) []types.Value {
	values := make([]types.Value, n)
	for i := range values {
		values[i] = types.NewInt(int64(i + 1))
	}
	return values
}

func assertQuota(t *testing.T, call string, result types.Result) {
	t.Helper()
	if result.Flow != types.FlowException || result.Error != types.E_QUOTA {
		t.Fatalf("%s: flow %v error %s, want E_QUOTA", call, result.Flow, result.Error)
	}
}

// Toast list.cc bf_setremove checks the result against the cached
// max_list_value_bytes, which load_server_options() has just refreshed in
// the same task (conformance server/limits.yaml
// setremove_fails_if_result_too_large).
func TestSetremoveUsesSameTaskReloadedListLimit(t *testing.T) {
	ctx := newValueLimitReloadExecution(t)
	list := types.NewList(intRange(1000))
	assertQuota(t, "setremove(list, 1)", admitTestBuiltin("setremove", ctx, []types.Value{list, types.NewInt(1)}))
}

// Toast list.cc bf_setremove checks the returned list even when the value
// is absent and the list comes back unchanged.
func TestSetremoveAbsentValueStillChecksListLimit(t *testing.T) {
	ctx := newValueLimitReloadExecution(t)
	list := types.NewList(intRange(1000))
	assertQuota(t, `setremove(list, "absent")`, admitTestBuiltin("setremove", ctx, []types.Value{list, types.NewStr("absent")}))
}

func TestMapmergeUsesSameTaskReloadedMapLimit(t *testing.T) {
	ctx := newValueLimitReloadExecution(t)
	left := types.NewEmptyMap()
	right := types.NewEmptyMap()
	for i, value := range intRange(200) {
		if i%2 == 0 {
			left = left.MapSet(value, value)
		} else {
			right = right.MapSet(value, value)
		}
	}
	assertQuota(t, "mapmerge(left, right)", admitTestBuiltin("mapmerge", ctx, []types.Value{left, right}))
}
