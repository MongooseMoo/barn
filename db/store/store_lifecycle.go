package store

import (
	"barn/types"
	"fmt"
	"slices"
	"unsafe"
)

func (s *Store) CreateObject(parents []types.ObjID, owner types.ObjID, anonymous bool) (types.ObjID, types.ErrorCode) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	if anonymous {
		// Anonymous objects live out-of-band in s.anonObjects, never in the
		// numbered s.objects map (see the field comment in store_core.go and
		// AddAnonymous). This mirrors ToastStunt, where a live anonymous object
		// has id == NOTHING and is removed from the numbered objects[] array
		// (db_make_anonymous, db_objects.cc:449); it is only assigned an
		// above-max serialization id at dump time (dbpriv_assign_anonymous_object,
		// db_objects.cc:415). Routing runtime creation here keeps the planner, GC
		// scan, and serializer over a single set of anonymous objects, so a
		// runtime-created anon survives a checkpoint just like a loaded one.
		//
		// We still bump highWaterID so the identity id (newID) is never handed
		// out again to a future allocation, but we do NOT touch maxObjID:
		// anonymous objects must not affect max_object().
		s.anonObjects[newID] = obj
		if newID > s.highWaterID {
			s.highWaterID = newID
		}
	} else {
		s.insertObjectLocked(obj)
	}
	s.attachChildToParentsLocked(newID, obj.parents, anonymous, false)
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

	// Resolve through liveObjectLocked so a valid anonymous value (loaded or
	// runtime-created via create(...,1), which now lives only in s.anonObjects)
	// is reported valid, not just numbered objects. liveObjectLocked already
	// excludes recycled/invalidated objects.
	return s.liveObjectLocked(id) != nil
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
				child.flags = child.flags.Set(FlagInvalid)
			}
		}
		current.anonymousChildren = nil

		queue = append(queue, current.children...)
	}
}

// Recycle marks an object as recycled
// Returns error if object doesn't exist or is already recycled

