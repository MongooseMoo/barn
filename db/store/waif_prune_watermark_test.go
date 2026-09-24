package store

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// A reader newer than the retained history floor cannot make another prune
// useful. Releasing it must not wait on the store's global publication lock.
func TestWaifPruneUnchangedFloorDoesNotTakePublicationLock(t *testing.T) {
	s := NewStore()
	s.clock.Store(16)
	old := s.BeginSnapshot(0)
	defer old.Release()
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	// Establish a completed full prune while the original reader pins the floor.
	s.BeginSnapshot(0).Release()
	newer := s.BeginSnapshot(0)
	done := make(chan struct{})
	s.mu.Lock()
	go func() { newer.Release(); close(done) }()
	completed := false
	select {
	case <-done:
		completed = true
	case <-time.After(time.Second):
	}
	s.mu.Unlock()
	<-done
	if !completed {
		t.Fatal("unchanged-floor release waited for the publication lock")
	}
	waifNumber(t, old, w, 0)
	old.Release()
	if len(s.waifHistory) != 0 {
		t.Fatal("last old reader did not promptly prune retained history")
	}
	if _, ok := w.WaifImageAt(s.waifDomain, 16); ok {
		t.Fatal("obsolete property image remains after last reader")
	}
}

// Explicit snapshots may be older or newer than the publication clock. A
// previously visited high floor must not hide new history after a lower reader.
func TestWaifPrunePublicationInvalidatesPreviouslyVisitedFloor(t *testing.T) {
	s := NewStore()
	s.clock.Store(20)
	future := s.BeginSnapshot(100)
	defer future.Release()
	for i := 0; i < 2; i++ {
		old := s.BeginSnapshot(10)
		w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
		if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
			old.Release()
			t.Fatal(ec)
		}
		waifNumber(t, old, w, 0)
		old.Release()
		if len(s.waifHistory) != 0 {
			t.Fatalf("publication %d retained history at repeated floor 100", i)
		}
		if _, ok := w.WaifImageAt(s.waifDomain, 10); ok {
			t.Fatalf("publication %d retained obsolete image", i)
		}
	}
}
