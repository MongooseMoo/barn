package vm

import (
	"testing"

	dbstore "barn/db/store"
	"barn/types"
)

func testObject(id types.ObjID, anonymous bool) *dbstore.Object {
	flags := dbstore.FlagRead
	if anonymous {
		flags = flags.Set(dbstore.FlagAnonymous)
	}
	b := dbstore.NewObjectBuilder(id)
	b.SetOwner(0)
	b.SetFlags(flags)
	b.SetAnonymous(anonymous)
	return b.Build()
}

func TestCollectPendingFinalizationValuesCapturesUnreachableAnonymousRefs(t *testing.T) {
	store := dbstore.NewStore()

	root := testObject(0, false)
	anon := testObject(4, true)

	if err := store.Add(root); err != nil {
		t.Fatalf("add root: %v", err)
	}
	if err := store.Add(anon); err != nil {
		t.Fatalf("add anon: %v", err)
	}

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{
		{
			Locals: []types.Value{
				types.NewList([]types.Value{types.NewInt(1), types.NewAnon(4)}),
			},
		},
	}
	exec.Stack = []types.Value{types.NewMap([][2]types.Value{
		{types.NewStr("x"), types.NewAnon(4)},
	})}
	exec.SP = 1

	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].String() != types.NewAnon(4).String() {
		t.Fatalf("got[0] = %s, want %s", got[0].String(), types.NewAnon(4).String())
	}
}

func TestCollectPendingFinalizationValuesSkipsPersistentAnonymousRefs(t *testing.T) {
	store := dbstore.NewStore()

	root := testObject(0, false)
	holder := testObject(4, false)
	anon := testObject(5, true)

	for _, obj := range []*dbstore.Object{root, holder, anon} {
		if err := store.Add(obj); err != nil {
			t.Fatalf("add object: %v", err)
		}
	}

	store.DefineProperty(4, dbstore.NewProperty("two", types.NewMap([][2]types.Value{{types.NewStr("foo"), types.NewAnon(5)}}), 0, dbstore.PropRead, false, false))
	store.DefineProperty(0, dbstore.NewProperty("one", types.NewObj(4), 0, dbstore.PropRead, false, false))
	store.DefineProperty(5, dbstore.NewProperty("foo", types.NewAnon(5), 0, dbstore.PropRead, false, false))

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{
		{
			Locals: []types.Value{
				types.NewList([]types.Value{types.NewAnon(5)}),
			},
		},
	}

	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

func TestCollectPendingFinalizationValuesSkipsBareAnonymousLocals(t *testing.T) {
	store := dbstore.NewStore()

	root := testObject(0, false)
	anonA := testObject(4, true)
	anonB := testObject(5, true)

	for _, obj := range []*dbstore.Object{root, anonA, anonB} {
		if err := store.Add(obj); err != nil {
			t.Fatalf("add object: %v", err)
		}
	}

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{
		{
			Locals: []types.Value{
				types.NewAnon(4),
				types.NewAnon(5),
			},
		},
	}

	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}
