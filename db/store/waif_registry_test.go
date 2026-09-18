package store

import (
	"runtime"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func TestWaifRegistryDoesNotKeepWaifsAlive(t *testing.T) {
	s := NewStore()
	func() {
		waif := types.NewWaif(57, 0)
		s.RegisterWaif(57, waif)
		if got := s.WaifCount(); got != 1 {
			t.Fatalf("WaifCount() while reachable = %d, want 1", got)
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for s.WaifCount() != 0 && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	if got := s.WaifCount(); got != 0 {
		t.Fatalf("WaifCount() after collection = %d, want 0", got)
	}
	if got := s.WaifCountByClass(); len(got) != 0 {
		t.Fatalf("WaifCountByClass() after collection = %#v, want empty", got)
	}
}
