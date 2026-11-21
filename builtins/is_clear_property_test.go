package builtins

import (
	"testing"

	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

func clearPropertyFixture(t testing.TB) *Execution {
	t.Helper()
	store := dbstore.NewStore()
	parent := dbstore.NewObjectBuilder(0)
	parent.SetProperty("visible", dbstore.NewProperty(types.NewInt(5), 0, dbstore.PropRead, false, true))
	parent.SetProperty("private", dbstore.NewProperty(types.NewInt(5), 0, 0, false, true))
	if err := store.Add(parent.Build()); err != nil {
		t.Fatal(err)
	}
	child := dbstore.NewObjectBuilder(1)
	child.SetParents([]types.ObjID{0})
	if err := store.Add(child.Build()); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(dbstore.NewObject(2, 0)); err != nil {
		t.Fatal(err)
	}
	if err := store.Recycle(2); err != nil {
		t.Fatal(err)
	}
	ctx := newTestExecution()
	ctx.Programmer = 1
	ctx.Store = store
	ctx.StoreTxn = store.BeginSnapshot(0)
	t.Cleanup(ctx.StoreTxn.Release)
	return ctx
}

func TestIsClearPropertyLookup(t *testing.T) {
	ctx := clearPropertyFixture(t)
	for _, tc := range []struct {
		obj  types.ObjID
		name string
		want int64
		err  types.ErrorCode
	}{
		{0, "visible", 0, types.E_NONE},
		{1, "visible", 1, types.E_NONE},
		{1, "private", 0, types.E_PERM},
		{1, "absent", 0, types.E_PROPNF},
		{1, "owner", 0, types.E_NONE},
		{2, "visible", 0, types.E_INVARG},
		{99, "owner", 0, types.E_INVARG},
		{99, "visible", 0, types.E_INVARG},
	} {
		got := builtinIsClearProperty(ctx, []types.Value{types.NewObj(tc.obj), types.NewStr(tc.name)})
		if got.Error != tc.err || (tc.err == types.E_NONE && got.Val.Int() != tc.want) {
			t.Errorf("is_clear_property(#%d, %q) = %v, want %d, %s", tc.obj, tc.name, got, tc.want, tc.err)
		}
	}
	args := []types.Value{types.NewObj(1), types.NewStr("visible")}
	if ec := ctx.StoreTxn.SetPropertyValue(1, "visible", types.NewInt(7)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if got := builtinIsClearProperty(ctx, args); got.Error != types.E_NONE || got.Val.Int() != 0 {
		t.Fatalf("staged override: %v", got)
	}
	if ec := ctx.StoreTxn.ClearPropertyOverride(1, "visible"); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if got := builtinIsClearProperty(ctx, args); got.Error != types.E_NONE || got.Val.Int() != 1 {
		t.Fatalf("staged clear: %v", got)
	}
}

func BenchmarkIsClearPropertyLookup(b *testing.B) {
	for _, tc := range []struct {
		label string
		obj   types.ObjID
		prop  string
	}{
		{"defined", 0, "visible"},
		{"inherited", 1, "visible"},
		{"builtin", 1, "owner"},
	} {
		b.Run(tc.label, func(b *testing.B) {
			ctx := clearPropertyFixture(b)
			args := []types.Value{types.NewObj(tc.obj), types.NewStr(tc.prop)}
			for b.Loop() {
				builtinIsClearProperty(ctx, args)
			}
		})
	}
}
