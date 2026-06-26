package store

import (
	"barn/types"
	"fmt"
	"strings"
	"sync"
)

type Store struct {
	mu          sync.RWMutex
	objects     map[types.ObjID]*Object
	maxObjID    types.ObjID   // Highest non-anonymous object ID (for max_object())
	highWaterID types.ObjID   // Highest allocated ID (including anonymous, for NextID())
	recycledID  []types.ObjID // Track recycled IDs (for future reuse via recreate)

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
	}
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
func (s *Store) liveObjectLocked(id types.ObjID) *Object {
	if obj := s.objects[id]; validLiveObject(obj) {
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

	s.insertObjectLocked(obj)
	return nil
}

func (s *Store) addLoadedObject(obj *Object) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	obj.name = name
	return types.E_NONE
}

func (s *Store) SetObjectOwner(objID types.ObjID, owner types.ObjID) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	obj.owner = owner
	return types.E_NONE
}

func (s *Store) SetObjectLocationRaw(objID types.ObjID, location types.ObjID) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	obj.location = location
	return types.E_NONE
}

func (s *Store) SetObjectFlag(objID types.ObjID, flag ObjectFlags, enabled bool) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}
	if enabled {
		obj.flags = obj.flags.Set(flag)
	} else {
		obj.flags = obj.flags.Clear(flag)
	}
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
	if obj := s.objects[objID]; obj != nil && obj.recycled {
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
