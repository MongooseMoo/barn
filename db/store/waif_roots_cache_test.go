package store

import (
	"testing"

	"github.com/MongooseMoo/barn/types"
)

func waifIdentities(values []types.Value) map[types.WaifIdentity]bool {
	out := make(map[types.WaifIdentity]bool, len(values))
	for _, v := range values {
		if v.Type() == types.TYPE_WAIF {
			out[v.WaifIdentity()] = true
		}
	}
	return out
}

func requireRoots(t *testing.T, s *Store, want ...types.Value) {
	t.Helper()
	got := waifIdentities(s.PersistentWaifRoots())
	if len(got) != len(want) {
		t.Fatalf("PersistentWaifRoots has %d waifs, want %d", len(got), len(want))
	}
	for _, w := range want {
		if !got[w.WaifIdentity()] {
			t.Fatalf("PersistentWaifRoots missing %v", w)
		}
	}
}

func requireCacheHit(t *testing.T, s *Store, hit bool) {
	t.Helper()
	entry := s.waifRootsCache.Load()
	if (entry != nil && entry.epoch == s.waifRootsEpoch.Load() && entry.graphEpoch == types.WaifGraphEpoch()) != hit {
		t.Fatalf("cache hit = %v, want %v", !hit, hit)
	}
}

// The memoized top-level scan is reused across writes that cannot change it
// and invalidated by every write that can.
func TestPersistentWaifRootsCacheTracksWrites(t *testing.T) {
	s, ids := immutFixture(t, 2)
	a, b := ids[0], ids[1]
	for _, id := range ids {
		for _, name := range []string{"w", "n"} {
			if ec := s.DirectTxn().DefineProperty(id, name, NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
				t.Fatalf("DefineProperty #%d.%s: %v", id, name, ec)
			}
		}
	}
	requireRoots(t, s)
	requireCacheHit(t, s, true)

	w1 := types.NewWaif(a, 0)
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.SetPropertyValue(a, "w", w1) })
	requireCacheHit(t, s, false) // a waif landed in a property
	requireRoots(t, s, w1)
	requireCacheHit(t, s, true)

	// An int write to another slot leaves the memo valid.
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.SetPropertyValue(b, "n", types.NewInt(7)) })
	requireCacheHit(t, s, true)
	requireRoots(t, s, w1)

	// A waif inside a list counts as a top-level root.
	w2 := types.NewWaif(a, 0)
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode {
		return tx.SetPropertyValue(b, "w", types.NewList([]types.Value{types.NewInt(1), w2}))
	})
	requireCacheHit(t, s, false)
	requireRoots(t, s, w1, w2)

	// Overwriting a waif-holding slot with an int drops the root.
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.SetPropertyValue(a, "w", types.NewInt(0)) })
	requireCacheHit(t, s, false)
	requireRoots(t, s, w2)

	// A direct (coarse) write invalidates too.
	if ec := s.DirectTxn().SetPropertyValue(b, "w", types.NewInt(0)); ec != types.E_NONE {
		t.Fatalf("direct write: %v", ec)
	}
	requireRoots(t, s)
}

// A WAIF's own properties mutate in place with no store write, so the closure
// must be expanded on every call rather than memoized.
func TestPersistentWaifRootsExpandsClosuresFresh(t *testing.T) {
	s, ids := immutFixture(t, 1)
	a := ids[0]
	if ec := s.DirectTxn().DefineProperty(a, "w", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	outer := types.NewWaif(a, 0)
	inner1 := types.NewWaif(a, 0)
	outer.SetProperty("child", inner1)
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.SetPropertyValue(a, "w", outer) })
	requireRoots(t, s, outer, inner1)
	requireCacheHit(t, s, true)

	inner2 := types.NewWaif(a, 0)
	outer.SetProperty("child", inner2) // in place, no store write: moves the graph epoch only
	requireCacheHit(t, s, false)
	requireRoots(t, s, outer, inner2)
	requireCacheHit(t, s, true)
}

// Recycling an object that holds a waif drops it from the roots.
func TestPersistentWaifRootsRecycleInvalidates(t *testing.T) {
	s, ids := immutFixture(t, 1)
	a := ids[0]
	if ec := s.DirectTxn().DefineProperty(a, "w", NewProperty(types.NewInt(0), 0, PropRead|PropWrite, false, true)); ec != types.E_NONE {
		t.Fatalf("DefineProperty: %v", ec)
	}
	w := types.NewWaif(a, 0)
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { return tx.SetPropertyValue(a, "w", w) })
	requireRoots(t, s, w)
	commitWrite(t, s, func(tx *StoreTxn) types.ErrorCode { _, ec := tx.RecycleObject(a); return ec })
	requireRoots(t, s)
}
