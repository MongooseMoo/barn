package store

import (
	"barn/types"
	"sort"
)

func collectAnonymousObjectRefs(value types.Value, out map[types.ObjID]struct{}) {
	switch val := value.(type) {
	case types.ObjValue:
		if val.IsAnonymous() {
			out[val.ID()] = struct{}{}
		}
	case types.ListValue:
		for _, elem := range val.Elements() {
			collectAnonymousObjectRefs(elem, out)
		}
	case types.MapValue:
		for _, pair := range val.Pairs() {
			collectAnonymousObjectRefs(pair[0], out)
			collectAnonymousObjectRefs(pair[1], out)
		}
	}
}

func (s *Store) PersistentAnonymousReachability() map[types.ObjID]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reachable := make(map[types.ObjID]struct{})
	queue := make([]types.ObjID, 0)

	for _, obj := range s.objects {
		if !validLiveObject(obj) || obj.Anonymous {
			continue
		}
		for _, prop := range obj.Properties {
			if prop == nil {
				continue
			}
			refs := make(map[types.ObjID]struct{})
			collectAnonymousObjectRefs(prop.value, refs)
			for id := range refs {
				queue = append(queue, id)
			}
		}
	}

	s.expandAnonymousReachabilityLocked(reachable, queue)
	return reachable
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

		obj := s.objects[id]
		if !validLiveObject(obj) || !obj.Anonymous {
			continue
		}

		reachable[id] = struct{}{}
		nested := make(map[types.ObjID]struct{})
		for _, prop := range obj.Properties {
			if prop == nil {
				continue
			}
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
		obj := s.objects[id]
		if !validLiveObject(obj) || !obj.Anonymous {
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
	for _, obj := range s.objects {
		if !validLiveObject(obj) || !obj.Anonymous {
			continue
		}
		if obj.ID < minID {
			continue
		}
		if obj.Flags.Has(FlagUser) {
			continue
		}
		if _, keep := reachable[obj.ID]; keep {
			continue
		}
		candidates = append(candidates, obj.ID)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i] < candidates[j] })
	return candidates
}

func collectWaifsFromValue(value types.Value, out *[]types.WaifValue) {
	switch val := value.(type) {
	case types.WaifValue:
		*out = append(*out, val)
	case types.ListValue:
		for _, elem := range val.Elements() {
			collectWaifsFromValue(elem, out)
		}
	case types.MapValue:
		for _, pair := range val.Pairs() {
			collectWaifsFromValue(pair[0], out)
			collectWaifsFromValue(pair[1], out)
		}
	}
}

func (s *Store) PersistentWaifRoots() []types.WaifValue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	roots := make([]types.WaifValue, 0)
	for _, obj := range s.objects {
		if !validLiveObject(obj) {
			continue
		}
		for _, prop := range obj.Properties {
			if prop == nil {
				continue
			}
			collectWaifsFromValue(prop.value, &roots)
		}
	}
	return roots
}

// LocalProperty returns a copy of the property slot defined on the object
// itself. It does not search ancestors.
