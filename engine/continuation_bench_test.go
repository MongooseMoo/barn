package engine

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/MongooseMoo/barn/config"
	dbstore "github.com/MongooseMoo/barn/db/store"
	"github.com/MongooseMoo/barn/task"
	"github.com/MongooseMoo/barn/types"
)

// Measure complete cohorts, including four threaded calls and the computation
// after each call. The same source is compiled into the control executable.
func BenchmarkThreadedContinuations(b *testing.B) {
	for _, players := range []int{1, 16} {
		for _, writes := range []bool{false, true} {
			b.Run(fmt.Sprintf("players=%d/writes=%v", players, writes), func(b *testing.B) {
				store := dbstore.NewStore()
				root := dbstore.NewObjectBuilder(0)
				root.SetOwner(0)
				root.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer)
				root.SetProperty("v", dbstore.NewProperty(types.NewInt(0), 0, dbstore.PropRead|dbstore.PropWrite, false, true))
				if err := store.Add(root.Build()); err != nil {
					b.Fatal(err)
				}
				rt := newRuntimeWithWorkerCount(store, config.Options{}, 4)
				defer rt.Stop()
				write := ""
				if writes {
					write = "#0.v = #0.v + 1;"
				}
				program, diagnostics := rt.registry.Compiler().CompileMOO([]string{`sum = 0; for round in [1..4] sorted = sort({2, 1}); for i in [1..500] sum = sum + i; endfor ` + write + ` endfor return sum;`})
				if len(diagnostics) != 0 {
					b.Fatal(diagnostics)
				}
				stop, stopped := make(chan struct{}), make(chan struct{})
				go func() {
					defer close(stopped)
					for {
						select {
						case <-stop:
							return
						default:
						}
						if rt.ProcessReadyTasks() == 0 {
							runtime.Gosched()
						}
					}
				}()
				defer func() { close(stop); <-stopped }()
				b.ReportAllocs()
				for b.Loop() {
					cohort := make([]*task.Task, players)
					for i := range cohort {
						ticks, seconds := foregroundTaskLimits(rt.session)
						queued := task.NewTaskFull(rt.newTaskID(), types.ObjID(i+1), program, ticks, seconds)
						queued.Context.IsWizard = true
						queued.Done = make(chan struct{})
						cohort[i] = queued
						rt.QueueTask(queued)
					}
					for _, queued := range cohort {
						select {
						case <-queued.Done:
						case <-time.After(10 * time.Second):
							b.Fatal("cohort did not drain")
						}
						if queued.Result.Flow != types.FlowReturn || queued.Result.Val.Int() != 501000 {
							b.Fatalf("terminal result=%+v", queued.Result)
						}
					}
					rt.CleanupFinishedTasks()
				}
				b.ReportMetric(float64(b.N*players)/b.Elapsed().Seconds(), "commands/s")
				b.ReportMetric(float64(store.CommitRetries())/float64(b.N*players), "retries/command")
				b.ReportMetric(float64(store.CommitEscalations())/float64(b.N*players), "escalations/command")
			})
		}
	}
}
