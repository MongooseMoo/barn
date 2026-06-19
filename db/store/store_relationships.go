package store

import "barn/types"

func (s *Store) attachChildToParentsLocked(childID types.ObjID, parents []types.ObjID, anonymous bool, chparent bool) {
	for _, parentID := range parents {
		parent := s.objects[parentID]
		if !validLiveObject(parent) {
			continue
		}
		if anonymous {
			parent.AnonymousChildren = append(parent.AnonymousChildren, childID)
			continue
		}
		parent.Children = append(parent.Children, childID)
		if chparent {
			if parent.ChparentChildren == nil {
				parent.ChparentChildren = make(map[types.ObjID]bool)
			}
			parent.ChparentChildren[childID] = true
		}
	}
}

func (s *Store) MoveObject(whatID types.ObjID, whereID types.ObjID, position int64) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	what := s.objects[whatID]
	if !validLiveObject(what) {
		return types.E_INVIND
	}

	if what.Location != types.ObjNothing {
		oldLoc := s.objects[what.Location]
		if validLiveObject(oldLoc) {
			oldLoc.Contents = removeObjID(oldLoc.Contents, whatID)
		}
	}

	what.Location = whereID

	if whereID != types.ObjNothing {
		where := s.objects[whereID]
		if validLiveObject(where) {
			where.Contents = insertObjIDAtMOOPosition(where.Contents, whatID, position)
		}
	}
	return types.E_NONE
}

func (s *Store) Parent(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	if len(obj.Parents) == 0 {
		return types.ObjNothing, types.E_NONE
	}
	return obj.Parents[0], types.E_NONE
}

func (s *Store) Parents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	return append([]types.ObjID(nil), obj.Parents...), types.E_NONE
}

func (s *Store) Children(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	return append([]types.ObjID(nil), obj.Children...), types.E_NONE
}

func (s *Store) Contents(objID types.ObjID) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}
	return append([]types.ObjID(nil), obj.Contents...), types.E_NONE
}

func (s *Store) Location(objID types.ObjID) (types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.ObjNothing, types.E_INVIND
	}
	return obj.Location, types.E_NONE
}

func (s *Store) Ancestors(objID types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}

	result := make([]types.ObjID, 0)
	seen := make(map[types.ObjID]bool)
	queue := make([]types.ObjID, 0, len(obj.Parents))
	if includeSelf {
		result = append(result, objID)
		seen[objID] = true
	}
	queue = append(queue, obj.Parents...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		result = append(result, currentID)
		current := s.objects[currentID]
		if validLiveObject(current) {
			queue = append(queue, current.Parents...)
		}
	}

	return result, types.E_NONE
}

func (s *Store) Descendants(objID types.ObjID, includeSelf bool) ([]types.ObjID, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return nil, types.E_INVIND
	}

	result := make([]types.ObjID, 0)
	seen := make(map[types.ObjID]bool)
	queue := make([]types.ObjID, 0, len(obj.Children))
	if includeSelf {
		result = append(result, objID)
		seen[objID] = true
	}
	queue = append(queue, obj.Children...)

	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		result = append(result, currentID)
		current := s.objects[currentID]
		if validLiveObject(current) {
			queue = append(queue, current.Children...)
		}
	}

	return result, types.E_NONE
}

func (s *Store) HasAncestor(objID, ancestorID types.ObjID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) || !validLiveObject(s.objects[ancestorID]) {
		return false
	}
	if objID == ancestorID {
		return true
	}

	seen := make(map[types.ObjID]bool)
	queue := append([]types.ObjID(nil), obj.Parents...)
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
		current := s.objects[currentID]
		if validLiveObject(current) {
			queue = append(queue, current.Parents...)
		}
	}
	return false
}

func (s *Store) HasDescendant(objID, descendantID types.ObjID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return false
	}
	return s.hasDescendantLocked(obj, descendantID)
}

func (s *Store) hasDescendantLocked(obj *Object, descendantID types.ObjID) bool {
	for _, childID := range obj.Children {
		if childID == descendantID {
			return true
		}
		child := s.objects[childID]
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

	if !validLiveObject(s.objects[objID]) {
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
		current := s.objects[currentID]
		if validLiveObject(current) {
			queue = append(queue, current.Contents...)
		}
	}
	return false
}

func (s *Store) ChangeParents(objID types.ObjID, newParents []types.ObjID) types.ErrorCode {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj := s.objects[objID]
	if !validLiveObject(obj) {
		return types.E_INVIND
	}

	for _, oldParentID := range obj.Parents {
		oldParent := s.objects[oldParentID]
		if !validLiveObject(oldParent) {
			continue
		}
		oldParent.Children = removeObjID(oldParent.Children, objID)
		if oldParent.ChparentChildren != nil {
			delete(oldParent.ChparentChildren, objID)
		}
	}

	obj.Parents = append([]types.ObjID(nil), newParents...)
	s.attachChildToParentsLocked(objID, obj.Parents, false, true)
	s.reseedInheritedPropertiesLocked(obj)
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
