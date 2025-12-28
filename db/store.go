package db

import (
	"barn/types"
	"fmt"
	"strings"
	"sync"
)

// Store is an in-memory object database
type Store struct {
	mu           sync.RWMutex
	objects      map[types.ObjID]*Object
	maxObjID     types.ObjID // Highest non-anonymous object ID (for max_object())
	highWaterID  types.ObjID // Highest allocated ID (including anonymous, for NextID())
	recycledID   []types.ObjID // Track recycled IDs (for future reuse via recreate)
	waifRegistry map[types.ObjID]map[*types.WaifValue]struct{} // Track live waifs by class
}

// NewStore creates a new empty object store
func NewStore() *Store {
	return &Store{
		objects:     make(map[types.ObjID]*Object),
		maxObjID:    -1,
		highWaterID: -1,
		recycledID:  []types.ObjID{},
	}
}

// Get retrieves an object by ID
// Returns nil if object doesn't exist or is recycled
func (s *Store) Get(id types.ObjID) *Object {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj, ok := s.objects[id]
	if !ok || obj.Recycled {
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

	return nil
}

// NextID returns the next available object ID
// Uses highWaterID to ensure unique IDs (including anonymous objects)
// Recycled slots are NOT automatically reused
func (s *Store) NextID() types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.highWaterID + 1
}

// MaxObject returns the highest allocated object ID
// Includes recycled objects (high-water mark)
func (s *Store) MaxObject() types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.maxObjID
}

// Valid checks if an object exists and is not recycled
func (s *Store) Valid(id types.ObjID) bool {
	// Negative IDs are sentinels (nothing, ambiguous, failed_match)
	if id < 0 {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if ID exceeds high water mark (includes anonymous objects)
	if id > s.highWaterID {
		return false
	}

	obj, ok := s.objects[id]
	if !ok {
		return false
	}

	// Check if recycled or explicitly invalidated
	if obj.Recycled || obj.Flags.Has(FlagInvalid) {
		return false
	}

	return true
}

// IsRecycled checks if an object ID was recycled (vs never existed)
// Returns true only if the object existed and was recycled
func (s *Store) IsRecycled(id types.ObjID) bool {
	if id < 0 {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	obj, ok := s.objects[id]
	if !ok {
		return false
	}

	return obj.Recycled
}

// Recycle marks an object as recycled
// Returns error if object doesn't exist or is already recycled
func (s *Store) Recycle(id types.ObjID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj, ok := s.objects[id]
	if !ok {
		return fmt.Errorf("object #%d does not exist", id)
	}

	if obj.Recycled {
		return fmt.Errorf("object #%d already recycled", id)
	}

	// Invalidate any anonymous children (before marking as recycled)
	for _, childID := range obj.AnonymousChildren {
		child := s.objects[childID]
		if child != nil && child.Anonymous {
			child.Flags = child.Flags.Set(FlagInvalid)
		}
	}
	obj.AnonymousChildren = nil

	// Mark as recycled and invalid
	obj.Recycled = true
	obj.Flags = obj.Flags.Set(FlagRecycled | FlagInvalid)

	// Track for potential reuse
	s.recycledID = append(s.recycledID, id)

	return nil
}

// Recreate recreates a recycled object slot (wizard only)
// Returns error if object is not recycled
func (s *Store) Recreate(id types.ObjID, parent types.ObjID, owner types.ObjID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj, ok := s.objects[id]
	if !ok {
		return fmt.Errorf("object #%d does not exist", id)
	}

	if !obj.Recycled {
		return fmt.Errorf("object #%d is not recycled", id)
	}

	// Reset object to fresh state
	newObj := NewObject(id, owner)
	newObj.Parents = []types.ObjID{parent}

	s.objects[id] = newObj

	return nil
}

// All returns all valid (non-recycled) objects
func (s *Store) All() []*Object {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Object, 0, len(s.objects))
	for _, obj := range s.objects {
		if !obj.Recycled {
			result = append(result, obj)
		}
	}
	return result
}

// Players returns all objects with the player flag set
func (s *Store) Players() []types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := []types.ObjID{}
	for _, obj := range s.objects {
		if !obj.Recycled && obj.Flags.Has(FlagUser) {
			result = append(result, obj.ID)
		}
	}
	return result
}

// LowestFreeID finds the lowest available object ID
// Checks recycled slots and gaps in the ID sequence
func (s *Store) LowestFreeID() types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// First check for recycled slots (lowest first)
	lowestRecycled := types.ObjID(-1)
	for _, id := range s.recycledID {
		if lowestRecycled == -1 || id < lowestRecycled {
			lowestRecycled = id
		}
	}
	if lowestRecycled != -1 {
		return lowestRecycled
	}

	// Check for gaps in ID sequence (0 to maxObjID)
	for id := types.ObjID(0); id <= s.maxObjID; id++ {
		obj, exists := s.objects[id]
		if !exists {
			return id
		}
		if obj.Recycled {
			return id
		}
	}

	// No gaps, use next sequential ID
	return s.maxObjID + 1
}

