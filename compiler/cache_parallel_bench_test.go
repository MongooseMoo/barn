package compiler

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/MongooseMoo/barn/bytecode"
	"github.com/MongooseMoo/barn/sourcekey"
)

// BenchmarkProgramCacheParallelGet measures cache hits from many goroutines,
// the verb-call hot path where every call looks up its compiled program.
func BenchmarkProgramCacheParallelGet(b *testing.B) {
	cache := newProgramCache(mooCacheCapacity)
	keys := make([]sourcekey.Key, 1024)
	for i := range keys {
		keys[i] = sourcekey.Of([]string{fmt.Sprintf("return %d;", i)})
		_, call, _ := cache.beginCompile(keys[i])
		call.program = &bytecode.Program{}
		cache.endCompile(keys[i], call)
	}
	var next atomic.Uint64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := next.Add(7919)
		for pb.Next() {
			i++
			if _, ok := cache.get(keys[i%uint64(len(keys))]); !ok {
				b.Fatal("cached key missed")
			}
		}
	})
}
