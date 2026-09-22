package store

import (
	"github.com/MongooseMoo/barn/types"
	"sort"
)

func collectAnonymousObjectRefs(value types.Value, out map[types.ObjID]struct{}) {
	collectAnonymousObjectRefsVisited(value, out, nil, false)
}

func collectAnonymousObjectRefsVisited(value types.Value, out map[types.ObjID]struct{}, visitedWaifs map[types.WaifIdentity]struct{}, retained bool) {
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
		if retained {
			value.VisitRetainedWaifValues(func(property types.Value) { collectAnonymousObjectRefsVisited(property, out, visitedWaifs, true) })
			return
		}
		for _, name := range value.PropertyNames() {
			if property, ok := value.GetProperty(name); ok {
				collectAnonymousObjectRefsVisited(property, out, visitedWaifs, retained)
			}
		}
	case types.TYPE_LIST:
		for _, elem := range value.Elements() {
			collectAnonymousObjectRefsVisited(elem, out, visitedWaifs, retained)
		}
	case types.TYPE_MAP:
		for _, pair := range value.Pairs() {
			collectAnonymousObjectRefsVisited(pair[0], out, visitedWaifs, retained)
			collectAnonymousObjectRefsVisited(pair[1], out, visitedWaifs, retained)
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
	return s.persistentAnonymousReachability(true)
}

// CommittedAnonymousReachability is the graph serialized by a checkpoint.
// Unlike semantic GC reachability, it excludes historical WAIF edges.
func (s *Store) CommittedAnonymousReachability() map[types.ObjID]struct{} {
	return s.persistentAnonymousReachability(false)
}

func (s *Store) persistentAnonymousReachability(retained bool) map[types.ObjID]struct{} {
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
			collectAnonymousObjectRefsVisited(prop.value, refs, nil, retained)
			for id := range refs {
				queue = append(queue, id)
			}
		}
		return true
	})

	s.expandAnonymousReachabilityLocked(reachable, queue, retained)
	return reachable
}

// ExpandCommittedAnonymousReachability expands only edges a checkpoint writes.
func (s *Store) ExpandCommittedAnonymousReachability(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	queue := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		queue = append(queue, id)
	}
	s.expandAnonymousReachabilityLocked(reachable, queue, false)
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

func (s *Store) expandAnonymousReachability(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	if len(refs) == 0 {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	queue := make([]types.ObjID, 0, len(refs))
	for id := range refs {
		queue = append(queue, id)
	}
	s.expandAnonymousReachabilityLocked(reachable, queue, true)
}

// ExpandAnonymousReachability expands anonymous roots through this transaction's
// object view, including property values staged by the current task. A live-store
// walk cannot see those values until commit and may otherwise collect an object
// that is reachable through the task's in-flight anonymous graph.
func (tx *StoreTxn) ExpandAnonymousReachability(reachable map[types.ObjID]struct{}, refs map[types.ObjID]struct{}) {
	if tx == nil || len(refs) == 0 {
		return
	}
	if tx.direct {
		tx.store.expandAnonymousReachability(reachable, refs)
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
			collectAnonymousObjectRefsVisited(prop.value, nested, nil, true)
		}
		for nestedID := range nested {
			queue = append(queue, nestedID)
		}
	}
}

