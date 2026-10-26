package store

import (
	"context"
	"fmt"
	"github.com/MongooseMoo/barn/internal/commitgate"
	"github.com/MongooseMoo/barn/types"
	"math"
	"strings"
	"sync"
	"sync/atomic"
)

// objectSlot is the per-id storage cell in Store.dir. The published image
// (slot.ptr) is an IMMUTABLE *Object once readers can reach it: its fields and
// the *Property/*Verb nodes it owns are never written again. Readers Load() the
// pointer once and use that frozen snapshot — no lock, no torn read. The
// decentralized committer builds a NEW *Object and publishes it via
// slot.ptr.Store while holding Store.commitMu, so one commit publishes at a
// time. A coarse writer publishes a fresh clone and mutates it while still
// holding store.mu.Lock, before any reader can load it (republishForMutation).
//
// A slot has stable identity for its lifetime in the directory.
type objectSlot struct {
	ptr atomic.Pointer[Object]
}

// readTSShardCount shards the live-readTS registry. Sized to scatter a typical
// worker fleet (GOMAXPROCS) so register/deregister rarely collide, while keeping
// historyFloor()'s per-commit cross-shard scan cheap.
const readTSShardCount = 16

// readTSShard is one shard of the live-readTS multiset. It is padded to a cache
// line so adjacent shards' mutexes never false-share.
type readTSShard struct {
	mu     sync.Mutex
	counts map[uint64]int
	// oldest is the smallest key in counts plus one, or zero when counts is
	// empty. It is written under mu and read without it by oldestLiveReadTS.
	oldest atomic.Uint64
	_      [32]byte // pad: Mutex(8) + map ptr(8) + oldest(8) + pad(32) = 56; rounded clear of a line
}

// idAllocator is the store's object id allocator.
type idAllocator struct {
	// max (highest non-anon id, for max_object()) and highWater (highest
	// allocated id incl. anon, for NextID()) are atomic so a decentralized
	// committer (holding only store.mu.RLock) can allocate an id and CAS-max them
	// without the exclusive lock. After NewStore, allocateID(), casMaxID() and
	// ResetMaxObject() are the only mutators.
	max        atomic.Int64
	highWater  atomic.Int64
	recycledMu sync.Mutex    // guards recycled against concurrent decentralized recyclers
	recycled   []types.ObjID // recycled numbered ids, offered again by LowestFreeID
}

// readerRegistry registers the read timestamps of live transactions, so
// historyFloor knows which old images a reader may still need. See
// store_history_gc.go.
type readerRegistry struct {
	// floorMu makes choosing/registering a read timestamp linearizable with
	// historyFloor's cross-shard scan. BeginSnapshot holds it shared from the clock
	// sample through the shard insertion; historyFloor holds it exclusively while
	// scanning. Registrations remain concurrent with each other, and deregistration
	// stays shard-local because missing a reader that has already released is safe.
	floorMu sync.RWMutex

	// shards holds the multiset of readTS values of currently-live read-only
	// transactions, SHARDED by readTS to cut the per-transaction lock contention on
	// the commit-dominated path: every Begin/Release/commit touches this registry, so
	// a single global mutex serialized all 32 workers here (measured 88% of mutex
	// contention). Sharding by readTS keeps each shard's multiset self-consistent
	// (a given readTS always maps to the same shard), and historyFloor() = min key
	// across shards (or clock if empty). floorMu prevents a scan from missing a
	// completed registration in a shard it already visited.
	shards [readTSShardCount]readTSShard
}

// historyIndex holds each object's superseded images, for readers whose
// snapshot predates the live image. See store_history_gc.go.
type historyIndex struct {
	mu   sync.Mutex // guards byID appends from concurrent COW committers
	byID map[types.ObjID][]objectHistory
}

// waifRootsMemo memoizes the PersistentWaifRoots top-level scan. epoch
// advances whenever the set of WAIFs held directly by live objects' property
// values may have changed; cache is valid for one epoch. See
// noteWaifRootsChanged.
type waifRootsMemo struct {
	epoch atomic.Uint64
	cache atomic.Pointer[persistentWaifRootsEntry]
}

