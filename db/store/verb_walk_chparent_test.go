package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// The objects of chparentProbeStore.
const (
	probeRoot  types.ObjID = 0 // parent of va and vb, defines nothing
	probeVA    types.ObjID = 1 // defines probe
	probeVB    types.ObjID = 2 // defines a different probe
	probeVC    types.ObjID = 3 // child of va, defines no probe
	probeLog   types.ObjID = 4 // unrelated, holds the property the reader writes
	probeChild types.ObjID = 5 // child of vc
)

// chparentProbeStore builds the dispatch shape a reparent changes: vc inherits
// probe from va, and vb defines its own. The store's dispatch memo is empty.
func chparentProbeStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore()
	for id := probeRoot; id <= probeChild; id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			t.Fatalf("Add #%d: %v", id, err)
		}
	}
	for _, edge := range [][2]types.ObjID{{probeVA, probeRoot}, {probeVB, probeRoot}, {probeVC, probeVA}, {probeChild, probeVC}} {
		if ec := s.ChangeParents(edge[0], []types.ObjID{edge[1]}); ec != types.E_NONE {
			t.Fatalf("ChangeParents #%d: %v", edge[0], ec)
		}
	}
	addVerbT(t, s, probeVA, []string{"probe"}, VerbRead|VerbExecute)
	addVerbT(t, s, probeVB, []string{"probe"}, VerbRead|VerbExecute)
	if ec := s.DirectTxn().DefineProperty(probeLog, "log", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	s.resetVerbDispatchMemoForTest()
	return s
}

// warmProbeMemo fills the store's dispatch memo for start:probe.
func warmProbeMemo(t *testing.T, s *Store, start types.ObjID) {
	t.Helper()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if _, definer, err := tx.FindCallableVerb(start, "probe"); err != nil || definer != probeVA {
		t.Fatalf("warming walk: definer=#%d err=%v", definer, err)
	}
	if !memoHas(s, start, "probe") {
		// memoHas keys by requireExecute=false; a callable lookup files its own key.
		if _, ok := s.verbMemo().Load(verbResolveKey{objID: start, name: "probe", requireExecute: true}); !ok {
			t.Fatalf("warming walk left no memo entry for #%d:probe", start)
		}
	}
}

func stageLogWrite(t *testing.T, tx *StoreTxn) {
	t.Helper()
	if ec := tx.SetPropertyValue(probeLog, "log", types.NewInt(1)); ec != types.E_NONE {
		t.Fatalf("SetPropertyValue: %v", ec)
	}
}

func reparentVC(t *testing.T, s *Store) {
	t.Helper()
	if ec := s.ChangeParents(probeVC, []types.ObjID{probeVB}); ec != types.E_NONE {
		t.Fatalf("ChangeParents: %v", ec)
	}
}

func requireProbeFrom(t *testing.T, definer types.ObjID, err error, want types.ObjID) {
	t.Helper()
	if err != nil || definer != want {
		t.Fatalf("probe resolved to #%d (err=%v), want #%d", definer, err, want)
	}
}

// A transaction that resolved vc:probe() by walking the ancestry, then wrote,
// must not commit after another commit reparented an object on that walk: its
// call dispatched to a verb the lookup no longer finds.
func TestVerbWalkConflictsWithConcurrentChparent(t *testing.T) {
	for _, start := range []types.ObjID{probeVC, probeChild} {
		t.Run(map[types.ObjID]string{probeVC: "receiver", probeChild: "descendant"}[start], func(t *testing.T) {
			s := chparentProbeStore(t)
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			_, definer, err := tx.FindCallableVerb(start, "probe")
			requireProbeFrom(t, definer, err, probeVA)
			if tx.usedVerbMemo {
				t.Fatalf("lookup on a cold memo used the memo")
			}
			stageLogWrite(t, tx)

			reparentVC(t, s)

			if ec := tx.Commit(); ec == types.E_NONE {
				t.Fatalf("commit succeeded after a concurrent chparent changed what #%d:probe() dispatches to", start)
			}
		})
	}
}

// A transaction with staged writes takes a memo hit as the walk's scan marks,
// not as the clock dependency; those marks must see the reparent too.
func TestVerbMemoHitWithStagedWritesConflictsWithConcurrentChparent(t *testing.T) {
	s := chparentProbeStore(t)
	warmProbeMemo(t, s, probeVC)

	tx := s.BeginSnapshot(0)
	defer tx.Release()
	stageLogWrite(t, tx)
	_, definer, err := tx.FindCallableVerb(probeVC, "probe")
	requireProbeFrom(t, definer, err, probeVA)
	if tx.usedVerbMemo || len(tx.verbScans) == 0 {
		t.Fatalf("expected the memo path replayed as scan marks, got used=%v scans=%v", tx.usedVerbMemo, tx.verbScans)
	}

	reparentVC(t, s)

	if ec := tx.Commit(); ec == types.E_NONE {
		t.Fatalf("commit succeeded after a concurrent chparent changed what the memoized dispatch finds")
	}
}

// PrepareLiveMutation turns memo hits into scan marks for the coarse commit;
// those marks must see a reparent that lands before the commit.
func TestVerbMemoMaterializedMarksConflictWithConcurrentChparent(t *testing.T) {
	s := chparentProbeStore(t)
	warmProbeMemo(t, s, probeVC)

	tx := s.BeginSnapshot(0)
	defer tx.Release()
	_, definer, err := tx.FindCallableVerb(probeVC, "probe")
	requireProbeFrom(t, definer, err, probeVA)
	if !tx.usedVerbMemo {
		t.Fatalf("expected a memo hit")
	}
	tx.PrepareLiveMutation()

	reparentVC(t, s) // another task's chparent
	tx.MarkLiveMutated()
	stageLogWrite(t, tx)

	if ec := tx.Commit(); ec == types.E_NONE {
		t.Fatalf("coarse commit ignored a concurrent chparent of a scanned object")
	}
}

// pass() resolves from the parents of the verb's definer; reparenting the
// definer changes what it finds.
func TestParentVerbLookupConflictsWithConcurrentChparent(t *testing.T) {
	s := chparentProbeStore(t)
	addVerbT(t, s, probeVC, []string{"probe"}, VerbRead|VerbExecute)

	tx := s.BeginSnapshot(0)
	defer tx.Release()
	_, definer, err := tx.FindParentVerb(probeVC, "probe")
	requireProbeFrom(t, definer, err, probeVA)
	stageLogWrite(t, tx)

	reparentVC(t, s)

	if ec := tx.Commit(); ec == types.E_NONE {
		t.Fatalf("commit succeeded after a concurrent chparent changed what pass() from #%d finds", probeVC)
	}
}

// A walk depends only on the objects it visited: reparenting an object off the
// walked chain leaves the transaction free to commit.
func TestVerbWalkIgnoresChparentOffItsChain(t *testing.T) {
	s := chparentProbeStore(t)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	_, definer, err := tx.FindCallableVerb(probeVA, "probe")
	requireProbeFrom(t, definer, err, probeVA)
	if tx.usedVerbMemo {
		t.Fatalf("lookup on a cold memo used the memo")
	}
	stageLogWrite(t, tx)

	reparentVC(t, s) // vc is below the walked object, not on its chain

	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatalf("commit = %v after a chparent off the walked chain, want E_NONE", ec)
	}
}
