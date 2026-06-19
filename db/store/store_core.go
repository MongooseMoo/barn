package store

import (
	"barn/types"
	"fmt"
	"strings"
	"sync"
)

type Store struct {
	mu              sync.RWMutex
	objects         map[types.ObjID]*Object
	maxObjID        types.ObjID                                   // Highest non-anonymous object ID (for max_object())
	highWaterID     types.ObjID                                   // Highest allocated ID (including anonymous, for NextID())
	recycledID      []types.ObjID                                 // Track recycled IDs (for future reuse via recreate)
	waifRegistry    map[types.ObjID]map[*types.WaifValue]struct{} // Track live waifs by class
	verbCacheClears int64
	verbCacheMisses int64
}

func NewStore() *Store {
	return &Store{
		objects:     make(map[types.ObjID]*Object),
		maxObjID:    -1,
		highWaterID: -1,
		recycledID:  []types.ObjID{},
	}
}

func (s *Store) Get(id types.ObjID) *Object {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj, ok := s.objects[id]
	if !ok || obj.Recycled || obj.Flags.Has(FlagInvalid) {
		return nil
	}
	return obj
}

// GetUnsafe retrieves an object without checking recycled status
// Used internally for operations that need to access recycled objects

func (s *Store) GetUnsafe(id types.ObjID) *Object {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.objects[id]
}

// Add adds a new object to the store
// Returns error if object ID already exists

func (s *Store) Add(obj *Object) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.objects[obj.ID]; exists {
		return fmt.Errorf("object #%d already exists", obj.ID)
	}

	s.insertObjectLocked(obj)
	return nil
}

func (s *Store) addLoadedObject(obj *Object) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.insertObjectLocked(obj)
}

func (s *Store) insertObjectLocked(obj *Object) {
	s.objects[obj.ID] = obj

	// Update high water ID (tracks all allocations including anonymous)
	if obj.ID > s.highWaterID {
		s.highWaterID = obj.ID
	}

	// Update max object ID (but NOT for anonymous objects)
	// Anonymous objects don't affect max_object()
	if !obj.Anonymous && obj.ID > s.maxObjID {
		s.maxObjID = obj.ID
	}
}

func (s *Store) SetObjectName(objID types.ObjID, name string) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	obj.Name = name
	return types.E_NONE
}

func (s *Store) SetObjectOwner(objID types.ObjID, owner types.ObjID) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	obj.Owner = owner
	return types.E_NONE
}

func (s *Store) SetObjectLocationRaw(objID types.ObjID, location types.ObjID) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	obj.Location = location
	return types.E_NONE
}

func (s *Store) SetObjectFlag(objID types.ObjID, flag ObjectFlags, enabled bool) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}
	if enabled {
		obj.Flags = obj.Flags.Set(flag)
	} else {
		obj.Flags = obj.Flags.Clear(flag)
	}
	return types.E_NONE
}

func (s *Store) ObjectName(objID types.ObjID) (string, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return "", types.E_INVIND
	}
	return obj.Name, types.E_NONE
}

func (s *Store) ObjectOwner(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	return obj.Owner, types.E_NONE
}

func (s *Store) ObjectFlags(objID types.ObjID) (ObjectFlags, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return 0, types.E_INVIND
	}
	return obj.Flags, types.E_NONE
}

func (s *Store) HasObjectFlag(objID types.ObjID, flag ObjectFlags) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	return obj.Flags.Has(flag), types.E_NONE
}

func (s *Store) ObjectIsAnonymous(objID types.ObjID) (bool, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return false, types.E_INVIND
	}
	return obj.Anonymous, types.E_NONE
}

func (s *Store) ObjectExists(objID types.ObjID) types.ErrorCode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if validLiveObject(obj) {
		return types.E_NONE
	}
	if obj != nil && obj.Recycled {
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
		name := strings.TrimSpace(obj.Name)
		if !caseSensitive {
			name = strings.ToLower(name)
		}
		if strings.Contains(name, searchNeedle) {
			result = append(result, obj.ID)
		}
	}
	return result
}

func (s *Store) ObjectsOwnedBy(owner types.ObjID) []types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]types.ObjID, 0)
	for _, obj := range s.objects {
		if validLiveObject(obj) && obj.Owner == owner {
			result = append(result, obj.ID)
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
	prop := obj.Properties["aliases"]
	if prop == nil {
		return nil, types.E_NONE
	}
	listVal, ok := prop.Value.(types.ListValue)
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
