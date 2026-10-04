package types

import (
	"sync"
	"testing"
)

func coldListMetadataFixture(tainted bool) Value {
	last := NewInt(2)
	if tainted {
		last = NewAnon(9)
	}
	return NewList([]Value{
		NewStr("outer"),
		NewList([]Value{NewInt(1), NewStr("x"), last}),
		NewList([]Value{NewInt(3), NewFloat(1.5)}),
	})
}

func uncachedListBytes(v Value) int {
	if v.Type() != TYPE_LIST {
		return ValueBytes(v)
	}
	size := listVarOverhead
	for _, element := range v.Elements() {
		size += uncachedListBytes(element)
	}
	return size
}

func TestConcurrentColdListMetadata(t *testing.T) {
	for _, tainted := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "nested anon"}[tainted], func(t *testing.T) {
			for trial := 0; trial < 20; trial++ {
				shared := coldListMetadataFixture(tainted)
				const workers = 16
				sizes := make([]int, workers)
				finalizable := make([]bool, workers)
				start := make(chan struct{})
				var wg sync.WaitGroup
				wg.Add(workers)
				for worker := 0; worker < workers; worker++ {
					go func() {
						defer wg.Done()
						<-start
						for read := 0; read < 8; read++ {
							sizes[worker] = ValueBytes(shared)
							finalizable[worker] = shared.MayHoldFinalizable()
						}
					}()
				}
				close(start)
				wg.Wait()
				for worker := 0; worker < workers; worker++ {
					if sizes[worker] != 208 || finalizable[worker] != tainted {
						t.Fatalf("trial %d worker %d: size=%d finalizable=%v, want 208 %v", trial, worker, sizes[worker], finalizable[worker], tainted)
					}
				}
			}
		})
	}
}

func TestConcurrentColdListMetadataAndDerivations(t *testing.T) {
	for _, tainted := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "nested anon"}[tainted], func(t *testing.T) {
			for trial := 0; trial < 20; trial++ {
				shared := coldListMetadataFixture(tainted)
				const workers = 21
				results := make([]Value, workers)
				sizes := make([]int, workers)
				finalizable := make([]bool, workers)
				start := make(chan struct{})
				var wg sync.WaitGroup
				wg.Add(workers)
				for worker := 0; worker < workers; worker++ {
					go func() {
						defer wg.Done()
						<-start
						result := shared
						switch worker % 7 {
						case 1:
							result = shared.Append(NewInt(42))
						case 2:
							result = shared.Set(1, NewStr("changed"))
						case 3:
							result = shared.DeleteAt(1)
						case 4:
							result = shared.Slice(2, 3)
						case 5:
							result = shared.Concat(shared)
						case 6:
							result = shared.InsertAt(2, NewInt(42))
						}
						results[worker] = result
						sizes[worker] = ValueBytes(result)
						finalizable[worker] = result.MayHoldFinalizable()
					}()
				}
				close(start)
				wg.Wait()
				if !shared.Equal(coldListMetadataFixture(tainted)) || ValueBytes(shared) != 208 || shared.MayHoldFinalizable() != tainted {
					t.Fatal("concurrent derivations changed the shared source")
				}
				for worker, result := range results {
					if sizes[worker] != uncachedListBytes(result) || finalizable[worker] != tainted {
						t.Fatalf("trial %d worker %d: size=%d finalizable=%v, want %d %v", trial, worker, sizes[worker], finalizable[worker], uncachedListBytes(result), tainted)
					}
				}
			}
		})
	}
}