// verbDispatch is the store-level verb dispatch memo. memo caches verb
// resolution (object, name, execute-required) -> (definer, map key) across
// transactions, and size counts its entries so it can be swapped whole when
// full. shapeChangeTS is the clock value taken after the most recent change
// that can alter any resolution (verb add/delete/rename/code image, parents,
// recycle, renumber). See StoreTxn.lookupVerbDispatchMemo.
type verbDispatch struct {
	memo          atomic.Pointer[sync.Map]
	size          atomic.Int64
	shapeChangeTS atomic.Uint64
}

// lockedPropPool interns the shared property bases of every object, loaded or
// created, for the life of the store. mu guards pool: transactional creates
// share their tables without holding the store's mu.
type lockedPropPool struct {
	mu   sync.Mutex
	pool *propSharePool
}

type Store struct {
	mu sync.RWMutex
	// commitMu serializes decentralized commits. It is taken inside
	// store.mu.RLock and held from read-set validation through the publication
	// of every image and the clock store that makes them visible, so no commit
	// validates against a state another is about to replace, and no snapshot
	// begins at a timestamp whose images are only partly published. Coarse
	// writers hold store.mu.Lock instead, which excludes every RLock holder.
	commitMu sync.Mutex
	dir      objectDir // segmented lock-free directory: id -> *objectSlot
	// directTxn is the shared transaction used when code executes outside an
	// optimistic task snapshot. It runs the same method bodies as a snapshot
	// transaction, but it reads what is published now and records nothing, and
	// each write is staged on a private transaction and published at once
	// (writeNow). It never accumulates transaction state.
	directTxn StoreTxn
	// writer is that private transaction: writeNow resets and uses it under
	// mu.Lock, so one writer serves every direct write (lockedWriter).
	writer StoreTxn
	// ids allocates object ids and keeps recycled ones (idAllocator).
	ids   idAllocator
	clock atomic.Uint64
	// history holds superseded object images (historyIndex).
	history            historyIndex
	waifDomain         *types.WaifDomain
	waifHistory        map[types.WaifIdentity]*waifHistoryEntry
	waifHistoryQueue   waifHistoryQueue // the same entries, ordered by due
	waifHistoryPending atomic.Bool
	// waifHistoryDue is the smallest due timestamp in waifHistoryQueue, or zero
	// when it is empty. A reader floor below it can prune nothing; every
	// publication lowers it to cover new history before sampling that floor.
	waifHistoryDue atomic.Uint64
	// waifPruneVisits counts the entries pruneWaifHistoryLocked has examined.
	waifPruneVisits uint64

	// readers registers live transactions' read timestamps (readerRegistry).
	readers readerRegistry

	// anonCreations is a monotonic counter bumped every time an anonymous object
	// is created via CreateObject(..., anonymous=true). It lets the orphan-anon GC
	// fast-path detect, without taking s.mu, whether any anonymous object could
	// have been created since a task's GC floor; if not, the orphan recycle
	// candidate set (anon with id >= floor) is provably empty and the O(N)
	// reachability sweep can be skipped. Read/written atomically.
	anonCreations atomic.Uint64

	// anonObjects holds anonymous objects out-of-band, keyed by the identity id
	// they were loaded/created with. Anonymous objects NEVER live in the regular
	// numbered object space (dir) and never occupy a regular numeric id:
	// in ToastStunt they exist only as _TYPE_ANON values at runtime and are
	// assigned above-max serialization ids at dump time. Keeping them here (not in
	// dir) preserves that invariant and avoids the id collisions that crash
	// Toast's loader.
	anonObjects map[types.ObjID]*Object

	// MVCC commit observability counters. Bumped lock-free, mirroring the
	// anonCreations pattern. commitAttempts/Successes/Conflicts are bumped inside
	// StoreTxn.Commit; commitRetries is bumped by the execution runtime each time
	// it loops back after a retryable conflict. These are observation-only and
	// never affect control flow.
	commitAttempts  atomic.Uint64
	commitSuccesses atomic.Uint64
	commitConflicts atomic.Uint64
	commitRetries   atomic.Uint64
	// propertyWriteElisions counts SetPropertyValue calls dropped because the
	// slot already held an Identical value (see SetPropertyValue).
	propertyWriteElisions atomic.Uint64
	// waifRoots memoizes the PersistentWaifRoots scan (waifRootsMemo).
	waifRoots waifRootsMemo
	// dispatch is the verb dispatch memo and its invalidation stamp
	// (verbDispatch).
	dispatch verbDispatch
	// hotProps tracks the properties whose reads keep losing validation, and
	// counts what renewing a transaction at such a read did. See
	// store_hot_read.go.
	hotProps hotProperties
	// conflicts counts, per key, what commits lost validation on. See
	// store_conflict_census.go.
	conflicts conflictCensus

	// commitGate serializes an escalated commit attempt against all ordinary
	// commits. Ordinary StoreTxn.Commit holds it shared (outermost, before any
	// store lock — lock order is commitGate, then s.mu). A task that keeps
	// losing validation acquires an exclusive grant, re-executes,
	// and commits a txn marked gateExempt: with no ordinary commit able to
	// interleave between its snapshot and its validation, it cannot lose again.
	commitGate        commitgate.Gate
	commitEscalations atomic.Uint64

	waifRegistry map[types.ObjID]map[types.WaifIdentity]struct{} // Track live waifs by class (keyed on waif identity)
	// verbCacheStats is verb_cache_stats()'s current interval: the
	// verbCacheCleared bit plus the miss count. One word, so a clear, a miss and
	// a consume each take effect whole.
	verbCacheStats atomic.Uint64

	pendingFinalizations []types.Value

	// propPool interns shared property bases (lockedPropPool).
	propPool lockedPropPool
}

