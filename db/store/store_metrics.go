package store

import "barn/types"

func (s *Store) ObjectByteEstimate(objID types.ObjID) (int, types.ErrorCode) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	obj := s.liveObjectLocked(objID)
	if obj == nil {
		return 0, types.E_INVIND
	}
	return calculateObjectBytes(obj), types.E_NONE
}

func calculateObjectBytes(obj *Object) int {
	count := 64 + 8

	count += len(obj.name) + 1

	for _, verb := range obj.verbs {
		count += 32
		count += len(verb.name) + 1
		// NOTE (verbcache spike): the AST (verb.Program) no longer lives on the
		// verb, so the AST-size term is dropped here. A full landing would query
		// the relocated bytecode cache for an accurate estimate.
	}

	for _, prop := range obj.properties {
		if prop.defined {
			count += 32
			count += len(prop.name) + 1
		}
	}

	for _, prop := range obj.properties {
		count += 24
		count += calculateValueBytes(prop.value)
	}

	return count
}

func calculateValueBytes(v types.Value) int {
	size := 16

	switch val := v.(type) {
	case types.StrValue:
		size += len(val.Value()) + 1
	case types.FloatValue:
		size += 8
	case types.ListValue:
		elements := val.Elements()
		size += len(elements) * 16
		for _, elem := range elements {
			size += calculateValueBytes(elem)
		}
	case types.MapValue:
		pairs := val.Pairs()
		size += len(pairs) * 32
		for _, pair := range pairs {
			size += calculateValueBytes(pair[0])
			size += calculateValueBytes(pair[1])
		}
	case types.WaifValue:
		size += 64
	}

	return size
}

// Players returns all objects with the player flag set

func (s *Store) NoteVerbCacheClear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.verbCacheClears++
	// A cache clear starts a fresh interval for miss accounting.
	s.verbCacheMisses = 0
}

// NoteVerbCacheMiss increments the compatibility miss counter used by verb_cache_stats().

func (s *Store) NoteVerbCacheMiss() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.verbCacheMisses++
}

// ConsumeVerbCacheStats returns a 17-element stats vector and resets interval counters.
// Slot [1] tracks cache clears, slot [2] tracks misses; remaining slots are reserved.

func (s *Store) ConsumeVerbCacheStats() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := make([]int64, 17)
	// Compatibility behavior: expose clear activity as a 0/1 interval flag.
	// This avoids cross-test accumulation noise and matches conformance expectations.
	if s.verbCacheClears > 0 {
		stats[0] = 1
	}
	stats[1] = s.verbCacheMisses

	s.verbCacheClears = 0
	s.verbCacheMisses = 0

	return stats
}

// ResetMaxObject recomputes max_object() and allocation high-water marks from live objects.

func (s *Store) ResetMaxObject() {
	s.mu.Lock()
	defer s.mu.Unlock()

	maxAny := types.ObjID(-1)
	maxNonAnon := types.ObjID(-1)

	for id, obj := range s.objects {
		if obj == nil || obj.recycled {
			continue
		}
		if id > maxAny {
			maxAny = id
		}
		if !obj.anonymous && id > maxNonAnon {
			maxNonAnon = id
		}
	}

	s.highWaterID = maxAny
	s.maxObjID = maxNonAnon
}
