package store

import (
	"barn/types"
	"sort"
	"unsafe"
)

func collectAnonymousObjectRefs(value types.Value, out map[types.ObjID]struct{}) {
	collectAnonymousObjectRefsVisited(value, out, nil)
}

func collectAnonymousObjectRefsVisited(value types.Value, out map[types.ObjID]struct{}, visitedWaifs map[unsafe.Pointer]struct{}) {
	switch value.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		if value.IsAnonymous() {
			out[value.ID()] = struct{}{}
		}
	case types.TYPE_WAIF:
		identity := value.WaifIdentity()
		if _, seen := visitedWaifs[identity]; seen {
			return
		}
		if visitedWaifs == nil {
			visitedWaifs = make(map[unsafe.Pointer]struct{})
		}
		visitedWaifs[identity] = struct{}{}
		for _, name := range value.PropertyNames() {
			if prop, ok := value.GetProperty(name); ok {
				collectAnonymousObjectRefsVisited(prop, out, visitedWaifs)
			}
		}
	case types.TYPE_LIST:
		for _, elem := range value.Elements() {
			collectAnonymousObjectRefsVisited(elem, out, visitedWaifs)
		}
	case types.TYPE_MAP:
		for _, pair := range value.Pairs() {
			collectAnonymousObjectRefsVisited(pair[0], out, visitedWaifs)
			collectAnonymousObjectRefsVisited(pair[1], out, visitedWaifs)
		}
	}
}

// collectDirectAnonymousObjectRefs finds anonymous candidates in ordinary
// container values but deliberately stops at WAIF boundaries. Anonymous values
// inside a pending WAIF are serialization dependencies of that WAIF, not separate
// pending-finalization roots.
func collectDirectAnonymousObjectRefs(value types.Value, out map[types.ObjID]struct{}) {
	switch value.Type() {
	case types.TYPE_OBJ, types.TYPE_ANON:
		if value.IsAnonymous() {
			out[value.ID()] = struct{}{}
		}
	case types.TYPE_LIST:
		for _, elem := range value.Elements() {
			collectDirectAnonymousObjectRefs(elem, out)
		}
	case types.TYPE_MAP:
		for _, pair := range value.Pairs() {
			collectDirectAnonymousObjectRefs(pair[0], out)
			collectDirectAnonymousObjectRefs(pair[1], out)
		}
	}
}

// lookupAnonymousLocked returns the live anonymous object with the given
// identity id. Runtime-created and database-loaded anonymous objects live only
// in s.anonObjects.
// Caller holds s.mu.
func (s *Store) lookupAnonymousLocked(id types.ObjID) *Object {
	if obj := s.anonObjects[id]; validLiveObject(obj) && obj.anonymous {
		return obj
	}
	return nil
}

// rangeAnonymousLocked invokes fn for every live anonymous object. Caller holds s.mu.
func (s *Store) rangeAnonymousLocked(fn func(*Object)) {
	for _, obj := range s.anonObjects {
		if validLiveObject(obj) && obj.anonymous {
			fn(obj)
		}
	}
}

func (s *Store) PersistentAnonymousReachability() map[types.ObjID]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reachable := make(map[types.ObjID]struct{})
	queue := make([]types.ObjID, 0)

	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj == nil || !validLiveObject(obj) || obj.anonymous {
			return true
		}
		for _, prop := range obj.properties {
			refs := make(map[types.ObjID]struct{})
			collectAnonymousObjectRefs(prop.value, refs)
			for id := range refs {
				queue = append(queue, id)
			}
		}
		return true
	})

	s.expandAnonymousReachabilityLocked(reachable, queue)
	return reachable
}

// HasAnonymousAtOrAbove reports whether any live anonymous object has an
// identity id >= minID. Orphan-anonymous collection restricts its recycle
// candidates to ids >= minID, so when this returns false the full
// persistent-reachability sweep is a guaranteed no-op and can be skipped —
// on large databases that sweep is far too expensive to pay after every task.
func (s *Store) HasAnonymousAtOrAbove(minID types.ObjID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	found := false
	s.rangeAnonymousLocked(func(obj *Object) {
		if obj.id >= minID {
			found = true
		}
	})
	return found
}

func (s *Store) ExpandAnonymousReachability(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	if len(refs) == 0 {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	queue := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		queue = append(queue, id)
	}
	s.expandAnonymousReachabilityLocked(reachable, queue)
}

func (s *Store) expandAnonymousReachabilityLocked(reachable map[types.ObjID]struct{}, queue []types.ObjID) {
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if _, seen := reachable[id]; seen {
			continue
		}

		obj := s.lookupAnonymousLocked(id)
		if obj == nil {
			continue
		}

		reachable[id] = struct{}{}
		nested := make(map[types.ObjID]struct{})
		for _, prop := range obj.properties {
			collectAnonymousObjectRefs(prop.value, nested)
		}
		for nestedID := range nested {
			queue = append(queue, nestedID)
		}
	}
}

func (s *Store) UnreachableAnonymousValues(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) []types.Value {
	if len(refs) == 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		if s.lookupAnonymousLocked(id) == nil {
			continue
		}
		if _, keep := reachable[id]; keep {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	values := make([]types.Value, 0, len(ids))
	for _, id := range ids {
		values = append(values, types.NewAnon(id))
	}
	return values
}

func (s *Store) AnonymousRecycleCandidates(reachable map[types.ObjID]struct{}, minID types.ObjID) []types.ObjID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	candidates := make([]types.ObjID, 0)
	s.rangeAnonymousLocked(func(obj *Object) {
		if obj.id < minID {
			return
		}
		if obj.flags.Has(FlagUser) {
			return
		}
		if _, keep := reachable[obj.id]; keep {
			return
		}
		candidates = append(candidates, obj.id)
	})
	sort.Slice(candidates, func(i, j int) bool { return candidates[i] < candidates[j] })
	return candidates
}

func collectWaifsFromValue(value types.Value, out *[]types.Value) {
	collectWaifsFromValueVisited(value, out, nil)
}

func collectWaifsFromValueVisited(value types.Value, out *[]types.Value, visited map[unsafe.Pointer]struct{}) {
	switch value.Type() {
	case types.TYPE_WAIF:
		identity := value.WaifIdentity()
		if _, seen := visited[identity]; seen {
			return
		}
		if visited == nil {
			visited = make(map[unsafe.Pointer]struct{})
		}
		visited[identity] = struct{}{}
		if !finalizationValueInList(value, *out) {
			*out = append(*out, value)
		}
		for _, name := range value.PropertyNames() {
			if prop, ok := value.GetProperty(name); ok {
				collectWaifsFromValueVisited(prop, out, visited)
			}
		}
	case types.TYPE_LIST:
		for _, elem := range value.Elements() {
			collectWaifsFromValueVisited(elem, out, visited)
		}
	case types.TYPE_MAP:
		for _, pair := range value.Pairs() {
			collectWaifsFromValueVisited(pair[0], out, visited)
			collectWaifsFromValueVisited(pair[1], out, visited)
		}
	}
}

func (s *Store) PersistentWaifRoots() []types.Value {
	s.mu.RLock()
	defer s.mu.RUnlock()

	roots := make([]types.Value, 0)
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj == nil || !validLiveObject(obj) {
			return true
		}
		for _, prop := range obj.properties {
			collectWaifsFromValue(prop.value, &roots)
		}
		return true
	})
	return roots
}

// LocalProperty returns a copy of the property slot defined on the object
// itself. It does not search ancestors.
