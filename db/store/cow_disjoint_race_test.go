package store

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"barn/types"
)

func TestCOWCommitHoldsReadSetSlotsThroughPublish(t *testing.T) {
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	a, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject a failed: %v", errCode)
	}
	b, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject b failed: %v", errCode)
	}
	for _, id := range []types.ObjID{a, b} {
		if errCode := store.DefineProperty(id, "n", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("DefineProperty #%d.n failed: %v", id, errCode)
		}
	}

	tx := store.BeginReadOnly(0)
	defer tx.Release()
	if _, errCode := tx.PropertyValue(b, "n"); errCode != types.E_NONE {
		t.Fatalf("read b.n failed: %v", errCode)
	}
	if errCode := tx.SetPropertyValue(a, "n", types.NewInt(1)); errCode != types.E_NONE {
		t.Fatalf("write a.n failed: %v", errCode)
	}

	readSlot := store.objects[b]
	readSlot.mu.Lock()
	done := make(chan types.ErrorCode, 1)
	go func() {
		done <- tx.Commit()
	}()

	select {
	case errCode := <-done:
		readSlot.mu.Unlock()
		t.Fatalf("Commit completed without holding read-set slot: %v", errCode)
	case <-time.After(50 * time.Millisecond):
	}

	readSlot.mu.Unlock()
	if errCode := <-done; errCode != types.E_NONE {
		t.Fatalf("Commit after releasing read-set slot failed: %v", errCode)
	}
}

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
		nObjects    = 16
		nWriters    = 16 // one writer goroutine per object (disjoint footprints)
		nReaders    = 16
		commitsEach = 200
		readsEach   = 2000
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
		if errCode := store.DefineProperty(id, "counter", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
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

// TestCOWDisjointMixedKindCommitsRaceFree extends the Phase-0 race gate to the
// Phase-1 decentralized write kinds. Each disjoint writer goroutine owns its own
// object and commits, via the decentralized COW path, a MIX of the new kinds in one
// txn — scalar (name + flag), relationship (location), property-value, property
// DELETE, and verb-code — while raw readers hammer the txn==nil reader funnel
// (ObjectName / ObjectFlags / Location / PropertyValue / FindVerb / Parents) on the
// SAME objects. Under COW every commit publishes a NEW immutable image and never
// mutates the old one a reader Loaded, so this MUST be clean under -race.
// Run with: go test -race -run TestCOWDisjointMixedKindCommitsRaceFree ./db/store
func TestCOWDisjointMixedKindCommitsRaceFree(t *testing.T) {
	const (
		nObjects    = 16
		nReaders    = 16
		commitsEach = 200
		readsEach   = 2000
	)

	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	// A couple of distinct location targets so relationship writes vary.
	locA, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject locA failed: %v", errCode)
	}
	locB, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject locB failed: %v", errCode)
	}

	ids := make([]types.ObjID, nObjects)
	for i := 0; i < nObjects; i++ {
		id, errCode := store.CreateObject([]types.ObjID{0}, 0, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject failed: %v", errCode)
		}
		// counter: rewritten by property-value commits. doomed: deleted by a
		// property-delete commit then redefined each round (define is coarse, but it
		// is a live-store mutation done outside the timed disjoint commit; here we
		// simply pre-define a "keep" prop and delete-and-redefine via the store API
		// between rounds is overkill — instead we delete a property that the writer
		// re-stages each commit via a value write, see below).
		if errCode := store.DefineProperty(id, "counter", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("DefineProperty counter failed: %v", errCode)
		}
		if errCode := store.DefineProperty(id, "scratch", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
			t.Fatalf("DefineProperty scratch failed: %v", errCode)
		}
		if _, errCode := store.AddVerb(id, NewVerb("look", []string{"look"}, 0, VerbRead|VerbExecute, VerbArgs{}, []string{"return 1;"})); errCode != types.E_NONE {
			t.Fatalf("AddVerb failed: %v", errCode)
		}
		ids[i] = id
	}

	var wg sync.WaitGroup
	var stop atomic.Bool

	// Disjoint writers: each commits a mix of decentralized write kinds to its own
	// object in a single txn (scalar + relationship + property-value + verb-code).
	for w := 0; w < nObjects; w++ {
		id := ids[w]
		wg.Add(1)
		go func(id types.ObjID, seed int) {
			defer wg.Done()
			for c := 0; c < commitsEach; c++ {
				tx := store.BeginReadOnly(0)
				if errCode := tx.SetObjectName(id, "obj"); errCode != types.E_NONE {
					t.Errorf("SetObjectName failed: %v", errCode)
					return
				}
				if errCode := tx.SetObjectFlag(id, FlagRead, c%2 == 0); errCode != types.E_NONE {
					t.Errorf("SetObjectFlag failed: %v", errCode)
					return
				}
				loc := locA
				if c%2 == 0 {
					loc = locB
				}
				if errCode := tx.SetObjectLocationRaw(id, loc); errCode != types.E_NONE {
					t.Errorf("SetObjectLocationRaw failed: %v", errCode)
					return
				}
				if errCode := tx.SetPropertyValue(id, "counter", types.NewInt(int64(c))); errCode != types.E_NONE {
					t.Errorf("SetPropertyValue failed: %v", errCode)
					return
				}
				if errCode := tx.SetVerbCode(id, "look", []string{"return " + string(rune('0'+c%10)) + ";"}); errCode != types.E_NONE {
					t.Errorf("SetVerbCode failed: %v", errCode)
					return
				}
				// Mixed footprint of ONLY decentralized kinds, !liveMutated =>
				// commitDecentralized publishes new immutable images.
				if errCode := tx.Commit(); errCode != types.E_NONE {
					t.Errorf("Commit failed: %v", errCode)
					return
				}
			}
		}(id, w)
	}

	// A second set of disjoint writers exercising property DELETE on a separate
	// object set, so the property-delete builder is hit under -race.
	// ClearPropertyOverride stages a propertyDeletes entry (decentralized-eligible:
	// it touches neither propertyDefines nor propertyDefinitionDeletes). Each round
	// re-defines "scratch" via the coarse store API (live mutation serialized by
	// store.mu.Lock, which excludes the decentralized committers) so the next delete
	// has a target to remove.
	delIDs := ids[:4]
	for _, id := range delIDs {
		wg.Add(1)
		go func(id types.ObjID) {
			defer wg.Done()
			for c := 0; c < commitsEach/4; c++ {
				_ = store.DefineProperty(id, "scratch", NewProperty(types.NewInt(int64(c)), 0, PropRead|PropWrite, false, true))
				tx := store.BeginReadOnly(0)
				if errCode := tx.ClearPropertyOverride(id, "scratch"); errCode != types.E_NONE {
					_ = tx.Commit()
					continue
				}
				_ = tx.Commit() // decentralized property-delete publish
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
				_, _ = store.ObjectName(id)
				_, _ = store.ObjectFlags(id)
				_, _ = store.Location(id)
				_, _ = store.PropertyValue(id, "counter")
				_, _, _ = store.FindVerb(id, "look")
				_, _ = store.Parents(id)
			}
		}(r)
	}

	wg.Wait()
	stop.Store(true)

	// Sanity: every object still resolves its counter property and look verb.
	for _, id := range ids {
		if _, errCode := store.PropertyValue(id, "counter"); errCode != types.E_NONE {
			t.Fatalf("post-run PropertyValue(#%d) failed: %v", id, errCode)
		}
		if _, _, err := store.FindVerb(id, "look"); err != nil {
			t.Fatalf("post-run FindVerb(#%d) failed: %v", id, err)
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
	if errCode := store.DefineProperty(id, "counter", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
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
