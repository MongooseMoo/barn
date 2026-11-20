package types

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"unsafe"
)

func TestSmallMapSetUsesOneAllocation(t *testing.T) {
	for size := 1; size <= mapFlatLimit; size++ {
		pairs := make([][2]Value, size)
		for i := range pairs {
			pairs[i] = [2]Value{NewInt(int64(i)), NewInt(0)}
		}
		base := NewMap(pairs)
		_ = base.MayHoldFinalizable()
		allocs := testing.AllocsPerRun(100, func() {
			mapBenchmarkResult = base.MapSet(NewInt(0), NewInt(7))
		})
		if allocs != 1 {
			t.Errorf("size %d replacement allocated %v times, want one owned header/entries allocation", size, allocs)
		}
		if size < mapFlatLimit {
			allocs = testing.AllocsPerRun(100, func() {
				mapBenchmarkResult = base.MapSet(NewInt(int64(size)), NewInt(7))
			})
			if allocs != 1 {
				t.Errorf("size %d insertion allocated %v times, want one owned header/entries allocation", size, allocs)
			}
		}
		if value, _ := base.MapGet(NewInt(0)); value.Int() != 0 || base.Len() != size {
			t.Fatal("derived update changed its source")
		}
	}
}

func TestMapPromotionPreservesColdAndWarmAliases(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(fmt.Sprint(warm), func(t *testing.T) {
			m := NewEmptyMap()
			var versions []Value
			for i := 0; i < 19; i++ {
				versions = append(versions, m)
				if warm {
					_ = m.Pairs()
				}
				m = m.MapSet(NewInt(int64(i)), NewInt(int64(i*3)))
			}
			m = m.MapDelete(NewInt(0)).MapSet(NewInt(0), NewInt(99))
			for n, version := range versions {
				if version.Len() != n {
					t.Fatalf("version %d has length %d", n, version.Len())
				}
				pairs := version.PairsInInsertionOrder()
				for i, pair := range pairs {
					if pair[0].Int() != int64(i) || pair[1].Int() != int64(i*3) {
						t.Fatalf("version %d pair %d changed: %v", n, i, pair)
					}
				}
				if ValueBytes(version) != ValueBytes(NewMap(pairs)) {
					t.Fatalf("version %d size cache changed", n)
				}
			}
			if pairs := m.PairsInInsertionOrder(); pairs[len(pairs)-1][0].Int() != 0 {
				t.Fatal("reinsert did not move to end")
			}
		})
	}
}

func TestMapPromotionPropagatesFinalization(t *testing.T) {
	m := NewEmptyMap()
	for i := 0; i < mapFlatLimit; i++ {
		m = m.MapSet(NewInt(int64(i)), NewInt(0))
	}
	if m.MayHoldFinalizable() {
		t.Fatal("plain map is finalizable")
	}
	tainted := m.MapSet(NewInt(99), NewList([]Value{NewAnon(5)}))
	if !tainted.MayHoldFinalizable() || m.MayHoldFinalizable() {
		t.Fatal("promotion changed source proof or lost nested finalizable value")
	}
	clean := tainted.MapDelete(NewInt(99))
	if clean.MayHoldFinalizable() || !tainted.MayHoldFinalizable() {
		t.Fatal("deletion changed alias proof or retained removed finalizable value")
	}
	t.Logf("map header bytes: %d", unsafe.Sizeof(goMap{}))
}

func BenchmarkMapPromotion(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		m := NewEmptyMap()
		for i := 1; i <= 17; i++ {
			m = m.MapSet(NewInt(int64(i)), NewInt(int64(i)))
		}
		mapBenchmarkResult = m.MapDelete(NewInt(17))
	}
}

func TestFlatMapStorageKeepsPointersAlive(t *testing.T) {
	for size := 1; size <= mapFlatLimit; size++ {
		m := NewEmptyMap()
		for i := 0; i < size; i++ {
			m = m.MapSet(NewStr(fmt.Sprintf("key-%d", i)), NewStr(fmt.Sprintf("value-%d", i)))
		}
		runtime.GC()
		for i := 0; i < size; i++ {
			value, ok := m.MapGet(NewStr(fmt.Sprintf("key-%d", i)))
			if !ok || value.Str() != fmt.Sprintf("value-%d", i) {
				t.Fatalf("size %d entry %d was lost after GC", size, i)
			}
		}
	}
}

