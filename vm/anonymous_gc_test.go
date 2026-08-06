package vm

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
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

func TestCollectPendingFinalizationValuesRetainsDirectRefsEvenWhenCurrentlyPersistent(t *testing.T) {
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
	if len(got) != 1 || !got[0].Equal(types.NewAnon(5)) {
		t.Fatalf("pending candidates = %v, want direct VM root %s", got, types.NewAnon(5).String())
	}
	store.AppendPendingFinalizations(got)

	snapshot := store.Snapshot()
	if len(snapshot.PendingFinalizations) != 0 {
		t.Fatalf("snapshot pending finalizations = %v, want none for persistent candidate", snapshot.PendingFinalizations)
	}
	if got := len(snapshot.AnonymousObjects); got != 1 {
		t.Fatalf("snapshot anonymous objects = %d, want persistent anonymous object", got)
	}
}

func TestCollectPendingFinalizationValuesKeepsEveryCyclicBareAnonymousLocalDeterministically(t *testing.T) {
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
	want := []types.Value{types.NewAnon(4), types.NewAnon(5)}
	if len(got) != len(want) {
		t.Fatalf("pending roots = %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("pending roots = %v, want identity order %v", got, want)
		}
	}
	store.AppendPendingFinalizations(got)

	snapshot := store.Snapshot()
	if got := len(snapshot.PendingFinalizations); got != 1 {
		t.Fatalf("snapshot pending finalizations = %d, want one canonical cycle root", got)
	}
	if got := len(snapshot.AnonymousObjects); got != 2 {
		t.Fatalf("snapshot anonymous objects = %d, want each cycle member emitted once", got)
	}
	if !snapshot.PendingFinalizations[0].Equal(types.NewAnon(snapshot.AnonymousObjects[0].ID)) {
		t.Fatalf("snapshot cycle root = %v, want lowest-identity serialized root %v", snapshot.PendingFinalizations[0], types.NewAnon(snapshot.AnonymousObjects[0].ID))
	}

	// Snapshot normalization must not discard candidates from the live queue.
	// If the frozen graph later separates the cycle, both original VM roots are
	// needed and must reappear in the next snapshot.
	if errCode := store.SetPropertyValue(4, "next", types.NewInt(0)); errCode != types.E_NONE {
		t.Fatalf("remove A.next after snapshot: %v", errCode)
	}
	if errCode := store.SetPropertyValue(5, "next", types.NewInt(0)); errCode != types.E_NONE {
		t.Fatalf("remove B.next after snapshot: %v", errCode)
	}
	nextSnapshot := store.Snapshot()
	if got := len(nextSnapshot.PendingFinalizations); got != 2 {
		t.Fatalf("next snapshot pending finalizations = %d, want two retained live candidates", got)
	}
}

func TestCollectPendingFinalizationValuesRetainsDirectRootAndReachableLeaf(t *testing.T) {
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
	want := []types.Value{types.NewAnon(4), types.NewAnon(5)}
	if len(got) != len(want) || !got[0].Equal(want[0]) || !got[1].Equal(want[1]) {
		t.Fatalf("pending roots = %v, want every direct root in identity order %v", got, want)
	}
	store.AppendPendingFinalizations(got)

	snapshot := store.Snapshot()
	if got := len(snapshot.PendingFinalizations); got != 1 {
		t.Fatalf("snapshot pending finalizations = %d, want only frozen reachability root", got)
	}
	if got := len(snapshot.AnonymousObjects); got != 2 {
		t.Fatalf("snapshot anonymous objects = %d, want root and reachable leaf", got)
	}
	root := snapshot.AnonymousObjects[len(snapshot.AnonymousObjects)-1]
	if !snapshot.PendingFinalizations[0].Equal(types.NewAnon(root.ID)) {
		t.Fatalf("snapshot pending root = %v, want serialized reachability root %v", snapshot.PendingFinalizations[0], types.NewAnon(root.ID))
	}
}

func TestCollectPendingFinalizationValuesSurvivesStagedEdgeRemovalBeforeSnapshot(t *testing.T) {
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
	if errCode := store.DefineProperty(4, "next", dbstore.NewProperty(types.NewAnon(5), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define A.next: %v", errCode)
	}

	// Stage removal of the live A -> B edge. Until commit, a collector that
	// minimizes roots against the live graph sees B as covered by A. The VM,
	// however, directly holds both roots and disappears before the checkpoint.
	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.SetPropertyValue(4, "next", types.NewInt(0)); errCode != types.E_NONE {
		t.Fatalf("stage A.next removal: %v", errCode)
	}

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{{
		Locals: []types.Value{types.NewList([]types.Value{
			types.NewAnon(5),
			types.NewAnon(4),
		})},
	}}
	pending := CollectPendingFinalizationValues(store, exec)
	want := []types.Value{types.NewAnon(4), types.NewAnon(5)}
	if len(pending) != len(want) || !pending[0].Equal(want[0]) || !pending[1].Equal(want[1]) {
		t.Fatalf("pending roots = %v, want every direct root in identity order %v", pending, want)
	}
	store.AppendPendingFinalizations(pending)

	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("commit A.next removal: %v", errCode)
	}
	snapshot := store.Snapshot()
	if got := len(snapshot.AnonymousObjects); got != 2 {
		t.Fatalf("anonymous objects after staged edge removal = %d, want 2", got)
	}
	if got := len(snapshot.PendingFinalizations); got != 2 {
		t.Fatalf("pending finalizations after staged edge removal = %d, want 2", got)
	}
}

