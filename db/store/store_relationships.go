package store

import "barn/types"

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

func (s *Store) MoveObject(whatID types.ObjID, whereID types.ObjID, position int64) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	what := s.load(whatID)
	if !validLiveObject(what) {
		return types.E_INVIND
	}
	if whereID != types.ObjNothing && !validLiveObject(s.load(whereID)) {
		return types.E_INVARG
	}

	ts := s.bumpClockLocked()
	if what.location != types.ObjNothing {
		if oldLoc := s.load(what.location); validLiveObject(oldLoc) {
			oldLoc = s.republishForMutation(oldLoc)
			oldLoc.contents = removeObjID(oldLoc.contents, whatID)
			stampObjectRelationship(oldLoc, ts)
		}
	}

	what = s.republishForMutation(what)
	what.location = whereID
	stampObjectRelationship(what, ts)

	if whereID != types.ObjNothing {
		if where := s.load(whereID); validLiveObject(where) {
			where = s.republishForMutation(where)
			where.contents = insertObjIDAtMOOPosition(where.contents, whatID, position)
			stampObjectRelationship(where, ts)
		}
	}
	return types.E_NONE
}

func (s *Store) Parent(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.ObjNothing, types.E_INVIND
	}
	if len(obj.parents) == 0 {
		return types.ObjNothing, types.E_NONE
	}
	return obj.parents[0], types.E_NONE
}

func (s *Store) Parents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}
	return append([]types.ObjID(nil), obj.parents...), types.E_NONE
}

func (s *Store) Children(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}
	return append([]types.ObjID(nil), obj.children...), types.E_NONE
}

func (s *Store) Contents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}
	return append([]types.ObjID(nil), obj.contents...), types.E_NONE
}

func (s *Store) Location(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return types.ObjNothing, types.E_INVIND
	}
	return obj.location, types.E_NONE
}

func (s *Store) Ancestors(objID types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}

	result := make([]types.ObjID, 0)
	seen := make(map[types.ObjID]bool)
	queue := make([]types.ObjID, 0, len(obj.parents))
	if includeSelf {
		result = append(result, objID)
		seen[objID] = true
	}
	queue = append(queue, obj.parents...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		result = append(result, currentID)
		current := s.liveObjectLocked(currentID)
		if validLiveObject(current) {
			queue = append(queue, current.parents...)
		}
	}

	return result, types.E_NONE
}

func (s *Store) Descendants(objID types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return nil, types.E_INVIND
	}

	result := make([]types.ObjID, 0)
	seen := make(map[types.ObjID]bool)
	queue := make([]types.ObjID, 0, len(obj.children))
	if includeSelf {
		result = append(result, objID)
		seen[objID] = true
	}
	queue = append(queue, obj.children...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		result = append(result, currentID)
		current := s.liveObjectLocked(currentID)
		if validLiveObject(current) {
			queue = append(queue, current.children...)
		}
	}

	return result, types.E_NONE
}

func (s *Store) HasAncestor(objID, ancestorID types.ObjID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil || s.liveObjectLocked(ancestorID) == nil {
		return false
	}
	if objID == ancestorID {
		return true
	}

	seen := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), obj.parents...)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		if currentID == ancestorID {
			return true
		}
		current := s.liveObjectLocked(currentID)
		if validLiveObject(current) {
			queue = append(queue, current.parents...)
		}
	}
	return false
}

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

func (s *Store) HasContentDescendant(objID, targetID types.ObjID) bool {
	if objID == targetID {
		return true
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.liveObjectLocked(objID) == nil {
		return false
	}

	queue := []types.ObjID{objID}
	visited := make(map[types.ObjID]bool)
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if visited[currentID] {
			continue
		}
		visited[currentID] = true
		if currentID == targetID {
			return true
		}
		current := s.liveObjectLocked(currentID)
		if validLiveObject(current) {
			queue = append(queue, current.contents...)
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
	for _, oldParentID := range obj.parents {
		oldParent := s.load(oldParentID)
		if !validLiveObject(oldParent) {
			continue
		}
		oldParent = s.republishForMutation(oldParent)
		oldParent.children = removeObjID(oldParent.children, objID)
		if oldParent.chparentChildren != nil {
			delete(oldParent.chparentChildren, objID)
		}
		stampObjectRelationship(oldParent, ts)
	}

	obj = s.republishForMutation(obj)
	obj.parents = append([]types.ObjID(nil), newParents...)
	for _, parentID := range obj.parents {
		// republish the new parent so attachChildToParentsLocked (which mutates
		// parent.children via s.load) writes the fresh image, not a shared alias.
		s.republishForMutation(s.load(parentID))
	}
	s.attachChildToParentsLocked(objID, obj.parents, false, true)
	s.reseedInheritedPropertiesLocked(obj)
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

// NextID returns the next available object ID
// Uses highWaterID to ensure unique IDs (including anonymous objects)
// Recycled slots are NOT automatically reused
