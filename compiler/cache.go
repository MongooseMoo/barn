package compiler

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"sync"

	"barn/bytecode"
)

const mooCacheCapacity = 8192

type programCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[[sha256.Size]byte]*list.Element
	lru      *list.List
}

type cacheEntry struct {
	key     [sha256.Size]byte
	program *bytecode.Program
}

func newProgramCache(capacity int) *programCache {
	return &programCache{
		capacity: capacity,
		entries:  make(map[[sha256.Size]byte]*list.Element),
		lru:      list.New(),
	}
}

func (c *programCache) get(key [sha256.Size]byte) (*bytecode.Program, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(element)
	return element.Value.(*cacheEntry).program, true
}

func (c *programCache) put(key [sha256.Size]byte, program *bytecode.Program) {
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

func sourceKey(lines []string) [sha256.Size]byte {
	hash := sha256.New()
	var length [8]byte
	for _, line := range lines {
		binary.LittleEndian.PutUint64(length[:], uint64(len(line)))
		hash.Write(length[:])
		hash.Write([]byte(line))
	}
	var key [sha256.Size]byte
	copy(key[:], hash.Sum(nil))
	return key
}

var mooProgramCache = newProgramCache(mooCacheCapacity)
