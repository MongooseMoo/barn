package types

import (
	"hash/maphash"
	"strings"
	"sync"
)

const internMaxBytes = 64

type stringInternShard struct {
	mu    sync.Mutex
	slots [256]*strRep
}

var shortStrings = struct {
	seed   maphash.Seed
	shards [64]stringInternShard
}{seed: maphash.MakeSeed()}

// InternStr creates a string Value using a bounded pool of exact short strings.
// It is for repetitive producers, not general string construction. The pool
// owns at most 16,384 representations and at most 64 text bytes per entry;
// replacing a slot never changes a Value already returned. Empty and longer
// strings use NewStr. Sharing and eviction are not observable MOO behavior.
func InternStr(s string) Value {
	if len(s) == 0 || len(s) > internMaxBytes {
		return NewStr(s)
	}
	h := maphash.String(shortStrings.seed, s)
	shard := &shortStrings.shards[h&63]
	index := (h >> 6) & 255
	shard.mu.Lock()
	defer shard.mu.Unlock()
	rep := shard.slots[index]
	if rep == nil || rep.val != s {
		// A short substring must not keep its much larger input allocation.
		// No watermark/data backing: appends cannot mutate this representation.
		rep = &strRep{val: strings.Clone(s)}
		shard.slots[index] = rep
	}
	return strValue(rep)
}
