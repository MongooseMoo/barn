package store

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func waifNumber(t *testing.T, tx *StoreTxn, w types.Value, want int64) {
	t.Helper()
	v, found, ec := tx.WaifProperty(w, "n")
	if ec != types.E_NONE || !found || v.Int() != want {
		t.Fatalf("WAIF n = %v, found %v, error %v; want %d", v, found, ec, want)
	}
}

func TestWaifTransactionIsolationAndAbort(t *testing.T) {
	s := NewStore()
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(5))
	a, b := s.BeginSnapshot(0), s.BeginSnapshot(0)
	defer a.Release()
	defer b.Release()
	if ec := a.SetWaifProperty(w, "n", types.NewInt(6)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	waifNumber(t, a, w, 6)
	waifNumber(t, b, w, 5)
	if ec := b.SetWaifProperty(w, "n", types.NewInt(6)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if !b.HasWrites() {
		t.Fatal("WAIF-only transaction has no writes")
	}
	if ec := b.Commit(); ec != types.E_NONE {
		t.Fatal(ec)
	}
	a.Release()
	waifNumber(t, s.DirectTxn(), w, 6)
}

func TestWaifFirstReadUsesSnapshotAndMissingReadConflicts(t *testing.T) {
	s := NewStore()
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	a := s.BeginSnapshot(0)
	defer a.Release()
	if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	waifNumber(t, a, w, 0)
	if _, found, ec := a.WaifProperty(w, "missing"); found || ec != types.E_NONE {
		t.Fatalf("missing property = %v, %v", found, ec)
	}
	if ec := a.SetWaifProperty(w, "missing", types.NewInt(2)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := a.Commit(); ec == types.E_NONE || !a.ValidationFailed() {
		t.Fatalf("stale WAIF commit = %v, conflict %v", ec, a.ValidationFailed())
	}
}

func TestWaifForeignStoreRejected(t *testing.T) {
	a, b := NewStore(), NewStore()
	w := types.NewWaif(0, 0)
	if ec := a.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if _, _, ec := b.DirectTxn().WaifProperty(w, "n"); ec != types.E_INVARG {
		t.Fatalf("foreign store read = %v, want E_INVARG", ec)
	}
}

func TestWaifMixedObjectCommitAndFlushValidateBeforePublication(t *testing.T) {
	for _, boundary := range []struct {
		name string
		run  func(*StoreTxn) types.ErrorCode
	}{
		{"commit", (*StoreTxn).Commit}, {"flush", (*StoreTxn).FlushStagedToLive},
	} {
		t.Run(boundary.name, func(t *testing.T) {
			s := NewStore()
			if err := s.Add(NewObject(0, 0)); err != nil {
				t.Fatal(err)
			}
			w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
			tx := s.BeginSnapshot(0)
			defer tx.Release()
			waifNumber(t, tx, w, 0)
			if ec := tx.SetObjectName(0, "private"); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(0)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := boundary.run(tx); ec != types.E_INVARG || !tx.ValidationFailed() {
				t.Fatalf("boundary = %v, conflict %v", ec, tx.ValidationFailed())
			}
			if got, ec := s.DirectTxn().ObjectName(0); ec != types.E_NONE || got == "private" {
				t.Fatalf("partial publication: %q, %v", got, ec)
			}
		})
	}
}

func TestWaifMixedPreflightFailurePublishesNeitherKind(t *testing.T) {
	s := NewStore()
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if ec := tx.SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	lazySet(&tx.scalarWrites, 99, objectScalarWrite{nameSet: true, name: "missing"})
	if ec := tx.Commit(); ec != types.E_INVIND {
		t.Fatalf("commit = %v", ec)
	}
	waifNumber(t, s.DirectTxn(), w, 0)
}

func TestWaifOverlappingDiscardedAttemptsDoNotResurrect(t *testing.T) {
	s := NewStore()
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	a, b := s.BeginSnapshot(0), s.BeginSnapshot(0)
	if ec := a.SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := b.SetWaifProperty(w, "n", types.NewInt(2)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	a.Release()
	b.Release()
	waifNumber(t, s.DirectTxn(), w, 0)
}

func TestWaifRenewAndFlushRebaseOwnWritesAndCarryDependencies(t *testing.T) {
	for _, boundary := range []string{"renew", "flush"} {
		t.Run(boundary, func(t *testing.T) {
			s := NewStore()
			w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
			tx := s.BeginSnapshot(0)
			defer func() { tx.Release() }()
			if ec := tx.SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if boundary == "renew" {
				next, published, ec := tx.CommitAndRenewCarryingReads()
				if ec != types.E_NONE || !published {
					t.Fatalf("renew = %v, %v", ec, published)
				}
				tx = next
			} else if ec := tx.FlushStagedToLive(); ec != types.E_NONE {
				t.Fatal(ec)
			}
			waifNumber(t, tx, w, 1)
			if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(2)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := tx.SetWaifProperty(w, "other", types.NewInt(3)); ec != types.E_NONE {
				t.Fatal(ec)
			}
			if ec := tx.Commit(); ec != types.E_INVARG || !tx.ValidationFailed() {
				t.Fatalf("carried conflict = %v, %v", ec, tx.ValidationFailed())
			}
		})
	}
}

func TestWaifHistoryReleasedAfterLastOldReader(t *testing.T) {
	s := NewStore()
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	tx := s.BeginSnapshot(0)
	if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if len(s.waifHistory) != 1 {
		t.Fatalf("history tracked = %d", len(s.waifHistory))
	}
	waifNumber(t, tx, w, 0)
	tx.Release()
	if len(s.waifHistory) != 0 {
		t.Fatalf("history remains after release = %d", len(s.waifHistory))
	}
	if _, ok := w.WaifImageAt(s.waifDomain, 0); ok {
		t.Fatal("obsolete version retained")
	}
}

func TestWaifSnapshotFreezesIdentityAndExternalAliases(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(1))
	if ec := s.DirectTxn().DefineProperty(0, "w", NewProperty(w, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	snapshot, rewrite := s.SnapshotWithRoots([]types.Value{types.NewList([]types.Value{w, w})})
	if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(2)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	frozen := snapshot.Objects[0].Properties["w"].Value
	if !frozen.Equal(w) {
		t.Fatal("checkpoint changed WAIF identity")
	}
	if got, _ := frozen.GetProperty("n"); got.Int() != 1 {
		t.Fatalf("snapshot changed to %v", got)
	}
	external := rewrite.Rewrite(types.NewList([]types.Value{w, w}))
	if external.Get(1) != frozen || external.Get(2) != frozen {
		t.Fatal("checkpoint aliases do not share one frozen payload")
	}
	frozen.SetProperty("n", types.NewInt(3)) // checkpoint payload is detached
	waifNumber(t, s.DirectTxn(), w, 2)
}

func TestWaifHistoricalChildrenRemainSemanticRootsUntilRelease(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	child := types.NewWaif(0, 0)
	parent := types.NewWaif(0, 0).SetProperty("child", child)
	if ec := s.DirectTxn().DefineProperty(0, "w", NewProperty(parent, 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	tx := s.BeginSnapshot(0) // deliberately no WAIF property read
	if ec := s.DirectTxn().SetWaifProperty(parent, "child", types.NewInt(0)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	requireRoots(t, s, parent, child)
	tx.Release()
	requireRoots(t, s, parent)
}

func TestWaifHistoryBookkeepingDoesNotRetainPayload(t *testing.T) {
	s := NewStore()
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	released := make(chan struct{})
	func() {
		w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
		w.AddWaifCleanup(func() { close(released) })
		if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
			t.Fatal(ec)
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		select {
		case <-released:
			return
		default:
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("weak history bookkeeping retained an unreachable WAIF")
}

func TestWaifHistoricalEdgeDoesNotRemoveCheckpointPendingRoot(t *testing.T) {
	s := NewStore()
	child := types.NewWaif(0, 0)
	parent := types.NewWaif(0, 0).SetProperty("child", child)
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	if ec := s.DirectTxn().SetWaifProperty(parent, "child", types.NewInt(0)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	s.SetPendingFinalizations([]types.Value{child})
	snapshot, _ := s.SnapshotWithRoots([]types.Value{parent})
	if len(snapshot.PendingFinalizations) != 1 || !snapshot.PendingFinalizations[0].Equal(child) {
		t.Fatalf("historical edge removed pending root: %v", snapshot.PendingFinalizations)
	}
	s.SetPendingFinalizations(nil)
	s.AppendPendingFinalizations([]types.Value{parent, child})
	if got := s.TakePendingFinalizations(); len(got) != 2 {
		t.Fatalf("historical edge removed queued root: %v", got)
	}
}

func TestWaifPublicationDoesNotMissLastReaderRelease(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(previousProcs) })
	s := NewStore()
	const readerTS = uint64(readTSShardCount) // shard 0
	s.clock.Store(readerTS)
	reader := s.BeginSnapshot(0)
	t.Cleanup(reader.Release)
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))

	// Stop the publisher after it samples the reader's shard, but before its
	// floor scan finishes. No production hook or timing delay is needed.
	lastShard := &s.readTSShards[readTSShardCount-1]
	lastShard.mu.Lock()
	shardLocked := true
	t.Cleanup(func() {
		if shardLocked {
			lastShard.mu.Unlock()
		}
	})
	published := make(chan types.ErrorCode, 1)
	go func() { published <- s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)) }()

	blockedIn := func(function string) bool {
		buffer := make([]byte, 256<<10)
		stack := string(buffer[:runtime.Stack(buffer, true)])
		for _, goroutine := range strings.Split(stack, "\n\n") {
			if strings.Contains(goroutine, t.Name()+".func") && strings.Contains(goroutine, function) && strings.Contains(goroutine, "Mutex.Lock") {
				return true
			}
		}
		return false
	}
	waitFor := func(description string, ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !ready() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", description)
			}
			runtime.Gosched()
		}
	}
	waitFor("publisher blocked in the floor scan", func() bool {
		return blockedIn("(*Store).historyFloor(")
	})

	released := make(chan struct{})
	go func() { reader.Release(); close(released) }()
	waitFor("release completion or its pending-history prune", func() bool {
		select {
		case <-released:
			return true
		default:
		}
		return blockedIn("(*StoreTxn).release(")
	})
	s.readTSShards[0].mu.Lock()
	remainingReaders := s.readTSShards[0].counts[readerTS]
	s.readTSShards[0].mu.Unlock()
	if remainingReaders != 0 || !reader.released.Load() {
		t.Fatal("reader did not deregister while the publisher's floor was stale")
	}

	lastShard.mu.Unlock()
	shardLocked = false
	select {
	case ec := <-published:
		if ec != types.E_NONE {
			t.Fatal(ec)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publisher did not finish")
	}
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("reader release did not finish")
	}
	if _, found := w.WaifImageAt(s.waifDomain, readerTS); found {
		t.Error("last reader released, but its obsolete WAIF image remains")
	}
	if len(s.waifHistory) != 0 || s.waifHistoryPending.Load() {
		t.Errorf("last reader released, but history registry has %d entries, pending=%v", len(s.waifHistory), s.waifHistoryPending.Load())
	}
}

// A WAIF dependency routes a commit through the coarse path. A value write to an
// existing property slot must not move the object's property shape there either,
// or every concurrent ancestry walk through the object fails validation.
func TestWaifCoarseCommitValueWriteKeepsPropertyShape(t *testing.T) {
	s := NewStore()
	if err := s.Add(NewObject(0, 0)); err != nil {
		t.Fatal(err)
	}
	if ec := s.DirectTxn().DefineProperty(0, "p", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	shape := s.liveObjectLocked(0).propertyShapeVersion
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	tx := s.BeginSnapshot(0)
	defer tx.Release()
	waifNumber(t, tx, w, 0)
	if ec := tx.SetPropertyValue(0, "p", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if ec := tx.Commit(); ec != types.E_NONE {
		t.Fatal(ec)
	}
	live := s.liveObjectLocked(0)
	if got, ec := s.DirectTxn().PropertyValue(0, "p"); ec != types.E_NONE || got.Int() != 1 {
		t.Fatalf("p = %v, %v; want 1", got, ec)
	}
	if live.propertyShapeVersion != shape {
		t.Fatalf("propertyShapeVersion moved %d -> %d on a value write", shape, live.propertyShapeVersion)
	}
}
