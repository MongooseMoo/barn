package types

import (
	"fmt"
	"sync"
	"testing"
)

func TestWaifImageScalarWritesPreserveReferenceGraphMemo(t *testing.T) {
	domain := new(WaifDomain)
	w := NewWaif(0, 0).SetProperty("child", NewWaif(0, 0)).SetProperty("n", NewInt(0))
	before, ok := w.WaifImageAt(domain, 0)
	if !ok {
		t.Fatal("attachment failed")
	}
	properties := before.Properties()
	properties["n"] = NewInt(1)
	epoch := WaifGraphEpoch()
	w.PublishWaifImage(domain, 1, properties)
	w.PruneWaifImages(domain, 1)
	if WaifGraphEpoch() != epoch {
		t.Fatal("scalar publication invalidated unchanged WAIF reference graph")
	}
	properties["child"] = NewWaif(0, 0)
	w.PublishWaifImage(domain, 2, properties)
	if WaifGraphEpoch() == epoch {
		t.Fatal("reference publication did not invalidate graph")
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
