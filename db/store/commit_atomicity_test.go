package store

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func atomicityFixture(t *testing.T, objects int) *Store {
	t.Helper()
	s := NewStore()
	for id := types.ObjID(0); int(id) < objects; id++ {
		if err := s.Add(NewObject(id, 0)); err != nil {
			t.Fatal(err)
		}
		if code := s.DirectTxn().DefineProperty(id, "n", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); code != types.E_NONE {
			t.Fatal(code)
		}
	}
	return s
}

// One commit writes the same value to every object. A snapshot, whenever it
// begins, must see all of that commit or none of it: every object it reads
// carries the same value.
func TestSnapshotNeverSeesPartOfACommit(t *testing.T) {
	const objects = 32
	s := atomicityFixture(t, objects)

	var stop atomic.Bool
	var torn atomic.Pointer[[2]int64]
	var snapshots atomic.Int64
	var readers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for !stop.Load() {
				tx := s.BeginSnapshot(0)
				first, code := tx.PropertyValue(0, "n")
				for id := types.ObjID(1); code == types.E_NONE && id < objects; id++ {
					var v types.Value
					if v, code = tx.PropertyValue(id, "n"); code == types.E_NONE && !v.Identical(first) {
						torn.CompareAndSwap(nil, &[2]int64{first.Int(), v.Int()})
					}
				}
				tx.Release()
				if code != types.E_NONE {
					t.Errorf("snapshot read: %v", code)
					return
				}
				snapshots.Add(1)
			}
		}()
	}

	deadline := time.Now().Add(time.Second)
	commits := int64(0)
	for torn.Load() == nil && commits < 5000 && time.Now().Before(deadline) {
		commits++
		tx := s.BeginSnapshot(0)
		for id := types.ObjID(0); id < objects; id++ {
			if code := tx.SetPropertyValue(id, "n", types.NewInt(commits)); code != types.E_NONE {
				t.Fatal(code)
			}
		}
		if code := tx.Commit(); code != types.E_NONE {
			t.Fatalf("commit %d: %v", commits, code)
		}
		tx.Release()
	}
	stop.Store(true)
	readers.Wait()

	if pair := torn.Load(); pair != nil {
		t.Fatalf("a snapshot saw #0.n=%d beside another object's n=%d, after %d commits and %d snapshots", pair[0], pair[1], commits, snapshots.Load())
	}
	if snapshots.Load() == 0 {
		t.Fatal("no snapshot completed; the test observed nothing")
	}
}

// Two transactions each read the object the other writes, then commit at the
// same moment. Serializable outcomes have one of them see the other's write,
// so exactly one may commit on the snapshot they both took; the other must
// lose validation.
func TestCommitsThatReadEachOthersWritesCannotBothCommit(t *testing.T) {
	s := atomicityFixture(t, 2)
	for round := int64(1); round <= 500; round++ {
		a, b := s.BeginSnapshot(0), s.BeginSnapshot(0)
		stage := func(tx *StoreTxn, read, write types.ObjID) {
			if _, code := tx.PropertyValue(read, "n"); code != types.E_NONE {
				t.Fatal(code)
			}
			if code := tx.SetPropertyValue(write, "n", types.NewInt(round)); code != types.E_NONE {
				t.Fatal(code)
			}
		}
		stage(a, 1, 0)
		stage(b, 0, 1)

		start := make(chan struct{})
		var codes [2]types.ErrorCode
		var wg sync.WaitGroup
		for i, tx := range []*StoreTxn{a, b} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				codes[i] = tx.Commit()
			}()
		}
		close(start)
		wg.Wait()

		committed := 0
		for i, tx := range []*StoreTxn{a, b} {
			if codes[i] == types.E_NONE {
				committed++
			} else if !tx.ValidationFailed() {
				t.Fatalf("round %d: commit %d failed with %v, not a validation loss", round, i, codes[i])
			}
			tx.Release()
		}
		if committed != 1 {
			t.Fatalf("round %d: %d of the two commits succeeded, want exactly 1 (write skew if 2)", round, committed)
		}
	}
}