func NewStore() *Store {
	s := &Store{
		anonObjects: make(map[types.ObjID]*Object),
		ids:         idAllocator{recycled: []types.ObjID{}},
		history:     historyIndex{byID: make(map[types.ObjID][]objectHistory)},
		waifDomain:  new(types.WaifDomain),
		propPool:    lockedPropPool{pool: newPropSharePool()},
	}
	s.directTxn.store = s
	s.directTxn.direct = true
	s.directTxn.readTS = math.MaxUint64
	s.ids.max.Store(-1)
	s.ids.highWater.Store(-1)
	return s
}

// DirectTxn returns the store-owned pass-through transaction. It is immutable,
// safe to share between goroutines, and performs every operation directly on
// live state without staging, validation, commit, or release bookkeeping.
func (s *Store) DirectTxn() *StoreTxn {
	if s == nil {
		return nil
	}
	return &s.directTxn
}

// maxObjectID returns the highest non-anonymous object id, or -1 if none. Lock-free.
func (s *Store) maxObjectID() types.ObjID { return types.ObjID(s.ids.max.Load()) }

// highWater returns the highest allocated id (including anonymous), or -1. Lock-free.
func (s *Store) highWater() types.ObjID { return types.ObjID(s.ids.highWater.Load()) }

// allocateID atomically allocates the next unique object id (bumping ids.highWater). It
// is the SINGLE id allocator for numbered, anonymous, and decentralized creates, so
// no two allocations ever collide even without the store lock.
func (s *Store) allocateID() types.ObjID { return types.ObjID(s.ids.highWater.Add(1)) }

// appendRecycledID records id as recycled, guarded by ids.recycledMu so a decentralized
// recycle committer (holding only store.mu.RLock) does not race LowestFreeID's read.
// Coarse recyclers append under store.mu.Lock (which excludes RLock committers), so the
// two never run concurrently; ids.recycledMu serializes concurrent decentralized appends
// and the RLock read.
func (s *Store) appendRecycledID(id types.ObjID) {
	s.ids.recycledMu.Lock()
	s.ids.recycled = append(s.ids.recycled, id)
	s.ids.recycledMu.Unlock()
}