func TestCollectPendingFinalizationValuesExcludesStagedPersistentEdgeAtSnapshot(t *testing.T) {
	store := dbstore.NewStore()
	for _, obj := range []*dbstore.Object{
		testObject(0, false),
		testObject(5, true),
	} {
		if err := store.Add(obj); err != nil {
			t.Fatalf("add object: %v", err)
		}
	}
	if errCode := store.DefineProperty(0, "keep", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define #0.keep: %v", errCode)
	}

	// The collector must retain the VM candidate before the caller transaction
	// commits. Snapshot then linearizes after commit and recognizes it as
	// persistent, so it is serialized but not scheduled for restart recycling.
	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.SetPropertyValue(0, "keep", types.NewAnon(5)); errCode != types.E_NONE {
		t.Fatalf("stage #0.keep addition: %v", errCode)
	}

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{{Locals: []types.Value{types.NewAnon(5)}}}
	pending := CollectPendingFinalizationValues(store, exec)
	if len(pending) != 1 || !pending[0].Equal(types.NewAnon(5)) {
		t.Fatalf("pending candidates = %v, want staged-persistent candidate %v", pending, types.NewAnon(5))
	}
	store.AppendPendingFinalizations(pending)

	if errCode := tx.Commit(); errCode != types.E_NONE {
		t.Fatalf("commit #0.keep addition: %v", errCode)
	}
	snapshot := store.Snapshot()
	if len(snapshot.PendingFinalizations) != 0 {
		t.Fatalf("snapshot pending finalizations = %v, want staged-persistent candidate filtered", snapshot.PendingFinalizations)
	}
	if got := len(snapshot.AnonymousObjects); got != 1 {
		t.Fatalf("snapshot anonymous objects = %d, want persistent candidate serialized", got)
	}
	keep := snapshot.Objects[0].Properties["keep"].Value
	if !keep.IsAnonymous() || keep.ID() == types.ObjNothing {
		t.Fatalf("snapshot #0.keep = %v, want serialized anonymous reference", keep)
	}
}

func TestCollectPendingFinalizationValuesCapturesAllLiveVMValueFields(t *testing.T) {
	store := dbstore.NewStore()
	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{{
		Locals:         []types.Value{types.NewAnon(4)},
		ThisValue:      types.NewAnon(5),
		Args:           []types.Value{types.NewList([]types.Value{types.NewAnon(6)})},
		SavedThisValue: types.NewAnon(7),
		PendingError: VMException{Code: types.E_INVARG, Value: types.NewMap([][2]types.Value{
			{types.NewStr("payload"), types.NewAnon(8)},
		})},
	}}
	exec.Stack = []types.Value{types.NewMap([][2]types.Value{
		{types.NewAnon(9), types.NewStr("stack key")},
	})}
	exec.SP = 1
	exec.Context = &kernel.TaskContext{
		ThisValue:   types.NewAnon(10),
		MapFirstKey: types.NewAnon(11),
		MapLastKey:  types.NewList([]types.Value{types.NewAnon(12)}),
		TaskLocal: types.NewMap([][2]types.Value{
			{types.NewStr("task"), types.NewAnon(13)},
		}),
	}
	exec.PendingWaifs = []types.Value{types.NewList([]types.Value{types.NewAnon(14)})}
	exec.yieldResult = types.Result{
		Val: types.NewAnon(15),
		ForkInfo: &types.ForkInfo{
			ThisValue: types.NewAnon(16),
			Variables: map[string]types.Value{
				"forked": types.NewMap([][2]types.Value{{types.NewStr("value"), types.NewAnon(17)}}),
			},
		},
	}

	got := CollectPendingFinalizationValues(store, exec)
	want := make([]types.Value, 0, 14)
	for id := types.ObjID(4); id <= 17; id++ {
		want = append(want, types.NewAnon(id))
	}
	if len(got) != len(want) {
		t.Fatalf("pending candidates = %v, want all live VM values %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("pending candidates = %v, want identity order %v", got, want)
		}
	}
}
