package store

import (
	"maps"
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// resetVerbDispatchMemoForTest empties the store-level dispatch memo so a test
// can observe a first-call walk.
func (s *Store) resetVerbDispatchMemoForTest() {
	s.verbDispatchMemo.Store(&sync.Map{})
	s.verbDispatchMemoSize.Store(0)
}

func memoHas(s *Store, objID types.ObjID, name string) bool {
	_, ok := s.verbMemo().Load(verbResolveKey{objID: objID, name: name})
	return ok
}

func TestVerbDispatchMemoPreservesDuplicateAliasWinner(t *testing.T) {
	for _, callable := range []bool{false, true} {
		t.Run(map[bool]string{false: "lookup", true: "callable"}[callable], func(t *testing.T) {
			s := testChainStore(t)
			addVerbT(t, s, 0, []string{"look", "first"}, VerbRead)
			addVerbT(t, s, 0, []string{"look", "second"}, VerbRead|VerbExecute)

			check := func(wantCode string) {
				t.Helper()
				var cold *Verb
				for i := 0; i < 2; i++ {
					tx := s.BeginSnapshot(0)
					verb, definer, err := tx.findVerb(2, "second", callable)
					usedMemo := tx.usedVerbMemo
					tx.Release()
					if err != nil || definer != 0 || verb == nil {
						t.Fatalf("lookup %d: verb=%v definer=%d err=%v", i, verb, definer, err)
					}
					if !verb.perms.Has(VerbExecute) || len(verb.names) != 2 || verb.names[1] != "second" || verb.code[0] != wantCode {
						t.Fatalf("lookup %d: names=%v perms=%v code=%v", i, verb.names, verb.perms, verb.code)
					}
					if i == 0 {
						cold = verb
					} else if !usedMemo || verb != cold {
						t.Fatalf("warm lookup: memo=%v verb=%p, cold=%p", usedMemo, verb, cold)
					}
				}
			}
			check("return 1;")
			if ec := s.setVerbCodeByIndex(0, 1, []string{"return 2;"}); ec != types.E_NONE {
				t.Fatalf("edit second verb: %v", ec)
			}
			check("return 2;")
			if ec := s.DeleteVerb(0, "first"); ec != types.E_NONE {
				t.Fatalf("delete first verb: %v", ec)
			}
			check("return 2;")
		})
	}
}

// A second transaction resolves through the store memo: same verb and
// definer, no per-ancestor scan marks, a read mark on the definer's verb, and
// the txn-level dependency flag.
func TestVerbDispatchMemoHitSkipsAncestryScans(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	s.resetVerbDispatchMemoForTest()

	first := s.BeginSnapshot(0)
	v1, d1, err := first.findVerb(2, "look", false)
	if err != nil || d1 != 0 {
		t.Fatalf("walk: %v definer=#%d", err, d1)
	}
	if first.usedVerbMemo {
		t.Fatalf("first resolution should walk, not hit")
	}
	if !memoHas(s, 2, "look") {
		t.Fatalf("walk did not populate the store memo")
	}
	first.Release()

	second := s.BeginSnapshot(0)
	defer second.Release()
	v2, d2, err := second.findVerb(2, "look", false)
	if err != nil || d2 != d1 || v2 != v1 {
		t.Fatalf("memo hit = (%p,#%d,%v), want (%p,#%d)", v2, d2, err, v1, d1)
	}
	if !second.usedVerbMemo {
		t.Fatalf("second resolution did not use the memo")
	}
	if len(second.verbScans) != 0 {
		t.Fatalf("memo hit recorded ancestry scans: %v", second.verbScans)
	}
	if _, ok := second.verbReads[verbReadKey{objID: 0, name: v1.mapKey()}]; !ok {
		t.Fatalf("memo hit did not mark the resolved verb read: %v", second.verbReads)
	}
	if ec := second.Commit(); ec != types.E_NONE {
		t.Fatalf("commit after memo hit: %v", ec)
	}
}

// Shape changes are seen by the next transaction: a shadowing verb on the
// child, a rename, and a reparent all change the resolution.
func TestVerbDispatchMemoFollowsShapeChanges(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)

	resolve := func() (types.ObjID, error) {
		tx := s.BeginSnapshot(0)
		defer tx.Release()
		_, definer, err := tx.findVerb(2, "look", false)
		return definer, err
	}
	warm := func() {
		resolve()
		resolve()
	}

	warm()
	if d, _ := resolve(); d != 0 {
		t.Fatalf("baseline definer #%d, want #0", d)
	}

	addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute)
	if d, _ := resolve(); d != 1 {
		t.Fatalf("after shadowing add, definer #%d, want #1", d)
	}
	warm()

	if ec := s.SetVerbInfo(1, "look", 0, VerbRead|VerbExecute, []string{"peek"}); ec != types.E_NONE {
		t.Fatalf("SetVerbInfo: %v", ec)
	}
	if d, _ := resolve(); d != 0 {
		t.Fatalf("after rename, definer #%d, want #0", d)
	}
	warm()

	if ec := s.DeleteVerb(0, "look"); ec != types.E_NONE {
		t.Fatalf("DeleteVerb: %v", ec)
	}
	if _, err := resolve(); err == nil {
		t.Fatalf("after delete, look still resolves")
	}
	warm()

	other, ec := s.DirectTxn().CreateObject(nil, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject: %v", ec)
	}
	addVerbT(t, s, other, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.ChangeParents(2, []types.ObjID{other}); ec != types.E_NONE {
		t.Fatalf("ChangeParents: %v", ec)
	}
	if d, err := resolve(); err != nil || d != other {
		t.Fatalf("after chparent, definer #%d (%v), want #%d", d, err, other)
	}
}