// casMaxID raises *a to v when v is larger — a monotonic max safe under concurrent
// committers (two decentralized creates may publish out of id order; a plain Store
// would lose the larger value).
func casMaxID(a *atomic.Int64, v types.ObjID) {
	for {
		cur := a.Load()
		if int64(v) <= cur {
			return
		}
		if a.CompareAndSwap(cur, int64(v)) {
			return
		}
	}
}

// load returns the currently-published immutable *Object for id, or nil if the
// slot does not exist. The returned *Object is a frozen snapshot: read its fields
// freely; never mutate it (it may be shared with concurrent readers). The
// directory lookup is lock-free; the atomic Load is the acquire barrier that
// publishes the image.
func (s *Store) load(id types.ObjID) *Object {
	if slot := s.dir.slot(id); slot != nil {
		return slot.ptr.Load()
	}
	return nil
}

// slotFor returns the slot for id, creating it if absent. The directory is a
// concurrent segmented array, so slot creation is a lock-free CAS and does NOT
// require store.mu exclusively — which is what lets a decentralized create() publish
// a new object without a stop-the-world lock.
func (s *Store) slotFor(id types.ObjID) *objectSlot {
	return s.dir.getOrCreate(id)
}

// publishLocked publishes obj into id's slot, creating the slot if needed.
// Callers hold store.mu.Lock, or store.mu.RLock plus commitMu (a shared-lock
// commit).
func (s *Store) publishLocked(id types.ObjID, obj *Object) {
	s.slotFor(id).ptr.Store(obj)
}

func (s *Store) bumpClock() uint64 {
	for {
		v := s.clock.Add(1)
		if v != 0 {
			return v
		}
		// wrapped to 0: skip it (0 means "unset"); loop bumps again
	}
}

// AnonCreationCount returns the number of anonymous objects created via
// CreateObject(..., anonymous=true) over the store's lifetime. It is read
// atomically without taking s.mu, so it can be sampled at task start (alongside
// the GC floor) and compared at task end to learn whether any anonymous object
// was created since the floor. When the count is unchanged, the orphan-anon
// recycle candidate set is provably empty and the O(N) reachability sweep can be
// skipped entirely.
func (s *Store) AnonCreationCount() uint64 {
	return s.anonCreations.Load()
}

// MVCC commit observability accessors. All lock-free.
func (s *Store) CommitAttempts() uint64  { return s.commitAttempts.Load() }
func (s *Store) CommitSuccesses() uint64 { return s.commitSuccesses.Load() }

// PropertyWriteElisions is the number of property writes dropped as no-ops
// because the target slot already held an Identical value.
func (s *Store) PropertyWriteElisions() uint64 { return s.propertyWriteElisions.Load() }
func (s *Store) CommitConflicts() uint64       { return s.commitConflicts.Load() }
func (s *Store) CommitRetries() uint64         { return s.commitRetries.Load() }

// ActiveReadTransactions returns the number of StoreTxn read timestamps currently
// registered with history GC, including multiple transactions at the same timestamp.
func (s *Store) ActiveReadTransactions() int {
	total := 0
	for i := range s.readers.shards {
		sh := &s.readers.shards[i]
		sh.mu.Lock()
		for _, count := range sh.counts {
			total += count
		}
		sh.mu.Unlock()
	}
	return total
}

// NoteCommitRetry records one engine-side conflict retry (loop-back). The
// engine, which does the retrying, lives in another package.
func (s *Store) NoteCommitRetry() { s.commitRetries.Add(1) }

func (s *Store) CommitEscalations() uint64 { return s.commitEscalations.Load() }

// AcquireExclusive returns owner-held FIFO admission. Cancellation only
// withdraws the wait; it never revokes a grant returned to executing code.
func (s *Store) AcquireExclusive(ctx context.Context) (*commitgate.Grant, error) {
	g, err := s.commitGate.Acquire(ctx, commitgate.Exclusive)
	if err == nil {
		s.commitEscalations.Add(1)
	}
	return g, err
}

func (s *Store) readTimestamp() uint64 {
	return s.clock.Load()
}

