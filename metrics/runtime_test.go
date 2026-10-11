package metrics

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"
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
