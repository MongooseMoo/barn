package db

import (
	"barn/types"
	"fmt"
)

func (s *Store) CreateObject(parents []types.ObjID, owner types.ObjID, anonymous bool) (types.ObjID, types.ErrorCode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	newID := s.highWaterID + 1
	if owner == types.ObjNothing {
		owner = newID
	}

	obj := NewObject(newID, owner)
	obj.Parents = append([]types.ObjID(nil), parents...)
	obj.Anonymous = anonymous
	if anonymous {
		obj.Flags = obj.Flags.Set(FlagAnonymous)
	}
	obj.Properties = s.copyInheritedPropertiesLocked(obj.Parents)

	s.insertObjectLocked(obj)
	s.attachChildToParentsLocked(newID, obj.Parents, anonymous, false)
	return newID, types.E_NONE
}

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

// invalidateAnonymousChildrenLocked marks anonymous children under rootID as invalid.
// Includes the root object's own anonymous children and all descendants' anonymous children.
// Caller must hold s.mu lock.

func (s *Store) invalidateAnonymousChildrenLocked(rootID types.ObjID) {
	queue := []types.ObjID{rootID}
	visited := make(map[types.ObjID]bool)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := s.objects[currentID]
		if current == nil || current.Recycled {
			continue
		}

		for _, childID := range current.AnonymousChildren {
			child := s.objects[childID]
			if child != nil && child.Anonymous {
				child.Flags = child.Flags.Set(FlagInvalid)
			}
		}
		current.AnonymousChildren = nil

		queue = append(queue, current.Children...)
	}
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

	// Note: recycling an object does NOT invalidate anonymous descendants in
	// ToastStunt; they remain valid (property access through a recycled parent
	// simply raises E_PROPNF). The anon is only invalidated when recycled itself.

	objParents := append([]types.ObjID(nil), obj.Parents...)
	for _, childID := range obj.Children {
		child := s.objects[childID]
		if !validLiveObject(child) {
			continue
		}

		newChildParents := []types.ObjID{}
		seen := make(map[types.ObjID]bool)
		for _, pid := range child.Parents {
			if pid == id {
				for _, op := range objParents {
					if !seen[op] {
						seen[op] = true
						newChildParents = append(newChildParents, op)
					}
				}
				continue
			}
			if !seen[pid] {
				seen[pid] = true
				newChildParents = append(newChildParents, pid)
			}
		}
		child.Parents = newChildParents

		for _, newParentID := range objParents {
			newParent := s.objects[newParentID]
			if validLiveObject(newParent) && !containsObjID(newParent.Children, childID) {
				newParent.Children = append(newParent.Children, childID)
			}
		}
	}

	for _, contentID := range obj.Contents {
		content := s.objects[contentID]
		if validLiveObject(content) {
			content.Location = types.ObjNothing
		}
	}
	obj.Contents = []types.ObjID{}

	if obj.Location != types.ObjNothing {
		oldLoc := s.objects[obj.Location]
		if validLiveObject(oldLoc) {
			oldLoc.Contents = removeObjID(oldLoc.Contents, id)
		}
	}
	obj.Location = types.ObjNothing

	obj.Properties = make(map[string]*Property)
	obj.Verbs = make(map[string]*Verb)

	for _, parentID := range obj.Parents {
		parent := s.objects[parentID]
		if validLiveObject(parent) {
			parent.Children = removeObjID(parent.Children, id)
		}
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
	newObj.Properties = s.copyInheritedPropertiesLocked(newObj.Parents)

	s.objects[id] = newObj
	s.attachChildToParentsLocked(id, newObj.Parents, false, false)

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

// GetAnonymousObjects returns all anonymous (non-recycled) objects

func (s *Store) GetAnonymousObjects() []*Object {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Object, 0)
	for _, obj := range s.objects {
		if !obj.Recycled && obj.Anonymous {
			result = append(result, obj)
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

	// Note: renumbering does NOT invalidate anonymous descendants in ToastStunt.

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

// FindProperty looks up a property on an object, following the inheritance chain
// breadth-first. Permission metadata comes from the nearest property slot, while
// a clear slot inherits the first non-clear value from an ancestor.

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

	s.invalidateAnonymousChildrenLocked(parentID)
}

// NoteVerbCacheClear increments the compatibility clear counter used by verb_cache_stats().
