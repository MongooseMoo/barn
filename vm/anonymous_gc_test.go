package vm

import (
	"barn/db"
	"barn/types"
	"testing"
)

func testObject(id types.ObjID, anonymous bool) *db.Object {
	flags := db.FlagRead
	if anonymous {
		flags = flags.Set(db.FlagAnonymous)
	}
	return &db.Object{
		ID:         id,
		Owner:      0,
		Flags:      flags,
		Anonymous:  anonymous,
		Properties: map[string]*db.Property{},
		Verbs:      map[string]*db.Verb{},
	}
}

func TestCollectPendingFinalizationValuesCapturesUnreachableAnonymousRefs(t *testing.T) {
	store := db.NewStore()

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
	store := db.NewStore()

	root := testObject(0, false)
	holder := testObject(4, false)
	anon := testObject(5, true)
	holder.Properties["two"] = &db.Property{
		Name:  "two",
		Value: types.NewMap([][2]types.Value{{types.NewStr("foo"), types.NewAnon(5)}}),
		Owner: 0,
		Perms: db.PropRead,
	}
	root.Properties["one"] = &db.Property{
		Name:  "one",
		Value: types.NewObj(4),
		Owner: 0,
		Perms: db.PropRead,
	}
	anon.Properties["foo"] = &db.Property{
		Name:  "foo",
		Value: types.NewAnon(5),
		Owner: 0,
		Perms: db.PropRead,
	}

	for _, obj := range []*db.Object{root, holder, anon} {
		if err := store.Add(obj); err != nil {
			t.Fatalf("add #%d: %v", obj.ID, err)
		}
	}

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
	store := db.NewStore()

	root := testObject(0, false)
	anonA := testObject(4, true)
	anonB := testObject(5, true)

	for _, obj := range []*db.Object{root, anonA, anonB} {
		if err := store.Add(obj); err != nil {
			t.Fatalf("add #%d: %v", obj.ID, err)
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
