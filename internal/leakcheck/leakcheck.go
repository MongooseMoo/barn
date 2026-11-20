// Package leakcheck captures resource lifetime evidence at workload boundaries.
// Callers must stop their workload and release its owned resources before using
// repeated heap samples as a retention oracle. RSS alone is not a leak oracle.
package leakcheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"time"
)

// Heap is a Go-runtime reading, without forcing a collection. Live is from the
// last completed GC; Retained is runtime-owned system memory, not live objects.
type Heap struct {
	Alloc, Inuse, Sys, TotalSys, Retained, NextGC, Live uint64
	Objects, StackBytes, GCCycles                       uint64
	Goroutines                                          int
}

// ReadHeap is shared with the real-workload diagnostic sampler.
func ReadHeap() Heap {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	live := []metrics.Sample{{Name: "/gc/heap/live:bytes"}}
	metrics.Read(live)
	return Heap{Alloc: m.HeapAlloc, Inuse: m.HeapInuse, Sys: m.HeapSys,
		TotalSys: m.Sys, Retained: m.Sys - m.HeapReleased, NextGC: m.NextGC,
		Live: live[0].Value.Uint64(), Objects: m.HeapObjects, StackBytes: m.StackInuse,
		GCCycles: uint64(m.NumGC), Goroutines: runtime.NumGoroutine()}
}

// Sample keeps attribution, logical owners and process resources with the heap.
type Sample struct {
	PID       int       `json:"pid"`
	Time      time.Time `json:"time"`
	GoVersion string    `json:"go_version"`
	Heap      Heap      `json:"heap"`
	Process   Process   `json:"process"`
	Owners    any       `json:"owners,omitempty"`
	Leaked    int       `json:"leaked_goroutines"`
	Boundary  string    `json:"boundary"`
	Settings  Settings  `json:"settings"`
}

// Settings records the collector configuration actually in effect.
type Settings struct {
	GCPercent, MemoryLimit uint64
	MaxProcs, ProfileRate  int
}

func readSettings() Settings {
	values := []metrics.Sample{{Name: "/gc/gogc:percent"}, {Name: "/gc/gomemlimit:bytes"}}
	metrics.Read(values)
	return Settings{GCPercent: values[0].Value.Uint64(), MemoryLimit: values[1].Value.Uint64(),
		MaxProcs: runtime.GOMAXPROCS(0), ProfileRate: runtime.MemProfileRate}
}

// WriteJSON persists a complete evidence record. Write errors are experiment
// errors; callers must never turn missing evidence into a healthy verdict.
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func writeProfile(path, name string, debug int) error {
	profile := pprof.Lookup(name)
	if profile == nil {
		return fmt.Errorf("profile %q unavailable in %s", name, runtime.Version())
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = profile.WriteTo(f, debug)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Capture forces the native goroutine-leak detection GC, then saves leak-only
// and complete goroutine profiles, a retained heap profile and raw measurements.
// Never use debug=2 for the leak-only profile: Go emits all goroutines then.
func Capture(dir, boundary string, owners any) (Sample, error) {
	s := Sample{PID: os.Getpid(), Time: time.Now().UTC(), GoVersion: runtime.Version(),
		Boundary: boundary, Owners: owners, Settings: readSettings()}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return s, err
	}
	if err := writeProfile(filepath.Join(dir, "goroutineleak.pprof"), "goroutineleak", 0); err != nil {
		return s, err
	}
	s.Leaked = pprof.Lookup("goroutineleak").Count()
	if err := writeProfile(filepath.Join(dir, "goroutines.txt"), "goroutine", 2); err != nil {
		return s, err
	}
	// Finish sweeping before sampling; a GC is distinct from Barn's own object
	// lifecycle cleanup, which this package must never drive on the caller's behalf.
	runtime.GC()
	runtime.GC()
	s.Heap = ReadHeap()
	s.Process = ReadProcess()
	if err := writeProfile(filepath.Join(dir, "heap.pprof"), "heap", 0); err != nil {
		return s, err
	}
	return s, WriteJSON(filepath.Join(dir, "sample.json"), s)
}

// Main adds opt-in captures around an existing test program, preserving its
// original failure status. No GC/profiling is added to ordinary fuzz mutations.
func Main(run func() int) int {
	dir := os.Getenv("BARN_LEAK_OUTPUT")
	if dir == "" {
		return run()
	}
	// A diagnostic test can re-exec this binary with the same output root.
	// Preserve each process's evidence instead of overwriting its parent's.
	dir = filepath.Join(dir, fmt.Sprintf("process-%d", os.Getpid()))
	before, firstErr := Capture(filepath.Join(dir, "before"), "test-process-start", nil)
	code := run()
	after, lastErr := Capture(filepath.Join(dir, "after"), "test-process-finished", nil)
	if firstErr != nil || lastErr != nil || before.Leaked != 0 || after.Leaked != 0 {
		fmt.Fprintf(os.Stderr, "memory check: before_leaked=%d after_leaked=%d capture_errors=%v,%v evidence=%s\n",
			before.Leaked, after.Leaked, firstErr, lastErr, dir)
		if code == 0 {
			code = 1
		}
	}
	return code
}
