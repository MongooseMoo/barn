package store

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// TestConcurrentAnonObjectWritesSerialize is the anon-object analogue of the
// COW same-object serialization gate (cow_disjoint_race_test.go), and the gate the
// anonfix coder's SEQUENTIAL -race run never exercised: CONCURRENT writes to the
// SAME anonymous object.
//
// Anonymous objects live out-of-band in s.anonObjects as a plain map[ObjID]*Object
// with NO COW slot and NO per-id slot mutex (see store_core.go liveObjectLocked).
// The anonfix routes any commit whose write footprint includes an anon id onto the
// COARSE exclusive store.mu.Lock path (writeFootprintHasAnon), where the anon is
// mutated IN PLACE. The claim under test: that exclusive lock fully serializes anon
// writes, so concurrent committers neither (a) trip -race on the unsynchronized
// in-place mutation the scout warned about in §5, nor (b) lose updates.
//
// The probe is a read-modify-write counter increment under MVCC optimistic
// validation: each successful commit reads counter@readTS and stages counter+1.
// The coarse path validates the read set (validatePropertyReadsLocked) before
// applying, so any commit whose read went stale aborts with E_INVARG (the correct
// optimistic-conflict contract, NOT corruption). Therefore every SUCCESSFUL commit
// increments counter by exactly 1, and the post-run invariant is:
//
//	final counter value == total successful commits   (no lost update)
//
// On the COMMIT path E_INVARG is tolerated (legitimate optimistic conflict) but any
// other non-E_NONE code — in particular E_INVIND, the original regression signature
// of a live anon failing to resolve under the exclusive lock — fails the test. The
// commit resolver runs under store.mu.Lock against the live anon, so it is NOT
// subject to read-snapshot visibility.
//
// On the READ/STAGE path, by contrast, a stale readTS may legitimately see the anon
// as nil: anon objects carry NO per-id history (scout §5), so once a concurrent
// committer bumps the anon's version past this txn's readTS, objectLocked returns
// nil and the read/stage yields E_PROPNF/E_INVIND. That is the documented MVCC
// limitation the fix inherits, not corruption — such iterations are skipped, not
// counted, and not failed. Reader goroutines hammer the raw Store reader funnel and
// a read-txn resolver on the same anon concurrently, exercising reader-vs-committer
// on the slotless anon under -race regardless of visibility outcome.
//
// Run: go test -race ./db/store -run TestConcurrentAnonObjectWritesSerialize -count=3
func TestConcurrentAnonObjectWritesSerialize(t *testing.T) {
	const (
		nIncrWriters   = 24 // >= 16 required; same-anon read-modify-write probe
		nScalarWriters = 8  // exercise the scalar (name) coarse apply path on the anon
		nReaders       = 16
		incrEach       = 150
		scalarEach     = 150
		readsEach      = 3000
	)

	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	// Inheritable property the anon writes through its parent #0 (matches how the
	// coder's regression test and real MOO anon objects carry properties).
	if errCode := store.DirectTxn().DefineProperty(0, "counter", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty #0.counter failed: %v", errCode)
	}

	// ONE anonymous object, shared by every writer and reader.
	anon, ec := store.DirectTxn().CreateObject([]types.ObjID{0}, 0, true /*anonymous*/)
	if ec != types.E_NONE {
		t.Fatalf("CreateObject anonymous failed: %v", ec)
	}

	var wg sync.WaitGroup
	var stop atomic.Bool
	var successes atomic.Int64
	var visSkips atomic.Int64

	// visibilitySkip reports whether errCode is the documented stale-readTS outcome
	// for a slotless, history-free anon (read/stage resolved the anon as nil because a
	// concurrent committer advanced its version past this txn's readTS). These are NOT
	// failures; they are skipped so the probe stays honest about what the fix changed.
	visibilitySkip := func(errCode types.ErrorCode) bool {
		return errCode == types.E_PROPNF || errCode == types.E_INVIND
	}

	// Increment writers: same-anon read-modify-write under optimistic validation.
	for w := 0; w < nIncrWriters; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := 0; c < incrEach; c++ {
				tx := store.BeginReadOnly(0)
				cur, errCode := tx.PropertyValue(anon, "counter")
				if errCode != types.E_NONE {
					if visibilitySkip(errCode) {
						visSkips.Add(1)
						continue
					}
					t.Errorf("tx.PropertyValue(anon, counter) = %v, unexpected", errCode)
					return
				}
				if cur.Type() != types.TYPE_INT {
					t.Errorf("counter value type = %T, want IntValue", cur)
					return
				}
				if errCode := tx.SetPropertyValue(anon, "counter", types.NewInt(cur.Int()+1)); errCode != types.E_NONE {
					if visibilitySkip(errCode) {
						visSkips.Add(1)
						continue
					}
					t.Errorf("tx.SetPropertyValue(anon, counter) = %v, unexpected", errCode)
					return
				}
				switch errCode := tx.Commit(); errCode {
				case types.E_NONE:
					successes.Add(1)
				case types.E_INVARG:
					// Legitimate optimistic conflict (stale read) — correct, not corruption.
				default:
					// The commit resolver runs live under store.mu.Lock; E_INVIND here is
					// the regression (coarse routing failed to resolve a live anon).
					t.Errorf("tx.Commit() = %v, want E_NONE or E_INVARG; E_INVIND is the regression", errCode)
					return
				}
			}
		}()
	}

	// Scalar writers: exercise the scalar (name) coarse apply path on the same anon.
	// No serializability invariant asserted here — just that the slotless anon scalar
	// write never panics, never data-races, and the commit never returns E_INVIND for
	// a live anon. The stage (SetObjectName) is subject to the same readTS visibility
	// window as the increment readers, so a stale-readTS E_INVIND there is skipped.
	for w := 0; w < nScalarWriters; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := 0; c < scalarEach; c++ {
				tx := store.BeginReadOnly(0)
				if errCode := tx.SetObjectName(anon, "anon"); errCode != types.E_NONE {
					if visibilitySkip(errCode) {
						visSkips.Add(1)
						continue
					}
					t.Errorf("tx.SetObjectName(anon) = %v, unexpected", errCode)
					return
				}
				switch errCode := tx.Commit(); errCode {
				case types.E_NONE, types.E_INVARG:
					// success or legitimate conflict
				default:
					t.Errorf("scalar tx.Commit() = %v, want E_NONE or E_INVARG; E_INVIND is the regression", errCode)
					return
				}
			}
		}()
	}

	// Readers: hammer both the raw Store reader funnel and a read-txn resolver on the
	// same anon, concurrently with the in-place coarse mutations.
	for r := 0; r < nReaders; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < readsEach && !stop.Load(); i++ {
				_, _ = store.DirectTxn().PropertyValue(anon, "counter")
				_, _ = store.DirectTxn().ObjectName(anon)
				_ = store.DirectTxn().Valid(anon)
				tx := store.BeginReadOnly(0)
				_, _ = tx.PropertyValue(anon, "counter")
				_, _ = tx.ObjectName(anon)
			}
		}()
	}

	wg.Wait()
	stop.Store(true)

	// Invariant: every successful increment commit added exactly 1, and stale-read
	// commits aborted, so the final counter equals the success count. A lost update
	// (two successes reading the same value) would make final < successes.
	final, errCode := store.DirectTxn().PropertyValue(anon, "counter")
	if errCode != types.E_NONE {
		t.Fatalf("post-run PropertyValue(anon, counter) = %v, want E_NONE", errCode)
	}
	got := final.Int()
	want := successes.Load()
	if got != want {
		t.Fatalf("LOST UPDATE: final counter = %d, want %d (successful commits); coarse anon serialization is broken", got, want)
	}
	if want == 0 {
		t.Fatalf("no commit succeeded (%d attempts each x %d writers) — probe did not exercise the path", incrEach, nIncrWriters)
	}
	t.Logf("anon coarse serialization OK: %d successful increments, final counter=%d, %d stale-readTS visibility skips (of %d incr attempts)",
		want, got, visSkips.Load(), int64(nIncrWriters)*incrEach)
}
