package store

import (
	"fmt"
	"github.com/MongooseMoo/barn/types"
	"slices"
)

func (s *Store) createObject(parents []types.ObjID, owner types.ObjID, anonymous bool) (types.ObjID, types.ErrorCode) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	newID := s.allocateID()
	if owner == types.ObjNothing {
		owner = newID
	}

	obj := NewObject(newID, owner)
	obj.parents = append([]types.ObjID(nil), parents...)
	obj.anonymous = anonymous
	if anonymous {
		obj.flags = obj.flags.Set(FlagAnonymous)
		// Record that an anonymous object was created so the orphan-anon GC
		// fast-path can detect creations since a task's floor without s.mu.
		s.anonCreations.Add(1)
	}
	obj.properties = s.copyInheritedPropertiesLocked(obj.parents)
	s.shareCreatedProperties(obj.properties)
	stampObjectAll(obj, ts)

	// Snapshot the numbered parents before attachChildToParentsLocked mutates
	// their children lists, so an in-flight read transaction keeps the pre-create
	// parent image (COW history).
	for _, parentID := range obj.parents {
		s.republishForMutation(s.load(parentID))
	}
	if anonymous {
		// Anonymous objects live out-of-band in s.anonObjects, never in the
		// numbered directory (see the field comment in store_core.go and
		// AddAnonymous). This mirrors ToastStunt, where a live anonymous object
		// has id == NOTHING and is removed from the numbered objects[] array
		// (db_make_anonymous, db_objects.cc:449); it is only assigned an
		// above-max serialization id at dump time (dbpriv_assign_anonymous_object,
		// db_objects.cc:415). Routing runtime creation here keeps the planner, GC
		// scan, and serializer over a single set of anonymous objects, so a
		// runtime-created anon survives a checkpoint just like a loaded one.
		//
		// allocateID already bumped highWaterID so the identity id (newID) is never
		// handed out again; we do NOT touch maxObjID (anonymous objects must not affect
		// max_object()).
		s.anonObjects[newID] = obj
	} else {
		s.insertObjectLocked(obj)
	}
	s.attachChildToParentsLocked(newID, obj.parents, anonymous, false)
	for _, parentID := range obj.parents {
		stampObjectRelationship(s.load(parentID), ts)
	}
	return newID, types.E_NONE
}

// NextID returns the next id the allocator will hand out: one past the highest
// id ever allocated, anonymous objects included. Recycled ids are not reused.
func (s *Store) NextID() types.ObjID {
	return s.highWater() + 1
}

// maxObject returns max_object(): the highest numbered (non-anonymous) id
// allocated, recycled ids included.
func (s *Store) maxObject() types.ObjID {
	return s.maxObjectID()
}