// A verb called in a loop resolves through the memo every time. What the txn
// must hold for that resolution is recorded once: the hit list must not grow
// per call, and a txn that starts writing still gets the walk's scan marks for
// a verb it had already resolved before its first write.
func TestVerbDispatchMemoHitIsRecordedOncePerTxn(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.DirectTxn().DefineProperty(2, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", false)
	warm.Release()

	user := s.BeginSnapshot(0)
	defer user.Release()
	for i := 0; i < 100; i++ {
		if _, definer, err := user.findVerb(2, "look", false); err != nil || definer != 0 {
			t.Fatalf("reading lookup %d: definer=%d err=%v", i, definer, err)
		}
	}
	if !user.usedVerbMemo || len(user.verbMemoHits) != 1 {
		t.Fatalf("100 memo hits recorded %d list entries (used=%v), want 1", len(user.verbMemoHits), user.usedVerbMemo)
	}
	if len(user.verbScans) != 0 {
		t.Fatalf("a txn without writes recorded scan marks: %v", user.verbScans)
	}

	if ec := user.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	for i := 0; i < 100; i++ {
		if _, definer, err := user.findVerb(2, "look", false); err != nil || definer != 0 {
			t.Fatalf("writing lookup %d: definer=%d err=%v", i, definer, err)
		}
	}
	for _, id := range []types.ObjID{0, 1, 2} {
		if _, marked := user.verbScans[id]; !marked {
			t.Fatalf("writing txn holds no scan mark for #%d on the walk path: %v", id, user.verbScans)
		}
	}
	if len(user.verbMemoHits) != 1 {
		t.Fatalf("hit list grew to %d after the txn started writing", len(user.verbMemoHits))
	}
}

// memoUserTxn returns a store whose (2, "look") resolution, defined on #0, is
// already in the dispatch memo, and a fresh transaction to resolve it through.
func memoUserTxn(t *testing.T) (*Store, *StoreTxn) {
	t.Helper()
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.DirectTxn().DefineProperty(2, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", true)
	warm.findVerb(2, "nosuch", true)
	warm.Release()
	user := s.BeginSnapshot(0)
	t.Cleanup(user.Release)
	return s, user
}

// lookTimes resolves (2, "look") n times and returns the last answer.
func lookTimes(t *testing.T, tx *StoreTxn, n int) (*Verb, types.ObjID) {
	t.Helper()
	var verb *Verb
	var definer types.ObjID
	for i := 0; i < n; i++ {
		var err error
		if verb, definer, err = tx.findVerb(2, "look", true); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	return verb, definer
}

// Repeat hits are answered from the txn's own record of the first. They must
// leave exactly the read set the first hit produced, and a txn that resolved
// this way must still lose its commit to a concurrent edit of the verb's code.
func TestVerbDispatchMemoRepeatHitKeepsVerbReadMark(t *testing.T) {
	s, user := memoUserTxn(t)
	first, _ := lookTimes(t, user, 1)
	want := snapshotReadSet(user)
	if _, marked := want.verbReads[verbReadKey{objID: 0, name: first.mapKey()}]; !marked {
		t.Fatalf("first hit left no read mark: %v", want.verbReads)
	}
	if again, definer := lookTimes(t, user, 50); again != first || definer != 0 {
		t.Fatalf("repeat hit = (%p, #%d), want (%p, #0)", again, definer, first)
	}
	requireSameReadSet(t, "repeat hits", want, snapshotReadSet(user))

	if ec := s.setVerbCodeByIndex(0, 0, []string{"return 2;"}); ec != types.E_NONE {
		t.Fatalf("concurrent code edit: %v", ec)
	}
	if again, _ := lookTimes(t, user, 1); again != first || again.code[0] != "return 1;" {
		t.Fatalf("snapshot saw a later code edit: %v", again.code)
	}
	if ec := user.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if ec := user.Commit(); ec == types.E_NONE {
		t.Fatal("commit ignored a concurrent edit of a verb resolved through repeat memo hits")
	}
}

// A verb added on a nearer ancestor, or a reparent, that commits while a txn is
// resolving through its memo record is not seen by the snapshot and fails the
// txn's commit, however many repeat hits came before and after it.
func TestVerbDispatchMemoRepeatHitConflictsWithShapeChange(t *testing.T) {
	for _, change := range []string{"nearer verb", "chparent"} {
		t.Run(change, func(t *testing.T) {
			s, user := memoUserTxn(t)
			lookTimes(t, user, 20)
			if !user.usedVerbMemo || len(user.verbScans) != 0 {
				t.Fatalf("expected clock-validated hits, got used=%v scans=%v", user.usedVerbMemo, user.verbScans)
			}

			switch change {
			case "nearer verb":
				addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute)
			case "chparent":
				other, ec := s.DirectTxn().CreateObject(nil, 0, false)
				if ec != types.E_NONE {
					t.Fatalf("CreateObject: %v", ec)
				}
				addVerbT(t, s, other, []string{"look"}, VerbRead|VerbExecute)
				if ec := s.ChangeParents(2, []types.ObjID{other}); ec != types.E_NONE {
					t.Fatalf("ChangeParents: %v", ec)
				}
			}
			if _, definer := lookTimes(t, user, 5); definer != 0 {
				t.Fatalf("snapshot resolved to #%d after a concurrent %s, want #0", definer, change)
			}
			// Past the shape change the memo no longer speaks for this snapshot,
			// nor does the txn's record of it: the lookups above walked.
			for _, id := range []types.ObjID{0, 1, 2} {
				if _, marked := user.verbScans[id]; !marked {
					t.Fatalf("lookup after a concurrent %s left no scan mark on #%d: %v", change, id, user.verbScans)
				}
			}
			if ec := user.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
				t.Fatalf("SetPropertyValue: %v", ec)
			}
			if _, definer := lookTimes(t, user, 5); definer != 0 {
				t.Fatalf("writing snapshot resolved to #%d after a concurrent %s, want #0", definer, change)
			}
			if ec := user.Commit(); ec == types.E_NONE {
				t.Fatalf("commit ignored a concurrent %s on the dispatch path", change)
			}
		})
	}
}

// A writing txn gets the walk's scan marks from its first hit and keeps exactly
// those through repeat hits, so a verb added on a scanned ancestor meanwhile
// fails its commit.
func TestVerbDispatchMemoRepeatHitWritingTxnKeepsScanMarks(t *testing.T) {
	s, user := memoUserTxn(t)
	reference := referenceVerbReadSet(t, s, 2, "look")
	if ec := user.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	for i := 0; i < 3; i++ {
		lookTimes(t, user, 20)
		got := snapshotReadSet(user)
		if !maps.Equal(got.verbScans, reference.verbScans) || !maps.Equal(got.verbReads, reference.verbReads) {
			t.Fatalf("pass %d: scans=%v reads=%v, want scans=%v reads=%v", i, got.verbScans, got.verbReads, reference.verbScans, reference.verbReads)
		}
	}
	if user.usedVerbMemo {
		t.Fatal("writing txn took a clock-validated memo resolution")
	}
	addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute)
	if _, definer := lookTimes(t, user, 5); definer != 0 {
		t.Fatalf("snapshot resolved to #%d after a concurrent verb add, want #0", definer)
	}
	if ec := user.Commit(); ec == types.E_NONE {
		t.Fatal("commit ignored a verb added on a scanned ancestor")
	}
}

