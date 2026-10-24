package metrics

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/types"
)

func TestDetailedRuntimeLogsRequireOptInAndContainOnlyMetadata(t *testing.T) {
	prior := detailedRuntime.Swap(false)
	priorLogger := slog.Default()
	t.Cleanup(func() { detailedRuntime.Store(prior); slog.SetDefault(priorLogger) })
	var out bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	id := Identity{TaskID: 7, This: 2, Verb: "say"}
	ObserveRuntime(VMAttempt, id, time.Millisecond, 3)
	if out.Len() != 0 {
		t.Fatal("ordinary debug logging enabled detailed runtime records")
	}
	detailedRuntime.Store(true)
	ObserveRuntime(VMAttempt, id, time.Millisecond, 3)
	var record map[string]any
	if err := json.Unmarshal(out.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"time": true, "level": true, "msg": true, "phase": true,
		"task_id": true, "this": true, "verb": true, "elapsed_ns": true, "ticks": true}
	for key := range record {
		if !allowed[key] {
			t.Fatalf("unexpected runtime field %s", key)
		}
	}
	if record["phase"] != "vm_attempt" || record["elapsed_ns"] != float64(time.Millisecond) || record["ticks"] != float64(3) {
		t.Fatalf("wrong observation: %v", record)
	}
}

func TestPhaseCounterConcurrentAccounting(t *testing.T) {
	var counter phaseCounter
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				counter.observe(time.Millisecond, 7)
			}
		})
	}
	wg.Wait()
	counter.observe(-1, -1)
	got := counter.snapshot()
	if got.Count != 801 || got.TotalNS != int64(800*time.Millisecond) || got.Ticks != 5600 || got.MaxNS != int64(time.Millisecond) {
		t.Fatalf("lost observations: %+v", got)
	}
	var observations int64
	for _, n := range got.Buckets {
		observations += n
	}
	if observations != got.Count || got.Buckets[0] != 1 || got.Buckets[3] != 800 {
		t.Fatalf("bucket accounting: %+v", got)
	}
}

func BenchmarkRuntimeObservation(b *testing.B) {
	id := Identity{TaskID: 1, This: 2, Verb: "say"}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ObserveRuntime(VMAttempt, id, time.Millisecond, 100)
		}
	})
}

func TestTerminalExceptionOptInAndFixedMetadata(t *testing.T) {
	prior := detailedRuntime.Swap(false)
	priorLogger := slog.Default()
	t.Cleanup(func() { detailedRuntime.Store(prior); slog.SetDefault(priorLogger) })
	var out bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	id := Identity{TaskID: 71, This: 42, Verb: "weave"}
	TerminalException(id, types.E_MAXREC, types.ExceptionReasonSecondsBudget, false, true)
	if out.Len() != 0 {
		t.Fatal("terminal exception logged without opt-in")
	}
	detailedRuntime.Store(true)
	TerminalException(id, types.E_MAXREC, types.ExceptionReasonSecondsBudget, false, true)
	var rec map[string]any
	if err := json.Unmarshal(out.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"time": true, "level": true, "msg": true, "task_id": true, "this": true, "verb": true, "error": true, "reason": true, "intrinsic_eval": true, "committed": true}
	if len(rec) != len(allowed) {
		t.Fatalf("unexpected fields: %v", rec)
	}
	for key := range rec {
		if !allowed[key] {
			t.Fatalf("unexpected field %q", key)
		}
	}
	if rec["task_id"] != float64(71) || rec["error"] != "E_MAXREC" || rec["reason"] != "seconds_budget" || rec["intrinsic_eval"] != false || rec["committed"] != true {
		t.Fatalf("wrong metadata: %v", rec)
	}
	out.Reset()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, nil)))
	TerminalException(id, types.E_MAXREC, types.ExceptionReasonSecondsBudget, false, true)
	if out.Len() != 0 {
		t.Fatal("terminal exception logged without debug level")
	}
}
