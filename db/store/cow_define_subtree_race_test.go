package store

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestCOWConcurrentDefineDeleteSubtreeRaceFree is the COW Phase 2 go/no-go race +
// correctness gate.
//
// Property DEFINE / DEFINITION-DELETE on an object O propagate to O's whole inheriting
// DESCENDANT subtree (a clear inherited slot is seeded on / removed from every
// descendant). Phase 2 moves these onto the decentralized COW publish path, so the
// committer must lock and atomically publish a NEW immutable image for O AND every
// inheriting descendant. This test stresses exactly that:
//
//   - Each of nSubtrees writers owns a DISJOINT subtree (root + a chain/fan of
//     descendants). It repeatedly DEFINEs a uniquely-named property on the subtree root
//     and then DELETEs that definition, each through StoreTxn.Commit (decentralized COW
//     path, !liveMutated). Between the define and the delete it ASSERTS that every
//     descendant resolves the inherited value (no LOST/torn define), and after the delete
//     that every descendant no longer resolves it (no LOST delete).
//   - Reader goroutines hammer the RAW Store.* reader funnel (FindProperty / PropertyValue
//     on the descendants' inherited slots, plus ObjectName / Parents) on the SAME objects
//     concurrently with the publishes. A raw reader must only ever observe a COMPLETE
//     image: either the property fully present (inherited value correct) on every
//     descendant or fully absent — never a half-applied subtree on one descendant.
//   - A separate set of disjoint writers commits DISJOINT property-value writes on
//     unrelated objects, proving define/delete commits run concurrently with ordinary
//     decentralized commits without interference.
//
// Under COW every commit publishes new immutable images and never mutates the old ones a
// reader Loaded, so this MUST be clean under -race AND the descendant-inheritance
// assertions must hold. Run with:
//
//	go test -race -run TestCOWConcurrentDefineDeleteSubtreeRaceFree ./db/store
func TestCOWConcurrentDefineDeleteSubtreeRaceFree(t *testing.T) {
	const (
		nSubtrees   = 8
		depthPerArm = 3 // chain depth below each root
		armsPerRoot = 2 // fan-out: two chains per root
		roundsEach  = 120
		nReaders    = 16
		readsEach   = 4000
		nDisjoint   = 8
		commitsEach = 200
	)

	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}

	createChild := func(parent types.ObjID) types.ObjID {
		id, errCode := store.CreateObject([]types.ObjID{parent}, 0, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject(parent=%d) failed: %v", parent, errCode)
		}
		return id
	}

	// Build nSubtrees disjoint subtrees. roots[i] has armsPerRoot chains of depthPerArm
	// descendants each. descendants[i] lists EVERY inheriting descendant of roots[i].
	roots := make([]types.ObjID, nSubtrees)
	descendants := make([][]types.ObjID, nSubtrees)
	for i := 0; i < nSubtrees; i++ {
		root := createChild(0)
		roots[i] = root
		for a := 0; a < armsPerRoot; a++ {
			cur := root
			for d := 0; d < depthPerArm; d++ {
				cur = createChild(cur)
				descendants[i] = append(descendants[i], cur)
			}
		}
	}

	// Disjoint objects for ordinary property-value commits.
	disjoint := make([]types.ObjID, nDisjoint)
	for i := 0; i < nDisjoint; i++ {
		id := createChild(0)
		if errCode := store.DefineProperty(id, "counter", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("DefineProperty counter failed: %v", errCode)
		}
		disjoint[i] = id
	}

	var wg sync.WaitGroup
	var stop atomic.Bool

	// Subtree define/delete writers. Each owns roots[i]; the property name is unique per
	// subtree so two subtree writers never share a footprint and the inheritance is
	// deterministic for this writer's own assertions.
	for i := 0; i < nSubtrees; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			root := roots[i]
			propName := "sub" + string(rune('A'+i))
			want := int64(1000 + i)
			for r := 0; r < roundsEach; r++ {
				// DEFINE on the root via the decentralized COW path.
				txd := store.BeginReadOnly(0)
				if errCode := txd.DefineProperty(root, propName, NewProperty(types.NewInt(want), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
					t.Errorf("subtree %d DefineProperty failed: %v", i, errCode)
					return
				}
				if errCode := txd.Commit(); errCode != types.E_NONE {
					t.Errorf("subtree %d define Commit failed: %v", i, errCode)
					return
				}
				// Every descendant must now inherit the defined value (no lost/torn define).
				for _, dID := range descendants[i] {
					pv, errCode := store.PropertyValue(dID, propName)
					if errCode != types.E_NONE {
						t.Errorf("subtree %d descendant #%d PropertyValue(%s) after define = %v, want value", i, dID, propName, errCode)
						return
					}
					if pv.Type() != types.TYPE_INT || pv.Int() != want {
						t.Errorf("subtree %d descendant #%d inherited %v after define, want %d", i, dID, pv, want)
						return
					}
				}

				// DELETE the definition via the decentralized COW path.
				txx := store.BeginReadOnly(0)
				if errCode := txx.DeleteDefinedProperty(root, propName); errCode != types.E_NONE {
					t.Errorf("subtree %d DeleteDefinedProperty failed: %v", i, errCode)
					return
				}
				if errCode := txx.Commit(); errCode != types.E_NONE {
					t.Errorf("subtree %d delete Commit failed: %v", i, errCode)
					return
				}
				// Every descendant must no longer resolve the property (no lost delete).
				for _, dID := range descendants[i] {
					if _, errCode := store.PropertyValue(dID, propName); errCode != types.E_PROPNF {
						t.Errorf("subtree %d descendant #%d PropertyValue(%s) after delete = %v, want E_PROPNF", i, dID, propName, errCode)
						return
					}
				}
			}
		}(i)
	}

	// Disjoint ordinary property-value committers.
	for w := 0; w < nDisjoint; w++ {
		wg.Add(1)
		go func(id types.ObjID) {
			defer wg.Done()
			for c := 0; c < commitsEach; c++ {
				tx := store.BeginReadOnly(0)
				if errCode := tx.SetPropertyValue(id, "counter", types.NewInt(int64(c))); errCode != types.E_NONE {
					t.Errorf("disjoint SetPropertyValue failed: %v", errCode)
					return
				}
				if errCode := tx.Commit(); errCode != types.E_NONE {
					t.Errorf("disjoint Commit failed: %v", errCode)
					return
				}
			}
		}(disjoint[w])
	}

	// Raw readers: hammer the txn==nil reader funnel on the descendants' inherited slots
	// while the publishes happen. We do not assert a specific resolved value here (the
	// owning writer races define<->delete so the slot legitimately flips between present
	// and E_PROPNF); the assertion is the -race detector: a reader must never observe a
	// torn image. We DO assert that whenever the property IS present, its value is the
	// single correct inherited value for that subtree (never a corrupted/partial value).
	for r := 0; r < nReaders; r++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < readsEach && !stop.Load(); i++ {
				s := (seed + i) % nSubtrees
				want := int64(1000 + s)
				propName := "sub" + string(rune('A'+s))
				for _, dID := range descendants[s] {
					pv, errCode := store.PropertyValue(dID, propName)
					if errCode == types.E_NONE {
						if pv.Type() != types.TYPE_INT || pv.Int() != want {
							t.Errorf("reader saw torn inherited value on #%d: %v want absent-or-%d", dID, pv, want)
							return
						}
					}
					_, _ = store.ObjectName(dID)
					_, _ = store.Parents(dID)
				}
			}
		}(r)
	}

	wg.Wait()
	stop.Store(true)

	// Final state: each define/delete writer ends on a DELETE, so no subtree property
	// should resolve on any descendant, and every object is still readable (no corruption).
	for i := 0; i < nSubtrees; i++ {
		propName := "sub" + string(rune('A'+i))
		for _, dID := range descendants[i] {
			if _, errCode := store.PropertyValue(dID, propName); errCode != types.E_PROPNF {
				t.Fatalf("post-run descendant #%d still resolves %s: %v, want E_PROPNF", dID, propName, errCode)
			}
			if _, errCode := store.ObjectName(dID); errCode != types.E_NONE {
				t.Fatalf("post-run ObjectName(#%d) failed: %v", dID, errCode)
			}
		}
		if _, errCode := store.ObjectName(roots[i]); errCode != types.E_NONE {
			t.Fatalf("post-run ObjectName(root #%d) failed: %v", roots[i], errCode)
		}
	}
	for _, id := range disjoint {
		if _, errCode := store.PropertyValue(id, "counter"); errCode != types.E_NONE {
			t.Fatalf("post-run disjoint PropertyValue(#%d) failed: %v", id, errCode)
		}
	}
}
