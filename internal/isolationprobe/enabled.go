//go:build elle

package isolationprobe

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const Enabled = true

// Faults are available only in the dedicated checker build, never a normal
// server binary. Each one removes a single protection, named where it is
// consulted; the generated manifest records which was active.
var fault = os.Getenv("BARN_ELLE_FAULT")

// Fault reports whether the named deliberate fault is active.
func Fault(name string) bool {
	return fault == name && (name != "commit-exclusion" || commitExclusionPair.Load())
}

var commitExclusionPair atomic.Bool

// SetCommitExclusionPair limits the selected commit-exclusion fault to the
// held write-skew pair. Other waves must retain publication serialization.
func SetCommitExclusionPair(active bool) { commitExclusionPair.Store(active) }

// ActiveFault is the fault this process runs under, or "".
func ActiveFault() string { return fault }

var pause atomic.Pointer[func(string, []string)]

// SetPause installs fn to run at every Pause point, or removes it when nil.
func SetPause(fn func(point string, artifacts []string)) {
	if fn == nil {
		pause.Store(nil)
		return
	}
	pause.Store(&fn)
}

// Pause lets a workload hold the calling goroutine at a named point inside the
// store or checkpoint path, so an interleaving that is otherwise microseconds
// wide happens every run. artifacts names the files that point concerns.
func Pause(point string, artifacts ...string) {
	if fn := pause.Load(); fn != nil {
		(*fn)(point, artifacts)
	}
}

// InstallCheckpointStop is called once at command startup. With
// BARN_ELLE_CHECKPOINT_STOP=<checkpoint point>:<occurrence> it installs a pause
// callback that, on that occurrence of that point, writes one boundary record
// line to record and then stops its own process with SIGSTOP. The parent that
// owns the process resumes or kills it; there is no control channel.
func InstallCheckpointStop(record io.Writer) error {
	spec := os.Getenv("BARN_ELLE_CHECKPOINT_STOP")
	if spec == "" {
		return nil
	}
	if !canStopSelf {
		return fmt.Errorf("BARN_ELLE_CHECKPOINT_STOP needs SIGSTOP, which this platform lacks")
	}
	fn, err := checkpointStop(spec, record, stopSelf)
	if err != nil {
		return err
	}
	SetPause(fn)
	return nil
}

func checkpointStop(spec string, record io.Writer, stop func() error) (func(string, []string), error) {
	point, count, found := strings.Cut(spec, ":")
	occurrence, err := strconv.ParseUint(count, 10, 64)
	if !found || err != nil || occurrence == 0 || !slices.Contains(CheckpointPoints, point) {
		return nil, fmt.Errorf("invalid BARN_ELLE_CHECKPOINT_STOP %q: want <checkpoint point>:<occurrence from 1>, points %v", spec, CheckpointPoints)
	}
	var reached atomic.Uint64
	return func(at string, artifacts []string) {
		if at != point || reached.Add(1) != occurrence {
			return
		}
		line, err := json.Marshal(struct {
			Msg        string   `json:"msg"`
			Point      string   `json:"point"`
			Occurrence uint64   `json:"occurrence"`
			PID        int      `json:"pid"`
			Artifacts  []string `json:"artifacts"`
		}{"checkpoint stop", point, occurrence, os.Getpid(), artifacts})
		if err == nil {
			// One unbuffered write: the record is out of the process before it stops.
			_, err = record.Write(append(line, '\n'))
		}
		if err == nil {
			err = stop()
		}
		if err != nil {
			panic(fmt.Sprintf("checkpoint stop at %s occurrence %d: %v", point, occurrence, err))
		}
	}, nil
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
