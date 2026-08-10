package store

import (
	"github.com/MongooseMoo/barn/types"
	"sort"
)

func collectAnonymousObjectRefs(value types.Value, out map[types.ObjID]struct{}) {
	collectAnonymousObjectRefsVisited(value, out, nil)
}

func collectAnonymousObjectRefsVisited(value types.Value, out map[types.ObjID]struct{}, visitedWaifs map[types.WaifIdentity]struct{}) {
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
			visitedWaifs = make(map[types.WaifIdentity]struct{})
		}
		visitedWaifs[identity] = struct{}{}
		for _, name := range value.PropertyNames() {
			if property, ok := value.GetProperty(name); ok {
				collectAnonymousObjectRefsVisited(property, out, visitedWaifs)
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

// lookupAnonymousLocked returns the live anonymous object with the given
// identity id, regardless of which backing map holds it. Runtime-created and
// database-loaded anonymous objects live in s.anonObjects; some test fixtures
// (and any object added via Add with the anonymous flag) live in s.objects.
// Anon scanning subsystems must consider both so the planner, GC candidate scan,
// and serializer all operate over one consistent set of anonymous objects.
// Caller holds s.mu.
func (s *Store) lookupAnonymousLocked(id types.ObjID) *Object {
	if obj := s.anonObjects[id]; validLiveObject(obj) && obj.anonymous {
		return obj
	}
	if obj := s.load(id); validLiveObject(obj) && obj.anonymous {
		return obj
	}
	return nil
}

// rangeAnonymousLocked invokes fn for every live anonymous object across both
// backing maps. Caller holds s.mu.
func (s *Store) rangeAnonymousLocked(fn func(*Object)) {
	for _, obj := range s.anonObjects {
		if validLiveObject(obj) && obj.anonymous {
			fn(obj)
		}
	}
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if validLiveObject(obj) && obj.anonymous {
			fn(obj)
		}
		return true
	})
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

// ExpandAnonymousReachability expands anonymous roots through this transaction's
// object view, including property values staged by the current task. A live-store
// walk cannot see those values until commit and may otherwise collect an object
// that is reachable through the task's in-flight anonymous graph.
func (tx *StoreTxn) ExpandAnonymousReachability(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	if tx == nil || len(refs) == 0 {
		return
	}

	queue := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		queue = append(queue, id)
	}
	visited := make(map[types.ObjID]struct{}, len(refs))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, seen := visited[id]; seen {
			continue
		}
		visited[id] = struct{}{}

		obj := tx.object(id)
		if !validLiveObject(obj) || !obj.anonymous {
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

func collectWaifsFromValueVisited(value types.Value, out *[]types.Value, visited map[types.WaifIdentity]struct{}) {
	switch value.Type() {
	case types.TYPE_WAIF:
		if !finalizationValueInList(value, *out) {
			*out = append(*out, value)
		}
		identity := value.WaifIdentity()
		if _, seen := visited[identity]; seen {
			return
		}
		if visited == nil {
			visited = make(map[types.WaifIdentity]struct{})
		}
		visited[identity] = struct{}{}
		for _, name := range value.PropertyNames() {
			if property, ok := value.GetProperty(name); ok {
				collectWaifsFromValueVisited(property, out, visited)
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
