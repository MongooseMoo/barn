package store

import (
	"runtime"

	"barn/types"
)

// store_history_gc.go — bounded history GC (COW Phase 4).
//
// s.history[id] holds the OLD immutable *Object versions a read-only transaction
// needs for time-travel: a reader at readTS=R sees the newest version with
// version<=R; if the live image's version is > R it walks history newest->oldest
// for the newest entry with ts<=R. Without pruning the list grows unbounded
// (one entry per committed write per object). This file adds the live-readTS
// floor tracker and the per-object prune that frees provably-dead old versions.
//
// THE INVARIANT (never violated): a history entry H for object O may be freed
// only if some NEWER version of O has ts<=floor, where
//   floor = min readTS of any currently-live StoreTxn (or the current clock if
//           no txn is live).
// Then every current/future reader runs at readTS>=floor and sees that newer
// version (or beyond), never H, and no reader at readTS<floor exists. Per object
// the rule is: keep the newest entry with ts<=floor plus every entry with
// ts>floor; drop the strictly-older entries. The newest entry with ts<=floor is
// retained because a reader at exactly floor still needs it.

// registerReadTS records a newly-begun transaction's readTS as live. Returns a
// token used to deregister it. floorMu guards the multiset.
func (s *Store) registerReadTS(readTS uint64) {
	s.floorMu.Lock()
	if s.activeReadTS == nil {
		s.activeReadTS = make(map[uint64]int)
	}
	s.activeReadTS[readTS]++
	s.floorMu.Unlock()
}

// deregisterReadTS removes one live registration for readTS. Idempotency is the
// caller's responsibility (StoreTxn.release uses a once-guard); calling it with a
// readTS that is not registered is a no-op (defensive — never drives a count
// negative).
func (s *Store) deregisterReadTS(readTS uint64) {
	s.floorMu.Lock()
	if n := s.activeReadTS[readTS]; n > 1 {
		s.activeReadTS[readTS] = n - 1
	} else if n == 1 {
		delete(s.activeReadTS, readTS)
	}
	s.floorMu.Unlock()
}

// historyFloor returns the minimum readTS of any currently-live transaction, or
// the current clock if none is live. A history entry strictly older than the
// newest-version-<=floor is provably unreachable by any current or future reader
// (future readers begin at readTS>=clock>=floor). When no txn is live the floor
// is the clock: nothing can be read below it, so everything but each object's
// newest image is dead.
//
// Using the clock as the no-live-txn floor is safe against a txn that begins
// concurrently: a new BeginReadOnly snapshots readTS=clock.Load() and registers
// it BEFORE it can issue any read (it holds store.mu while doing both, and the
// pruner holds store.mu.RLock which excludes that exclusive section for the
// coarse path; the decentralized pruner reads the floor and the registry both
// under the same locks the new txn must pass through). In all cases the floor
// returned is <= the readTS of every txn that can still read.
func (s *Store) historyFloor() uint64 {
	s.floorMu.Lock()
	min := uint64(0)
	have := false
	for ts := range s.activeReadTS {
		if !have || ts < min {
			min = ts
			have = true
		}
	}
	s.floorMu.Unlock()
	if !have {
		return s.clock.Load()
	}
	return min
}

// pruneHistoryBelowFloorLocked drops the dead old versions of object id from
// s.history given floor. It is called holding s.historyMu (so it is serialized
// with objectLocked's header capture and with concurrent committers' appends).
//
// entries are append-ordered, hence ascending by ts (each commit draws a strictly
// larger clock value). Find the largest index k with entries[k].ts <= floor; that
// entry is the newest version a reader at exactly floor still needs, so keep it
// and everything after it, dropping [0:k). If no entry has ts<=floor (k undefined)
// every entry is still needed by some reader below floor — keep all. Reslicing
// forward (entries[k:]) leaves the backing array intact for any objectLocked walk
// that already captured the old header; the dropped entries become unreachable
// only once no header references them, so Go's GC reclaims them with no data race.
func pruneHistoryBelowFloorLocked(entries []objectHistory, floor uint64) []objectHistory {
	if len(entries) == 0 {
		return entries
	}
	// Largest index with ts <= floor (entries ascending by ts).
	k := -1
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].ts <= floor {
			k = i
			break
		}
	}
	if k <= 0 {
		// k==-1: nothing <= floor, all entries still needed. k==0: newest-<=floor
		// is already the first entry, nothing to drop.
		return entries
	}
	return entries[k:]
}

// pruneObjectHistory prunes object id's history to the current floor under
// historyMu. Safe to call from the decentralized publish path (which holds
// store.mu.RLock + the slot mutex) and from the coarse path (store.mu.Lock).
func (s *Store) pruneObjectHistory(id types.ObjID, floor uint64) {
	s.historyMu.Lock()
	if entries, ok := s.history[id]; ok {
		pruned := pruneHistoryBelowFloorLocked(entries, floor)
		if len(pruned) == 0 {
			delete(s.history, id)
		} else if len(pruned) != len(entries) {
			s.history[id] = pruned
		}
	}
	s.historyMu.Unlock()
}

// finalizeStoreTxnRelease is the runtime-finalizer backstop: if a StoreTxn is
// dropped without an explicit Release (the scheduler re-begins/drops txns without
// a Close call in several paths), its readTS registration would otherwise leak
// and pin the floor forever, defeating GC. A finalizer can only run once the
// txn is unreachable — i.e. provably no longer live — so it can NEVER deregister
// a txn a reader could still use. It is the safe direction to err in.
func finalizeStoreTxnRelease(tx *StoreTxn) {
	tx.release()
}

// release deregisters this transaction's readTS exactly once and clears its
// finalizer. Called explicitly by the scheduler when it drops/replaces a txn
// (promptness) and, as a backstop, by the runtime finalizer (correctness even if
// an explicit call is missed). Idempotent: the second caller is a no-op.
func (tx *StoreTxn) release() {
	if tx == nil {
		return
	}
	if tx.released.Swap(true) {
		return
	}
	if tx.store != nil {
		tx.store.deregisterReadTS(tx.readTS)
	}
	runtime.SetFinalizer(tx, nil)
}

// Release deregisters this transaction's readTS from the live-read floor. The
// scheduler calls it when it finishes with a txn (commit+re-begin, or drop), so
// the floor advances promptly and dead history can be pruned. It is safe to call
// multiple times and safe to never call (the finalizer backstop releases a
// dropped txn eventually). After Release the txn must not be used to read again.
func (tx *StoreTxn) Release() {
	tx.release()
}
