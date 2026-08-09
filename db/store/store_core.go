package store

import (
	"fmt"
	"github.com/MongooseMoo/barn/types"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

// objectSlot is the per-id storage cell in Store.objects. The published image
// (slot.ptr) is an IMMUTABLE *Object: once a pointer is Stored into a slot, that
// *Object's fields and the *Property/*Verb nodes it owns are never written again.
// Readers Load() the pointer once and use that frozen snapshot — no lock, no torn
// read. The copy-on-write property-value committer builds a NEW *Object and
// publishes it via slot.ptr.Store under slot.mu; same-object committers serialize
// on slot.mu, disjoint committers never share a slot.
//
// The slot struct (not the *Object) carries the sync.Mutex, so the mutex is never
// value-copied with the image (no copylocks). The map of *objectSlot has stable
// slot identity for the slot's lifetime; the map skeleton (adding a key) is still
// mutated only under store.mu.
type objectSlot struct {
	ptr atomic.Pointer[Object]
	mu  sync.Mutex
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
	_      [40]byte // pad: Mutex(8) + map ptr(8) + pad(40) = 56; rounded clear of a line
}

type Store struct {
	mu  sync.RWMutex
	dir objectDir // segmented lock-free directory: id -> *objectSlot
	// maxObjID (highest non-anon id, for max_object()) and highWaterID (highest
	// allocated id incl. anon, for NextID()) are atomic so a decentralized committer
	// (holding only store.mu.RLock) can allocate an id and CAS-max them without the
	// exclusive lock. allocateID()/casMaxID() are the only mutators.
	maxObjID    atomic.Int64
	highWaterID atomic.Int64
	recycledMu  sync.Mutex    // guards recycledID against concurrent decentralized recyclers
	recycledID  []types.ObjID // Track recycled IDs (for future reuse via recreate)
	clock       atomic.Uint64
	historyMu   sync.Mutex // guards history-map appends from concurrent COW committers
	history     map[types.ObjID][]objectHistory

	// readTSFloorMu makes choosing/registering a read timestamp linearizable with
	// historyFloor's cross-shard scan. BeginReadOnly holds it shared from the clock
	// sample through the shard insertion; historyFloor holds it exclusively while
	// scanning. Registrations remain concurrent with each other, and deregistration
	// stays shard-local because missing a reader that has already released is safe.
	readTSFloorMu sync.RWMutex

	// readTSShards holds the multiset of readTS values of currently-live read-only
	// transactions, SHARDED by readTS to cut the per-transaction lock contention on
	// the commit-dominated path: every Begin/Release/commit touches this registry, so
	// a single global mutex serialized all 32 workers here (measured 88% of mutex
	// contention). Sharding by readTS keeps each shard's multiset self-consistent
	// (a given readTS always maps to the same shard), and historyFloor() = min key
	// across shards (or clock if empty). readTSFloorMu prevents a scan from missing a
	// completed registration in a shard it already visited. COW Phase 4 history GC.
	readTSShards [readTSShardCount]readTSShard

	// anonCreations is a monotonic counter bumped every time an anonymous object
	// is created via CreateObject(..., anonymous=true). It lets the orphan-anon GC
	// fast-path detect, without taking s.mu, whether any anonymous object could
	// have been created since a task's GC floor; if not, the orphan recycle
	// candidate set (anon with id >= floor) is provably empty and the O(N)
	// reachability sweep can be skipped. Read/written atomically.
	anonCreations atomic.Uint64

	// anonObjects holds anonymous objects out-of-band, keyed by the identity id
	// they were loaded/created with. Anonymous objects NEVER live in the regular
	// numbered object space (objects map) and never occupy a regular numeric id:
	// in ToastStunt they exist only as _TYPE_ANON values at runtime and are
	// assigned above-max serialization ids at dump time. Keeping them here (not in
	// objects) preserves that invariant and avoids the id collisions that crash
	// Toast's loader.
	anonObjects map[types.ObjID]*Object

	// MVCC commit observability counters (Phase A instrumentation). Bumped
	// lock-free, mirroring the anonCreations pattern. commitAttempts/Successes/
	// Conflicts are bumped inside StoreTxn.Commit; commitRetries is bumped by the
	// execution runtime each time it loops back after a retryable conflict. These are
	// observation-only and never affect control flow.
	commitAttempts  atomic.Uint64
	commitSuccesses atomic.Uint64
	commitConflicts atomic.Uint64
	commitRetries   atomic.Uint64

	// commitGate serializes an escalated commit attempt against all ordinary
	// commits. Ordinary StoreTxn.Commit holds it shared (outermost, before any
	// store lock — lock order is commitGate, then s.mu). A task that keeps
	// losing validation acquires it exclusively via EscalationLock, re-executes,
	// and commits a txn marked gateExempt: with no ordinary commit able to
	// interleave between its snapshot and its validation, it cannot lose again.
	commitGate        sync.RWMutex
	commitEscalations atomic.Uint64

	waifRegistry    map[types.ObjID]map[unsafe.Pointer]struct{} // Track live waifs by class (keyed on waif identity)
	verbCacheClears int64
	verbCacheMisses int64

	pendingFinalizations []types.Value
}

func NewStore() *Store {
	s := &Store{
		anonObjects: make(map[types.ObjID]*Object),
		recycledID:  []types.ObjID{},
		history:     make(map[types.ObjID][]objectHistory),
	}
	s.maxObjID.Store(-1)
	s.highWaterID.Store(-1)
	return s
}

// maxObjectID returns the highest non-anonymous object id, or -1 if none. Lock-free.
func (s *Store) maxObjectID() types.ObjID { return types.ObjID(s.maxObjID.Load()) }

// highWater returns the highest allocated id (including anonymous), or -1. Lock-free.
func (s *Store) highWater() types.ObjID { return types.ObjID(s.highWaterID.Load()) }

// allocateID atomically allocates the next unique object id (bumping highWaterID). It
// is the SINGLE id allocator for numbered, anonymous, and decentralized creates, so
// no two allocations ever collide even without the store lock.
func (s *Store) allocateID() types.ObjID { return types.ObjID(s.highWaterID.Add(1)) }

// appendRecycledID records id as recycled, guarded by recycledMu so a decentralized
// recycle committer (holding only store.mu.RLock) does not race LowestFreeID's read.
// Coarse recyclers append under store.mu.Lock (which excludes RLock committers), so the
// two never run concurrently; recycledMu serializes concurrent decentralized appends
// and the RLock read.
func (s *Store) appendRecycledID(id types.ObjID) {
	s.recycledMu.Lock()
	s.recycledID = append(s.recycledID, id)
	s.recycledMu.Unlock()
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
// freely; never mutate it (it may be shared with concurrent readers). Callers on
// the read paths hold store.mu.RLock so the map skeleton is stable during the
// lookup; the atomic Load itself is the acquire barrier that publishes the image.
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

// publishLocked publishes obj into id's slot. Used by the coarse (store.mu-held)
// writers and by object insertion; the map skeleton is mutated under store.mu.
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

// MVCC commit observability accessors (Phase A). All lock-free.
func (s *Store) CommitAttempts() uint64  { return s.commitAttempts.Load() }
func (s *Store) CommitSuccesses() uint64 { return s.commitSuccesses.Load() }
func (s *Store) CommitConflicts() uint64 { return s.commitConflicts.Load() }
func (s *Store) CommitRetries() uint64   { return s.commitRetries.Load() }

// ActiveReadTransactions returns the number of StoreTxn read timestamps currently
// registered with history GC, including multiple transactions at the same timestamp.
func (s *Store) ActiveReadTransactions() int {
	total := 0
	for i := range s.readTSShards {
		sh := &s.readTSShards[i]
		sh.mu.Lock()
		for _, count := range sh.counts {
			total += count
		}
		sh.mu.Unlock()
	}
	return total
}

// NoteCommitRetry records one engine-side conflict retry (loop-back). It is
// the only exported mutator; the engine lives in another package and cannot
// touch the unexported counter fields directly.
func (s *Store) NoteCommitRetry() { s.commitRetries.Add(1) }

func (s *Store) CommitEscalations() uint64 { return s.commitEscalations.Load() }

// EscalationLock acquires the commit gate exclusively for a bounded-escalation
// attempt: while held, no ordinary commit can start, so a gateExempt txn
// snapshotted and committed under it validates against a frozen store. Direct
// live-store mutations (the LiveStoreMutated paths) bypass the gate; the
// runtime's retry cap remains the backstop for that rare interleaving.
func (s *Store) EscalationLock() {
	s.commitGate.Lock()
	s.commitEscalations.Add(1)
}

func (s *Store) EscalationUnlock() { s.commitGate.Unlock() }

func (s *Store) ReadTimestamp() uint64 {
	return s.clock.Load()
}

// bumpClockLocked advances the global commit clock and returns the new value.
// The clock is an atomic counter (Option B): bumping it does not require
// store.mu, so the decentralized COW committer (which holds only store.mu.RLock)
// and the coarse writers (store.mu.Lock) draw distinct, globally-monotonic
// timestamps from the same source. The "_Locked" suffix is retained for the many
// coarse callers; the operation itself is lock-free.
func (s *Store) bumpClockLocked() uint64 {
	return s.bumpClock()
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

func (s *Store) rememberObjectLocked(obj *Object) {
	if obj == nil {
		return
	}
	ts := objectVersion(obj)
	entries := s.history[obj.id]
	if len(entries) > 0 && entries[len(entries)-1].ts == ts {
		return
	}
	s.history[obj.id] = append(entries, objectHistory{
		ts:  ts,
		obj: cloneObjectForReadTxn(obj),
	})

	// History GC (Phase 4): prune this object's now-dead old versions below the
	// live-read floor. The coarse path holds store.mu.Lock (exclusive), so this is
	// safe against objectLocked and the decentralized committer (both hold only
	// store.mu.RLock); pruneObjectHistory takes historyMu as a leaf lock (always
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
	// s.history directly is safe: the caller holds s.mu.Lock (exclusive), which
	// excludes the decentralized committers that guard history with historyMu.
	ts := objectVersion(old)
	entries := s.history[old.id]
	if len(entries) == 0 || entries[len(entries)-1].ts != ts {
		s.history[old.id] = append(entries, objectHistory{ts: ts, obj: old})
	}
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
// the numbered s.objects map or out-of-band in s.anonObjects. It is the single
// source of truth for "where does a live object with this id live", and every
// per-id resolver routes through it so that a valid object value resolves
// identically regardless of which map backs it — a numbered object, a
// database-loaded anonymous object, or a runtime-created anonymous object
// (create(...,1) routes to s.anonObjects; see CreateObject). Unlike
// lookupAnonymousLocked it does NOT require obj.anonymous: it resolves ANY live
// object. Recycled/invalid objects resolve to nil (validLiveObject filter).
// Caller holds s.mu. This completes F2 (commit 7318d24), which taught the
// snapshot/GC scans about s.anonObjects but left the per-id resolvers numbered-only.
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

// Add adds a new object to the store
// Returns error if object ID already exists

func (s *Store) Add(obj *Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.load(obj.id) != nil {
		return fmt.Errorf("object #%d already exists", obj.id)
	}

	ts := s.bumpClockLocked()
	stampObjectAll(obj, ts)
	s.insertObjectLocked(obj)
	return nil
}

// AddAnonymous ingests an anonymous object loaded from the database into the
// store's out-of-band anonymous collection. The object is keyed by the
// above-max serialization id it was loaded with; this id never enters the
// objects map or maxObjID (max_object() excludes anons). It MUST raise
// highWaterID, though: a Barn anon's identity id is permanent (values are
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
	stampObjectAll(obj, ts)
	s.anonObjects[obj.id] = obj
	casMaxID(&s.highWaterID, obj.id)
}

func (s *Store) insertObjectLocked(obj *Object) {
	s.publishLocked(obj.id, obj)

	// High water ID tracks all allocations (including anonymous); max object ID
	// excludes anonymous. Both are monotonic maxes (CAS) so concurrent decentralized
	// committers never lose the larger value.
	casMaxID(&s.highWaterID, obj.id)
	if !obj.anonymous {
		casMaxID(&s.maxObjID, obj.id)
	}
}

func (s *Store) SetObjectName(objID types.ObjID, name string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	obj.name = name
	stampObjectScalar(obj, ts)
	return types.E_NONE
}

func (s *Store) SetObjectOwner(objID types.ObjID, owner types.ObjID) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	obj.owner = owner
	stampObjectScalar(obj, ts)
	return types.E_NONE
}

func (s *Store) SetObjectLocationRaw(objID types.ObjID, location types.ObjID) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	obj.location = location
	stampObjectRelationship(obj, ts)
	return types.E_NONE
}

func (s *Store) SetObjectFlag(objID types.ObjID, flag ObjectFlags, enabled bool) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	if enabled {
		obj.flags = obj.flags.Set(flag)
	} else {
		obj.flags = obj.flags.Clear(flag)
	}
	stampObjectScalar(obj, ts)
	return types.E_NONE
}

