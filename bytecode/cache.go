package bytecode

import (
	"container/list"
	"hash/fnv"
	"sync"
)

// verbCacheCapacity bounds the number of distinct compiled programs kept in
// memory. Eviction is a pure memory-management concern (NOT correctness): a
// changed verb hashes to a new key and recompiles, so an evicted entry only
// costs a recompile, never stale code. The cap is generous; toastcore-class
// databases have a few thousand distinct verb bodies.
const verbCacheCapacity = 8192

// programCache is a bounded, content-addressed cache of compiled verb programs.
// It is keyed by a hash of the RAW stored verb source (the []string Code joined
// deterministically), so correctness is automatic: identical source -> same key
// -> cached *Program; changed source -> new key -> recompile. Eviction is LRU.
//
// The cached *Program is treated as immutable: the VM keeps all per-execution
// mutable state (IP, Locals, LoopStack, ExceptStack) on the StackFrame, never on
// the Program, so a single *Program is safely shared across concurrent and
// repeated executions. This matches the master behavior, where one *Program was
// cached per verb and reused.
type programCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[uint64]*list.Element // key -> list element holding *cacheEntry
	lru      *list.List               // front = most recently used
}

type cacheEntry struct {
	key  uint64
	prog *Program
}

func newProgramCache(capacity int) *programCache {
	return &programCache{
		capacity: capacity,
		entries:  make(map[uint64]*list.Element),
		lru:      list.New(),
	}
}

// get returns the cached program for key and marks it most-recently-used.
func (c *programCache) get(key uint64) (*Program, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.lru.MoveToFront(el)
		return el.Value.(*cacheEntry).prog, true
	}
	return nil, false
}

// put stores prog under key, evicting the least-recently-used entry if the cache
// is over capacity. A concurrent put of the same key (lost compile race) keeps
// the first stored program; both are equivalent since they share source.
func (c *programCache) put(key uint64, prog *Program) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.lru.MoveToFront(el)
		el.Value.(*cacheEntry).prog = prog
		return
	}
	el := c.lru.PushFront(&cacheEntry{key: key, prog: prog})
	c.entries[key] = el
	if c.capacity > 0 && c.lru.Len() > c.capacity {
		oldest := c.lru.Back()
		if oldest != nil {
			c.lru.Remove(oldest)
			delete(c.entries, oldest.Value.(*cacheEntry).key)
		}
	}
}

// len reports the current number of cached programs (used by tests/benchmarks).
func (c *programCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

// hashCode computes a fast, non-cryptographic FNV-1a hash over the raw stored
// verb source. Lines are joined with a '\n' separator that is also written
// between lines (not just concatenated) so that, e.g., ["ab","c"] and ["a","bc"]
// hash differently. This mirrors the deterministic join used by the parser.
func hashCode(code []string) uint64 {
	h := fnv.New64a()
	for i, line := range code {
		if i > 0 {
			h.Write([]byte{'\n'})
		}
		h.Write([]byte(line))
	}
	return h.Sum64()
}

// verbProgramCache is the package-global compiled-verb cache used by
// CompileVerbBytecode.
var verbProgramCache = newProgramCache(verbCacheCapacity)
