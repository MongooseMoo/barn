package store

import (
	"log/slog"
	"strings"

	"github.com/MongooseMoo/barn/types"
)

// PropertySlotCensus counts the property slots held by live objects: how many
// objects, how many slots in all, how many of those slots are clear
// (inheriting their value), and how many slots objects store privately
// instead of reading them from a shared base.
func (s *Store) PropertySlotCensus() (objects, slots, clearSlots, privateSlots int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj == nil || obj.recycled {
			return true
		}
		objects++
		slots += obj.properties.count()
		privateSlots += obj.properties.privateCount()
		for _, prop := range obj.properties.all() {
			if prop.clear {
				clearSlots++
			}
		}
		return true
	})
	return objects, slots, clearSlots, privateSlots
}

// ShareLoadedPropertySlots moves every numbered object's property slots onto
// shared bases: objects with the same slot names and the same owner and perms
// on each slot point at one base and keep privately only the slots they set.
// What any object's properties read as does not change.
//
// The store keeps pool, the one the loader built shared tables through, or
// its own when pool is nil, so objects created afterwards find the same bases.
//
// Call it once, after the last loaded object is added and before the store is
// served.
func (s *Store) ShareLoadedPropertySlots(pool *PropSharePool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.propPool.mu.Lock()
	defer s.propPool.mu.Unlock()

	if pool != nil {
		s.propPool.pool = pool
	}
	s.dir.forEach(func(_ types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj != nil && !obj.recycled {
			obj.properties.share(s.propPool.pool)
		}
		return true
	})
}

// shareCreatedProperties moves a new object's table onto a shared base. The
// table must not yet be visible to any other goroutine.
func (s *Store) shareCreatedProperties(props propTable) {
	s.propPool.mu.Lock()
	defer s.propPool.mu.Unlock()
	props.share(s.propPool.pool)
}

func (tx *StoreTxn) ObjectByteEstimate(objID types.ObjID) (int, types.ErrorCode) {
	obj := tx.object(objID)
	if !validLiveObject(obj) {
		return 0, types.E_INVIND
	}
	tx.markObjectScalarRead(objID, obj)
	tx.markPropertyScan(objID, obj)
	for name, prop := range obj.properties.all() {
		tx.markPropertyRead(objID, name, prop)
	}
	return calculateObjectBytes(obj), types.E_NONE
}

func calculateObjectBytes(obj *Object) int {
	count := 64 + 8

	count += len(obj.name) + 1

	for _, verb := range obj.verbList {
		count += 32
		count += len(strings.Join(verb.names, " ")) + 1
		// Compiled programs are cached outside the verb (package bytecode), so
		// the estimate leaves them out.
	}

	for name, prop := range obj.properties.all() {
		if prop.defined {
			count += 32
			count += len(name) + 1
		}
	}

	for _, prop := range obj.properties.all() {
		count += 24
		count += calculateValueBytes(prop.value)
	}

	return count
}

func calculateValueBytes(v types.Value) int {
	size := 16

	switch v.Type() {
	case types.TYPE_STR:
		size += len(v.Str()) + 1
	case types.TYPE_FLOAT:
		size += 8
	case types.TYPE_LIST:
		elements := v.Elements()
		size += len(elements) * 16
		for _, elem := range elements {
			size += calculateValueBytes(elem)
		}
	case types.TYPE_MAP:
		pairs := v.Pairs()
		size += len(pairs) * 32
		for _, pair := range pairs {
			size += calculateValueBytes(pair[0])
			size += calculateValueBytes(pair[1])
		}
	case types.TYPE_WAIF:
		size += 64
	}

	return size
}

// verbCacheCleared is the bit of Store.verbCacheStats that records a cache
// clear in the current interval; the bits below it count misses.
const verbCacheCleared = uint64(1) << 63

// NoteVerbCacheClear records a verb cache clear for verb_cache_stats(). A clear
// starts a fresh interval for miss accounting.
func (s *Store) NoteVerbCacheClear() {
	s.verbCacheStats.Store(verbCacheCleared)
}

// NoteVerbCacheMiss counts a verb cache miss for verb_cache_stats().
func (s *Store) NoteVerbCacheMiss() {
	s.verbCacheStats.Add(1)
}

// ConsumeVerbCacheStats returns a 17-element stats vector and starts a new
// interval. Slot [0] is 1 if the cache was cleared in the interval, slot [1]
// counts misses since the last clear; the remaining slots are always 0.
func (s *Store) ConsumeVerbCacheStats() []int64 {
	return verbCacheStatsVector(s.verbCacheStats.Swap(0))
}

// VerbCacheStatsSnapshot reads the current interval without resetting it. The
// returned vector is owned by the caller.
func (s *Store) VerbCacheStatsSnapshot() []int64 {
	return verbCacheStatsVector(s.verbCacheStats.Load())
}

// LogVerbCacheStats emits real counters using the caller's task-scoped logger.
// Snapshotting finishes before invoking external logging handlers.
func (s *Store) LogVerbCacheStats(logger *slog.Logger) {
	stats := s.VerbCacheStatsSnapshot()
	logger.Info("Verb cache stat summary",
		slog.Int64("clear", stats[0]), slog.Int64("misses", stats[1]), slog.Any("stats", stats))
}

func verbCacheStatsVector(word uint64) []int64 {
	stats := make([]int64, 17)
	// Compatibility behavior: expose clear activity as a 0/1 interval flag.
	// This avoids cross-test accumulation noise and matches conformance expectations.
	if word&verbCacheCleared != 0 {
		stats[0] = 1
	}
	stats[1] = int64(word &^ verbCacheCleared)
	return stats
}

// ResetMaxObject recomputes max_object() and allocation high-water marks from live objects.
func (s *Store) ResetMaxObject() {
	s.mu.Lock()
	defer s.mu.Unlock()

	maxAny := types.ObjID(-1)
	maxNonAnon := types.ObjID(-1)

	s.dir.forEach(func(id types.ObjID, slot *objectSlot) bool {
		obj := slot.ptr.Load()
		if obj == nil || obj.recycled {
			return true
		}
		if id > maxAny {
			maxAny = id
		}
		if !obj.anonymous && id > maxNonAnon {
			maxNonAnon = id
		}
		return true
	})

	s.highWaterID.Store(int64(maxAny))
	s.maxObjID.Store(int64(maxNonAnon))
}
