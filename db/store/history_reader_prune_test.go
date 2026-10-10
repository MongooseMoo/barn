package store

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/MongooseMoo/barn/types"
)

// servedEntry is the snapshot read's rule: the newest entry at or below readTS.
func servedEntry(entries []objectHistory, readTS uint64) *Object {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].ts <= readTS {
			return entries[i].obj
		}
	}
	return nil
}

// Pruning to the readers must not change what any registered reader, or any
// transaction that begins at or above the clock, is served.
func TestPruneHistoryToReadersServesEveryReaderTheSameImage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 2000; round++ {
		var entries []objectHistory
		ts := uint64(rng.Intn(5))
		for n := rng.Intn(40); n > 0; n-- {
			ts += 1 + uint64(rng.Intn(4))
			entries = append(entries, objectHistory{ts: ts, obj: &Object{}})
		}
		// The clock may trail the newest entry: a commit can append between the
		// registry scan and the prune.
		clock := uint64(rng.Intn(int(ts) + 6))
		readers := []uint64{clock}
		for n := rng.Intn(6); n > 0; n-- {
			readers = append(readers, uint64(rng.Intn(int(ts)+6)))
		}
		slices.Sort(readers)
		before := slices.Clone(entries)

		pruned := pruneHistoryToReaders(entries, readers, clock)

		if !slices.Equal(entries, before) {
			t.Fatalf("round %d: the input entries were modified", round)
		}
		if !slices.IsSortedFunc(pruned, func(a, b objectHistory) int { return int(a.ts) - int(b.ts) }) {
			t.Fatalf("round %d: pruned entries are not ascending", round)
		}
		check := slices.Clone(readers)
		for future := clock; future <= ts+3; future++ {
			check = append(check, future)
		}
		for _, readTS := range check {
			if got, want := servedEntry(pruned, readTS), servedEntry(entries, readTS); got != want {
				t.Fatalf("round %d: reader at %d is served a different image after the prune (entries %v, readers %v, clock %d)",
					round, readTS, entryTimes(entries), readers, clock)
			}
		}
	}
}

func entryTimes(entries []objectHistory) []uint64 {
	times := make([]uint64, len(entries))
	for i, entry := range entries {
		times[i] = entry.ts
	}
	return times
}

// One old reader and the clock need two versions, however many were written
// in between.
func TestPruneHistoryToReadersDropsVersionsBetweenReaders(t *testing.T) {
	var entries []objectHistory
	for ts := uint64(10); ts < 110; ts++ {
		entries = append(entries, objectHistory{ts: ts, obj: &Object{}})
	}
	pruned := pruneHistoryToReaders(entries, []uint64{12, 200}, 200)
	if got := entryTimes(pruned); !slices.Equal(got, []uint64{12, 109}) {
		t.Fatalf("kept versions %v, want [12 109]", got)
	}
}

func commitCounter(t *testing.T, store *Store, value int64) {
	t.Helper()
	w := store.BeginSnapshot(0)
	if errCode := w.SetPropertyValue(0, "n", types.NewInt(value)); errCode != types.E_NONE {
		t.Fatalf("SetPropertyValue(%d) failed: %v", value, errCode)
	}
	if errCode := w.Commit(); errCode != types.E_NONE {
		t.Fatalf("Commit(%d) failed: %v", value, errCode)
	}
	w.Release()
}

func counterStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore()
	if err := store.Add(NewObject(0, 0)); err != nil {
		t.Fatalf("Add root failed: %v", err)
	}
	if errCode := store.DirectTxn().DefineProperty(0, "n", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); errCode != types.E_NONE {
		t.Fatalf("DefineProperty failed: %v", errCode)
	}
	return store
}

// A reader that stays open while an object is rewritten many times must not
// make the store keep every version written meanwhile, and must go on reading
// its own.
func TestHistoryStaysBoundedWhileOneReaderHoldsTheFloor(t *testing.T) {
	store := counterStore(t)
	commitCounter(t, store, 1)
	old := store.BeginSnapshot(0)
	defer old.Release()

	const commits = 1000
	var middle *StoreTxn
	for i := int64(2); i < 2+commits; i++ {
		commitCounter(t, store, i)
		if i == 500 {
			middle = store.BeginSnapshot(0)
			defer middle.Release()
		}
		if n := store.historyLen(0); n > 2*historyReaderPruneMin {
			t.Fatalf("after commit %d history holds %d versions, want at most %d", i, n, 2*historyReaderPruneMin)
		}
	}
	for _, reader := range []struct {
		tx   *StoreTxn
		want int64
	}{{old, 1}, {middle, 500}} {
		prop, errCode := reader.tx.FindProperty(0, "n")
		if errCode != types.E_NONE {
			t.Fatalf("FindProperty failed: %v", errCode)
		}
		if got := prop.Value.Int(); got != reader.want {
			t.Fatalf("reader begun at n=%d reads %d", reader.want, got)
		}
	}
}

// The conflict census walks every image newer than a losing transaction's
// snapshot, so while it tracks, only the floor prunes.
func TestHistoryKeepsEveryVersionAboveTheFloorWhileTheCensusTracks(t *testing.T) {
	store := counterStore(t)
	store.SetConflictCensusTracking(true)
	commitCounter(t, store, 1)
	old := store.BeginSnapshot(0)
	defer old.Release()

	const commits = 100
	for i := int64(2); i < 2+commits; i++ {
		commitCounter(t, store, i)
	}
	if n := store.historyLen(0); n != commits {
		t.Fatalf("history holds %d versions, want %d", n, commits)
	}
}