// bumpClockLocked advances the global commit clock and returns the new value.
// The caller holds store.mu.Lock. That is a requirement, not a habit: a
// shared-lock commit reserves clock+1 under store.mu.RLock and moves the clock
// to it only after publishing (publishStagedLocked), which is sound only if
// nothing else moves the clock while an RLock is held. The one bump made under
// RLock is that commit's own noteVerbShapeChanged, after its clock store and
// still under commitMu.
func (s *Store) bumpClockLocked() uint64 {
	return s.bumpClock()
}

// noteWaifRootsChanged invalidates the PersistentWaifRoots memo. Every path
// that can change which WAIFs live directly in a persistent property value
// must call it AFTER the change is visible: the direct writers (under
// store.mu.Lock, where the scan cannot interleave) and a commit after its
// publishes. A value write whose old and
// new values cannot hold a finalizable (MayHoldFinalizable false) leaves the
// epoch alone, which is what keeps the memo hot on ordinary workloads.
func (s *Store) noteWaifRootsChanged() { s.waifRoots.epoch.Add(1) }

// noteVerbShapeChanged records that verb resolution may now differ from any
// memoized result. It must run AFTER the change is visible (after publish under
// a shared lock; anywhere inside an exclusive section). Memo entries and
// transactions whose snapshot predates this clock
// value stop using the memo, and a transaction that already dispatched
// through it fails validation and retries.
func (s *Store) noteVerbShapeChanged() { s.dispatch.shapeChangeTS.Store(s.bumpClock()) }

// verbDispatchMemoCap bounds the memo; when exceeded the whole map is swapped
// for a fresh one rather than evicting entry by entry.
const verbDispatchMemoCap = 1 << 17

func (s *Store) verbMemo() *sync.Map {
	if m := s.dispatch.memo.Load(); m != nil {
		return m
	}
	fresh := &sync.Map{}
	if s.dispatch.memo.CompareAndSwap(nil, fresh) {
		return fresh
	}
	return s.dispatch.memo.Load()
}

type objectHistory struct {
	ts  uint64
	obj *Object
}

func objectVersion(obj *Object) uint64 {
	if obj == nil {
		return 0
	}
	version := obj.scalarVersion
	if obj.relationshipVersion > version {
		version = obj.relationshipVersion
	}
	if obj.propertyVersion > version {
		version = obj.propertyVersion
	}
	if obj.verbVersion > version {
		version = obj.verbVersion
	}
	return version
}

// withHistoryEntry returns entries with entry recorded as the newest superseded
// image. One timestamp identifies one commit, and a commit can supersede the same
// object more than once at its timestamp, so an entry with the newest entry's ts
// is the later image of that commit and replaces it. The replacement is a new
// slice: readers walk captured slice headers outside history.mu.
func withHistoryEntry(entries []objectHistory, entry objectHistory) []objectHistory {
	n := len(entries)
	if n == 0 || entries[n-1].ts != entry.ts {
		return append(entries, entry)
	}
	replaced := make([]objectHistory, n)
	copy(replaced, entries[:n-1])
	replaced[n-1] = entry
	return replaced
}

func (s *Store) rememberObjectLocked(obj *Object) {
	if obj == nil {
		return
	}
	s.history.byID[obj.id] = withHistoryEntry(s.history.byID[obj.id], objectHistory{
		ts:  objectVersion(obj),
		obj: cloneObjectForReadTxn(obj),
	})

	// History GC: prune this object's now-dead old versions below the
	// live-read floor. The coarse path holds store.mu.Lock (exclusive), so this is
	// safe against objectLocked and the decentralized committer (both hold only
	// store.mu.RLock); pruneObjectHistory takes history.mu as a leaf lock (always
	// nested inside store.mu — no lock-order inversion). The just-appended entry
	// carries the pre-mutation version; the caller stamps a strictly-newer version
	// onto the live image right after, so the live (current) image is never in
	// history and never pruned. Keeping the newest history entry <= floor preserves
	// every live reader's snapshot.
	s.pruneObjectHistory(obj.id, s.historyFloor())
}

