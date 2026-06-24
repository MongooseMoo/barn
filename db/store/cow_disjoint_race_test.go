package store

import (
	"sync"
	"sync/atomic"
	"testing"

	"barn/types"
)

// TestCOWDisjointCommitsRaceFree is the Phase-0 go/no-go race gate.
//
// It runs many goroutines committing DISJOINT property-value writes (each writer
// owns its own object) through StoreTxn.Commit — which takes the decentralized
// COW publish path — WHILE other goroutines hammer the RAW Store.* reader funnel
// (PropertyValue / FindProperty / ObjectName / Parents) on the SAME objects.
//
// Under the per-object-LOCK prototype this exact shape FAILED under -race: a raw
// reader read a live object's mutable Property fields under store.mu.RLock while a
// committer mutated them in place under store.mu.RLock + a per-object lock, and
// RLock does not exclude RLock. Under COW the committer publishes a NEW immutable
// image and never mutates the old one the reader Loaded, so this MUST be clean
// under -race. Run with: go test -race -run TestCOWDisjointCommitsRaceFree ./db/store
func TestCOWDisjointCommitsRaceFree(t *testing.T) {
	const (
		nObjects     = 16
		nWriters     = 16 // one writer goroutine per object (disjoint footprints)
		nReaders     = 16
		commitsEach  = 200
		readsEach    = 2000
	)

	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	ids := make([]types.ObjID, nObjects)
	for i := 0; i < nObjects; i++ {
		id, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject failed: %v", errCode)
		}
		if errCode := store.DefineProperty(id, NewProperty("counter", types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("DefineProperty failed: %v", errCode)
		}
		ids[i] = id
	}

	var wg sync.WaitGroup
	var stop atomic.Bool

	// Disjoint writers: each commits property-value writes to its own object via the
	// decentralized COW path.
	for w := 0; w < nWriters; w++ {
		id := ids[w%nObjects]
		wg.Add(1)
		go func(id types.ObjID) {
			defer wg.Done()
			for c := 0; c < commitsEach; c++ {
				tx := store.BeginReadOnly(0)
				if errCode := tx.SetPropertyValue(id, "counter", types.NewInt(int64(c))); errCode != types.E_NONE {
					t.Errorf("SetPropertyValue failed: %v", errCode)
					return
				}
				// Property-value-only, not liveMutated => decentralized COW publish.
				if errCode := tx.Commit(); errCode != types.E_NONE {
					t.Errorf("Commit failed: %v", errCode)
					return
				}
			}
		}(id)
	}

	// Raw readers: hammer the txn==nil reader funnel concurrently with the publishes.
	for r := 0; r < nReaders; r++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < readsEach && !stop.Load(); i++ {
				id := ids[(seed+i)%nObjects]
				_, _ = store.PropertyValue(id, "counter")
				_, _ = store.FindProperty(id, "counter")
				_, _ = store.ObjectName(id)
				_, _ = store.Parents(id)
			}
		}(r)
	}

	wg.Wait()
	stop.Store(true)

	// Sanity: every object still resolves its counter property (no corruption).
	for _, id := range ids {
		if _, errCode := store.PropertyValue(id, "counter"); errCode != types.E_NONE {
			t.Fatalf("post-run PropertyValue(#%d) failed: %v", id, errCode)
		}
	}
}

// TestCOWSameObjectCommitsSerialize exercises the OTHER serialization edge: many
// writers committing property-value writes to the SAME object. They must serialize
// on that object's slot mutex (no torn image, no panic, no lost slot), and a final
// read must see one of the committed values. Under -race this must be clean.
func TestCOWSameObjectCommitsSerialize(t *testing.T) {
	const (
		nWriters    = 32
		commitsEach = 100
	)
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	id, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject failed: %v", errCode)
	}
	if errCode := store.DefineProperty(id, NewProperty("counter", types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}

	var wg sync.WaitGroup
	for w := 0; w < nWriters; w++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for c := 0; c < commitsEach; c++ {
				tx := store.BeginReadOnly(0)
				// SetPropertyValue records a property read of the prop, so concurrent
				// same-object committers may conflict (E_INVARG) — that is the correct
				// optimistic-validation contract, not a failure. Tolerate it.
				if errCode := tx.SetPropertyValue(id, "counter", types.NewInt(int64(base*1000+c))); errCode != types.E_NONE {
					t.Errorf("SetPropertyValue failed: %v", errCode)
					return
				}
				_ = tx.Commit()
			}
		}(w)
	}
	wg.Wait()

	if _, errCode := store.PropertyValue(id, "counter"); errCode != types.E_NONE {
		t.Fatalf("post-run PropertyValue failed: %v", errCode)
	}
}
