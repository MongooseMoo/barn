package vm

import (
	"testing"

	"barn/builtins"
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

func TestExpandAnonymousReachabilityIncludesTransactionPropertyWrites(t *testing.T) {
	store := dbstore.NewStore()
	root := testObject(0, false)
	head := testObject(4, true)
	middle := testObject(5, true)
	for _, obj := range []*dbstore.Object{root, head, middle} {
		if err := store.Add(obj); err != nil {
			t.Fatalf("add object: %v", err)
		}
	}
	for _, id := range []types.ObjID{4, 5} {
		if errCode := store.DefineProperty(id, "next", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("DefineProperty(%d) = %v", id, errCode)
		}
	}

	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if errCode := tx.SetPropertyValue(4, "next", types.NewAnon(5)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue(head) = %v", errCode)
	}
	if errCode := tx.SetPropertyValue(5, "next", types.NewAnon(4)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue(middle) = %v", errCode)
	}

	reachable := make(map[types.ObjID]struct{})
	refs := map[types.ObjID]struct{}{4: {}}
	expandAnonymousReachability(store, tx, reachable, refs)
	for _, id := range []types.ObjID{4, 5} {
		if _, ok := reachable[id]; !ok {
			t.Errorf("anonymous object %d not reachable through staged cycle: %#v", id, reachable)
		}
	}
}

func TestRecycleOrphanAnonymousBatchFreezesCandidatesBeforeRecycleHooks(t *testing.T) {
	store := dbstore.NewStore()
	root := testObject(0, false)
	if err := store.Add(root); err != nil {
		t.Fatalf("add root: %v", err)
	}
	if errCode := store.DefineProperty(0, "stash", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("define stash: %v", errCode)
	}
	const candidateCount = 64
	for id := types.ObjID(1); id <= candidateCount; id++ {
		if err := store.Add(testObject(id, true)); err != nil {
			t.Fatalf("add anonymous candidate #%d: %v", id, err)
		}
	}

	registry := builtins.NewRegistry()
	var createdByHook types.ObjID
	recycleCalls := 0
	registry.Register("recycle", func(_ *kernel.TaskContext, args []types.Value) types.Result {
		recycleCalls++
		if len(args) != 1 || args[0].Type() != types.TYPE_ANON {
			return types.Err(types.E_INVARG)
		}
		if err := store.Recycle(args[0].Obj()); err != nil {
			return types.Err(types.E_INVARG)
		}
		if createdByHook == 0 {
			var errCode types.ErrorCode
			createdByHook, errCode = store.CreateObject(nil, 0, true)
			if errCode != types.E_NONE {
				return types.Err(errCode)
			}
			if errCode := store.SetPropertyValue(0, "stash", types.NewAnon(createdByHook)); errCode != types.E_NONE {
				return types.Err(errCode)
			}
		}
		return types.Ok(types.NewInt(0))
	})

	ctx := kernel.NewTaskContext()
	ctx.Store = store
	ctx.Registry = registry
	const requestCount = 128
	requests := make([]AnonGCRequest, requestCount)
	for i := range requests {
		requests[i] = AnonGCRequest{Ctx: ctx, MinID: 1}
	}
	RecycleOrphanAnonymousBatch(store, registry, requests, nil)

	if recycleCalls != candidateCount {
		t.Fatalf("recycle calls = %d, want only %d pre-snapshot candidates across %d requests", recycleCalls, candidateCount, requestCount)
	}
	if createdByHook == 0 || !store.Valid(createdByHook) {
		t.Fatalf("recycle-hook-created persistent anonymous object #%d did not survive current batch", createdByHook)
	}
	stash, errCode := store.PropertyValue(0, "stash")
	if errCode != types.E_NONE || stash.Type() != types.TYPE_ANON || stash.Obj() != createdByHook {
		t.Fatalf("persistent stash = %v (%v), want anonymous #%d", stash, errCode, createdByHook)
	}
}

func TestRecycleFrozenAnonymousCandidatesAllocationsDoNotScaleWithRequests(t *testing.T) {
	ctx := kernel.NewTaskContext()
	const requestCount = 128
	requests := make([]AnonGCRequest, requestCount)
	for i := range requests {
		requests[i] = AnonGCRequest{Ctx: ctx, MinID: 1}
	}
	const candidateCount = 64
	candidates := make([]types.ObjID, candidateCount)
	for i := range candidates {
		candidates[i] = types.ObjID(i + 1)
	}
	recycle := func(_ *kernel.TaskContext, _ types.ObjID) {}

	allocs := testing.AllocsPerRun(50, func() {
		recycleFrozenAnonymousCandidates(requests, candidates, recycle)
	})
	if allocs > 8 {
		t.Fatalf("routing %d candidates through %d requests allocated %.1f times, want <= 8 independent of request count", candidateCount, requestCount, allocs)
	}
}