// republishForMutation supersedes old's published image with a fresh, in-place-
// mutable CLONE and retains the OLD image immutably in history, returning the fresh
// clone for the caller to mutate. It replaces the rememberObjectLocked(obj) +
// mutate-obj-in-place pattern at every coarse (store.mu.Lock-held) mutation site.
//
// Callers hold s.mu.Lock EXCLUSIVE, so the freshly published image is not yet
// aliasable by any reader (readers take s.mu.RLock) and may be mutated in place
// race-free; the OLD image — which pre-existing lock-free read aliases may still
// hold — is never mutated. This makes published images truly immutable (the
// objectSlot contract above), which is what lets read transactions ALIAS the
// published image instead of deep-cloning it on every touch.
//
// Callers MUST mutate the RETURNED image, not the object they passed in. Anonymous
// objects have no COW slot and no history: the fresh image is republished into
// s.anonObjects and a pre-existing aliaser keeps the old immutable image.
func (s *Store) republishForMutation(old *Object) *Object {
	if old == nil {
		return nil
	}
	if old.anonymous {
		// Anonymous objects are NOT aliased on read — read transactions still
		// deep-clone them (they are rare, live out-of-band in s.anonObjects with no
		// COW slot and no history, and may be referenced by direct pointer). So
		// mutating an anon image in place remains safe: no reader holds an alias of
		// it. Return it unchanged for the caller to mutate as before.
		return old
	}
	fresh := cloneObjectForReadTxn(old)
	s.publishLocked(old.id, fresh)
	// Retain the OLD (now superseded, immutable) image as the history node — no
	// clone needed, mirroring rememberObjectLocked minus the copy. Touching
	// s.history.byID directly is safe: the caller holds s.mu.Lock (exclusive), which
	// excludes the decentralized committers that guard it with history.mu.
	s.history.byID[old.id] = withHistoryEntry(s.history.byID[old.id], objectHistory{ts: objectVersion(old), obj: old})
	s.pruneObjectHistory(old.id, s.historyFloor())
	return fresh
}

func stampObjectScalar(obj *Object, ts uint64) {
	if obj != nil {
		obj.scalarVersion = ts
	}
}

func stampObjectRelationship(obj *Object, ts uint64) {
	if obj != nil {
		obj.relationshipVersion = ts
	}
}

func stampObjectProperties(obj *Object, ts uint64) {
	if obj != nil {
		obj.propertyVersion = ts
		obj.propertyShapeVersion = ts
	}
}

func stampObjectVerbs(obj *Object, ts uint64) {
	if obj != nil {
		obj.verbVersion = ts
	}
}

func stampVerb(verb *Verb, ts uint64) {
	if verb != nil {
		verb.version = ts
	}
}

func stampObjectAll(obj *Object, ts uint64) {
	stampObjectScalar(obj, ts)
	stampObjectRelationship(obj, ts)
	stampObjectProperties(obj, ts)
	stampObjectVerbs(obj, ts)
}

// liveObjectLocked resolves the live object with identity id, whether it lives in
// the numbered directory (s.dir) or out-of-band in s.anonObjects. It is the single
// source of truth for "where does a live object with this id live", and every
// per-id resolver routes through it so that a valid object value resolves
// identically regardless of which map backs it — a numbered object, a
// database-loaded anonymous object, or a runtime-created anonymous object
// (create(...,1) routes to s.anonObjects; see CreateObject). Unlike
// lookupAnonymousLocked it does NOT require obj.anonymous: it resolves ANY live
// object. Recycled/invalid objects resolve to nil (validLiveObject filter).
// Caller holds s.mu.
//
// MVCC note: the numbered lookup goes through s.load (slot.ptr.Load) so the
// resolved image is the currently-published immutable snapshot, not a raw slot.
func (s *Store) liveObjectLocked(id types.ObjID) *Object {
	if obj := s.load(id); validLiveObject(obj) {
		return obj
	}
	if obj := s.anonObjects[id]; validLiveObject(obj) {
		return obj
	}
	return nil
}