// Renumber moves an object from oldID to newID, updating all references
// Returns the new ID, or error if object doesn't exist
func (s *Store) Renumber(oldID, newID types.ObjID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get the object to renumber
	obj, ok := s.objects[oldID]
	if !ok || obj.Recycled {
		return fmt.Errorf("object #%d does not exist", oldID)
	}

	// If old and new are the same, nothing to do
	if oldID == newID {
		return nil
	}

	// Check new ID is available
	if existing, exists := s.objects[newID]; exists && !existing.Recycled {
		return fmt.Errorf("object #%d already exists", newID)
	}

	// Invalidate any anonymous children (parent is being renumbered)
	for _, childID := range obj.AnonymousChildren {
		child := s.objects[childID]
		if child != nil && child.Anonymous {
			child.Flags = child.Flags.Set(FlagInvalid)
		}
	}
	obj.AnonymousChildren = nil

	// Update the object's ID
	obj.ID = newID

	// Move in store
	delete(s.objects, oldID)
	s.objects[newID] = obj

	// Update recycledID list - remove newID if present, add oldID
	newRecycled := []types.ObjID{}
	for _, rid := range s.recycledID {
		if rid != newID {
			newRecycled = append(newRecycled, rid)
		}
	}
	newRecycled = append(newRecycled, oldID)
	s.recycledID = newRecycled

	// Update all references in ALL objects
	for _, other := range s.objects {
		if other.Recycled {
			continue
		}

		// Update Parents
		for i, pid := range other.Parents {
			if pid == oldID {
				other.Parents[i] = newID
			}
		}

		// Update Children
		for i, cid := range other.Children {
			if cid == oldID {
				other.Children[i] = newID
			}
		}

		// Update ChparentChildren
		if other.ChparentChildren != nil {
			if other.ChparentChildren[oldID] {
				delete(other.ChparentChildren, oldID)
				other.ChparentChildren[newID] = true
			}
		}

		// Update Location
		if other.Location == oldID {
			other.Location = newID
		}

		// Update Contents
		for i, cid := range other.Contents {
			if cid == oldID {
				other.Contents[i] = newID
			}
		}

		// Update Owner
		if other.Owner == oldID {
			other.Owner = newID
		}
	}

	return nil
}

// matchVerbName checks if a search name matches a MOO verb name pattern
// Supports MOO wildcard matching where * means "zero or more characters"
// Example: "co*nnect" matches "connect" because:
//   - Must start with "co"
//   - Must end with "nnect"
//   - Any characters (including none) can appear where * is
func matchVerbName(verbPattern, searchName string) bool {
	// Case-insensitive matching
	pattern := strings.ToLower(verbPattern)
	search := strings.ToLower(searchName)

	// Find the wildcard position
	starPos := strings.Index(pattern, "*")
	if starPos == -1 {
		// No wildcard, exact match required
		return pattern == search
	}

	// Split pattern at wildcard: "co*nnect" -> prefix="co", suffix="nnect"
	prefix := pattern[:starPos]
	suffix := pattern[starPos+1:]

	// Check if search string has the required prefix and suffix
	if !strings.HasPrefix(search, prefix) {
		return false
	}
	if !strings.HasSuffix(search, suffix) {
		return false
	}

	// Ensure prefix and suffix don't overlap
	// For pattern "co*nnect" matching "connect":
	//   prefix="co" (len 2), suffix="nnect" (len 5)
	//   search="connect" (len 7)
	//   Need: len(search) >= len(prefix) + len(suffix)
	//   7 >= 2 + 5 = 7, valid
	return len(search) >= len(prefix)+len(suffix)
}

// FindVerb looks up a verb on an object, following inheritance chain
// Uses breadth-first search per spec
// Returns the verb and the object it's defined on, or error
func (s *Store) FindVerb(objID types.ObjID, verbName string) (*Verb, types.ObjID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Track visited objects to prevent infinite loops
	visited := make(map[types.ObjID]bool)
	queue := []types.ObjID{objID}

	for len(queue) > 0 {
		// Pop from front (FIFO for breadth-first)
		current := queue[0]
		queue = queue[1:]

		// Skip if already visited (cycle detection)
		if visited[current] {
			continue
		}
		visited[current] = true

		// Get object (skip if invalid)
		obj := s.objects[current]
		if obj == nil || obj.Recycled {
			continue
		}

		// Check if verb exists on this object
		// Try exact name match first
		if verb, ok := obj.Verbs[verbName]; ok {
			return verb, current, nil
		}

		// Also check verb aliases (names field) with wildcard matching
		for _, verb := range obj.Verbs {
			for _, alias := range verb.Names {
				if matchVerbName(alias, verbName) {
					return verb, current, nil
				}
			}
		}

		// Not found on this object, add parents to queue
		queue = append(queue, obj.Parents...)
	}

	// Verb not found in entire inheritance chain
	return nil, types.ObjNothing, fmt.Errorf("verb not found: %s", verbName)
}

// RegisterWaif registers a waif with its class object for invalidation tracking
func (s *Store) RegisterWaif(classID types.ObjID, waif *types.WaifValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.waifRegistry == nil {
		s.waifRegistry = make(map[types.ObjID]map[*types.WaifValue]struct{})
	}

	if s.waifRegistry[classID] == nil {
		s.waifRegistry[classID] = make(map[*types.WaifValue]struct{})
	}

	s.waifRegistry[classID][waif] = struct{}{}
}

// WaifCount returns the total number of live waifs across all classes
func (s *Store) WaifCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, waifs := range s.waifRegistry {
		count += len(waifs)
	}
	return count
}

// WaifCountByClass returns a map of class ID to waif count
func (s *Store) WaifCountByClass() map[types.ObjID]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[types.ObjID]int)
	for classID, waifs := range s.waifRegistry {
		result[classID] = len(waifs)
	}
	return result
}

// InvalidateAnonymousChildren marks all anonymous children of an object as invalid
// This is called when the parent hierarchy changes (recycle, chparents, add_property, delete_property, renumber)
func (s *Store) InvalidateAnonymousChildren(parentID types.ObjID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parent := s.objects[parentID]
	if parent == nil {
		return
	}

	// Invalidate all anonymous children
	for _, childID := range parent.AnonymousChildren {
		child := s.objects[childID]
		if child != nil && child.Anonymous {
			child.Flags = child.Flags.Set(FlagInvalid)
		}
	}

	// Clear the list (children are now invalid)
	parent.AnonymousChildren = nil
}
