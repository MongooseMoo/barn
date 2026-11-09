package types

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

func TestInternStrOwnsExactBytes(t *testing.T) {
	for _, text := range []string{"tell", "Tell", "te*ll", "tell speak", "héllo", "\xff\x00A"} {
		backing := strings.Repeat("x", 1<<20) + text
		input := backing[len(backing)-len(text):]
		value := InternStr(input)
		if value.Str() != text || InternStr(strings.Clone(text)) != value {
			t.Fatalf("interned content or immediate reuse differs for %q", text)
		}
		if unsafe.StringData(value.Str()) == unsafe.StringData(input) {
			t.Fatalf("intern retained input backing for %q", text)
		}
		appended := value.StrAppend(NewStr("!"))
		if appended.Str() != text+"!" || value.Str() != text || InternStr(text).Str() != text {
			t.Fatalf("append changed interned content for %q", text)
		}
	}
	if InternStr("Tell") == InternStr("tell") {
		t.Fatal("intern folded case")
	}
	if InternStr("") != NewStr("") || InternStr(strings.Repeat("x", 65)).Str() != strings.Repeat("x", 65) {
		t.Fatal("empty/long constructor fallback changed content")
	}
}

func TestInternStrEvictionPreservesReturnedValues(t *testing.T) {
	// More distinct values than slots guarantees collisions, independently of
	// the process hash seed. An evicted representation must stay immutable.
	values := make([]Value, 16385)
	for i := range values {
		values[i] = InternStr(fmt.Sprintf("eviction%05d", i))
	}
	for i, value := range values {
		if want := fmt.Sprintf("eviction%05d", i); value.Str() != want {
			t.Fatalf("eviction changed value %d: %q, want %q", i, value.Str(), want)
		}
	}
}

func TestInternStrConcurrentCharacterCountsAndAppend(t *testing.T) {
	texts := []string{"tell", "héllo", "\xff\x00A"}
	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() {
			for i := range 1000 {
				text := texts[i%len(texts)]
				value := InternStr(text)
				if value.StrCharLen() != CharCount(text) || value.StrAppend(NewStr("!")).Str() != text+"!" || value.Str() != text {
					t.Error("concurrent intern/append changed string contents or character count")
					return
				}
			}
		})
	}
	workers.Wait()
}

func TestInternStrRetainedBudgetUnderUniqueInputs(t *testing.T) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := range 100000 {
		InternStr(fmt.Sprintf("%064d", i))
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	// Entry reps and owned text total at most about 2 MiB. Leave 1 MiB for
	// unrelated test/runtime allocation, rather than require an exact GC delta.
	if retained := int64(after.HeapAlloc) - int64(before.HeapAlloc); retained > 3<<20 {
		t.Fatalf("unique inputs retained %d bytes beyond the bounded pool", retained)
	}
}

func TestInternStrWarmHitDoesNotAllocate(t *testing.T) {
	InternStr("warm_hit")
	if allocations := testing.AllocsPerRun(1000, func() { InternStr("warm_hit") }); allocations != 0 {
		t.Fatalf("warm intern allocated %g objects", allocations)
	}
}