// The txn's record of a hit must not outlive the txn's own changes to what it
// resolved: staged code is read back, also when the definer was already
// private, and a staged delete uncovers the next definition.
func TestVerbDispatchMemoRepeatHitSeesOwnStagedVerbChanges(t *testing.T) {
	s, user := memoUserTxn(t)
	shared, _ := lookTimes(t, user, 10)

	if ec := user.SetVerbCode(2, "look", []string{"return 2;"}); ec != types.E_NONE {
		t.Fatalf("SetVerbCode: %v", ec)
	}
	private, definer := lookTimes(t, user, 10)
	if private == shared || definer != 0 || private.code[0] != "return 2;" {
		t.Fatalf("after staged code write: shared=%v definer=#%d code=%v", private == shared, definer, private.code)
	}
	// #0 is private now, so this write replaces no binding in the txn.
	if ec := user.SetVerbCode(2, "look", []string{"return 3;"}); ec != types.E_NONE {
		t.Fatalf("second SetVerbCode: %v", ec)
	}
	if again, _ := lookTimes(t, user, 10); again.code[0] != "return 3;" {
		t.Fatalf("after second staged code write: code=%v", again.code)
	}
	if ec := user.Commit(); ec != types.E_NONE {
		t.Fatalf("commit: %v", ec)
	}

	addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute)
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", true)
	warm.Release()
	deleter := s.BeginSnapshot(0)
	defer deleter.Release()
	if _, definer := lookTimes(t, deleter, 10); definer != 1 {
		t.Fatalf("nearer definition: definer #%d, want #1", definer)
	}
	resolved, err := deleter.ResolveVerbOnObject(1, "look")
	if err != nil {
		t.Fatalf("ResolveVerbOnObject: %v", err)
	}
	if ec := deleter.DeleteResolvedVerb(resolved); ec != types.E_NONE {
		t.Fatalf("DeleteResolvedVerb: %v", ec)
	}
	if verb, definer := lookTimes(t, deleter, 10); definer != 0 || verb.code[0] != "return 3;" {
		t.Fatalf("after staged delete: definer #%d code=%v, want #0 return 3;", definer, verb.code)
	}
}

