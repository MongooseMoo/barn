package store

import (
	"maps"
	"testing"

	"barn/types"
)

// The resolution memo's ONLY safety obligation beyond returning the right
// answer is that a hit leaves the transaction's MVCC read set byte-identical to
// an uncached walk. Every test below that resolves twice compares the full read
// set after the (cached) second call against the read set a fresh transaction
// records for a single uncached call.

func testChainStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	// #0 root, #1 middle (parent #0), #2 leaf (parent #1).
	for id := types.ObjID(0); id <= 2; id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			t.Fatalf("Add #%d: %v", id, err)
		}
	}
	if ec := s.ChangeParents(1, []types.ObjID{0}); ec != types.E_NONE {
		t.Fatalf("ChangeParents #1: %v", ec)
	}
	if ec := s.ChangeParents(2, []types.ObjID{1}); ec != types.E_NONE {
		t.Fatalf("ChangeParents #2: %v", ec)
	}
	return s
}

func addVerbT(t *testing.T, s *Store, objID types.ObjID, names []string, perms VerbPerms) {
	t.Helper()
	v := NewVerb(names[0], names, 0, perms, VerbArgs{This: "none", Prep: "none", That: "none"}, []string{"return 1;"})
	if _, ec := s.AddVerb(objID, v); ec != types.E_NONE {
		t.Fatalf("AddVerb #%d %v: %v", objID, names, ec)
	}
}

type readSetSnapshot struct {
	propertyReads map[propertyReadKey]uint64
	propertyScans map[types.ObjID]uint64
	verbReads     map[verbReadKey]uint64
	verbScans     map[types.ObjID]uint64
}

func snapshotReadSet(tx *StoreTxn) readSetSnapshot {
	return readSetSnapshot{
		propertyReads: maps.Clone(tx.propertyReads),
		propertyScans: maps.Clone(tx.propertyScans),
		verbReads:     maps.Clone(tx.verbReads),
		verbScans:     maps.Clone(tx.verbScans),
	}
}

func requireSameReadSet(t *testing.T, what string, want, got readSetSnapshot) {
	t.Helper()
	if !maps.Equal(want.propertyReads, got.propertyReads) {
		t.Fatalf("%s: propertyReads = %v, want %v", what, got.propertyReads, want.propertyReads)
	}
	if !maps.Equal(want.propertyScans, got.propertyScans) {
		t.Fatalf("%s: propertyScans = %v, want %v", what, got.propertyScans, want.propertyScans)
	}
	if !maps.Equal(want.verbReads, got.verbReads) {
		t.Fatalf("%s: verbReads = %v, want %v", what, got.verbReads, want.verbReads)
	}
	if !maps.Equal(want.verbScans, got.verbScans) {
		t.Fatalf("%s: verbScans = %v, want %v", what, got.verbScans, want.verbScans)
	}
}

// referenceVerbReadSet is the read set an UNCACHED single resolution produces:
// a brand-new transaction has an empty memo, so its first call always walks.
func referenceVerbReadSet(t *testing.T, s *Store, objID types.ObjID, name string) readSetSnapshot {
	t.Helper()
	tx := s.BeginReadOnly(0)
	defer tx.Release()
	tx.findVerb(objID, name, false)
	return snapshotReadSet(tx)
}

func referencePropertyReadSet(t *testing.T, s *Store, objID types.ObjID, name string) readSetSnapshot {
	t.Helper()
	tx := s.BeginReadOnly(0)
	defer tx.Release()
	tx.findProperty(objID, name)
	return snapshotReadSet(tx)
}

func TestVerbResolveCacheHitPreservesReadSetAndResult(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)

	want := referenceVerbReadSet(t, s, 2, "look")

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	v1, d1, err1 := tx.findVerb(2, "look", false)
	if err1 != nil {
		t.Fatalf("first findVerb: %v", err1)
	}
	verbs, _ := tx.resolveCacheLenForTest()
	if verbs != 1 {
		t.Fatalf("verb memo size after first call = %d, want 1", verbs)
	}

	v2, d2, err2 := tx.findVerb(2, "look", false)
	if err2 != nil {
		t.Fatalf("second findVerb: %v", err2)
	}
	if v1 != v2 || d1 != d2 {
		t.Fatalf("cached resolution = (%p,#%d), want (%p,#%d)", v2, d2, v1, d1)
	}
	if d1 != 0 {
		t.Fatalf("definer = #%d, want #0", d1)
	}
	requireSameReadSet(t, "verb cache hit", want, snapshotReadSet(tx))
}

