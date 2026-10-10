package store

import (
	"runtime"
	"slices"

	"github.com/MongooseMoo/barn/types"
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
//
// The floor alone lets one long-lived reader retain every version written
// while it lives. pruneHistoryToReaders applies the same invariant per reader:
// of the entries above the floor it keeps only those some live or future
// reader can be served.

// registerReadTS records an explicit transaction readTS as live. It participates
// in the same registration-vs-floor-scan gate as currentReadTSAndRegister so a
// completed registration cannot be missed by an in-progress cross-shard scan.
func (s *Store) registerReadTS(readTS uint64) {
	s.readTSFloorMu.RLock()
	s.registerReadTSInShard(readTS)
	s.readTSFloorMu.RUnlock()
}

// currentReadTSAndRegister samples the current clock and registers that timestamp
// as one linearizable operation with respect to historyFloor. Without this gate a
// floor scan could pass the target shard after the clock sample but before the
// registration, then prune above the reader before BeginSnapshot returned.
func (s *Store) currentReadTSAndRegister() uint64 {
	s.readTSFloorMu.RLock()
	readTS := s.clock.Load()
	s.registerReadTSInShard(readTS)
	s.readTSFloorMu.RUnlock()
	return readTS
}

func (s *Store) registerReadTSInShard(readTS uint64) {
	sh := &s.readTSShards[readTS%readTSShardCount]
	sh.mu.Lock()
	if sh.counts == nil {
		sh.counts = make(map[uint64]int)
	}
	sh.counts[readTS]++
	if oldest := sh.oldest.Load(); oldest == 0 || readTS+1 < oldest {
		sh.oldest.Store(readTS + 1)
	}
	sh.mu.Unlock()
}

// deregisterReadTS removes one live registration for readTS. Idempotency is the
// caller's responsibility (StoreTxn.release uses a once-guard); calling it with a
// readTS that is not registered is a no-op (defensive — never drives a count
// negative).
func (s *Store) deregisterReadTS(readTS uint64) {
	sh := &s.readTSShards[readTS%readTSShardCount]
	sh.mu.Lock()
	if n := sh.counts[readTS]; n > 1 {
		sh.counts[readTS] = n - 1
	} else if n == 1 {
		delete(sh.counts, readTS)
		if sh.oldest.Load() == readTS+1 {
			oldest := uint64(0)
			for ts := range sh.counts {
				if oldest == 0 || ts+1 < oldest {
					oldest = ts + 1
				}
			}
			sh.oldest.Store(oldest)
		}
	}
	sh.mu.Unlock()
}

// oldestLiveReadTS returns a readTS that was registered at some moment during
// the call and is the smallest this lock-free pass saw, or false if it saw
// none. It is not the floor: it can miss a reader that is still registering,
// and its shards are sampled at different moments. It only ever proves that
// the floor was at or below the value returned, which is all a caller needs to
// decide that the floor has not reached some timestamp.
func (s *Store) oldestLiveReadTS() (uint64, bool) {
	oldest := uint64(0)
	for i := range s.readTSShards {
		if seen := s.readTSShards[i].oldest.Load(); seen != 0 && (oldest == 0 || seen < oldest) {
			oldest = seen
		}
	}
	if oldest == 0 {
		return 0, false
	}
	return oldest - 1, true
}

// historyFloor returns the minimum readTS of any currently-live transaction, or
// the current clock if none is live. A history entry strictly older than the
// newest-version-<=floor is provably unreachable by any current or future reader
// (future readers begin at readTS>=clock>=floor). When no txn is live the floor
// is the clock: nothing can be read below it, so everything but each object's
// newest image is dead.
//
// Using the clock as the no-live-txn floor is safe against a txn that begins
// concurrently because readTSFloorMu makes the floor scan exclusive with the
// clock-sample-through-registration interval in currentReadTSAndRegister. A scan
// therefore linearizes either before the sample or after the registration. In all
// cases the returned floor is <= the readTS of every transaction that can read.
func (s *Store) historyFloor() uint64 {
	s.readTSFloorMu.Lock()
	min := uint64(0)
	have := false
	for i := range s.readTSShards {
		sh := &s.readTSShards[i]
		sh.mu.Lock()
		if oldest := sh.oldest.Load(); oldest != 0 && (!have || oldest-1 < min) {
			min = oldest - 1
			have = true
		}
		sh.mu.Unlock()
	}
	if !have {
		min = s.clock.Load()
	}
	s.readTSFloorMu.Unlock()
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

// historyReaderPruneMin is the shortest history that is pruned to its readers.
const historyReaderPruneMin = 8

// readerPruneDue reports whether a history of n entries left by the floor
// prune should also be pruned to its readers. That prune reads every
// registration and copies what it keeps, so it runs each time the length
// reaches a power of two: a history that some reader really does need is
// rescanned once per doubling, and one that was cut back is rescanned only
// after it has grown to the same length again.
func readerPruneDue(n int) bool {
	return n >= historyReaderPruneMin && n&(n-1) == 0
}

// liveReadTimestamps returns the clock and, in ascending order, the readTS of
// every live transaction together with that clock. Like historyFloor it
// excludes registration for the length of the scan, so a transaction it did
// not see begins at or above the clock it returns.
func (s *Store) liveReadTimestamps() (readers []uint64, clock uint64) {
	s.readTSFloorMu.Lock()
	for i := range s.readTSShards {
		sh := &s.readTSShards[i]
		sh.mu.Lock()
		for ts := range sh.counts {
			readers = append(readers, ts)
		}
		sh.mu.Unlock()
	}
	clock = s.clock.Load()
	s.readTSFloorMu.Unlock()
	readers = append(readers, clock)
	slices.Sort(readers)
	return readers, clock
}

// pruneHistoryToReaders returns entries without the versions no reader can be
// served. readers and clock are what liveReadTimestamps returned.
//
// Entry i is the object's image from entries[i].ts until the next entry's ts,
// and the newest entry until the live image's version, which is not known here.
// A reader at R is served the newest entry with ts<=R, so entry i is needed by
// exactly the readers in that interval. A transaction that begins after the
// scan reads at or above clock, so it is served either the entry whose
// interval holds clock or one above it. Kept are therefore: the newest entry,
// every entry above clock, and every entry whose interval holds a reader or
// clock.
//
// The floor prune is the special case of one reader. It keeps everything above
// the oldest reader; this drops the versions that were written and superseded
// between two readers, which is all of them while one long task holds the
// floor and short tasks come and go at the clock.
//
// The kept entries are copied to a new slice: a snapshot read walks a captured
// header without the lock, so entries cannot be moved in place.
func pruneHistoryToReaders(entries []objectHistory, readers []uint64, clock uint64) []objectHistory {
	if len(entries) < 2 {
		return entries
	}
	needed := func(i int) bool {
		if i == len(entries)-1 || entries[i].ts > clock {
			return true
		}
		r, _ := slices.BinarySearch(readers, entries[i].ts)
		return r < len(readers) && readers[r] < entries[i+1].ts
	}
	keep := 0
	for i := range entries {
		if needed(i) {
			keep++
		}
	}
	if keep == len(entries) {
		return entries
	}
	kept := make([]objectHistory, 0, keep)
	for i := range entries {
		if needed(i) {
			kept = append(kept, entries[i])
		}
	}
	return kept
}

// pruneObjectHistory prunes object id's history to the current floor under
// historyMu, and to its readers when that is due. Safe to call from the
// decentralized publish path (which holds store.mu.RLock + the slot mutex) and
// from the coarse path (store.mu.Lock).
//
// The conflict census dates a rewrite by walking every image newer than the
// losing transaction's snapshot (propertyWasStaleAtRead), so while it is
// tracking, history is pruned to the floor only.
func (s *Store) pruneObjectHistory(id types.ObjID, floor uint64) {
	s.historyMu.Lock()
	remaining := 0
	if entries, ok := s.history[id]; ok {
		pruned := pruneHistoryBelowFloorLocked(entries, floor)
		if len(pruned) == 0 {
			delete(s.history, id)
		} else if len(pruned) != len(entries) {
			s.history[id] = pruned
		}
		remaining = len(pruned)
	}
	s.historyMu.Unlock()
	if !readerPruneDue(remaining) || s.conflictTracking.Load() {
		return
	}
	// The registry is scanned outside historyMu. Entries appended in between
	// are kept by the rule above whatever the scan saw.
	readers, clock := s.liveReadTimestamps()
	s.historyMu.Lock()
	if entries, ok := s.history[id]; ok {
		if pruned := pruneHistoryToReaders(entries, readers, clock); len(pruned) != len(entries) {
			s.history[id] = pruned
		}
	}
	s.historyMu.Unlock()
}

// HistoryStats is a point-in-time count of the superseded object images the
// store retains and of what holds the live-read floor behind the clock.
type HistoryStats struct {
	Objects, Entries int         // objects with history, and their entries in total
	MostObject       types.ObjID // the object with the most entries
	MostEntries      int
	Floor, Clock     uint64
	Readers          int // live readTS registrations
}

// HistoryStats counts retained history for diagnostics.
func (s *Store) HistoryStats() HistoryStats {
	stats := HistoryStats{Floor: s.historyFloor(), Clock: s.clock.Load()}
	s.mu.RLock()
	s.historyMu.Lock()
	for id, entries := range s.history {
		stats.Objects++
		stats.Entries += len(entries)
		if len(entries) > stats.MostEntries {
			stats.MostObject, stats.MostEntries = id, len(entries)
		}
	}
	s.historyMu.Unlock()
	s.mu.RUnlock()
	for i := range s.readTSShards {
		sh := &s.readTSShards[i]
		sh.mu.Lock()
		for _, n := range sh.counts {
			stats.Readers += n
		}
		sh.mu.Unlock()
	}
	return stats
}

// finalizeStoreTxnRelease is the runtime-finalizer backstop: if a StoreTxn is
// dropped without an explicit Release (the runtime re-begins/drops txns without
// a Close call in several paths), its readTS registration would otherwise leak
// and pin the floor forever, defeating GC. A finalizer can only run once the
// txn is unreachable — i.e. provably no longer live — so it can NEVER deregister
// a txn a reader could still use. It is the safe direction to err in.
func finalizeStoreTxnRelease(tx *StoreTxn) {
	tx.release()
}

// release deregisters this transaction's readTS exactly once and clears its
// finalizer. Called explicitly by the runtime when it drops/replaces a txn
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
		if tx.store.waifHistoryPending.Load() {
			tx.store.pruneWaifHistory()
		}
	}
	runtime.SetFinalizer(tx, nil)
}

// Release deregisters this transaction's readTS from the live-read floor. The
// runtime calls it when it finishes with a txn (commit+re-begin, or drop), so
// the floor advances promptly and dead history can be pruned. It is safe to call
// multiple times and safe to never call (the finalizer backstop releases a
// dropped txn eventually). After Release the txn must not be used to read again.
func (tx *StoreTxn) Release() {
	if tx == nil || tx.direct {
		return
	}
	tx.release()
}
