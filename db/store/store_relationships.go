package store

import (
	"github.com/MongooseMoo/barn/types"
)

func (s *Store) attachChildToParentsLocked(childID types.ObjID, parents []types.ObjID, anonymous bool, chparent bool) {
	for _, parentID := range parents {
		parent := s.load(parentID)
		if !validLiveObject(parent) {
			continue
		}
		if anonymous {
			parent.anonymousChildren = append(parent.anonymousChildren, childID)
			continue
		}
		parent.children = append(parent.children, childID)
		if chparent {
			if parent.chparentChildren == nil {
				parent.chparentChildren = make(map[types.ObjID]bool)
			}
			parent.chparentChildren[childID] = true
		}
	}
}

// ancestorWalkInline is how many queued parent ids a HasAncestor walk holds on
// the stack before it spills to the heap.
const ancestorWalkInline = 32

func (s *Store) HasDescendant(objID, descendantID types.ObjID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return false
	}
	return s.hasDescendantLocked(obj, descendantID)
}

func (s *Store) hasDescendantLocked(obj *Object, descendantID types.ObjID) bool {
	for _, childID := range obj.children {
		if childID == descendantID {
			return true
		}
		child := s.liveObjectLocked(childID)
		if validLiveObject(child) && s.hasDescendantLocked(child, descendantID) {
			return true
		}
	}
	return false
}

func (s *Store) ChangeParents(objID types.ObjID, newParents []types.ObjID) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.E_INVIND
	}

	ts := s.bumpClockLocked()
	s.noteWaifRootsChanged()
	s.noteVerbShapeChanged()
	for _, oldParentID := range obj.parents {
		oldParent := s.load(oldParentID)
		if !validLiveObject(oldParent) {
			continue
		}
		oldParent = s.republishForMutation(oldParent)
		if obj.anonymous {
			oldParent.anonymousChildren = removeObjID(oldParent.anonymousChildren, objID)
		}
		oldParent.children = removeObjID(oldParent.children, objID)
		if oldParent.chparentChildren != nil {
			delete(oldParent.chparentChildren, objID)
		}
		stampObjectRelationship(oldParent, ts)
	}

	obj = s.republishForMutation(obj)
	oldParents := obj.parents
	obj.parents = append([]types.ObjID(nil), newParents...)
	for _, parentID := range obj.parents {
		// republish the new parent so attachChildToParentsLocked (which mutates
		// parent.children via s.load) writes the fresh image, not a shared alias.
		s.republishForMutation(s.load(parentID))
	}
	s.attachChildToParentsLocked(objID, obj.parents, obj.anonymous, true)
	s.reseedInheritedPropertiesLocked(obj, oldParents, ts)
	stampObjectRelationship(obj, ts)
	stampObjectProperties(obj, ts)
	for _, parentID := range obj.parents {
		stampObjectRelationship(s.load(parentID), ts)
	}
	return types.E_NONE
}

func removeObjID(slice []types.ObjID, id types.ObjID) []types.ObjID {
	result := make([]types.ObjID, 0, len(slice))
	for _, item := range slice {
		if item != id {
			result = append(result, item)
		}
	}
	return result
}

func insertObjIDAtMOOPosition(slice []types.ObjID, id types.ObjID, position int64) []types.ObjID {
	if position == 0 || position > int64(len(slice)+1) {
		return append(slice, id)
	}

	index := int(position - 1)
	result := make([]types.ObjID, len(slice)+1)
	copy(result[:index], slice[:index])
	result[index] = id
	copy(result[index+1:], slice[index:])
	return result
}