func (s *Store) ObjectName(objID types.ObjID) (string, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return "", types.E_INVIND
	}
	return obj.name, types.E_NONE
}

func (s *Store) ObjectOwner(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.ObjNothing, types.E_INVIND
	}
	return obj.owner, types.E_NONE
}

func (s *Store) ObjectFlags(objID types.ObjID) (ObjectFlags, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return 0, types.E_INVIND
	}
	return obj.flags, types.E_NONE
}

func (s *Store) HasObjectFlag(objID types.ObjID, flag ObjectFlags) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return false, types.E_INVIND
	}
	return obj.flags.Has(flag), types.E_NONE
}

func (s *Store) ObjectIsAnonymous(objID types.ObjID) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return false, types.E_INVIND
	}
	return obj.anonymous, types.E_NONE
}

func (s *Store) ObjectExists(objID types.ObjID) types.ErrorCode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.liveObjectLocked(objID) != nil {
		return types.E_NONE
	}
	if obj := s.load(objID); obj != nil && obj.recycled {
		return types.E_INVARG
	}
	if obj := s.anonObjects[objID]; obj != nil && obj.recycled {
		return types.E_INVARG
	}
	return types.E_INVIND
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

func (s *Store) AliasStrings(objID types.ObjID) ([]string, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}
	prop, ok := obj.properties["aliases"]
	if !ok {
		return nil, types.E_NONE
	}
	listVal := prop.value
	if listVal.Type() != types.TYPE_LIST {
		return nil, types.E_NONE
	}
	aliases := make([]string, 0, listVal.Len())
	for i := 1; i <= listVal.Len(); i++ {
		if elem := listVal.Get(i); elem.Type() == types.TYPE_STR {
			aliases = append(aliases, elem.Str())
		}
	}
	return aliases, types.E_NONE
}
