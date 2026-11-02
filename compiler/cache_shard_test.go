package compiler

import (
	"fmt"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/sourcekey"
)

// sameShardKeys returns n distinct keys that select the same shard.
func sameShardKeys(n int) []sourcekey.Key {
	var cache programCache
	keys := []sourcekey.Key{sourcekey.Of([]string{"return 0;"})}
	target := cache.shardFor(keys[0])
	for i := 1; len(keys) < n; i++ {
		key := sourcekey.Of([]string{fmt.Sprintf("return %d;", i)})
		if cache.shardFor(key) == target {
			keys = append(keys, key)
		}
	}
	return keys
}

// A lookup must not wait on the lock of another key's shard.
func TestProgramCacheShardsDoNotShareALock(t *testing.T) {
	cache := newProgramCache(mooCacheCapacity)
	held := sourcekey.Of([]string{"return 0;"})
	other := held
	for i := 1; cache.shardFor(other) == cache.shardFor(held); i++ {
		if i > 1000 {
			t.Fatal("1000 distinct keys all selected the same shard")
		}
		other = sourcekey.Of([]string{fmt.Sprintf("return %d;", i)})
	}
	shard := cache.shardFor(held)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		cache.get(other)
		_, call, _ := cache.beginCompile(other)
		call.program = &bytecode.Program{}
		cache.endCompile(other, call)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a key in another shard waited on the held shard's lock")
	}
}

// The total capacity is split across the shards and still bounds the cache.
func TestProgramCacheEvictsAtTotalCapacity(t *testing.T) {
	cache := newProgramCache(mooCacheCapacity)
	for i := range 2 * mooCacheCapacity {
		key := sourcekey.Of([]string{fmt.Sprintf("return %d;", i)})
		_, call, _ := cache.beginCompile(key)
		call.program = &bytecode.Program{}
		cache.endCompile(key, call)
	}
	total := 0
	for i := range cache.shards {
		shard := &cache.shards[i]
		if len(shard.entries) > mooCacheCapacity/cacheShards || shard.lru.Len() != len(shard.entries) {
			t.Fatalf("shard %d holds %d entries (lru %d), capacity %d", i, len(shard.entries), shard.lru.Len(), mooCacheCapacity/cacheShards)
		}
		total += len(shard.entries)
	}
	if total != mooCacheCapacity {
		t.Fatalf("cache holds %d entries after %d inserts, want capacity %d", total, 2*mooCacheCapacity, mooCacheCapacity)
	}
}