// valid reports whether id names a live object, numbered or anonymous.
func (s *Store) valid(id types.ObjID) bool {
	// Negative IDs are sentinels (nothing, ambiguous, failed_match)
	if id < 0 {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if ID exceeds high water mark (includes anonymous objects)
	if id > s.highWater() {
		return false
	}

	// Resolve through liveObjectLocked so a valid anonymous value (loaded or
	// runtime-created via create(...,1), which lives only in s.anonObjects)
	// is reported valid, not just numbered objects. liveObjectLocked already
	// excludes recycled/invalidated objects.
	return s.liveObjectLocked(id) != nil
}

// isRecycled reports whether id names a recycled numbered object, as opposed to
// a live one or an id that never existed.
func (s *Store) isRecycled(id types.ObjID) bool {
	if id < 0 {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.load(id)
	if obj == nil {
		return false
	}

	return obj.recycled
}

// Recycle marks an object as recycled.
// Returns error if object doesn't exist or is already recycled.
func (s *Store) Recycle(id types.ObjID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Resolve through both maps so recycle() of a valid anonymous value (which
	// lives in s.anonObjects) finds it. The relationship cleanup below operates
	// over numbered relatives (an anon's parents are numbered; it has no
	// children/contents), so it is left numbered-structural.
	obj := s.liveObjectLocked(id)
	if obj == nil {
		if r := s.load(id); r != nil && r.recycled {
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

	obj = s.republishForMutation(obj)
	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	s.noteVerbShapeChanged()
	objParents := append([]types.ObjID(nil), obj.parents...)
	for _, childID := range obj.children {
		child := s.load(childID)
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
		child = s.republishForMutation(child)
		child.parents = newChildParents
		stampObjectRelationship(child, ts)

		for _, newParentID := range objParents {
			newParent := s.load(newParentID)
			if validLiveObject(newParent) && !slices.Contains(newParent.children, childID) {
				newParent = s.republishForMutation(newParent)
				newParent.children = append(newParent.children, childID)
				stampObjectRelationship(newParent, ts)
			}
		}
	}

	for _, contentID := range obj.contents {
		content := s.load(contentID)
		if validLiveObject(content) {
			content = s.republishForMutation(content)
			content.location = types.ObjNothing
			stampObjectRelationship(content, ts)
		}
	}
	obj.contents = []types.ObjID{}

	if obj.location != types.ObjNothing {
		oldLoc := s.load(obj.location)
		if validLiveObject(oldLoc) {
			oldLoc = s.republishForMutation(oldLoc)
			oldLoc.contents = removeObjID(oldLoc.contents, id)
			stampObjectRelationship(oldLoc, ts)
		}
	}
	obj.location = types.ObjNothing

	obj.properties = newPropTable(0)

	for _, parentID := range obj.parents {
		parent := s.load(parentID)
		if validLiveObject(parent) {
			parent = s.republishForMutation(parent)
			parent.children = removeObjID(parent.children, id)
			stampObjectRelationship(parent, ts)
		}
	}

	// Mark as recycled and invalid
	obj.recycled = true
	obj.flags = obj.flags.Set(FlagRecycled | FlagInvalid)
	stampObjectAll(obj, ts)

	// Track for potential reuse. An anonymous object's id is its permanent
	// identity (see AddAnonymous), so it is never reused.
	if !obj.anonymous {
		s.recycledID = append(s.recycledID, id)
	}

	return nil
}

// Recreate recreates a recycled object slot.
// Returns error if object is not recycled.
func (s *Store) Recreate(id types.ObjID, parent types.ObjID, owner types.ObjID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.load(id)
	if obj == nil {
		return fmt.Errorf("object #%d does not exist", id)
	}

	if !obj.recycled {
		return fmt.Errorf("object #%d is not recycled", id)
	}

	// Reset object to fresh state
	s.rememberObjectLocked(obj)
	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	s.noteVerbShapeChanged()
	newObj := NewObject(id, owner)
	if parent != types.ObjNothing {
		parentObj := s.load(parent)
		if !validLiveObject(parentObj) {
			return fmt.Errorf("parent #%d is not valid", parent)
		}
		newObj.parents = []types.ObjID{parent}
	}
	newObj.properties = s.copyInheritedPropertiesLocked(newObj.parents)
	s.shareCreatedProperties(newObj.properties)
	stampObjectAll(newObj, ts)

	s.publishLocked(id, newObj)
	s.recycledID = removeRecycledID(s.recycledID, id)
	if parent != types.ObjNothing {
		s.republishForMutation(s.load(parent))
		s.attachChildToParentsLocked(id, newObj.parents, false, false)
		stampObjectRelationship(s.load(parent), ts)
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

	result := make([]ObjectView, 0, s.dir.len())
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj != nil && !obj.recycled {
			result = append(result, obj.view())
		}
		return true
	})
	return result
}

// Players returns all objects with the player flag set.
func (s *Store) Players() []types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := []types.ObjID{}
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj != nil && !obj.recycled && obj.flags.Has(FlagUser) {
			result = append(result, obj.id)
		}
		return true
	})
	return result
}

// LowestFreeID finds the lowest available object ID.
// Checks recycled slots and gaps in the ID sequence.
func (s *Store) LowestFreeID() types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// First check for recycled slots (lowest first). recycledMu guards against a
	// concurrent decentralized recycle append (also under store.mu.RLock).
	lowestRecycled := types.ObjID(-1)
	s.recycledMu.Lock()
	for _, id := range s.recycledID {
		if lowestRecycled == -1 || id < lowestRecycled {
			lowestRecycled = id
		}
	}
	s.recycledMu.Unlock()
	if lowestRecycled != -1 {
		return lowestRecycled
	}

	// Check for gaps in ID sequence (0 to maxObjID)
	maxID := s.maxObjectID()
	for id := types.ObjID(0); id <= maxID; id++ {
		obj := s.load(id)
		if obj == nil {
			// An id with no numbered slot may belong to an anonymous object,
			// which never gives up its id to a numbered object.
			if s.anonObjects[id] == nil {
				return id
			}
			continue
		}
		if obj.recycled {
			return id
		}
	}

	// No gaps, use next sequential ID
	return maxID + 1
}

