package types

import (
	"sync"
	"testing"
)

func TestConcurrentAppendFromSharedListPreservesEachValue(t *testing.T) {
	const (
		workers = 32
		trials  = 100
	)

	for trial := 0; trial < trials; trial++ {
		shared := NewList([]Value{NewInt(0)}).Append(NewInt(1)).Append(NewInt(2))
		if got, want := cap(shared.sliceList().elements), shared.Len()+1; got < want {
			t.Fatalf("shared list capacity = %d, want at least %d to exercise in-place append", got, want)
		}

		start := make(chan struct{})
		results := make([]Value, workers)
		var wg sync.WaitGroup
		wg.Add(workers)
		for worker := 0; worker < workers; worker++ {
			go func() {
				defer wg.Done()
				<-start
				results[worker] = shared.Append(NewInt(int64(worker + 100)))
			}()
		}

		close(start)
		wg.Wait()

		for worker, result := range results {
			if got, want := result.Len(), shared.Len()+1; got != want {
				t.Fatalf("trial %d worker %d appended list length = %d, want %d", trial, worker, got, want)
			}
			if got, want := result.Get(result.Len()).Int(), int64(worker+100); got != want {
				t.Fatalf("trial %d worker %d appended value = %d, want %d", trial, worker, got, want)
			}
		}
	}
}