// A task's own live verb-shape change (add_verb, chparent) is visible to its
// next call even though earlier calls were answered from the memo record.
func TestVerbDispatchMemoRepeatHitSeesOwnLiveShapeChange(t *testing.T) {
	s, user := memoUserTxn(t)
	lookTimes(t, user, 10)
	for i := 0; i < 10; i++ {
		if _, _, err := user.findVerb(2, "nosuch", true); err == nil || err.Error() != "verb not found: nosuch" {
			t.Fatalf("missing verb lookup %d: err=%v", i, err)
		}
	}

	user.PrepareLiveMutation()
	addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute)
	addVerbT(t, s, 1, []string{"nosuch"}, VerbRead|VerbExecute)
	user.MarkLiveMutated()
	if ec := user.AdoptLiveVerbs(1); ec != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs: %v", ec)
	}
	if _, definer := lookTimes(t, user, 3); definer != 1 {
		t.Fatalf("after own add_verb on a nearer ancestor: definer #%d, want #1", definer)
	}
	if _, definer, err := user.findVerb(2, "nosuch", true); err != nil || definer != 1 {
		t.Fatalf("after own add_verb of a missing verb: definer #%d err=%v", definer, err)
	}

	other, ec := s.DirectTxn().CreateObject(nil, 0, false)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject: %v", ec)
	}
	addVerbT(t, s, other, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.ChangeParents(2, []types.ObjID{other}); ec != types.E_NONE {
		t.Fatalf("ChangeParents: %v", ec)
	}
	user.MarkLiveMutated()
	if ec := user.AdoptLiveObject(other); ec != types.E_NONE {
		t.Fatalf("AdoptLiveObject: %v", ec)
	}
	if ec := user.AdoptLiveRelationships(2, 1, other); ec != types.E_NONE {
		t.Fatalf("AdoptLiveRelationships: %v", ec)
	}
	if _, definer := lookTimes(t, user, 3); definer != other {
		t.Fatalf("after own chparent: definer #%d, want #%d", definer, other)
	}
}

