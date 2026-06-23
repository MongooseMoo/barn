package store

import (
	"barn/types"
	"fmt"
	"slices"
)

func (s *Store) CreateObject(parents []types.ObjID, owner types.ObjID, anonymous bool) (types.ObjID, types.ErrorCode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := s.bumpClockLocked()
	newID := s.highWaterID + 1
	if owner == types.ObjNothing {
		owner = newID
	}

	obj := NewObject(newID, owner)
	obj.parents = append([]types.ObjID(nil), parents...)
	obj.anonymous = anonymous
	if anonymous {
		obj.flags = obj.flags.Set(FlagAnonymous)
	}
	obj.properties = s.copyInheritedPropertiesLocked(obj.parents)
	stampObjectAll(obj, ts)

	for _, parentID := range obj.parents {
		s.rememberObjectLocked(s.objects[parentID])
	}
	s.insertObjectLocked(obj)
	s.attachChildToParentsLocked(newID, obj.parents, anonymous, false)
	for _, parentID := range obj.parents {
		stampObjectRelationship(s.objects[parentID], ts)
	}
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
	if obj.recycled || obj.flags.Has(FlagInvalid) {
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

	return obj.recycled
}

// invalidateAnonymousChildrenLocked marks anonymous children under rootID as invalid.
// Includes the root object's own anonymous children and all descendants' anonymous children.
// Caller must hold s.mu lock.

func (s *Store) invalidateAnonymousChildrenLocked(rootID types.ObjID) {
	queue := []types.ObjID{rootID}
	visited := make(map[types.ObjID]bool)

	ts := s.bumpClockLocked()
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]

		if visited[currentID] {
			continue
		}
		visited[currentID] = true

		current := s.objects[currentID]
		if current == nil || current.recycled {
			continue
		}

		for _, childID := range current.anonymousChildren {
			child := s.objects[childID]
			if child != nil && child.anonymous {
				s.rememberObjectLocked(child)
				child.flags = child.flags.Set(FlagInvalid)
				stampObjectScalar(child, ts)
			}
		}
		s.rememberObjectLocked(current)
		current.anonymousChildren = nil
		stampObjectRelationship(current, ts)

		queue = append(queue, current.children...)
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

	if obj.recycled {
		return fmt.Errorf("object #%d already recycled", id)
	}

	// Note: recycling an object does NOT invalidate anonymous descendants in
	// ToastStunt; they remain valid (property access through a recycled parent
	// simply raises E_PROPNF). The anon is only invalidated when recycled itself.

	s.rememberObjectLocked(obj)
	ts := s.bumpClockLocked()
	objParents := append([]types.ObjID(nil), obj.parents...)
	for _, childID := range obj.children {
		child := s.objects[childID]
		if !validLiveObject(child) {
			continue
		}

		newChildParents := []types.ObjID{}
		seen := make(map[types.ObjID]bool)
		for _, pid := range child.parents {
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
		s.rememberObjectLocked(child)
		child.parents = newChildParents
		stampObjectRelationship(child, ts)

		for _, newParentID := range objParents {
			newParent := s.objects[newParentID]
			if validLiveObject(newParent) && !slices.Contains(newParent.children, childID) {
				s.rememberObjectLocked(newParent)
				newParent.children = append(newParent.children, childID)
				stampObjectRelationship(newParent, ts)
			}
		}
	}

	for _, contentID := range obj.contents {
		content := s.objects[contentID]
		if validLiveObject(content) {
			s.rememberObjectLocked(content)
			content.location = types.ObjNothing
			stampObjectRelationship(content, ts)
		}
	}
	obj.contents = []types.ObjID{}

	if obj.location != types.ObjNothing {
		oldLoc := s.objects[obj.location]
		if validLiveObject(oldLoc) {
			s.rememberObjectLocked(oldLoc)
			oldLoc.contents = removeObjID(oldLoc.contents, id)
			stampObjectRelationship(oldLoc, ts)
		}
	}
	obj.location = types.ObjNothing

	obj.properties = make(map[string]*Property)
	obj.verbs = make(map[string]*Verb)

	for _, parentID := range obj.parents {
		parent := s.objects[parentID]
		if validLiveObject(parent) {
			s.rememberObjectLocked(parent)
			parent.children = removeObjID(parent.children, id)
			stampObjectRelationship(parent, ts)
		}
	}

	// Mark as recycled and invalid
	obj.recycled = true
	obj.flags = obj.flags.Set(FlagRecycled | FlagInvalid)
	stampObjectAll(obj, ts)

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

	if !obj.recycled {
		return fmt.Errorf("object #%d is not recycled", id)
	}

	// Reset object to fresh state
	s.rememberObjectLocked(obj)
	ts := s.bumpClockLocked()
	newObj := NewObject(id, owner)
	if parent != types.ObjNothing {
		parentObj := s.objects[parent]
		if !validLiveObject(parentObj) {
			return fmt.Errorf("parent #%d is not valid", parent)
		}
		newObj.parents = []types.ObjID{parent}
	}
	newObj.properties = s.copyInheritedPropertiesLocked(newObj.parents)
	stampObjectAll(newObj, ts)

	s.objects[id] = newObj
	s.recycledID = removeRecycledID(s.recycledID, id)
	if parent != types.ObjNothing {
		s.rememberObjectLocked(s.objects[parent])
		s.attachChildToParentsLocked(id, newObj.parents, false, false)
		stampObjectRelationship(s.objects[parent], ts)
	}

	return nil
}

func removeRecycledID(ids []types.ObjID, id types.ObjID) []types.ObjID {
	for i, candidate := range ids {
		if candidate == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
}

// All returns flat, read-only ObjectViews for every valid (non-recycled)
// object. The store never hands out live *Object values to external callers.

func (s *Store) All() []ObjectView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ObjectView, 0, len(s.objects))
	for _, obj := range s.objects {
		if !obj.recycled {
			result = append(result, obj.view())
		}
	}
	return result
}

