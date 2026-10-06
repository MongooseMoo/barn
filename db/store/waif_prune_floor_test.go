package store

import (
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

// Releasing a transaction used to scan the reader floor, under the exclusive
// lock that every BeginSnapshot takes shared, whenever any WAIF history was
// tracked. A release that leaves an older reader live cannot have raised the
// floor to a tracked history, and must find that out without the scan.
func TestReleaseBehindOlderReaderDoesNotScanTheFloor(t *testing.T) {
	s := NewStore()
	old := s.BeginSnapshot(0)
	w := types.NewWaif(0, 0).SetProperty("n", types.NewInt(0))
	if ec := s.DirectTxn().SetWaifProperty(w, "n", types.NewInt(1)); ec != types.E_NONE {
		t.Fatal(ec)
	}
	if got := len(s.waifHistory); got != 1 {
		t.Fatalf("tracked histories = %d, want 1 pinned by the old reader", got)
	}
	young := s.BeginSnapshot(0)

	// A floor scan needs this lock exclusively, so it cannot run while it is
	// held shared.
	s.readTSFloorMu.RLock()
	released := make(chan struct{})
	go func() {
		young.Release()
		close(released)
	}()
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		s.readTSFloorMu.RUnlock()
		t.Fatal("release behind an older reader waited for the floor scan's lock")
	}
	s.readTSFloorMu.RUnlock()
	if got := len(s.waifHistory); got != 1 {
		t.Fatalf("tracked histories = %d after the young reader left, want 1", got)
	}

	old.Release()
	if got := len(s.waifHistory); got != 0 {
		t.Fatalf("tracked histories = %d after the last reader left, want 0", got)
	}
	runtime.KeepAlive(w)
}

// The per-shard oldest timestamp must equal the smallest registered one after
// every registration and deregistration, including duplicates of a timestamp
// and a shard that empties.
func TestOldestLiveReadTSFollowsTheRegistry(t *testing.T) {
	s := NewStore()
	rng := rand.New(rand.NewSource(3))
	live := map[uint64]int{}
	var order []uint64
	for step := 0; step < 5000; step++ {
		if len(order) == 0 || rng.Intn(5) < 3 {
			ts := uint64(rng.Intn(200))
			s.registerReadTS(ts)
			live[ts]++
			order = append(order, ts)
		} else {
			i := rng.Intn(len(order))
			ts := order[i]
			order[i] = order[len(order)-1]
			order = order[:len(order)-1]
			s.deregisterReadTS(ts)
			if live[ts]--; live[ts] == 0 {
				delete(live, ts)
			}
		}
		want, have := uint64(0), false
		for ts := range live {
			if !have || ts < want {
				want, have = ts, true
			}
		}
		got, ok := s.oldestLiveReadTS()
		if ok != have || got != want {
			t.Fatalf("step %d: oldestLiveReadTS() = %d, %v; want %d, %v", step, got, ok, want, have)
		}
		if !have {
			want = s.clock.Load()
		}
		if floor := s.historyFloor(); floor != want {
			t.Fatalf("step %d: historyFloor() = %d, want %d", step, floor, want)
		}
	}
}
