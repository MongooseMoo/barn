package compiler

import (
	"container/list"
	"sync"

	"barn/bytecode"
	"barn/sourcekey"
)

const mooCacheCapacity = 8192

type programCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[sourcekey.Key]*list.Element
	lru      *list.List
}

type cacheEntry struct {
	key     sourcekey.Key
	program *bytecode.Program
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

func (c *programCache) put(key sourcekey.Key, program *bytecode.Program) {
	c.mu.Lock()
	defer c.mu.Unlock()
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

var mooProgramCache = newProgramCache(mooCacheCapacity)
