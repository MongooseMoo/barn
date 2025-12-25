package db

import (
	"barn/types"
	"fmt"
	"sync"
)

// Store is an in-memory object database
type Store struct {
	mu         sync.RWMutex
	objects    map[types.ObjID]*Object
	maxObjID   types.ObjID
	recycledID []types.ObjID // Track recycled IDs (for future reuse via recreate)
}

// NewStore creates a new empty object store
func NewStore() *Store {
	return &Store{
		objects:    make(map[types.ObjID]*Object),
		maxObjID:   -1,
		recycledID: []types.ObjID{},
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

	// Update max object ID
	if obj.ID > s.maxObjID {
		s.maxObjID = obj.ID
	}

	return nil
}

// NextID returns the next available object ID
// Per spec: Sequential allocation from max_object() + 1
// Recycled slots are NOT automatically reused
func (s *Store) NextID() types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.maxObjID + 1
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

	// Check if ID exceeds max_object
	if id > s.maxObjID {
		return false
	}

	obj, ok := s.objects[id]
	if !ok {
		return false
	}

	// Check if recycled
	return !obj.Recycled
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

		// Also check verb aliases (names field)
		for _, verb := range obj.Verbs {
			for _, alias := range verb.Names {
				if alias == verbName {
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