func (s *Store) Recycle(id types.ObjID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Resolve through both maps so recycle() of a valid anonymous value (which
	// lives in s.anonObjects) finds it. The relationship cleanup below operates
	// over numbered relatives (an anon's parents are numbered; it has no
	// children/contents), so it is left numbered-structural.
	obj := s.liveObjectLocked(id)
	if obj == nil {
		if r := s.objects[id]; r != nil && r.recycled {
			return fmt.Errorf("object #%d already recycled", id)
		}
		if r := s.anonObjects[id]; r != nil && r.recycled {
			return fmt.Errorf("object #%d already recycled", id)
		}
		return fmt.Errorf("object #%d does not exist", id)
	}

	// Note: recycling an object does NOT invalidate anonymous descendants in
	// ToastStunt; they remain valid (property access through a recycled parent
	// simply raises E_PROPNF). The anon is only invalidated when recycled itself.

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
		child.parents = newChildParents

		for _, newParentID := range objParents {
			newParent := s.objects[newParentID]
			if validLiveObject(newParent) && !slices.Contains(newParent.children, childID) {
				newParent.children = append(newParent.children, childID)
			}
		}
	}

	for _, contentID := range obj.contents {
		content := s.objects[contentID]
		if validLiveObject(content) {
			content.location = types.ObjNothing
		}
	}
	obj.contents = []types.ObjID{}

	if obj.location != types.ObjNothing {
		oldLoc := s.objects[obj.location]
		if validLiveObject(oldLoc) {
			oldLoc.contents = removeObjID(oldLoc.contents, id)
		}
	}
	obj.location = types.ObjNothing

	obj.properties = make(map[string]*Property)
	obj.verbs = make(map[string]*Verb)

	for _, parentID := range obj.parents {
		parent := s.objects[parentID]
		if validLiveObject(parent) {
			parent.children = removeObjID(parent.children, id)
		}
	}

	// Mark as recycled and invalid
	obj.recycled = true
	obj.flags = obj.flags.Set(FlagRecycled | FlagInvalid)

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
	newObj := NewObject(id, owner)
	newObj.parents = []types.ObjID{parent}
	newObj.properties = s.copyInheritedPropertiesLocked(newObj.parents)

	s.objects[id] = newObj
	s.attachChildToParentsLocked(id, newObj.parents, false, false)

	return nil
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
	obj.id = newID

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

	// rewriteOwner mirrors ToastStunt's owner-fix rule in db_renumber_object
	// (src/db_objects.cc:666-669, applied identically to verbdef owners at
	// 671-675 and propval owners at 679-683): an owner equal to the freed new id
	// is cleared to NOTHING, and an owner equal to the old id becomes the new id.
	rewriteOwner := func(o types.ObjID) types.ObjID {
		switch o {
		case newID:
			return types.ObjNothing
		case oldID:
			return newID
		default:
			return o
		}
	}

	// Update all references in ALL objects
	for _, other := range s.objects {
		if other.recycled {
			continue
		}

		// Update Parents
		for i, pid := range other.parents {
			if pid == oldID {
				other.parents[i] = newID
			}
		}

		// Update Children
		for i, cid := range other.children {
			if cid == oldID {
				other.children[i] = newID
			}
		}

		// Update ChparentChildren
		if other.chparentChildren != nil {
			if other.chparentChildren[oldID] {
				delete(other.chparentChildren, oldID)
				other.chparentChildren[newID] = true
			}
		}

		// Update Location
		if other.location == oldID {
			other.location = newID
		}

		// Update Contents
		for i, cid := range other.contents {
			if cid == oldID {
				other.contents[i] = newID
			}
		}

		// Fix owners of the object, its verbdefs and its propvals, matching
		// ToastStunt db_renumber_object (db_objects.cc:653-684).
		other.owner = rewriteOwner(other.owner)
		for _, v := range other.verbs {
			v.owner = rewriteOwner(v.owner)
		}
		for _, p := range other.properties {
			p.owner = rewriteOwner(p.owner)
		}
	}

	// Fix references in anonymous objects as well. Before F2 (commit 7318d24)
	// runtime anonymous objects lived in the numbered s.objects map, so the
	// structural walk above rewrote their parent/location/etc. references to a
	// renumbered object. F2 moved them out-of-band into s.anonObjects, so they
	// must be walked here to preserve that behavior — otherwise an anonymous
	// child of a renumbered object keeps a stale parent id and property access
	// through it fails (E_PROPNF). Toast walks anonymous_objects in
	// db_renumber_object for the same reason (db_objects.cc:686-705); anonymous
	// objects carry no verbdefs, so only object/propval owners are rewritten for
	// ownership, alongside the structural parent/child/location/contents refs.
	for _, anon := range s.anonObjects {
		if anon == nil {
			continue
		}
		for i, pid := range anon.parents {
			if pid == oldID {
				anon.parents[i] = newID
			}
		}
		for i, cid := range anon.children {
			if cid == oldID {
				anon.children[i] = newID
			}
		}
		for i, cid := range anon.anonymousChildren {
			if cid == oldID {
				anon.anonymousChildren[i] = newID
			}
		}
		if anon.chparentChildren != nil && anon.chparentChildren[oldID] {
			delete(anon.chparentChildren, oldID)
			anon.chparentChildren[newID] = true
		}
		if anon.location == oldID {
			anon.location = newID
		}
		for i, cid := range anon.contents {
			if cid == oldID {
				anon.contents[i] = newID
			}
		}
		anon.owner = rewriteOwner(anon.owner)
		for _, p := range anon.properties {
			p.owner = rewriteOwner(p.owner)
		}
	}

	return nil
}

// FindProperty looks up a property on an object, following the inheritance chain
// breadth-first. Permission metadata comes from the nearest property slot, while
// a clear slot inherits the first non-clear value from an ancestor.

func (s *Store) RegisterWaif(classID types.ObjID, waif types.Value) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.waifRegistry == nil {
		s.waifRegistry = make(map[types.ObjID]map[unsafe.Pointer]struct{})
	}

	if s.waifRegistry[classID] == nil {
		s.waifRegistry[classID] = make(map[unsafe.Pointer]struct{})
	}

	s.waifRegistry[classID][waif.WaifIdentity()] = struct{}{}
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