// A transaction that dispatched through the memo loses validation if a
// verb-shape change committed after its snapshot.
func TestVerbDispatchMemoUserConflictsWithShapeChange(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.DirectTxn().DefineProperty(2, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", false)
	warm.Release()

	user := s.BeginSnapshot(0)
	defer user.Release()
	if _, _, err := user.findVerb(2, "look", false); err != nil {
		t.Fatalf("findVerb: %v", err)
	}
	if !user.usedVerbMemo {
		t.Fatalf("expected a memo hit")
	}
	addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute) // concurrent shape change
	if ec := user.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if ec := user.Commit(); ec != types.E_INVARG {
		t.Fatalf("commit after concurrent verb add = %v, want E_INVARG", ec)
	}

	// Without a shape change the same pattern commits.
	user2 := s.BeginSnapshot(0)
	defer user2.Release()
	user2.findVerb(2, "look", false)
	if ec := user2.SetPropertyValue(2, "p", types.NewInt(2)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if ec := user2.Commit(); ec != types.E_NONE {
		t.Fatalf("commit without shape change = %v", ec)
	}
}

// Anonymous objects are never memoized; missing verbs are.
func TestVerbDispatchMemoSkipsAnonymousAndCachesMisses(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	s.resetVerbDispatchMemoForTest()

	anon, ec := s.DirectTxn().CreateObject([]types.ObjID{0}, 0, true)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject anon: %v", ec)
	}
	tx := s.BeginSnapshot(0)
	if _, d, err := tx.findVerb(anon, "look", false); err != nil || d != 0 {
		t.Fatalf("anon findVerb: %v definer=#%d", err, d)
	}
	if _, _, err := tx.findVerb(2, "nosuch", false); err == nil {
		t.Fatalf("nosuch resolved")
	}
	tx.Release()
	if memoHas(s, anon, "look") {
		t.Fatalf("anonymous object was memoized")
	}
	if !memoHas(s, 2, "nosuch") {
		t.Fatalf("negative resolution was not memoized")
	}

	tx2 := s.BeginSnapshot(0)
	defer tx2.Release()
	if _, _, err := tx2.findVerb(2, "nosuch", false); err == nil {
		t.Fatalf("memoized miss resolved")
	}
	if !tx2.usedVerbMemo {
		t.Fatalf("negative entry was not used")
	}
	addVerbT(t, s, 2, []string{"nosuch"}, VerbRead|VerbExecute)
	tx3 := s.BeginSnapshot(0)
	defer tx3.Release()
	if _, d, err := tx3.findVerb(2, "nosuch", false); err != nil || d != 2 {
		t.Fatalf("after add, nosuch = %v definer=#%d", err, d)
	}
}

