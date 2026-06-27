package store

import (
	"barn/types"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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

type Store struct {
	mu          sync.RWMutex
	objects     map[types.ObjID]*objectSlot
	maxObjID    types.ObjID   // Highest non-anonymous object ID (for max_object())
	highWaterID types.ObjID   // Highest allocated ID (including anonymous, for NextID())
	recycledID  []types.ObjID // Track recycled IDs (for future reuse via recreate)
	clock       atomic.Uint64
	historyMu   sync.Mutex // guards history-map appends from concurrent COW committers
	history     map[types.ObjID][]objectHistory

	// floorMu guards activeReadTS, the multiset of readTS values of currently-live
	// read-only transactions. historyFloor() = min key (or clock if empty) is the
	// boundary below which old history versions are provably dead and may be pruned
	// (COW Phase 4 history GC). A registration leak only keeps the floor low
	// (conservative: no prune), never prunes a live-needed version.
	floorMu      sync.Mutex
	activeReadTS map[uint64]int

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
	// scheduler each time it loops back after a retryable conflict. These are
	// observation-only and never affect control flow.
	commitAttempts  atomic.Uint64
	commitSuccesses atomic.Uint64
	commitConflicts atomic.Uint64
	commitRetries   atomic.Uint64

	waifRegistry    map[types.ObjID]map[*types.WaifValue]struct{} // Track live waifs by class
	verbCacheClears int64
	verbCacheMisses int64

	pendingFinalizations []types.Value
}

func NewStore() *Store {
	return &Store{
		objects:     make(map[types.ObjID]*objectSlot),
		anonObjects: make(map[types.ObjID]*Object),
		maxObjID:    -1,
		highWaterID: -1,
		recycledID:  []types.ObjID{},
		history:     make(map[types.ObjID][]objectHistory),
	}
}

// load returns the currently-published immutable *Object for id, or nil if the
// slot does not exist. The returned *Object is a frozen snapshot: read its fields
// freely; never mutate it (it may be shared with concurrent readers). Callers on
// the read paths hold store.mu.RLock so the map skeleton is stable during the
// lookup; the atomic Load itself is the acquire barrier that publishes the image.
func (s *Store) load(id types.ObjID) *Object {
	if slot := s.objects[id]; slot != nil {
		return slot.ptr.Load()
	}
	return nil
}

// slotFor returns the slot for id, creating it under store.mu if absent. Callers
// hold store.mu (exclusive) when this may create a slot (map-skeleton mutation).
func (s *Store) slotFor(id types.ObjID) *objectSlot {
	slot := s.objects[id]
	if slot == nil {
		slot = &objectSlot{}
		s.objects[id] = slot
	}
	return slot
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

// NoteCommitRetry records one scheduler-side conflict retry (loop-back). It is
// the only exported mutator; the scheduler lives in another package and cannot
// touch the unexported counter fields directly.
func (s *Store) NoteCommitRetry() { s.commitRetries.Add(1) }

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

func stampProperty(prop *Property, ts uint64) {
	if prop != nil {
		prop.version = ts
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

func (s *Store) addLoadedObject(obj *Object) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := s.bumpClockLocked()
	stampObjectAll(obj, ts)
	s.insertObjectLocked(obj)
}

// AddAnonymous ingests an anonymous object loaded from the database into the
// store's out-of-band anonymous collection. The object is keyed by the identity
// id it was loaded with; this id is NOT a regular numbered-object id and never
// enters the objects map, maxObjID, or highWaterID. Anonymous objects only ever
// surface as _TYPE_ANON values; the dump path assigns them above-max
// serialization ids.
func (s *Store) AddAnonymous(obj *Object) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !obj.anonymous {
		obj.anonymous = true
	}
	ts := s.bumpClockLocked()
	stampObjectAll(obj, ts)
	s.anonObjects[obj.id] = obj
}

func (s *Store) insertObjectLocked(obj *Object) {
	s.publishLocked(obj.id, obj)

	// Update high water ID (tracks all allocations including anonymous)
	if obj.id > s.highWaterID {
		s.highWaterID = obj.id
	}

	// Update max object ID (but NOT for anonymous objects)
	// Anonymous objects don't affect max_object()
	if !obj.anonymous && obj.id > s.maxObjID {
		s.maxObjID = obj.id
	}
}

func (s *Store) SetObjectName(objID types.ObjID, name string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	s.rememberObjectLocked(obj)
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
	s.rememberObjectLocked(obj)
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
	s.rememberObjectLocked(obj)
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
	s.rememberObjectLocked(obj)
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
	for _, slot := range s.objects {
		obj := slot.ptr.Load()
		if obj == nil {
			continue
		}
		if !validLiveObject(obj) {
			continue
		}
		name := strings.TrimSpace(obj.name)
		if !caseSensitive {
			name = strings.ToLower(name)
		}
		if strings.Contains(name, searchNeedle) {
			result = append(result, obj.id)
		}
	}
	return result
}

func (s *Store) ObjectsOwnedBy(owner types.ObjID) []types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.ObjID, 0)
	for _, slot := range s.objects {
		obj := slot.ptr.Load()
		if obj == nil {
			continue
		}
		if validLiveObject(obj) && obj.owner == owner {
			result = append(result, obj.id)
		}
	}
	return result
}

func (s *Store) AliasStrings(objID types.ObjID) ([]string, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}
	prop := obj.properties["aliases"]
	if prop == nil {
		return nil, types.E_NONE
	}
	listVal, ok := prop.value.(types.ListValue)
	if !ok {
		return nil, types.E_NONE
	}
	aliases := make([]string, 0, listVal.Len())
	for i := 1; i <= listVal.Len(); i++ {
		if strVal, ok := listVal.Get(i).(types.StrValue); ok {
			aliases = append(aliases, strVal.Value())
		}
	}
	return aliases, types.E_NONE
}
