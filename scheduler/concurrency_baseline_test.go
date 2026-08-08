package scheduler

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

// TestConcurrencyBaseline measures, empirically, how much concurrency the MVCC
// branch actually delivers today. It runs N independent CPU-bound tasks that
// touch DISJOINT objects (no real data conflict) two ways:
//
//	serial: s.runTask(t) in a loop on one goroutine. This is exactly what the
//	        interactive command path does today (ExecuteVerbTaskSync runs runTask
//	        inline on the single input goroutine), so it is the live reality for
//	        every player command.
//	pool:   QueueTask all N, then ProcessReadyTasks once, dispatching to the
//	        GOMAXPROCS worker pool with the commute-batching machinery active.
//
// Two task shapes:
//
//	arith:    pure VM arithmetic loop (no verb/builtin/property) -> the footprint
//	          analyzer proves it commutes, so the pool is ALLOWED to parallelize.
//	          This is the ceiling: what the engine can do when the gate opens.
//	verbcall: each task calls a verb on its own disjoint object (return #N:work()).
//	          This is the realistic shape of every player command. A verb call
//	          poisons the footprint to "unknown", forcing singleton batches.
//
// The point of the benchmark: arith/pool should beat arith/serial (engine works),
// while verbcall/pool should NOT beat verbcall/serial (the commute gate erases the
// win for realistic work). That gap is what "make this allow more concurrency"
// has to close.
func TestConcurrencyBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("baseline timing test skipped in -short")
	}

	cores := runtime.GOMAXPROCS(0)
	n := cores
	if n < 2 {
		t.Skipf("need >=2 GOMAXPROCS to show concurrency, have %d", cores)
	}
	const loop = 600000 // arithmetic iterations per task; ~a few ms of CPU each

	t.Logf("GOMAXPROCS=%d, tasks=%d, loop=%d per task", cores, n, loop)

	arithSerial := measureConcurrency(t, n, loop, false /*useVerb*/, false /*pool*/)
	arithPool := measureConcurrency(t, n, loop, false, true)
	verbSerial := measureConcurrency(t, n, loop, true, false)
	verbPool := measureConcurrency(t, n, loop, true, true)

	report := func(name string, serial, pool time.Duration) {
		speedup := float64(serial) / float64(pool)
		t.Logf("%-9s serial=%8s  pool=%8s  speedup=%.2fx", name, serial.Round(time.Microsecond), pool.Round(time.Microsecond), speedup)
	}
	t.Log("--- concurrency baseline (higher pool speedup = more real concurrency) ---")
	report("arith", arithSerial, arithPool)
	report("verbcall", verbSerial, verbPool)
	t.Logf("ceiling (arith pool speedup) vs realistic (verbcall pool speedup): "+
		"%.2fx vs %.2fx", float64(arithSerial)/float64(arithPool), float64(verbSerial)/float64(verbPool))
}

// TestConcurrencyScalingSweep measures how arith pool throughput scales as worker
// count climbs 1->GOMAXPROCS, exposing where the speedup curve plateaus. Run under
// `GOGC=off go test ...` to test whether allocation/GC pressure is the ceiling.
func TestConcurrencyScalingSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling sweep skipped in -short")
	}
	cores := runtime.GOMAXPROCS(0)
	const loop = 600000
	n := cores
	base := measureConcurrencyWorkers(t, n, loop, false, false, 1) // serial reference
	t.Logf("serial (1 task at a time) baseline for %d tasks: %s", n, base.Round(time.Microsecond))
	for w := 1; w <= cores; w *= 2 {
		d := measureConcurrencyWorkers(t, n, loop, false, true, w)
		t.Logf("workers=%-3d pool=%9s speedup=%.2fx", w, d.Round(time.Microsecond), float64(base)/float64(d))
	}
}

// measureConcurrency builds a fresh store + scheduler + N tasks and times running
// them either serially (inline runTask) or through the worker pool.
func measureConcurrency(t *testing.T, n, loop int, useVerb, pool bool) time.Duration {
	return measureConcurrencyWorkers(t, n, loop, useVerb, pool, runtime.GOMAXPROCS(0))
}

func measureConcurrencyWorkers(t *testing.T, n, loop int, useVerb, pool bool, workers int) time.Duration {
	t.Helper()
	store, ids := buildConcurrencyStore(t, n, loop)
	s := newSchedulerWithWorkerCount(store, config.Options{}, workers)
	defer s.Stop()
	defer removeTasksForOwner(s, 3)

	// Build all tasks (including parse) BEFORE timing so only execution is timed.
	tasks := make([]*task.Task, n)
	for k := 0; k < n; k++ {
		var code string
		if useVerb {
			code = fmt.Sprintf("return #%d:work();", ids[k])
		} else {
			code = fmt.Sprintf("x = 0; for i in [1..%d]; x = x + i; endfor; return x;", loop)
		}
		tasks[k] = s.buildBenchTask(t, int64(5000+k), code)
	}

	start := time.Now()
	if pool {
		for _, tk := range tasks {
			s.QueueTask(tk)
		}
		ran := s.ProcessReadyTasks()
		if ran != n {
			t.Fatalf("ProcessReadyTasks ran %d tasks, want %d", ran, n)
		}
	} else {
		for _, tk := range tasks {
			if err := s.runTask(tk); err != nil {
				t.Fatalf("runTask failed: %v", err)
			}
		}
	}
	elapsed := time.Since(start)

	for _, tk := range tasks {
		if tk.Result.Flow != types.FlowReturn {
			t.Fatalf("task %d flow = %v err=%v, want return (tick limit?)", tk.ID, tk.Result.Flow, tk.Result.Error)
		}
	}
	return elapsed
}

func (s *Scheduler) buildBenchTask(t *testing.T, id int64, code string) *task.Task {
	t.Helper()
	stmts := compileTestProgram(t, s.registry, code)
	// Generous tick/second budget so CPU loops never hit foreground limits.
	tk := task.NewTaskFull(id, types.ObjID(3), stmts, 1<<50, 1e9)
	s.populateTaskContextDependencies(tk.Context)
	tk.Context.IsWizard = true
	tk.Programmer = 3
	tk.Context.Programmer = 3
	tk.StartTime = time.Now().Add(-time.Second)
	tk.Done = make(chan struct{})
	tk.ForkCreator = s
	return tk
}

func buildConcurrencyStore(t *testing.T, n, loop int) (*dbstore.Store, []types.ObjID) {
	t.Helper()
	store := dbstore.NewStore()

	wiz := dbstore.NewObjectBuilder(3)
	wiz.SetOwner(3)
	wiz.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wiz.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}

	verbCode := []string{fmt.Sprintf("x = 0; for i in [1..%d]; x = x + i; endfor; return x;", loop)}
	ids := make([]types.ObjID, n)
	for k := 0; k < n; k++ {
		id, errCode := store.CreateObject(nil, 3, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject failed: %v", errCode)
		}
		verb := dbstore.NewVerb(
			"work",
			[]string{"work"},
			3,
			dbstore.VerbRead|dbstore.VerbExecute|dbstore.VerbDebug,
			dbstore.VerbArgs{This: "this", Prep: "none", That: "none"},
			verbCode,
		)
		if _, errCode := store.AddVerb(id, verb); errCode != types.E_NONE {
			t.Fatalf("AddVerb failed: %v", errCode)
		}
		ids[k] = id
	}
	return store, ids
}
