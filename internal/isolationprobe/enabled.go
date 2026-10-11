//go:build elle

package isolationprobe

import (
	"os"
	"sync"
)

const Enabled = true

// This fault is available only in the dedicated checker build, never a normal
// server binary. The generated manifest records whether it was enabled.
var DisablePropertyReads = os.Getenv("BARN_ELLE_FAULT") == "property-reads"

var counters = struct {
	sync.Mutex
	values map[string]uint64
	failed map[any]bool
}{values: make(map[string]uint64), failed: make(map[any]bool)}

// Failed publications are outcome metadata, never an isolation witness.
func MarkFailed(context any) { counters.Lock(); counters.failed[context] = true; counters.Unlock() }
func Failed(context any) bool {
	counters.Lock()
	defer counters.Unlock()
	return counters.failed[context]
}

func Note(path string) {
	counters.Lock()
	counters.values[path]++
	counters.Unlock()
}

func Reset() {
	counters.Lock()
	counters.values = make(map[string]uint64)
	counters.failed = make(map[any]bool)
	counters.Unlock()
}

func Snapshot() map[string]uint64 {
	counters.Lock()
	defer counters.Unlock()
	out := make(map[string]uint64, len(counters.values))
	for key, value := range counters.values {
		out[key] = value
	}
	return out
}