// Renumber moves an object from oldID to newID, updating all references.
// Returns an error if the object doesn't exist or newID is taken.
func (s *Store) Renumber(oldID, newID types.ObjID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get the object to renumber
	obj := s.load(oldID)
	if obj == nil || obj.recycled {
		return fmt.Errorf("object #%d does not exist", oldID)
	}

	// If old and new are the same, nothing to do
	if oldID == newID {
		return nil
	}

	// Check new ID is available
	var recycledTarget *Object
	if existing := s.load(newID); existing != nil {
		if !existing.recycled {
			return fmt.Errorf("object #%d already exists", newID)
		}
		recycledTarget = existing
	}

	// Note: renumbering does NOT invalidate anonymous descendants in ToastStunt.

	// Update the object's ID
	if recycledTarget != nil {
		s.rememberObjectLocked(recycledTarget)
	}
	s.rememberObjectLocked(obj)
	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	s.noteVerbShapeChanged()
	tombstone := NewObject(oldID, obj.owner)
	tombstone.recycled = true
	tombstone.flags = tombstone.flags.Set(FlagRecycled | FlagInvalid)
	stampObjectAll(tombstone, ts)
	// Publish a FRESH image at newID rather than mutating obj.id in place: a read
	// transaction that has aliased obj@oldID must never see its id change underfoot.
	// The old obj is retained immutably in history[oldID] by the remember above.
	renumbered := cloneObjectForReadTxn(obj)
	renumbered.id = newID
	stampObjectAll(renumbered, ts)

	// Move in store
	s.publishLocked(oldID, tombstone)
	s.publishLocked(newID, renumbered)

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
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		other := slot.ptr.Load()
		if other == nil || other.recycled {
			return true
		}

		// Update Parents
		for i, pid := range other.parents {
			if pid == oldID {
				other = s.republishForMutation(other)
				other.parents[i] = newID
				stampObjectRelationship(other, ts)
			}
		}

		// Update Children
		for i, cid := range other.children {
			if cid == oldID {
				other = s.republishForMutation(other)
				other.children[i] = newID
				stampObjectRelationship(other, ts)
			}
		}

		// Update ChparentChildren
		if other.chparentChildren != nil {
			if other.chparentChildren[oldID] {
				other = s.republishForMutation(other)
				delete(other.chparentChildren, oldID)
				other.chparentChildren[newID] = true
				stampObjectRelationship(other, ts)
			}
		}

		// Update Location
		if other.location == oldID {
			other = s.republishForMutation(other)
			other.location = newID
			stampObjectRelationship(other, ts)
		}

		// Update Contents
		for i, cid := range other.contents {
			if cid == oldID {
				other = s.republishForMutation(other)
				other.contents[i] = newID
				stampObjectRelationship(other, ts)
			}
		}

		// Fix owners of the object, its verbdefs and its propvals, matching
		// ToastStunt db_renumber_object (db_objects.cc:653-684). Snapshot and
		// re-stamp other under COW only when an owner reference actually changes.
		ownerTouched := other.owner == oldID || other.owner == newID
		if !ownerTouched {
			for _, v := range other.verbList {
				if v.owner == oldID || v.owner == newID {
					ownerTouched = true
					break
				}
			}
		}
		if !ownerTouched {
			for _, p := range other.properties.all() {
				if p.owner == oldID || p.owner == newID {
					ownerTouched = true
					break
				}
			}
		}
		if ownerTouched {
			other = s.republishForMutation(other)
			other.owner = rewriteOwner(other.owner)
			for _, v := range other.verbList {
				v.owner = rewriteOwner(v.owner)
			}
			for name, p := range other.properties.all() {
				p.owner = rewriteOwner(p.owner)
				other.properties.put(name, p)
			}
			stampObjectAll(other, ts)
		}
		return true
	})

	// Fix references in anonymous objects as well. They live in s.anonObjects,
	// not the directory walked above, so without this an anonymous child of a
	// renumbered object keeps a stale parent id and property access through it
	// fails (E_PROPNF). Toast walks anonymous_objects in
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
		for name, p := range anon.properties.all() {
			p.owner = rewriteOwner(p.owner)
			anon.properties.put(name, p)
		}
	}

	return nil
}

// RegisterWaif registers a waif with its class object, and unregisters it when
// the waif is collected.
func (s *Store) RegisterWaif(classID types.ObjID, waif types.Value) {
	identity := waif.WaifIdentity()
	s.mu.Lock()
	if s.waifRegistry == nil {
		s.waifRegistry = make(map[types.ObjID]map[types.WaifIdentity]struct{})
	}

	if s.waifRegistry[classID] == nil {
		s.waifRegistry[classID] = make(map[types.WaifIdentity]struct{})
	}

	if _, registered := s.waifRegistry[classID][identity]; registered {
		s.mu.Unlock()
		return
	}
	s.waifRegistry[classID][identity] = struct{}{}
	s.mu.Unlock()

	waif.AddWaifCleanup(func() { s.unregisterWaif(classID, identity) })
}

func (s *Store) unregisterWaif(classID types.ObjID, identity types.WaifIdentity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	waifs := s.waifRegistry[classID]
	delete(waifs, identity)
	if len(waifs) == 0 {
		delete(s.waifRegistry, classID)
	}
}

// WaifCount returns the total number of live waifs across all classes.
func (s *Store) WaifCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, waifs := range s.waifRegistry {
		count += len(waifs)
	}
	return count
}

// WaifCountByClass returns a map of class ID to waif count.
func (s *Store) WaifCountByClass() map[types.ObjID]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[types.ObjID]int)
	for classID, waifs := range s.waifRegistry {
		if len(waifs) != 0 {
			result[classID] = len(waifs)
		}
	}
	return result
}
