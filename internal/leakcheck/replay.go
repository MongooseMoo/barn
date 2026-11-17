package leakcheck

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"
)

// ReplayResult deliberately separates a heap candidate from a native proven
// goroutine leak or an exact descriptor lifetime failure.
type ReplayResult struct {
	Verdict string   `json:"verdict"`
	Samples []Sample `json:"samples"`
	Reasons []string `json:"reasons"`
}

// RetentionCandidate requires material growth in every post-warmup interval.
// It is a bounded discovery heuristic, not proof that a growing heap is leaked.
// The caller must establish equal logical state and confirm in a fresh process.
func RetentionCandidate(samples []Sample) bool {
	if len(samples) < 5 {
		return false
	}
	for i := 1; i < len(samples); i++ {
		if samples[i].Heap.Live <= samples[i-1].Heap.Live+32*1024 {
			return false
		}
	}
	return samples[len(samples)-1].Heap.Live > samples[0].Heap.Live+1024*1024
}

// Replay runs one bounded lifecycle sequence in a single process. The callback
// returns only after completing normal cleanup, with no deliberately surviving
// descriptors or growing world. Use live-world workload samples as evidence,
// not as input to this equal-state retention predicate.
func Replay(dir string, cycle func(), owners func() any) (ReplayResult, error) {
	r := ReplayResult{Verdict: "incomplete"}
	readOwners := func() any {
		if owners == nil {
			return nil
		}
		return owners()
	}
	for range 2 {
		cycle()
	}
	first, err := Capture(filepath.Join(dir, "baseline"), "warmed-cleanup", readOwners())
	if err != nil {
		return r, err
	}
	r.Samples = append(r.Samples, first)
	for batch := range 6 {
		for range 16 {
			cycle()
		}
		runtime.GC()
		runtime.GC()
		s := Sample{PID: first.PID, GoVersion: first.GoVersion, Boundary: fmt.Sprintf("cleanup-%d", batch+1),
			Time: time.Now().UTC(), Heap: ReadHeap(), Process: ReadProcess(), Owners: readOwners(), Settings: readSettings()}
		r.Samples = append(r.Samples, s)
		if err := WriteJSON(filepath.Join(dir, "replay.json"), r); err != nil {
			return r, err
		}
	}
	last, err := Capture(filepath.Join(dir, "final"), "repeated-cleanup", readOwners())
	if err != nil {
		return r, err
	}
	r.Verdict = "healthy"
	if first.Leaked != 0 || last.Leaked != 0 {
		r.Verdict = "leak"
		r.Reasons = append(r.Reasons, "native goroutine leak profile is nonempty")
	}
	// Require growth across all windows to exclude transient close/reaper work.
	fdGrowth := first.Process.Available
	for _, s := range r.Samples[1:] {
		fdGrowth = fdGrowth && s.Process.Available && s.Process.FDs > first.Process.FDs
	}
	if fdGrowth && last.Process.Available && last.Process.FDs > first.Process.FDs {
		r.Verdict = "leak"
		r.Reasons = append(r.Reasons, "owned descriptors remain open after every cleanup window")
	}
	if RetentionCandidate(r.Samples) {
		if r.Verdict == "healthy" {
			r.Verdict = "heap-candidate"
		}
		r.Reasons = append(r.Reasons, "retained heap grows materially across all cleanup windows")
	}
	if runtime.GOOS == "linux" {
		for _, sample := range append(r.Samples, last) {
			if !sample.Process.Available {
				r.Verdict = "incomplete"
				r.Reasons = append(r.Reasons, "Linux process-resource readings are incomplete")
				break
			}
		}
	}
	return r, WriteJSON(filepath.Join(dir, "replay.json"), r)
}