// A task that dispatched through the memo and then mutates the live store
// itself (a coarse builtin) must not be condemned by its own clock bump.
// PrepareLiveMutation re-walks the memoized resolutions into ordinary scan
// marks on the snapshot, after which the coarse commit validates those marks.
func TestVerbDispatchMemoSurvivesOwnLiveMutation(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.DirectTxn().DefineProperty(2, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", false)
	warm.Release()

	user := s.BeginSnapshot(0)
	defer user.Release()
	if _, _, err := user.findVerb(2, "look", false); err != nil {
		t.Fatalf("findVerb: %v", err)
	}
	if !user.usedVerbMemo || len(user.verbScans) != 0 {
		t.Fatalf("expected a mark-less memo hit, got used=%v scans=%v", user.usedVerbMemo, user.verbScans)
	}

	// What flushStagedBeforeCoarse does before add_verb touches the live store.
	user.PrepareLiveMutation()
	if user.usedVerbMemo {
		t.Fatalf("PrepareLiveMutation left the txn on the memo")
	}
	for _, id := range []types.ObjID{2, 1, 0} {
		if _, ok := user.verbScans[id]; !ok {
			t.Fatalf("PrepareLiveMutation did not materialize the scan mark on #%d: %v", id, user.verbScans)
		}
	}
	// Later resolutions in this txn walk and mark; they never re-enter the memo.
	if _, _, err := user.findVerb(2, "look", true); err != nil || user.usedVerbMemo {
		t.Fatalf("post-prepare resolution: err=%v used=%v", err, user.usedVerbMemo)
	}

	// The task's own coarse mutation moves the shape clock, then the builtin
	// marks the txn live-mutated and adopts the changed facet.
	addVerbT(t, s, 0, []string{"own_new_verb"}, VerbRead|VerbExecute)
	user.MarkLiveMutated()
	if ec := user.AdoptLiveVerbs(0); ec != types.E_NONE {
		t.Fatalf("AdoptLiveVerbs: %v", ec)
	}
	if ec := user.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if ec := user.Commit(); ec != types.E_NONE {
		t.Fatalf("coarse commit after own verb-shape change = %v, want E_NONE", ec)
	}
}

// The materialized marks still catch a concurrent shape change that lands
// between the memo hit and the coarse commit.
func TestVerbDispatchMemoMaterializedMarksCatchConcurrentChange(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.DirectTxn().DefineProperty(2, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", false)
	warm.Release()

	user := s.BeginSnapshot(0)
	defer user.Release()
	if _, _, err := user.findVerb(2, "look", false); err != nil || !user.usedVerbMemo {
		t.Fatalf("expected a memo hit, err=%v", err)
	}
	user.PrepareLiveMutation()
	addVerbT(t, s, 1, []string{"look"}, VerbRead|VerbExecute) // another task shadows the verb
	user.MarkLiveMutated()
	if ec := user.SetPropertyValue(2, "p", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
	if ec := user.Commit(); ec == types.E_NONE {
		t.Fatalf("coarse commit ignored a concurrent verb-shape change on a scanned ancestor")
	}
}

// A live-mutated txn never resolves through the memo: the coarse path
// validates scan marks, not the clock.
func TestVerbDispatchMemoSkippedAfterLiveMutation(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	warm := s.BeginSnapshot(0)
	warm.findVerb(2, "look", false)
	warm.Release()

	user := s.BeginSnapshot(0)
	defer user.Release()
	user.MarkLiveMutated()
	if _, _, err := user.findVerb(2, "look", false); err != nil {
		t.Fatalf("findVerb: %v", err)
	}
	if user.usedVerbMemo || len(user.verbScans) == 0 {
		t.Fatalf("live-mutated txn used the memo (used=%v scans=%v)", user.usedVerbMemo, user.verbScans)
	}
}