func (s *Store) Players() []types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := []types.ObjID{}
	for _, obj := range s.objects {
		if !obj.recycled && obj.flags.Has(FlagUser) {
			result = append(result, obj.id)
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
		if obj.recycled {
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
	if !ok || obj.recycled {
		return fmt.Errorf("object #%d does not exist", oldID)
	}

	// If old and new are the same, nothing to do
	if oldID == newID {
		return nil
	}

	// Check new ID is available
	if existing, exists := s.objects[newID]; exists && !existing.recycled {
		return fmt.Errorf("object #%d already exists", newID)
	}

	// Note: renumbering does NOT invalidate anonymous descendants in ToastStunt.

	// Update the object's ID
	s.rememberObjectLocked(obj)
	ts := s.bumpClockLocked()
	obj.id = newID
	stampObjectAll(obj, ts)

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
		if other.recycled {
			continue
		}

		// Update Parents
		for i, pid := range other.parents {
			if pid == oldID {
				s.rememberObjectLocked(other)
				other.parents[i] = newID
				stampObjectRelationship(other, ts)
			}
		}

		// Update Children
		for i, cid := range other.children {
			if cid == oldID {
				s.rememberObjectLocked(other)
				other.children[i] = newID
				stampObjectRelationship(other, ts)
			}
		}

		// Update ChparentChildren
		if other.chparentChildren != nil {
			if other.chparentChildren[oldID] {
				s.rememberObjectLocked(other)
				delete(other.chparentChildren, oldID)
				other.chparentChildren[newID] = true
				stampObjectRelationship(other, ts)
			}
		}

		// Update Location
		if other.location == oldID {
			s.rememberObjectLocked(other)
			other.location = newID
			stampObjectRelationship(other, ts)
		}

		// Update Contents
		for i, cid := range other.contents {
			if cid == oldID {
				s.rememberObjectLocked(other)
				other.contents[i] = newID
				stampObjectRelationship(other, ts)
			}
		}

		// Update Owner
		if other.owner == oldID {
			s.rememberObjectLocked(other)
			other.owner = newID
			stampObjectScalar(other, ts)
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
