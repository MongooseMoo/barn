package store

import (
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

// A second transaction resolves through the store memo: same verb and
// definer, no per-ancestor scan marks, a read mark on the definer's verb, and
// the txn-level dependency flag.
func TestVerbDispatchMemoHitSkipsAncestryScans(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	s.resetVerbDispatchMemoForTest()

	first := s.BeginReadOnly(0)
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

	second := s.BeginReadOnly(0)
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
		tx := s.BeginReadOnly(0)
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

// A transaction that dispatched through the memo loses validation if a
// verb-shape change committed after its snapshot.
func TestVerbDispatchMemoUserConflictsWithShapeChange(t *testing.T) {
	s := testChainStore(t)
	addVerbT(t, s, 0, []string{"look"}, VerbRead|VerbExecute)
	if ec := s.DirectTxn().DefineProperty(2, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	warm := s.BeginReadOnly(0)
	warm.findVerb(2, "look", false)
	warm.Release()

	user := s.BeginReadOnly(0)
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
	user2 := s.BeginReadOnly(0)
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
	tx := s.BeginReadOnly(0)
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

	tx2 := s.BeginReadOnly(0)
	defer tx2.Release()
	if _, _, err := tx2.findVerb(2, "nosuch", false); err == nil {
		t.Fatalf("memoized miss resolved")
	}
	if !tx2.usedVerbMemo {
		t.Fatalf("negative entry was not used")
	}
	addVerbT(t, s, 2, []string{"nosuch"}, VerbRead|VerbExecute)
	tx3 := s.BeginReadOnly(0)
	defer tx3.Release()
	if _, d, err := tx3.findVerb(2, "nosuch", false); err != nil || d != 2 {
		t.Fatalf("after add, nosuch = %v definer=#%d", err, d)
	}
}