// Get returns a flat, read-only ObjectView for a live object, plus ok=false if
// the object does not exist or is recycled/invalid. The store never hands out a
// live *Object to external callers.
func (s *Store) Get(id types.ObjID) (ObjectView, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(id)
	if obj == nil {
		return ObjectView{}, false
	}
	return obj.view(), true
}

// GetUnsafe returns a flat, read-only ObjectView without checking recycled
// status, plus ok=false if the slot was never allocated. Used by the database
// round-trip tool, which must inspect recycled slots too.
func (s *Store) GetUnsafe(id types.ObjID) (ObjectView, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.load(id)
	if obj == nil {
		return ObjectView{}, false
	}
	return obj.view(), true
}

// Add adds a new object to the store.
// Returns error if object ID already exists.
func (s *Store) Add(obj *Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.load(obj.id) != nil {
		return fmt.Errorf("object #%d already exists", obj.id)
	}

	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	s.noteVerbShapeChanged()
	stampObjectAll(obj, ts)
	s.insertObjectLocked(obj)
	return nil
}

// AddAnonymous ingests an anonymous object loaded from the database into the
// store's out-of-band anonymous collection. The object is keyed by the
// above-max serialization id it was loaded with; this id never enters the
// directory or ids.max (max_object() excludes anons). It MUST raise
// ids.highWater, though: a Barn anon's identity id is permanent (values are
// NewAnon(id), unlike Toast's pointer identity), so if allocateID later handed
// the same id to a regular object, every loaded _TYPE_ANON reference would
// silently resolve to that object — recycle(loaded_anon) then recycles (and
// boots!) an unrelated player. Conformance map_dump_persistence caught exactly
// that: Test.db's per-connect login create() took the first anon's serial id
// after restart.
func (s *Store) AddAnonymous(obj *Object) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !obj.anonymous {
		obj.anonymous = true
	}
	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	stampObjectAll(obj, ts)
	s.anonObjects[obj.id] = obj
	casMaxID(&s.ids.highWater, obj.id)
}

func (s *Store) insertObjectLocked(obj *Object) {
	s.publishLocked(obj.id, obj)

	// High water ID tracks all allocations (including anonymous); max object ID
	// excludes anonymous. Both are monotonic maxes (CAS) so concurrent decentralized
	// committers never lose the larger value.
	casMaxID(&s.ids.highWater, obj.id)
	if !obj.anonymous {
		casMaxID(&s.ids.max, obj.id)
	}
}

func (s *Store) ObjectIDsByNameSubstring(needle string, caseSensitive bool) []types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	searchNeedle := needle
	if !caseSensitive {
		searchNeedle = strings.ToLower(searchNeedle)
	}

	result := make([]types.ObjID, 0)
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj == nil || !validLiveObject(obj) {
			return true
		}
		name := strings.TrimSpace(obj.name)
		if !caseSensitive {
			name = strings.ToLower(name)
		}
		if strings.Contains(name, searchNeedle) {
			result = append(result, obj.id)
		}
		return true
	})
	return result
}

func (s *Store) ObjectsOwnedBy(owner types.ObjID) []types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.ObjID, 0)
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj != nil && validLiveObject(obj) && obj.owner == owner {
			result = append(result, obj.id)
		}
		return true
	})
	return result
}

// VisitAliasStrings calls fn with each string element of objID's "aliases"
// property, resolved as any property read is (a clear slot reads its
// ancestor's value), until fn returns false. It does nothing for an invalid
// object or a missing or non-list value. fn runs under the store read lock and
// must not call back into the store.
func (s *Store) VisitAliasStrings(objID types.ObjID, fn func(string) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, prop, errCode := s.findPropertyLocked(objID, "aliases")
	if errCode != types.E_NONE {
		return
	}
	listVal := prop.value
	if listVal.Type() != types.TYPE_LIST {
		return
	}
	for i := 1; i <= listVal.Len(); i++ {
		if elem := listVal.Get(i); elem.Type() == types.TYPE_STR && !fn(elem.Str()) {
			return
		}
	}
}