func TestVerbResolveCacheNegativeEntryPreservesReadSet(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)

	want := referenceVerbReadSet(t, s, 2, "nosuchverb")

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	if _, _, err := tx.findVerb(2, "nosuchverb", false); err == nil {
		t.Fatal("first findVerb of a missing verb returned nil error")
	}
	verbs, _ := tx.resolveCacheLenForTest()
	if verbs != 1 {
		t.Fatalf("verb memo size = %d, want 1 (negative entry cached)", verbs)
	}
	if _, definer, err := tx.findVerb(2, "nosuchverb", false); err == nil {
		t.Fatal("cached findVerb of a missing verb returned nil error")
	} else if definer != types.ObjNothing {
		t.Fatalf("definer on miss = #%d, want ObjNothing", definer)
	}
	// The negative walk scans the WHOLE chain; the replay must record all of it.
	got := snapshotReadSet(tx)
	requireSameReadSet(t, "verb negative cache hit", want, got)
	if len(got.verbScans) != 3 {
		t.Fatalf("verbScans = %v, want all three chain objects", got.verbScans)
	}
}

func TestVerbResolveCacheRequireExecuteIsPartOfTheKey(t *testing.T) {
	s := testChainStore(t)
	// #1 has a non-executable "look"; #0 has an executable one. Call dispatch
	// must skip #1's and keep walking; non-dispatch lookup must stop at #1.
	addVerbT(t, s, 1, []string{"look"}, VerbRead)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	_, d, err := tx.findVerb(2, "look", false)
	if err != nil || d != 1 {
		t.Fatalf("findVerb(requireExecute=false) definer = #%d err=%v, want #1", d, err)
	}
	_, dx, err := tx.findVerb(2, "look", true)
	if err != nil || dx != 0 {
		t.Fatalf("findVerb(requireExecute=true) definer = #%d err=%v, want #0", dx, err)
	}
	// Re-resolve both from the memo; the two keys must not collide.
	if _, d, _ := tx.findVerb(2, "look", false); d != 1 {
		t.Fatalf("cached findVerb(false) definer = #%d, want #1", d)
	}
	if _, dx, _ := tx.findVerb(2, "look", true); dx != 0 {
		t.Fatalf("cached findVerb(true) definer = #%d, want #0", dx)
	}
}

func TestVerbResolveCachePreservesWildcardAliasSemantics(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"l*ook", "exam*ine"}, VerbRead|VerbExecute)

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	// Every prefix the wildcard admits must resolve, cached and uncached, and a
	// non-matching name must stay unresolved.
	for _, name := range []string{"l", "lo", "loo", "look", "exam", "examine"} {
		for pass := 0; pass < 2; pass++ {
			v, d, err := tx.findVerb(2, name, false)
			if err != nil {
				t.Fatalf("findVerb(%q) pass %d: %v", name, pass, err)
			}
			if d != 0 || v.names[0] != "l*ook" {
				t.Fatalf("findVerb(%q) pass %d = %q on #%d, want l*ook on #0", name, pass, v.names[0], d)
			}
		}
	}
	for _, name := range []string{"lx", "looking", "exa", "ex"} {
		for pass := 0; pass < 2; pass++ {
			if _, _, err := tx.findVerb(2, name, false); err == nil {
				t.Fatalf("findVerb(%q) pass %d resolved, want not found", name, pass)
			}
		}
	}
}

func TestVerbResolveCacheIsCaseInsensitive(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"Look"}, VerbRead|VerbExecute)

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	var first *Verb
	for _, name := range []string{"look", "LOOK", "LoOk", "look"} {
		v, d, err := tx.findVerb(2, name, false)
		if err != nil {
			t.Fatalf("findVerb(%q): %v", name, err)
		}
		if d != 0 {
			t.Fatalf("findVerb(%q) definer = #%d, want #0", name, d)
		}
		if first == nil {
			first = v
		} else if v != first {
			t.Fatalf("findVerb(%q) resolved to a different verb node", name)
		}
	}
}

