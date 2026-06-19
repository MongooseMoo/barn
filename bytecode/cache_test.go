package bytecode

import (
	"fmt"
	"testing"
)

// stubRegistry satisfies the Registry interface for compile tests without
// pulling in barn/builtins (which would create an import cycle).
type stubRegistry struct{}

func (stubRegistry) GetID(name string) (int, bool) { return 0, false }

var sampleVerb = []string{
	"x = 1;",
	"y = 2;",
	"return x + y;",
}

// TestCompileVerbBytecodeCachesByContent proves the cache returns the SAME
// *Program for identical source (a true cache hit, not a recompile) and a
// DIFFERENT *Program for changed source.
func TestCompileVerbBytecodeCachesByContent(t *testing.T) {
	verbProgramCache = newProgramCache(verbCacheCapacity)

	p1, err := CompileVerbBytecode(sampleVerb, stubRegistry{})
	if err != nil {
		t.Fatalf("first compile failed: %v", err)
	}
	if got := verbProgramCache.len(); got != 1 {
		t.Fatalf("after first compile cache len = %d, want 1", got)
	}

	p2, err := CompileVerbBytecode(sampleVerb, stubRegistry{})
	if err != nil {
		t.Fatalf("second compile failed: %v", err)
	}
	if p1 != p2 {
		t.Fatalf("second compile returned a different *Program (%p vs %p): cache miss, recompiled", p1, p2)
	}
	if got := verbProgramCache.len(); got != 1 {
		t.Fatalf("after warm hit cache len = %d, want 1 (no new entry)", got)
	}

	changed := append([]string(nil), sampleVerb...)
	changed[0] = "x = 99;"
	p3, err := CompileVerbBytecode(changed, stubRegistry{})
	if err != nil {
		t.Fatalf("changed compile failed: %v", err)
	}
	if p3 == p1 {
		t.Fatalf("changed source returned the cached program: content key not honored")
	}
	if got := verbProgramCache.len(); got != 2 {
		t.Fatalf("after changed compile cache len = %d, want 2", got)
	}
}

// TestProgramCacheEviction proves the LRU cap bounds memory and that an evicted
// entry simply recompiles (no stale-code / correctness issue).
func TestProgramCacheEviction(t *testing.T) {
	c := newProgramCache(2)
	c.put(1, &Program{})
	c.put(2, &Program{})
	c.put(3, &Program{}) // should evict key 1 (least recently used)

	if _, ok := c.get(1); ok {
		t.Fatalf("key 1 should have been evicted")
	}
	if _, ok := c.get(2); !ok {
		t.Fatalf("key 2 should still be cached")
	}
	if _, ok := c.get(3); !ok {
		t.Fatalf("key 3 should still be cached")
	}
	if got := c.len(); got != 2 {
		t.Fatalf("cache len = %d, want 2 (capped)", got)
	}
}

// BenchmarkCompileVerbCold measures compiling a fresh, never-seen verb source
// every iteration (cache always misses -> parse + compile).
func BenchmarkCompileVerbCold(b *testing.B) {
	reg := stubRegistry{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Unique source each iteration -> guaranteed cache miss -> recompile.
		code := []string{fmt.Sprintf("return %d;", i)}
		verbProgramCache = newProgramCache(verbCacheCapacity)
		if _, err := CompileVerbBytecode(code, reg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompileVerbWarm measures repeated compilation of the SAME source: the
// first call compiles, every subsequent call is a content-hash cache hit. This
// is the steady-state cost the master pointer-identity cache also achieved.
func BenchmarkCompileVerbWarm(b *testing.B) {
	reg := stubRegistry{}
	verbProgramCache = newProgramCache(verbCacheCapacity)
	if _, err := CompileVerbBytecode(sampleVerb, reg); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := CompileVerbBytecode(sampleVerb, reg); err != nil {
			b.Fatal(err)
		}
	}
}
