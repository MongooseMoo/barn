package types

import (
	"fmt"
	"sync"
	"testing"
)

func TestWaifWriteRevertRestoresPreviousValue(t *testing.T) {
	w := NewWaif(1, 2)
	w.SetProperty("n", NewInt(5))
	write := w.SwapProperty("n", NewInt(6))
	write.Revert()
	if got, _ := w.GetProperty("n"); got.Int() != 5 {
		t.Fatalf("n = %v after revert, want 5", got)
	}
}

func TestWaifWriteRevertRemovesPropertyItAdded(t *testing.T) {
	w := NewWaif(1, 2)
	write := w.SwapProperty("fresh", NewInt(1))
	write.Revert()
	if _, ok := w.GetProperty("fresh"); ok {
		t.Fatal("revert left a property the write had added")
	}
}

// Another task may have overwritten the property since; its write wins, so the
// revert must leave it alone rather than restore the stale value.
func TestWaifWriteRevertSkipsForeignOverwrite(t *testing.T) {
	w := NewWaif(1, 2)
	w.SetProperty("n", NewInt(5))
	write := w.SwapProperty("n", NewInt(6))
	w.SetProperty("n", NewInt(7))
	write.Revert()
	if got, _ := w.GetProperty("n"); got.Int() != 7 {
		t.Fatalf("n = %v after revert, want the foreign 7 kept", got)
	}
}

func TestWaifConcurrentAccessIsSynchronized(t *testing.T) {
	w := NewWaif(1, 2)
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				w.SetProperty(fmt.Sprintf("p%d", i%7), NewInt(int64(g)))
				w.GetProperty("p1")
				w.PropertyNames()
			}
		}(g)
	}
	wg.Wait()
}
