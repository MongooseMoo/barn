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

	store.DefineProperty(4, "two", dbstore.NewProperty(types.NewMap([][2]types.Value{{types.NewStr("foo"), types.NewAnon(5)}}), 0, dbstore.PropRead, false, false))
	store.DefineProperty(0, "one", dbstore.NewProperty(types.NewObj(4), 0, dbstore.PropRead, false, false))
	store.DefineProperty(5, "foo", dbstore.NewProperty(types.NewAnon(5), 0, dbstore.PropRead, false, false))

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

func TestCollectPendingFinalizationValuesKeepsOneRootForCyclicBareAnonymousLocals(t *testing.T) {
	store := dbstore.NewStore()

	root := testObject(0, false)
	anonA := testObject(4, true)
	anonB := testObject(5, true)

	for _, obj := range []*dbstore.Object{root, anonA, anonB} {
		if err := store.Add(obj); err != nil {
			t.Fatalf("add object: %v", err)
		}
	}
	if errCode := store.DefineProperty(4, "next", dbstore.NewProperty(types.NewAnon(5), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define A.next: %v", errCode)
	}
	if errCode := store.DefineProperty(5, "next", dbstore.NewProperty(types.NewAnon(4), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define B.next: %v", errCode)
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
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want one root for the anonymous cycle", len(got))
	}
	if !got[0].Equal(types.NewAnon(4)) {
		t.Fatalf("got[0] = %s, want cycle root %s", got[0].String(), types.NewAnon(4).String())
	}
}

func TestCollectPendingFinalizationValuesChoosesReachabilityRootBeforeLowerIDLeaf(t *testing.T) {
	store := dbstore.NewStore()
	for _, obj := range []*dbstore.Object{
		testObject(0, false),
		testObject(4, true),
		testObject(5, true),
	} {
		if err := store.Add(obj); err != nil {
			t.Fatalf("add object: %v", err)
		}
	}
	if errCode := store.DefineProperty(5, "next", dbstore.NewProperty(types.NewAnon(4), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define root.next: %v", errCode)
	}

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{{
		Locals: []types.Value{types.NewAnon(4), types.NewAnon(5)},
	}}

	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 1 || !got[0].Equal(types.NewAnon(5)) {
		t.Fatalf("pending roots = %v, want only reachability root %s", got, types.NewAnon(5).String())
	}
}
