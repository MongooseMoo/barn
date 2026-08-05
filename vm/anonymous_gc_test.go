package vm

import (
	"testing"

	dbstore "barn/db/store"
	"barn/kernel"
	"barn/task"
	"barn/types"
)

func testObject(id types.ObjID) *dbstore.Object {
	b := dbstore.NewObjectBuilder(id)
	b.SetOwner(0)
	b.SetFlags(dbstore.FlagRead)
	return b.Build()
}

func createAnonymousTestObject(t *testing.T, store *dbstore.Store, parents ...types.ObjID) types.ObjID {
	t.Helper()
	id, errCode := store.CreateObject(parents, 0, true)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject(..., true): %v", errCode)
	}
	return id
}

func TestCollectPendingFinalizationValuesCapturesUnreachableAnonymousRefs(t *testing.T) {
	store := dbstore.NewStore()

	if err := store.Add(testObject(0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anonID := createAnonymousTestObject(t, store, 0)

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{
		{
			Locals: []types.Value{
				types.NewList([]types.Value{types.NewInt(1), types.NewAnon(anonID)}),
			},
		},
	}
	exec.Stack = []types.Value{types.NewMap([][2]types.Value{
		{types.NewStr("x"), types.NewAnon(anonID)},
	})}
	exec.SP = 1

	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].String() != types.NewAnon(anonID).String() {
		t.Fatalf("got[0] = %s, want %s", got[0].String(), types.NewAnon(anonID).String())
	}
}

func TestCollectPendingFinalizationValuesReadsCanonicalTaskLocal(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(testObject(0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anonID := createAnonymousTestObject(t, store, 0)

	tk := task.NewTask(1, 0, 100, 1)
	waif := types.NewWaif(0, 0)
	tk.SetTaskLocal(types.NewList([]types.Value{types.NewAnon(anonID), waif}))
	exec := NewVM(store, nil)
	exec.Context = kernel.NewTaskContext()
	exec.Context.Task = tk

	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 2 {
		t.Fatalf("pending values = %v, want anonymous and WAIF task-local roots", got)
	}
	if !got[0].Equal(types.NewAnon(anonID)) {
		t.Errorf("pending[0] = %v, want %v", got[0], types.NewAnon(anonID))
	}
	if !got[1].Equal(waif) {
		t.Errorf("pending[1] = %v, want task-local WAIF identity %p", got[1], waif.WaifIdentity())
	}
}

type nonTaskLocalOwner struct {
	value types.Value
}

func (o *nonTaskLocalOwner) GetTaskLocal() types.Value { return o.value }

func TestCollectPendingFinalizationValuesRejectsNonTaskTaskLocalOwner(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(testObject(0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	exec := NewVM(store, nil)
	exec.Context = kernel.NewTaskContext()
	exec.Context.Task = &nonTaskLocalOwner{value: types.NewAnon(4)}

	if got := CollectPendingFinalizationValues(store, exec); len(got) != 0 {
		t.Fatalf("pending values = %v, want no roots from non-*task.Task owner", got)
	}
}

func TestCollectPendingFinalizationValuesKeepsNestedAnonUnderSingleWaifRoot(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(testObject(0)); err != nil {
		t.Fatalf("add WAIF class: %v", err)
	}
	anonID := createAnonymousTestObject(t, store, 0)
	waif := types.NewWaif(0, 0)
	waif.SetProperty("anon", types.NewAnon(anonID))
	exec := NewVM(store, nil)
	exec.PendingFinalizations = []types.Value{waif}

	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 1 || !got[0].Equal(waif) {
		t.Fatalf("pending roots = %v, want exactly WAIF identity %p", got, waif.WaifIdentity())
	}
}

func TestReturnFramePopKeepsNestedWaifUnderOneDirectRoot(t *testing.T) {
	inner := types.NewWaif(0, 0)
	outer := types.NewWaif(0, 0)
	outer.SetProperty("nested", inner)
	exec := NewVM(nil, nil)
	frame := &StackFrame{
		Locals:        []types.Value{types.NewList([]types.Value{outer})},
		DiscardReturn: true,
	}
	exec.Frames = []*StackFrame{frame}
	exec.frame = frame

	exec.Return(types.None)
	if got := exec.TakePendingWaifs(); len(got) != 1 || !got[0].Equal(outer) {
		t.Fatalf("frame-pop pending WAIF roots = %v, want exactly outer identity %p", got, outer.WaifIdentity())
	}
	if got := exec.PendingFinalizations; len(got) != 1 || !got[0].Equal(outer) {
		t.Fatalf("frame-pop pending finalization roots = %v, want exactly outer identity %p", got, outer.WaifIdentity())
	}
}

func TestCollectPendingFinalizationValuesRetainsDirectRefsEvenWhenCurrentlyPersistent(t *testing.T) {
	store := dbstore.NewStore()

	for _, obj := range []*dbstore.Object{testObject(0), testObject(4)} {
		if err := store.Add(obj); err != nil {
			t.Fatalf("add object: %v", err)
		}
	}
	anonID := createAnonymousTestObject(t, store, 0)

	store.DefineProperty(4, "two", dbstore.NewProperty(types.NewMap([][2]types.Value{{types.NewStr("foo"), types.NewAnon(anonID)}}), 0, dbstore.PropRead, false, false))
	store.DefineProperty(0, "one", dbstore.NewProperty(types.NewObj(4), 0, dbstore.PropRead, false, false))
	store.DefineProperty(anonID, "foo", dbstore.NewProperty(types.NewAnon(anonID), 0, dbstore.PropRead, false, false))

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{
		{
			Locals: []types.Value{
				types.NewList([]types.Value{types.NewAnon(anonID)}),
			},
		},
	}

	got := CollectPendingFinalizationValues(store, exec)
	if len(got) != 1 || !got[0].Equal(types.NewAnon(anonID)) {
		t.Fatalf("pending candidates = %v, want direct VM root %s", got, types.NewAnon(anonID).String())
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

	if err := store.Add(testObject(0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anonA := createAnonymousTestObject(t, store, 0)
	anonB := createAnonymousTestObject(t, store, 0)
	if errCode := store.DefineProperty(anonA, "next", dbstore.NewProperty(types.NewAnon(anonB), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define A.next: %v", errCode)
	}
	if errCode := store.DefineProperty(anonB, "next", dbstore.NewProperty(types.NewAnon(anonA), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define B.next: %v", errCode)
	}

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{
		{
			Locals: []types.Value{
				types.NewAnon(anonA),
				types.NewAnon(anonB),
			},
		},
	}

	got := CollectPendingFinalizationValues(store, exec)
	want := []types.Value{types.NewAnon(anonA), types.NewAnon(anonB)}
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
	if errCode := store.SetPropertyValue(anonA, "next", types.NewInt(0)); errCode != types.E_NONE {
		t.Fatalf("remove A.next after snapshot: %v", errCode)
	}
	if errCode := store.SetPropertyValue(anonB, "next", types.NewInt(0)); errCode != types.E_NONE {
		t.Fatalf("remove B.next after snapshot: %v", errCode)
	}
	nextSnapshot := store.Snapshot()
	if got := len(nextSnapshot.PendingFinalizations); got != 2 {
		t.Fatalf("next snapshot pending finalizations = %d, want two retained live candidates", got)
	}
}

func TestCollectPendingFinalizationValuesRetainsDirectRootAndReachableLeaf(t *testing.T) {
	store := dbstore.NewStore()
	if err := store.Add(testObject(0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	leafID := createAnonymousTestObject(t, store, 0)
	rootID := createAnonymousTestObject(t, store, 0)
	if errCode := store.DefineProperty(rootID, "next", dbstore.NewProperty(types.NewAnon(leafID), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define root.next: %v", errCode)
	}

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{{
		Locals: []types.Value{types.NewAnon(leafID), types.NewAnon(rootID)},
	}}

	got := CollectPendingFinalizationValues(store, exec)
	want := []types.Value{types.NewAnon(leafID), types.NewAnon(rootID)}
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
	if err := store.Add(testObject(0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anonA := createAnonymousTestObject(t, store, 0)
	anonB := createAnonymousTestObject(t, store, 0)
	if errCode := store.DefineProperty(anonA, "next", dbstore.NewProperty(types.NewAnon(anonB), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define A.next: %v", errCode)
	}

	// Stage removal of the live A -> B edge. Until commit, a collector that
	// minimizes roots against the live graph sees B as covered by A. The VM,
	// however, directly holds both roots and disappears before the checkpoint.
	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.SetPropertyValue(anonA, "next", types.NewInt(0)); errCode != types.E_NONE {
		t.Fatalf("stage A.next removal: %v", errCode)
	}

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{{
		Locals: []types.Value{types.NewList([]types.Value{
			types.NewAnon(anonB),
			types.NewAnon(anonA),
		})},
	}}
	pending := CollectPendingFinalizationValues(store, exec)
	want := []types.Value{types.NewAnon(anonA), types.NewAnon(anonB)}
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
	if err := store.Add(testObject(0)); err != nil {
		t.Fatalf("add root: %v", err)
	}
	anonID := createAnonymousTestObject(t, store, 0)
	if errCode := store.DefineProperty(0, "keep", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead, false, true)); errCode != types.E_NONE {
		t.Fatalf("define #0.keep: %v", errCode)
	}

	// The collector must retain the VM candidate before the caller transaction
	// commits. Snapshot then linearizes after commit and recognizes it as
	// persistent, so it is serialized but not scheduled for restart recycling.
	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.SetPropertyValue(0, "keep", types.NewAnon(anonID)); errCode != types.E_NONE {
		t.Fatalf("stage #0.keep addition: %v", errCode)
	}

	exec := NewVM(store, nil)
	exec.Frames = []*StackFrame{{Locals: []types.Value{types.NewAnon(anonID)}}}
	pending := CollectPendingFinalizationValues(store, exec)
	if len(pending) != 1 || !pending[0].Equal(types.NewAnon(anonID)) {
		t.Fatalf("pending candidates = %v, want staged-persistent candidate %v", pending, types.NewAnon(anonID))
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
	}
	tk := task.NewTask(1, 0, 100, 1)
	tk.SetTaskLocal(types.NewMap([][2]types.Value{{types.NewStr("task"), types.NewAnon(13)}}))
	exec.Context.Task = tk
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
