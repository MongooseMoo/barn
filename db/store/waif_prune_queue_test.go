package store

import (
	"math/rand"
	"runtime"
	"sync"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

type waifVersion struct {
	ts uint64
	n  int64
}

// pruneWaifVersions is the retention rule: the newest version at or below the
// floor, and every later one.
func pruneWaifVersions(versions []waifVersion, floor uint64) []waifVersion {
	for n := len(versions) - 1; n > 0; n-- {
		if versions[n].ts <= floor {
			return versions[n:]
		}
	}
	return versions
}

// Every live snapshot must keep reading the image it began with, and each WAIF
// must hold exactly the images the retention rule allows, across interleaved
// readers, direct publications, transactional commits and releases.
func TestWaifPruneKeepsExactlyTheImagesLiveSnapshotsRead(t *testing.T) {
	s := NewStore()
	rng := rand.New(rand.NewSource(1))
	waifs := make([]types.Value, 8)
	published := make([][]waifVersion, len(waifs)) // every version ever published
	held := make([][]waifVersion, len(waifs))      // the versions that must remain
	for i := range waifs {
		waifs[i] = types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
		if _, ok := waifs[i].WaifImageAt(s.waifDomain, 0); !ok {
			t.Fatal("attachment failed")
		}
		published[i] = []waifVersion{{}}
		held[i] = []waifVersion{{}}
	}
	var readers []*StoreTxn
	floor := func() uint64 {
		floor := s.clock.Load()
		for _, reader := range readers {
			floor = min(floor, reader.readTS)
		}
		return floor
	}
	pruneAll := func() {
		for i := range held {
			held[i] = pruneWaifVersions(held[i], floor())
		}
	}
	check := func(step int) {
		t.Helper()
		tracked := 0
		for i, w := range waifs {
			if len(held[i]) > 1 {
				tracked++
			}
			remaining := held[i]
			for _, version := range published[i] {
				want := len(remaining) != 0 && remaining[0] == version
				if want {
					remaining = remaining[1:]
				}
				image, ok := w.WaifImageAt(s.waifDomain, version.ts)
				if got := ok && image.Timestamp() == version.ts; got != want {
					t.Fatalf("step %d: WAIF %d image at %d held = %v, want %v", step, i, version.ts, got, want)
				}
			}
			for _, reader := range readers {
				var want waifVersion
				for _, version := range published[i] {
					if version.ts <= reader.readTS {
						want = version
					}
				}
				image, ok := w.WaifImageAt(s.waifDomain, reader.readTS)
				if !ok || image.Timestamp() != want.ts {
					t.Fatalf("step %d: reader at %d sees WAIF %d image %v (found %v), want timestamp %d", step, reader.readTS, i, image, ok, want.ts)
				}
				waifNumber(t, reader, w, want.n)
			}
		}
		if len(s.waifHistory) != tracked || len(s.waifHistoryQueue) != tracked || s.waifHistoryPending.Load() != (tracked != 0) {
			t.Fatalf("step %d: tracked histories = %d (queue %d, pending %v), want %d", step, len(s.waifHistory), len(s.waifHistoryQueue), s.waifHistoryPending.Load(), tracked)
		}
	}
	for step := 1; step <= 3000; step++ {
		i := rng.Intn(len(waifs))
		switch action := rng.Intn(10); {
		case action < 3 && len(readers) < 6:
			readers = append(readers, s.BeginSnapshot(0))
		case action < 6 && len(readers) != 0:
			n := rng.Intn(len(readers))
			readers[n].Release()
			readers = append(readers[:n], readers[n+1:]...)
			pruneAll()
		case action < 8:
			if ec := s.DirectTxn().SetWaifProperty(waifs[i], "n", types.NewInt(int64(step))); ec != types.E_NONE {
				t.Fatal(ec)
			}
			version := waifVersion{s.clock.Load(), int64(step)}
			published[i] = append(published[i], version)
			held[i] = pruneWaifVersions(append(held[i], version), floor())
		default:
			tx := s.BeginSnapshot(0)
			if ec := tx.SetWaifProperty(waifs[i], "n", types.NewInt(int64(step))); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := tx.Commit(); ec != types.E_NONE {
				t.Fatal(ec)
			}
			version := waifVersion{s.clock.Load(), int64(step)}
			published[i] = append(published[i], version)
			held[i] = append(held[i], version)
			tx.Release()
			pruneAll()
		}
		check(step)
	}
	for _, reader := range readers {
		reader.Release()
	}
	readers = nil
	pruneAll()
	check(3001)
	if len(s.waifHistory) != 0 {
		t.Fatalf("history after the last reader = %d", len(s.waifHistory))
	}
}

// A release must examine the histories its floor advance can prune, not every
// history the store tracks.
func TestWaifPruneVisitsOnlyDueHistories(t *testing.T) {
	const retained, prunable = 1000, 4
	s := NewStore()
	// Weak history bookkeeping must not be the only reference.
	waifs := make([]types.Value, 0, retained+prunable)
	publish := func() {
		w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
		if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
			t.Fatal(ec)
		}
		waifs = append(waifs, w)
	}
	old := s.BeginSnapshot(0)
	for i := 0; i < prunable; i++ {
		publish()
	}
	pin := s.BeginSnapshot(0)
	defer pin.Release()
	for i := 0; i < retained; i++ {
		publish()
	}
	if got := len(s.waifHistory); got != retained+prunable {
		t.Fatalf("history before release = %d, want %d", got, retained+prunable)
	}
	before := s.waifPruneVisits
	old.Release()
	if got := len(s.waifHistory); got != retained {
		t.Fatalf("history after release = %d, want %d", got, retained)
	}
	if visits := s.waifPruneVisits - before; visits != prunable {
		t.Fatalf("release that could prune %d of %d histories examined %d", prunable, retained+prunable, visits)
	}
	before = s.waifPruneVisits
	s.BeginSnapshot(0).Release()
	if visits := s.waifPruneVisits - before; visits != 0 {
		t.Fatalf("release that could prune nothing examined %d histories", visits)
	}
	runtime.KeepAlive(waifs)
}

