//go:build elle

package isolationprobe

import (
	"os"
	"sync"
	"sync/atomic"
)

const Enabled = true

// Faults are available only in the dedicated checker build, never a normal
// server binary. Each one removes a single protection, named where it is
// consulted; the generated manifest records which was active.
var fault = os.Getenv("BARN_ELLE_FAULT")

// Fault reports whether the named deliberate fault is active.
func Fault(name string) bool { return fault == name }

// ActiveFault is the fault this process runs under, or "".
func ActiveFault() string { return fault }

var pause atomic.Pointer[func(string)]

// SetPause installs fn to run at every Pause point, or removes it when nil.
func SetPause(fn func(point string)) {
	if fn == nil {
		pause.Store(nil)
		return
	}
	pause.Store(&fn)
}

// Pause lets a workload hold the calling goroutine at a named point inside the
// store, so an interleaving that is otherwise microseconds wide happens every run.
func Pause(point string) {
	if fn := pause.Load(); fn != nil {
		(*fn)(point)
	}
}

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
