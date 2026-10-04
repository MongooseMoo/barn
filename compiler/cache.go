package compiler

import (
	"container/list"
	"sync"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/sourcekey"
)

const mooCacheCapacity = 8192

type programCache struct {
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

func newProgramCache(capacity int) *programCache {
	return &programCache{
		capacity: capacity,
		entries:  make(map[sourcekey.Key]*list.Element),
		lru:      list.New(),
	}
}

func (c *programCache) get(key sourcekey.Key) (*bytecode.Program, bool) {
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
func (c *programCache) beginCompile(key sourcekey.Key) (*bytecode.Program, *compilation, bool) {
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

func (c *programCache) runCompile(key sourcekey.Key, call *compilation, compile func() (*bytecode.Program, []Diagnostic)) (*bytecode.Program, []Diagnostic) {
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
func (c *programCache) endCompile(key sourcekey.Key, call *compilation) {
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

func (c *programCache) putLocked(key sourcekey.Key, program *bytecode.Program) {
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