// Once the readers that pinned it are gone, the registry holds nothing: not for
// WAIFs published many times, and not for WAIFs that became unreachable while
// their history was still pinned.
func TestWaifHistoryReturnsToBaselineAfterReadersFinish(t *testing.T) {
	s := NewStore()
	rng := rand.New(rand.NewSource(2))
	waifs := make([]types.Value, 200)
	for i := range waifs {
		waifs[i] = types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	}
	var readers []*StoreTxn
	for round := 1; round <= 50; round++ {
		readers = append(readers, s.BeginSnapshot(0))
		for i := 0; i < 20; i++ {
			if ec := s.DirectTxn().SetWaifProperty(waifs[rng.Intn(len(waifs))], "n", types.NewInt(int64(round))); ec != types.E_NONE {
				t.Fatal(ec)
			}
		}
	}
	if len(s.waifHistory) == 0 {
		t.Fatal("no history was pinned")
	}
	kept := waifs[:len(waifs)/2]
	clear(waifs[len(waifs)/2:])
	runtime.GC()
	rng.Shuffle(len(readers), func(i, j int) { readers[i], readers[j] = readers[j], readers[i] })
	for _, reader := range readers {
		reader.Release()
	}
	if len(s.waifHistory) != 0 || len(s.waifHistoryQueue) != 0 || s.waifHistoryPending.Load() || s.waifHistoryDue.Load() != 0 {
		t.Fatalf("after the last reader: history %d, queue %d, pending %v, due %d", len(s.waifHistory), len(s.waifHistoryQueue), s.waifHistoryPending.Load(), s.waifHistoryDue.Load())
	}
	for i, w := range kept {
		image, ok := w.WaifImageAt(s.waifDomain, s.clock.Load())
		if !ok {
			t.Fatalf("WAIF %d has no current image", i)
		}
		if image.Timestamp() != 0 {
			if _, ok := w.WaifImageAt(s.waifDomain, image.Timestamp()-1); ok {
				t.Fatalf("WAIF %d retains an image older than its current one", i)
			}
		}
	}
}

// The same, with readers and publishers racing: whichever reader deregisters
// last must leave nothing behind.
func TestWaifHistoryReturnsToBaselineAfterConcurrentReaders(t *testing.T) {
	s := NewStore()
	waifs := make([]types.Value, 16)
	for i := range waifs {
		waifs[i] = types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(g)))
			for i := 0; i < 500; i++ {
				reader := s.BeginSnapshot(0)
				w := waifs[rng.Intn(len(waifs))]
				before, _, ec := reader.WaifProperty(w, "n")
				if ec != types.E_NONE {
					t.Error(ec)
				}
				if ec := s.DirectTxn().SetWaifProperty(waifs[rng.Intn(len(waifs))], "n", types.NewInt(int64(i))); ec != types.E_NONE {
					t.Error(ec)
				}
				runtime.Gosched()
				// Not through the transaction, which would answer from its cache.
				if image, ok := w.WaifImageAt(s.waifDomain, reader.readTS); !ok {
					t.Errorf("image at %d was pruned under a live reader", reader.readTS)
				} else if after, _ := image.Property("n"); !after.Equal(before) {
					t.Errorf("snapshot read changed from %v to %v", before, after)
				}
				reader.Release()
			}
		}(g)
	}
	wg.Wait()
	if len(s.waifHistory) != 0 || len(s.waifHistoryQueue) != 0 || s.waifHistoryPending.Load() {
		t.Fatalf("after the last reader: history %d, queue %d, pending %v", len(s.waifHistory), len(s.waifHistoryQueue), s.waifHistoryPending.Load())
	}
}
