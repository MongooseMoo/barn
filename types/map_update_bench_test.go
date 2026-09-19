package types

import (
	"fmt"
	"os"
	"runtime"
	"testing"
)

var mapBenchmarkResult Value

func BenchmarkMapBulkConstructor(b *testing.B) {
	for _, size := range []int{1, 2, 4, 8, 16, 2000} {
		pairs := make([][2]Value, size)
		for i := range pairs {
			pairs[i] = [2]Value{NewInt(int64(i)), NewInt(int64(i))}
		}
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				mapBenchmarkResult = NewMap(pairs)
			}
		})
	}
}

// Opt-in because retained heap requires full GC cycles and many live maps.
// This test uses only the public API so it can also run on the baseline.
func TestMapRetainedFootprint(t *testing.T) {
	if os.Getenv("BARN_MAP_FOOTPRINT") != "1" {
		t.Skip("set BARN_MAP_FOOTPRINT=1 for retained heap measurements")
	}
	for _, size := range []int{1, 2, 4, 8, 16, 2000} {
		pairs := make([][2]Value, size)
		for i := range pairs {
			pairs[i] = [2]Value{NewInt(int64(i)), NewInt(int64(i))}
		}
		count := 4096
		if size > 16 {
			count = 128
		}
		values := make([]Value, count)
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for i := range values {
			values[i] = NewMap(pairs)
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(values)
		t.Logf("size=%d copies=%d retained_B/map=%.1f objects/map=%.1f", size, count,
			float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/float64(count),
			float64(int64(after.HeapObjects)-int64(before.HeapObjects))/float64(count))
	}
}

func BenchmarkMapBuild(b *testing.B) {
	for _, size := range []int{100, 2000, 10000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m := NewEmptyMap()
				for i := 0; i < size; i++ {
					m = m.MapSet(NewInt(int64(i)), NewInt(int64(i)))
				}
				mapBenchmarkResult = m
			}
		})
	}
}

func BenchmarkMapOperations(b *testing.B) {
	pairs := make([][2]Value, 2000)
	for i := range pairs {
		pairs[i] = [2]Value{NewInt(int64(i)), NewInt(int64(i))}
	}
	m := NewMap(pairs)
	b.Run("overwrite", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mapBenchmarkResult = m.MapSet(NewInt(1000), NewInt(7))
		}
	})
	b.Run("delete", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mapBenchmarkResult = m.MapDelete(NewInt(1000))
		}
	})
	b.Run("get", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mapBenchmarkResult, _ = m.MapGet(NewInt(1000))
		}
	})
	b.Run("pairs-after-write", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = m.MapSet(NewInt(1000), NewInt(7)).Pairs()
		}
	})
}
