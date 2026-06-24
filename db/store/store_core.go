package store

import (
	"barn/types"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

type Store struct {
	mu          sync.RWMutex
	objects     map[types.ObjID]*Object
	maxObjID    types.ObjID   // Highest non-anonymous object ID (for max_object())
	highWaterID types.ObjID   // Highest allocated ID (including anonymous, for NextID())
	recycledID  []types.ObjID // Track recycled IDs (for future reuse via recreate)
	clock       uint64
	history     map[types.ObjID][]objectHistory

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

	waifRegistry    map[types.ObjID]map[*types.WaifValue]struct{} // Track live waifs by class
	verbCacheClears int64
	verbCacheMisses int64

	pendingFinalizations []types.Value
}

func NewStore() *Store {
	return &Store{
		objects:     make(map[types.ObjID]*Object),
		anonObjects: make(map[types.ObjID]*Object),
		maxObjID:    -1,
		highWaterID: -1,
		recycledID:  []types.ObjID{},
		history:     make(map[types.ObjID][]objectHistory),
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

func (s *Store) ReadTimestamp() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.clock
}

func (s *Store) bumpClockLocked() uint64 {
	s.clock++
	if s.clock == 0 {
		s.clock++
	}
	return s.clock
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

// Get returns a flat, read-only ObjectView for a live object, plus ok=false if
// the object does not exist or is recycled/invalid. The store never hands out a
// live *Object to external callers.
func (s *Store) Get(id types.ObjID) (ObjectView, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj, ok := s.objects[id]
	if !ok || obj.recycled || obj.flags.Has(FlagInvalid) {
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

	obj, ok := s.objects[id]
	if !ok || obj == nil {
		return ObjectView{}, false
	}
	return obj.view(), true
}

// Add adds a new object to the store
// Returns error if object ID already exists

func (s *Store) Add(obj *Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.objects[obj.id]; exists {
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
	s.objects[obj.id] = obj

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

	obj := s.objects[objID]
	if !validLiveObject(obj) {
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

	obj := s.objects[objID]
	if !validLiveObject(obj) {
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

	obj := s.objects[objID]
	if !validLiveObject(obj) {
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

	obj := s.objects[objID]
	if !validLiveObject(obj) {
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

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return "", types.E_INVIND
	}
	return obj.name, types.E_NONE
}

func (s *Store) ObjectOwner(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	return obj.owner, types.E_NONE
}

func (s *Store) ObjectFlags(objID types.ObjID) (ObjectFlags, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return 0, types.E_INVIND
	}
	return obj.flags, types.E_NONE
}

func (s *Store) HasObjectFlag(objID types.ObjID, flag ObjectFlags) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	return obj.flags.Has(flag), types.E_NONE
}

func (s *Store) ObjectIsAnonymous(objID types.ObjID) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	return obj.anonymous, types.E_NONE
}

func (s *Store) ObjectExists(objID types.ObjID) types.ErrorCode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if validLiveObject(obj) {
		return types.E_NONE
	}
	if obj != nil && obj.recycled {
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
	for _, obj := range s.objects {
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
	for _, obj := range s.objects {
		if validLiveObject(obj) && obj.owner == owner {
			result = append(result, obj.id)
		}
	}
	return result
}

func (s *Store) AliasStrings(objID types.ObjID) ([]string, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
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
