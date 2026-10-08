package engine

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/types"
)

// Opt-in, identical baseline/candidate measurement. No pass condition assumes
// the candidate policy: acknowledgements and workload identity are mandatory.
func TestDeferredGCConcurrentMeasurement(t *testing.T) {
	if os.Getenv("BARN_GC_CONCURRENT_MEASURE") != "1" {
		t.Skip("set BARN_GC_CONCURRENT_MEASURE=1 for concurrent latency measurements")
	}
	for _, iterations := range []int{1000, 100000} {
		t.Run(fmt.Sprint(iterations), func(t *testing.T) {
			store := newConflictTestStore(t)
			for id := types.ObjID(1); id <= 2; id++ {
				player := dbstore.NewObjectBuilder(id)
				player.SetOwner(id)
				player.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
				if err := store.Add(player.Build()); err != nil {
					t.Fatal(err)
				}
			}
			opts := config.DefaultOptions()
			opts.AdmissionLimit = 8
			var entered builtins.BuiltinFunc
			rt := newTestRuntimeWithWorkersAndBuiltins(t, store, opts, 4,
				testBuiltinSlot("gc_measure_entered", 0, 0, nil, &entered))
			defer rt.Stop()
			rt.HoldFinalizationUntilStarted()
			ready := make(chan struct{}, 2)
			var seen [3]atomic.Bool
			entered = func(ctx *builtins.Execution, _ []types.Value) types.Result {
				if !seen[ctx.Player].Swap(true) {
					ready <- struct{}{}
				}
				return types.Ok(types.NewInt(0))
			}
			program := compileTestProgram(t, rt.registry, fmt.Sprintf(
				"gc_measure_entered(); create(#0, 1); n = 0; for i in [1..%d] n = n + i; endfor; return n;", iterations))
			want := int64(iterations) * int64(iterations+1) / 2
			stop := make(chan struct{})
			var stopOnce sync.Once
			stopWorkers := func() { stopOnce.Do(func() { close(stop) }) }
			var workers sync.WaitGroup
			var completed atomic.Int64
			var failed atomic.Int64
			for owner := types.ObjID(1); owner <= 2; owner++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					for {
						select {
						case <-stop:
							return
						default:
						}
						id := rt.CreateBackgroundTask(owner, program, 0)
						task := rt.GetTask(id)
						task.TicksLimit = 10_000_000
						task.SecondsLimit = 30
						err := rt.runTask(task)
						if err != nil || task.Result.IsError() || task.Result.Val.Type() != types.TYPE_INT || task.Result.Val.Int() != want {
							failed.Add(1)
							t.Logf("background failure: err=%v result=%+v", err, task.Result)
							stopWorkers()
						}
						completed.Add(1)
						rt.taskManager.RemoveTask(id)
					}
				}()
			}
			defer func() { stopWorkers(); workers.Wait() }()
			for range 2 {
				select {
				case <-ready:
				case <-time.After(3 * time.Second):
					t.Fatal("background workload failed to start")
				}
			}
			start := time.Now()
			rt.lifecycle.Mu.Lock()
			rt.lifecycle.LastGCCost = cheapGCSweep
			rt.lifecycle.LastGCSweep = start
			rt.lifecycle.Mu.Unlock()
			rt.ReleaseStartupFinalization()
			const samples = 1200
			command, lateness, admissionWait := make([]int64, samples), make([]int64, samples), make([]int64, samples)
			var probes sync.WaitGroup
			probes.Add(2)
			go func() {
				defer probes.Done()
				for i := range samples {
					due := start.Add(time.Duration(i) * 5 * time.Millisecond)
					time.Sleep(time.Until(due))
					request := time.Now()
					if got := rt.EvalCommandOutput(0, "return 1;"); got != "{1, 1}" {
						failed.Add(1)
						t.Logf("foreground failure: %s", got)
					}
					end := time.Now()
					command[i], lateness[i] = end.Sub(request).Nanoseconds(), end.Sub(due).Nanoseconds()
				}
			}()
			go func() {
				defer probes.Done()
				for i := range samples {
					time.Sleep(time.Until(start.Add(time.Duration(i) * 5 * time.Millisecond)))
					request := time.Now()
					scope, err := rt.enterInput(3, false)
					admissionWait[i] = time.Since(request).Nanoseconds()
					if err != nil {
						failed.Add(1)
						return
					}
					scope.Finish()
				}
			}()
			probes.Wait()
			elapsed := time.Since(start)
			count := completed.Load()
			anonymous := len(store.AnonymousRecycleCandidates(store.PersistentAnonymousReachability(), 0))
			maintenance := rt.AdmissionStats().Maintenance
			stopWorkers()
			workers.Wait()
			if failed.Load() != 0 {
				t.Fatalf("%d workload acknowledgements failed", failed.Load())
			}
			t.Logf("gc_concurrent iterations=%d samples=%d completed=%d elapsed_ms=%d anonymous_snapshot=%d sweep_service_ms=%.3f",
				iterations, samples, count, elapsed.Milliseconds(), anonymous, float64(maintenance)/float64(time.Millisecond))
			for _, series := range []struct {
				name   string
				values []int64
			}{
				{"command", command}, {"lateness", lateness}, {"admission", admissionWait},
			} {
				sort.Slice(series.values, func(i, j int) bool { return series.values[i] < series.values[j] })
				t.Logf("gc_concurrent metric=%s p99_ms=%.3f p999_ms=%.3f max_ms=%.3f", series.name,
					float64(series.values[(samples-1)*99/100])/1e6,
					float64(series.values[(samples-1)*999/1000])/1e6,
					float64(series.values[samples-1])/1e6)
			}
		})
	}
}