func (s *Store) expandAnonymousReachabilityLocked(reachable map[types.ObjID]struct{}, queue []types.ObjID, retained bool) {
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
			collectAnonymousObjectRefsVisited(prop.value, nested, nil, retained)
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

// collectWaifsInto records every waif reachable from value (through lists,
// maps, and waif properties) in set. A waif already present is not re-expanded,
// which also terminates on cyclic waif graphs.
func collectWaifsInto(value types.Value, set *types.WaifSet, retained bool) {
	switch value.Type() {
	case types.TYPE_WAIF:
		if !set.Add(value) {
			return
		}
		if retained {
			value.VisitRetainedWaifValues(func(property types.Value) { collectWaifsInto(property, set, true) })
		} else {
			for _, name := range value.PropertyNames() {
				if property, ok := value.GetProperty(name); ok {
					collectWaifsInto(property, set, false)
				}
			}
		}
	case types.TYPE_LIST:
		if !value.MayHoldFinalizable() {
			return
		}
		for _, elem := range value.Elements() {
			collectWaifsInto(elem, set, retained)
		}
	case types.TYPE_MAP:
		if !value.MayHoldFinalizable() {
			return
		}
		for _, pair := range value.Pairs() {
			collectWaifsInto(pair[0], set, retained)
			collectWaifsInto(pair[1], set, retained)
		}
	}
}

// persistentWaifRootsEntry memoizes the top-level scan for one waifRootsEpoch
// and, for one (waifRootsEpoch, types.WaifGraphEpoch) pair, the expanded
// closure as an immutable set shared by every caller.
type persistentWaifRootsEntry struct {
	epoch      uint64
	top        []types.Value // waifs held directly in property values (through lists/maps), no closure expansion
	graphEpoch uint64
	closure    *types.WaifSet // top expanded through waif properties; read-only once published
}

// propertyValueMayHoldFinalizable reports whether obj's slot for name currently
// holds a value that may reference a WAIF or anonymous object.
func propertyValueMayHoldFinalizable(obj *Object, name string) bool {
	_, prop, ok := propertyByName(obj.properties, name)
	return ok && prop.value.MayHoldFinalizable()
}

// collectTopLevelWaifsInto records the waifs directly inside value (descending
// through lists and maps) without expanding the waifs' own properties.
func collectTopLevelWaifsInto(value types.Value, set *types.WaifSet) {
	switch value.Type() {
	case types.TYPE_WAIF:
		set.Add(value)
	case types.TYPE_LIST:
		if !value.MayHoldFinalizable() {
			return
		}
		for _, elem := range value.Elements() {
			collectTopLevelWaifsInto(elem, set)
		}
	case types.TYPE_MAP:
		if !value.MayHoldFinalizable() {
			return
		}
		for _, pair := range value.Pairs() {
			collectTopLevelWaifsInto(pair[0], set)
			collectTopLevelWaifsInto(pair[1], set)
		}
	}
}

// PersistentWaifRoots returns every waif reachable from a live object's
// property values, deduplicated by identity. The slice is shared and must be
// treated as read-only; PersistentWaifRootSet is the O(1)-membership form.
func (s *Store) PersistentWaifRoots() []types.Value {
	set := s.PersistentWaifRootSet()
	if set.Values == nil {
		return []types.Value{}
	}
	return set.Values
}

// PersistentWaifRootSet returns the identity set of every waif reachable from
// a live object's property values. The set is immutable and shared: layer
// additions on it with types.NewWaifSetOver.
//
// Two memos, because the two inputs change at different rates. The walk over
// every property of every object (the finalizer asks after every task that let
// a WAIF leave scope, and on a large database that scan dominated the task) is
// memoized per waifRootsEpoch, which every store write that can add or remove
// a waif from a property value bumps (noteWaifRootsChanged). The expansion of
// those roots through current and retained WAIF images is memoized per
// (waifRootsEpoch, types.WaifGraphEpoch), including history retirement.
func (s *Store) PersistentWaifRootSet() *types.WaifSet {
	return s.persistentWaifRootSet(true)
}

// CommittedWaifRootSet excludes history when deciding serialized ownership.
func (s *Store) CommittedWaifRootSet() *types.WaifSet {
	return s.persistentWaifRootSet(false)
}

func (s *Store) persistentWaifRootSet(retained bool) *types.WaifSet {
	s.mu.RLock()
	epoch := s.waifRootsEpoch.Load()
	graphEpoch := types.WaifGraphEpoch()
	entry := s.waifRootsCache.Load()
	if retained && entry != nil && entry.epoch == epoch && entry.graphEpoch == graphEpoch {
		s.mu.RUnlock()
		return entry.closure
	}
	var top []types.Value
	if entry != nil && entry.epoch == epoch {
		top = entry.top
	} else {
		set := types.NewWaifSet(nil)
		s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
			obj := slot.ptr.Load()
			if obj == nil || !validLiveObject(obj) {
				return true
			}
			for _, prop := range obj.properties {
				if prop.value.MayHoldFinalizable() {
					collectTopLevelWaifsInto(prop.value, set)
				}
			}
			return true
		})
		top = set.Values
	}
	s.mu.RUnlock()

	// Expand outside the store lock over immutable WAIF images. A concurrent
	// reference publication or history retirement moves the graph epoch, so
	// the entry stored below is already stale and the next call recomputes.
	closure := types.NewWaifSet(nil)
	for _, waif := range top {
		collectWaifsInto(waif, closure, retained)
	}
	if retained {
		s.waifRootsCache.Store(&persistentWaifRootsEntry{epoch: epoch, top: top, graphEpoch: graphEpoch, closure: closure})
	}
	return closure
}

// LocalProperty returns a copy of the property slot defined on the object
// itself. It does not search ancestors.
