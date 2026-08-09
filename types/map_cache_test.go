package types

import (
	"fmt"
	"sync"
	"testing"
)

func TestToastRootIsCachedAcrossReads(t *testing.T) {
	m := newBenchmarkMap(1_000).goMap()
	first := m.toastRoot()
	if first == nil {
		t.Fatal("toastRoot() returned nil for a non-empty map")
	}

	if second := m.toastRoot(); second != first {
		t.Fatal("toastRoot() rebuilt the tree instead of returning the cached root")
	}

	if allocs := testing.AllocsPerRun(100, func() {
		if root := m.toastRoot(); root != first {
			t.Fatal("toastRoot() returned a different root")
		}
	}); allocs != 0 {
		t.Fatalf("cached toastRoot() allocated %v times per read, want 0", allocs)
	}
}

func TestToastRootConcurrentReadersShareRoot(t *testing.T) {
	m := newBenchmarkMap(1_000).goMap()
	const readers = 32

	roots := make([]*toastLookupNode, readers)
	var ready sync.WaitGroup
	ready.Add(readers)
	start := make(chan struct{})
	var done sync.WaitGroup
	done.Add(readers)
	for i := range roots {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			roots[i] = m.toastRoot()
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()

	for i, root := range roots[1:] {
		if root != roots[0] {
			t.Fatalf("reader %d received a different tree root", i+1)
		}
	}
}

func TestGetWithCaseInsensitiveUsesHashIndex(t *testing.T) {
	smallAllocs := getWithCaseInsensitiveAllocs(t, 10)
	largeAllocs := getWithCaseInsensitiveAllocs(t, 1_000)
	if largeAllocs > smallAllocs {
		t.Fatalf("case-insensitive lookup allocations grew with map size: size 10 = %v, size 1000 = %v", smallAllocs, largeAllocs)
	}
}

func getWithCaseInsensitiveAllocs(t *testing.T, size int) float64 {
	t.Helper()
	m := newBenchmarkMap(size)
	key := NewStr(fmt.Sprintf("KEY-%d", size/2))
	allocs := testing.AllocsPerRun(100, func() {
		got, ok := m.GetWithCase(key, false)
		if !ok || got.Int() != int64(size/2) {
			t.Fatalf("GetWithCase(%q, false) = (%v, %v), want (%d, true)", key.Str(), got, ok, size/2)
		}
	})
	if m.goMap().root != nil {
		t.Fatal("case-insensitive lookup built the ordered tree")
	}
	return allocs
}

func BenchmarkMapGetWithCase(b *testing.B) {
	for _, size := range []int{100, 1_000, 5_000} {
		m := newBenchmarkMap(size)
		key := NewStr(fmt.Sprintf("key-%d", size/2))

		b.Run(fmt.Sprintf("size=%d/case-insensitive", size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m.GetWithCase(key, false)
			}
		})

		b.Run(fmt.Sprintf("size=%d/case-sensitive", size), func(b *testing.B) {
			m.GetWithCase(key, true) // Build the ordered lookup tree outside the timer.
			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				m.GetWithCase(key, true)
			}
		})
	}
}

func newBenchmarkMap(size int) Value {
	pairs := make([][2]Value, size)
	for i := range pairs {
		pairs[i] = [2]Value{NewStr(fmt.Sprintf("key-%d", i)), NewInt(int64(i))}
	}
	return NewMap(pairs)
}