func TestVerbResolveCacheBypassedAfterStagedWrite(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.DefineProperty(0, "x", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	if _, _, err := tx.findVerb(2, "look", false); err != nil {
		t.Fatalf("findVerb before write: %v", err)
	}
	if verbs, _ := tx.resolveCacheLenForTest(); verbs != 1 {
		t.Fatalf("verb memo size before write = %d, want 1", verbs)
	}

	// Any staged write privatizes an object; from that point the memo is off.
	if ec := tx.SetPropertyValue(2, "x", types.NewInt(7)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if tx.resolveCacheActive() {
		t.Fatal("resolveCacheActive() is true after a staged write")
	}
	if verbs, props := tx.resolveCacheLenForTest(); verbs != 0 || props != 0 {
		t.Fatalf("memo sizes after staged write = (%d,%d), want (0,0)", verbs, props)
	}
	if _, _, err := tx.findVerb(2, "look", false); err != nil {
		t.Fatalf("findVerb after write: %v", err)
	}
	if verbs, _ := tx.resolveCacheLenForTest(); verbs != 0 {
		t.Fatalf("verb memo repopulated after a staged write (size %d)", verbs)
	}
}

// A staged property write must be read back by the transaction that staged it,
// never served from a pre-write memo entry.
func TestPropertyResolveReadsOwnStagedWrite(t *testing.T) {
	s := testChainStore(t)
	if ec := s.DefineProperty(0, "x", NewProperty(types.NewInt(1), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	if v, ec := tx.PropertyValue(2, "x"); ec != types.E_NONE || v.Int() != 1 {
		t.Fatalf("PropertyValue before write = (%v,%v), want 1", v, ec)
	}
	if ec := tx.SetPropertyValue(2, "x", types.NewInt(42)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if v, ec := tx.PropertyValue(2, "x"); ec != types.E_NONE || v.Int() != 42 {
		t.Fatalf("PropertyValue after write = (%v,%v), want 42", v, ec)
	}
}

func TestPropertyResolveCacheHitPreservesReadSetThroughClearChain(t *testing.T) {
	s := testChainStore(t)
	// Defined on #0, so #1 and #2 hold inherited CLEAR slots: the walk must pass
	// through both and land on #0's value.
	if ec := s.DefineProperty(0, "desc", NewProperty(types.NewStr("root"), 0, PropRead, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}

	want := referencePropertyReadSet(t, s, 2, "desc")

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	p1, n1, ec1 := tx.findProperty(2, "desc")
	if ec1 != types.E_NONE {
		t.Fatalf("first findProperty: %v", ec1)
	}
	if _, props := tx.resolveCacheLenForTest(); props != 1 {
		t.Fatalf("property memo size = %d, want 1", props)
	}
	p2, n2, ec2 := tx.findProperty(2, "desc")
	if ec2 != types.E_NONE || n1 != n2 || p1 != p2 {
		t.Fatalf("cached findProperty = (%v,%q,%v), want (%v,%q,%v)", p2, n2, ec2, p1, n1, ec1)
	}
	if p2.value.Str() != "root" {
		t.Fatalf("cached property value = %q, want root", p2.value.Str())
	}
	requireSameReadSet(t, "property cache hit", want, snapshotReadSet(tx))
}

func TestPropertyResolveCacheNegativeEntryPreservesReadSet(t *testing.T) {
	s := testChainStore(t)

	want := referencePropertyReadSet(t, s, 2, "nosuchprop")

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	if _, _, ec := tx.findProperty(2, "nosuchprop"); ec != types.E_PROPNF {
		t.Fatalf("first findProperty = %v, want E_PROPNF", ec)
	}
	if _, _, ec := tx.findProperty(2, "nosuchprop"); ec != types.E_PROPNF {
		t.Fatalf("cached findProperty = %v, want E_PROPNF", ec)
	}
	got := snapshotReadSet(tx)
	requireSameReadSet(t, "property negative cache hit", want, got)
	if len(got.propertyScans) != 3 {
		t.Fatalf("propertyScans = %v, want all three chain objects", got.propertyScans)
	}
}

func TestPropertyResolveCacheIsCaseInsensitive(t *testing.T) {
	s := testChainStore(t)
	if ec := s.DefineProperty(0, "Desc", NewProperty(types.NewStr("root"), 0, PropRead, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	for _, name := range []string{"desc", "DESC", "DeSc", "desc"} {
		p, _, ec := tx.findProperty(2, name)
		if ec != types.E_NONE {
			t.Fatalf("findProperty(%q) = %v", name, ec)
		}
		if p.value.Str() != "root" {
			t.Fatalf("findProperty(%q) value = %q, want root", name, p.value.Str())
		}
	}
}

// A committed write by another transaction must not be served from this
// transaction's memo — and, more importantly, must not be served at all: the
// snapshot is fixed. The NEXT transaction sees the new value.
func TestResolveCacheIsScopedToTheSnapshot(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)

	tx := s.BeginReadOnly(0)
	if _, d, err := tx.findVerb(2, "look", false); err != nil || d != 0 {
		t.Fatalf("findVerb = #%d err=%v, want #0", d, err)
	}

	// Another writer defines a nearer "look" on #1 and commits it to live.
	addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute)

	// The open snapshot legitimately keeps its answer (it already cached #1's
	// image at the old version), while a FRESH transaction sees the new verb.
	if _, d, err := tx.findVerb(2, "look", false); err != nil || d != 0 {
		t.Fatalf("same-snapshot findVerb = #%d err=%v, want #0", d, err)
	}
	tx.Release()

	tx2 := s.BeginReadOnly(0)
	defer tx2.Release()
	if _, d, err := tx2.findVerb(2, "look", false); err != nil || d != 1 {
		t.Fatalf("new-snapshot findVerb = #%d err=%v, want #1", d, err)
	}
}

func TestResolveCacheInvalidatedByForgetObject(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	if _, _, err := tx.findVerb(2, "look", false); err != nil {
		t.Fatalf("findVerb: %v", err)
	}
	if verbs, _ := tx.resolveCacheLenForTest(); verbs != 1 {
		t.Fatalf("verb memo size = %d, want 1", verbs)
	}
	tx.ForgetObject(1)
	if verbs, props := tx.resolveCacheLenForTest(); verbs != 0 || props != 0 {
		t.Fatalf("memo sizes after ForgetObject = (%d,%d), want (0,0)", verbs, props)
	}
}

func TestResolveCacheInvalidatedByAdoptLiveObject(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	if _, _, err := tx.findVerb(2, "look", false); err != nil {
		t.Fatalf("findVerb: %v", err)
	}
	if ec := tx.AdoptLiveObject(0); ec != types.E_NONE {
		t.Fatalf("AdoptLiveObject: %v", ec)
	}
	if verbs, props := tx.resolveCacheLenForTest(); verbs != 0 || props != 0 {
		t.Fatalf("memo sizes after AdoptLiveObject = (%d,%d), want (0,0)", verbs, props)
	}
	// Re-resolution after the adopt must still be correct.
	if _, d, err := tx.findVerb(2, "look", false); err != nil || d != 0 {
		t.Fatalf("findVerb after adopt = #%d err=%v, want #0", d, err)
	}
}

// A memoized entry whose recorded objects were rebound in the txn cache must be
// treated as a miss even if the wholesale invalidation were somehow skipped.
func TestResolveCacheStepIdentityCheckRejectsRebind(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	if _, _, err := tx.findVerb(2, "look", false); err != nil {
		t.Fatalf("findVerb: %v", err)
	}
	entry, ok := tx.verbResolve[verbResolveKey{objID: 2, name: "look"}]
	if !ok {
		t.Fatal("no memo entry recorded")
	}
	if !tx.verbStepsCurrent(entry.steps) {
		t.Fatal("verbStepsCurrent = false immediately after the walk")
	}
	// Rebind one recorded object behind the memo's back.
	tx.objects[1] = cloneObjectForReadTxn(tx.objects[1])
	if tx.verbStepsCurrent(entry.steps) {
		t.Fatal("verbStepsCurrent = true after the txn rebound a recorded object")
	}
}

func TestResolveCacheIsBounded(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	for i := 0; i < resolveCacheCap*3; i++ {
		name := "missing_" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('a'+(i/676)%26))
		tx.findVerb(2, name, false)
		tx.findProperty(2, name)
		if verbs, props := tx.resolveCacheLenForTest(); verbs > resolveCacheCap || props > resolveCacheCap {
			t.Fatalf("memo exceeded cap %d: verbs=%d props=%d", resolveCacheCap, verbs, props)
		}
	}
	verbs, props := tx.resolveCacheLenForTest()
	if verbs == 0 || props == 0 {
		t.Fatalf("memo empty after %d distinct lookups (verbs=%d props=%d)", resolveCacheCap*3, verbs, props)
	}
}

func TestObjIDSetLinearAndPromoted(t *testing.T) {
	var s objIDSet
	// Well past the linear window, so the promotion path is exercised.
	const n = objIDSetLinearMax * 4
	for i := 0; i < n; i++ {
		if !s.add(types.ObjID(i)) {
			t.Fatalf("add(%d) reported already-present on first insert", i)
		}
	}
	if s.set == nil {
		t.Fatalf("objIDSet did not promote to a map after %d inserts", n)
	}
	for i := 0; i < n; i++ {
		if s.add(types.ObjID(i)) {
			t.Fatalf("add(%d) reported new on second insert", i)
		}
	}
	s.reset()
	for i := 0; i < n; i++ {
		if !s.add(types.ObjID(i)) {
			t.Fatalf("add(%d) reported already-present after reset", i)
		}
	}
	// Below the promotion threshold the set must still behave.
	var small objIDSet
	if !small.add(7) || small.add(7) || !small.add(9) {
		t.Fatal("small objIDSet membership is wrong")
	}
	if small.set != nil {
		t.Fatal("small objIDSet promoted to a map prematurely")
	}
}

// The walk scratch is reused across calls; a deep/wide graph must still resolve
// correctly after many walks share the same buffers.
func TestWalkScratchReuseAcrossManyWalks(t *testing.T) {
	s := NewStore()
	const depth = 60
	for id := types.ObjID(0); id < depth; id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			t.Fatalf("Add #%d: %v", id, err)
		}
		if id > 0 {
			if ec := s.ChangeParents(id, []types.ObjID{id - 1}); ec != types.E_NONE {
				t.Fatalf("ChangeParents #%d: %v", id, ec)
			}
		}
	}
	addVerbT(t, s, 0, []string{"deep"}, VerbRead|VerbExecute)
	if ec := s.DefineProperty(0, "deepprop", NewProperty(types.NewInt(5), 0, PropRead, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	for start := types.ObjID(0); start < depth; start++ {
		if _, d, err := tx.findVerb(start, "deep", false); err != nil || d != 0 {
			t.Fatalf("findVerb from #%d = #%d err=%v, want #0", start, d, err)
		}
		p, _, ec := tx.findProperty(start, "deepprop")
		if ec != types.E_NONE || p.value.Int() != 5 {
			t.Fatalf("findProperty from #%d = (%v,%v), want 5", start, p.value, ec)
		}
		if _, _, err := tx.findVerb(start, "absent", false); err == nil {
			t.Fatalf("findVerb(absent) from #%d resolved", start)
		}
	}
}

func TestFindParentVerbScratchWalk(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute)

	tx := s.BeginReadOnly(0)
	defer tx.Release()

	// pass() from #1's definition must find #0's, twice in a row (the scratch is
	// reused between the two calls).
	for pass := 0; pass < 2; pass++ {
		v, d, err := tx.FindParentVerb(1, "look")
		if err != nil {
			t.Fatalf("FindParentVerb pass %d: %v", pass, err)
		}
		if d != 0 || v.Names[0] != "look" {
			t.Fatalf("FindParentVerb pass %d = %q on #%d, want look on #0", pass, v.Names[0], d)
		}
	}
	if _, _, err := tx.FindParentVerb(0, "look"); err == nil {
		t.Fatal("FindParentVerb from the root resolved, want not found")
	}
}