func TestMapFinalizationConcurrentColdReaders(t *testing.T) {
	for _, tainted := range []bool{false, true} {
		pairs := make([][2]Value, 32)
		for i := range pairs {
			pairs[i] = [2]Value{NewInt(int64(i)), NewInt(0)}
		}
		if tainted {
			pairs[31][1] = NewList([]Value{NewWaif(1, 2)})
		}
		m := NewMap(pairs)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 32 {
			wg.Go(func() {
				<-start
				if m.MayHoldFinalizable() != tainted {
					t.Error("cold readers disagree about finalization")
				}
			})
		}
		close(start)
		wg.Wait()
	}
}

func TestMapBulkConversionWarmWriteAllocations(t *testing.T) {
	pairs := make([][2]Value, 2000)
	for i := range pairs {
		pairs[i] = [2]Value{NewInt(int64(i)), NewInt(int64(i))}
	}
	bulk := NewMap(pairs)
	direct := NewMap(append(pairs, pairs[1000])) // Duplicate forces the indexed constructor.
	_ = bulk.MapSet(NewInt(1000), NewInt(7))
	_ = direct.MayHoldFinalizable()
	allocations := func(m Value) float64 {
		return testing.AllocsPerRun(100, func() {
			mapBenchmarkResult = m.MapSet(NewInt(1000), NewInt(7))
		})
	}
	converted, indexed := allocations(bulk), allocations(direct)
	t.Logf("warm write allocations: bulk-converted=%v directly-indexed=%v", converted, indexed)
	if converted != indexed {
		t.Fatal("bulk conversion adds an allocation on later writes")
	}
}

func BenchmarkMapBulkWrite(b *testing.B) {
	pairs := make([][2]Value, 2000)
	for i := range pairs {
		pairs[i] = [2]Value{NewInt(int64(i)), NewInt(int64(i))}
	}
	duplicates := append(append([][2]Value(nil), pairs...), pairs[1000])
	b.Run("cold", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			mapBenchmarkResult = NewMap(pairs).MapSet(NewInt(1000), NewInt(7))
		}
	})
	b.Run("warm", func(b *testing.B) {
		m := NewMap(pairs)
		_ = m.MapSet(NewInt(1000), NewInt(7))
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			mapBenchmarkResult = m.MapSet(NewInt(1000), NewInt(7))
		}
	})
	b.Run("duplicate-constructor", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			mapBenchmarkResult = NewMap(duplicates)
		}
	})
	b.Run("warm-directly-indexed", func(b *testing.B) {
		m := NewMap(duplicates)
		_ = m.MayHoldFinalizable()
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			mapBenchmarkResult = m.MapSet(NewInt(1000), NewInt(7))
		}
	})
}

// The same opt-in as TestMapRetainedFootprint. Count unique constructed payloads,
// with input pairs allocated before the snapshot and kept live in every mode.
func TestMapMutationRetainedFootprint(t *testing.T) {
	if os.Getenv("BARN_MAP_FOOTPRINT") != "1" {
		t.Skip("set BARN_MAP_FOOTPRINT=1 for retained heap measurements")
	}
	for _, size := range []int{16, 17, 2000} {
		for _, mode := range []string{"bulk", "grown", "write-source-retained", "write-source-dropped"} {
			pairs := make([][2]Value, size)
			for i := range pairs {
				pairs[i] = [2]Value{NewInt(int64(i)), NewInt(int64(i))}
			}
			count := 4096
			if size > 17 {
				count = 128
			}
			values, sources := make([]Value, count), make([]Value, count)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for i := range values {
				m := NewMap(pairs)
				switch mode {
				case "grown":
					m = NewEmptyMap()
					for _, pair := range pairs {
						m = m.MapSet(pair[0], pair[1])
					}
				case "write-source-retained", "write-source-dropped":
					if mode == "write-source-retained" {
						sources[i] = m
					}
					m = m.MapSet(NewInt(int64(size/2)), NewInt(-1))
				}
				values[i] = m
			}
			runtime.GC()
			runtime.ReadMemStats(&after)
			runtime.KeepAlive(values)
			runtime.KeepAlive(sources)
			runtime.KeepAlive(pairs)
			t.Logf("size=%d mode=%s retained_B/map=%.1f objects/map=%.1f", size, mode,
				float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/float64(count),
				float64(int64(after.HeapObjects)-int64(before.HeapObjects))/float64(count))
		}
	}
}
