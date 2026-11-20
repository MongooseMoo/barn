package compiler

import (
	"container/list"
	"sync"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/sourcekey"
)

const mooCacheCapacity = 8192

// cacheShards is the number of independently locked partitions. Every verb
// call looks up its program here, so one mutex serializes all running tasks;
// 64 shards puts concurrent hits on different locks with high probability for
// any realistic number of cores while keeping 128 entries per shard at the
// default capacity. It must be a power of two (shardFor masks the hash).
const cacheShards = 64

// programCache is a bounded LRU of compiled programs keyed by source content,
// partitioned into shards by the key's hash. Each shard has its own lock, LRU
// and in-flight compilation table, so eviction is LRU within a shard.
type programCache struct {
	shards [cacheShards]cacheShard
}

type cacheShard struct {
	mu       sync.Mutex
	capacity int
	entries  map[sourcekey.Key]*list.Element
	lru      *list.List
	flights  map[sourcekey.Key]*compilation
}

type cacheEntry struct {
	key     sourcekey.Key
	program *bytecode.Program
}

type compilation struct {
	done        chan struct{}
	program     *bytecode.Program
	diagnostics []Diagnostic
	panicked    bool
	panicValue  any
}

func (call *compilation) wait() (*bytecode.Program, []Diagnostic) {
	<-call.done
	if call.panicked {
		panic(call.panicValue)
	}
	// The flight owns its diagnostic storage; callers may edit their own slices.
	return call.program, append([]Diagnostic(nil), call.diagnostics...)
}

// newProgramCache splits capacity evenly across the shards. A positive
// capacity smaller than the shard count still bounds every shard.
func newProgramCache(capacity int) *programCache {
	perShard := capacity / cacheShards
	if capacity > 0 && perShard == 0 {
		perShard = 1
	}
	c := &programCache{}
	for i := range c.shards {
		c.shards[i] = cacheShard{
			capacity: perShard,
			entries:  make(map[sourcekey.Key]*list.Element),
			lru:      list.New(),
		}
	}
	return c
}

func (c *programCache) shardFor(key sourcekey.Key) *cacheShard {
	return &c.shards[key.Uint64()&(cacheShards-1)]
}

func (c *programCache) get(key sourcekey.Key) (*bytecode.Program, bool) {
	return c.shardFor(key).get(key)
}

func (c *programCache) beginCompile(key sourcekey.Key) (*bytecode.Program, *compilation, bool) {
	return c.shardFor(key).beginCompile(key)
}

func (c *programCache) runCompile(key sourcekey.Key, call *compilation, compile func() (*bytecode.Program, []Diagnostic)) (*bytecode.Program, []Diagnostic) {
	return c.shardFor(key).runCompile(key, call, compile)
}

func (c *programCache) endCompile(key sourcekey.Key, call *compilation) {
	c.shardFor(key).endCompile(key, call)
}

func (c *cacheShard) get(key sourcekey.Key) (*bytecode.Program, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(element)
	return element.Value.(*cacheEntry).program, true
}

// beginCompile rechecks the cache after a cold lookup, then joins or owns a
// flight. Parsing and lowering take place after this mutex has been released.
func (c *cacheShard) beginCompile(key sourcekey.Key) (*bytecode.Program, *compilation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		c.lru.MoveToFront(element)
		return element.Value.(*cacheEntry).program, nil, false
	}
	if call := c.flights[key]; call != nil {
		return nil, call, false
	}
	if c.flights == nil {
		c.flights = make(map[sourcekey.Key]*compilation)
	}
	call := &compilation{done: make(chan struct{})}
	c.flights[key] = call
	return nil, call, true
}

func (c *cacheShard) runCompile(key sourcekey.Key, call *compilation, compile func() (*bytecode.Program, []Diagnostic)) (*bytecode.Program, []Diagnostic) {
	completed := false
	defer func() {
		if !completed {
			call.panicked = true
			call.panicValue = recover()
		}
		c.endCompile(key, call)
		if call.panicked {
			panic(call.panicValue)
		}
	}()
	call.program, call.diagnostics = compile()
	completed = true
	return call.program, append([]Diagnostic(nil), call.diagnostics...)
}

// endCompile publishes success and releases every waiter, including on errors
// or panic. Failed compilations never become persistent cache entries.
func (c *cacheShard) endCompile(key sourcekey.Key, call *compilation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !call.panicked && call.program != nil && len(call.diagnostics) == 0 {
		c.putLocked(key, call.program)
	}
	delete(c.flights, key)
	if len(c.flights) == 0 {
		c.flights = nil
	}
	close(call.done)
}

func (c *cacheShard) putLocked(key sourcekey.Key, program *bytecode.Program) {
	if element, ok := c.entries[key]; ok {
		c.lru.MoveToFront(element)
		element.Value.(*cacheEntry).program = program
		return
	}
	element := c.lru.PushFront(&cacheEntry{key: key, program: program})
	c.entries[key] = element
	if c.capacity > 0 && c.lru.Len() > c.capacity {
		oldest := c.lru.Back()
		c.lru.Remove(oldest)
		delete(c.entries, oldest.Value.(*cacheEntry).key)
	}
}
