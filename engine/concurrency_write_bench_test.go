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

// TestConcurrencyWriteScalingSweep is the WRITE-HEAVY companion to
// TestConcurrencyScalingSweep. The read-only arith bench never exercises the
// store COMMIT path (read-only tasks skip commit), so the single global commit
// lock (the exclusive store.mu.Lock in StoreTxn.Commit) is invisible to it.
//
// Each task here does a small CPU loop and then STAGES A PROPERTY WRITE and
// COMMITS, in two variants:
//
//	disjoint:  each task writes a property on ITS OWN distinct object (#N.prop).
//	           There is no real data conflict between tasks, so an ideal MVCC
//	           store could commit them fully in parallel. This is the decisive
//	           test: if disjoint-write tasks do NOT scale with worker count, the
//	           global commit lock is proven to serialize independent commits.
//	contended: every task writes the SAME object's property (#shared.prop),
//	           which forces conflict/retry at commit and is an upper bound on
//	           contention cost.
//
// Run with -mutexprofile / -blockprofile (and SetMutexProfileFraction below) to
// confirm whether store.mu is the dominant contended lock.
func TestConcurrencyWriteScalingSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("write scaling sweep skipped in -short")
	}
	cores := runtime.GOMAXPROCS(0)
	const loop = 20000 // small CPU body so commit cost dominates, not arithmetic
	n := cores

	for _, contended := range []bool{false, true} {
		label := "disjoint"
		if contended {
			label = "contended"
		}
		base := measureWriteWorkers(t, n, loop, contended, false, 1) // serial reference
		t.Logf("[%s] serial (1 task at a time) baseline for %d tasks: %s", label, n, base.Round(time.Microsecond))
		for w := 1; w <= cores; w *= 2 {
			d := measureWriteWorkers(t, n, loop, contended, true, w)
			t.Logf("[%s] workers=%-3d pool=%10s speedup=%.2fx", label, w, d.Round(time.Microsecond), float64(base)/float64(d))
		}
	}
}

// TestConcurrencyCommitDominatedDisjoint isolates the COMMIT path from the
// arithmetic/allocation tax that the read-only bench is bound by. With loop=0
// the task body is essentially just "stage one property write + Commit", so if
// the disjoint pool fails to beat the serial baseline the per-commit serial work
// (the exclusive store.mu critical section in StoreTxn.Commit, plus validation)
// is the thing that does not parallelize.
func TestConcurrencyCommitDominatedDisjoint(t *testing.T) {
	if testing.Short() {
		t.Skip("commit-dominated disjoint test skipped in -short")
	}
	cores := runtime.GOMAXPROCS(0)
	const loop = 0 // no arithmetic: commit dominates
	n := 2000      // many tiny disjoint commits to amortize dispatch overhead

	base := measureWriteWorkers(t, n, loop, false, false, 1)
	t.Logf("[commit-only disjoint] serial baseline for %d tasks: %s (%.2f us/commit)",
		n, base.Round(time.Microsecond), float64(base.Microseconds())/float64(n))
	for w := 1; w <= cores; w *= 2 {
		d := measureWriteWorkers(t, n, loop, false, true, w)
		t.Logf("[commit-only disjoint] workers=%-3d pool=%10s speedup=%.2fx (%.2f us/commit)",
			w, d.Round(time.Microsecond), float64(base)/float64(d), float64(d.Microseconds())/float64(n))
	}
}

// measureWriteWorkers builds a fresh store + scheduler + N property-writing
// tasks and times running them either serially (inline runTask) or through the
// worker pool with `workers` workers. Each task body does a short arithmetic
// loop and then assigns a property, so the task has staged writes and hits
// StoreTxn.Commit.
//
// It is now a thin wrapper over measureWriteWorkersDistinct: the disjoint regime
// (contended==false) maps to distinct==n (each task its own object); the fully
// contended regime (contended==true) maps to distinct==1 (all tasks share one
// object). The signature is preserved byte-for-byte so existing callers
// (TestConcurrencyCommitDominatedDisjoint, TestConcurrencyWriteScalingSweep)
// compile and behave unchanged.
func measureWriteWorkers(t *testing.T, n, loop int, contended, pool bool, workers int) time.Duration {
	t.Helper()
	distinct := n
	if contended {
		distinct = 1
	}
	return measureWriteWorkersDistinct(t, n, distinct, loop, pool, workers)
}

// measureWriteWorkersDistinct generalizes measureWriteWorkers to K-way
// contention via a STATIC property write (#target.counter = v). distinct>=n is
// fully disjoint (each task its own object); distinct==1 is all-share-one;
// distinct==K is K-way. A static property target gives the scheduler a KNOWN
// access footprint, so non-commuting same-object tasks are placed in SEPARATE
// batches (the conflict-free fast path, scheduler.candidateCanJoinBatch) and never
// actually race — exactly the behavior the existing write benches
// (TestConcurrencyCommitDominatedDisjoint, TestConcurrencyWriteScalingSweep) have
// always measured. Returns the elapsed run time only.
func measureWriteWorkersDistinct(t *testing.T, n, distinct, loop int, pool bool, workers int) time.Duration {
	t.Helper()
	elapsed, _ := runWriteWorkers(t, n, distinct, loop, pool, workers, false /*dynamicProp*/)
	return elapsed
}

// measureWriteWorkersDistinctCounted is the contention-matrix measurement: like
// measureWriteWorkersDistinct but using a DYNAMIC property target (#target.(pn)),
// which the footprint analyzer cannot resolve and so marks UNKNOWN. Unknown
// footprints make the scheduler co-schedule conflict-retryable tasks
// OPTIMISTICALLY in one batch (scheduler.candidateCanJoinBatch), so same-object
// tasks really run concurrently and a losing committer's read-set validation fails
// — producing the MVCC conflicts/retries this matrix exists to observe. It samples
// the store's commit counters immediately before and after the timed run and
// returns both the elapsed time and the counter deltas. A fresh store per call
// makes each delta the exact commit activity of that run (no reset method, no
// reset races).
func measureWriteWorkersDistinctCounted(t *testing.T, n, distinct, loop int, pool bool, workers int) (time.Duration, commitCounterDelta) {
	t.Helper()
	return runWriteWorkers(t, n, distinct, loop, pool, workers, true /*dynamicProp*/)
}

// runWriteWorkers builds a fresh store + scheduler + n property-writing tasks
// (each targeting ids[k%distinct]) and times running them serially or through the
// pool, sampling the MVCC commit counters around the timed run. When dynamicProp
// is false the property is addressed statically (#t.counter = v) — a known
// footprint, conflict-free fast-path batching, the existing-bench behavior. When
// true it is addressed dynamically: the value is READ EARLY into c (recording the
// read-set version), a CPU loop widens the read->commit window, then the value is
// written back through a DYNAMIC property name (#t.(pn)). That combination —
// recorded read + unknown footprint — is what makes contended commits actually
// conflict and retry; with loop==0 the window collapses and co-scheduled tasks may
// serialize before conflicting, so conflict-seeking callers pass loop>0.
func runWriteWorkers(t *testing.T, n, distinct, loop int, pool bool, workers int, dynamicProp bool) (time.Duration, commitCounterDelta) {
	t.Helper()
	if distinct < 1 {
		distinct = 1
	}
	store, ids, _ := buildWriteStore(t, n)
	s := newRuntimeWithWorkerCount(store, config.Options{}, workers)
	defer s.Stop()
	defer removeTasksForOwner(s, 3)

	tasks := make([]*task.Task, n)
	for k := 0; k < n; k++ {
		target := ids[k%distinct]
		var code string
		if dynamicProp {
			code = fmt.Sprintf(
				"pn = \"counter\"; c = #%d.(pn); x = 0; for i in [1..%d]; x = x + i; endfor; #%d.(pn) = c + %d; return x;",
				target, loop, target, 1+k)
		} else {
			code = fmt.Sprintf(
				"x = 0; for i in [1..%d]; x = x + i; endfor; #%d.counter = %d; return x;",
				loop, target, 1000+k)
		}
		tasks[k] = s.buildBenchTask(t, int64(7000+k), code)
	}

	before := sampleCommitCounters(store)
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
	delta := sampleCommitCounters(store).sub(before)

	for _, tk := range tasks {
		if tk.Result.Flow != types.FlowReturn {
			t.Fatalf("task %d flow = %v err=%v, want return", tk.ID, tk.Result.Flow, tk.Result.Error)
		}
	}
	return elapsed, delta
}

// buildWriteStore creates the wizard, n disjoint objects each carrying a
// writable "counter" property, plus one extra shared object (also carrying
// "counter") used by the contended variant. Returns the n disjoint ids and the
// shared id.
func buildWriteStore(t *testing.T, n int) (*dbstore.Store, []types.ObjID, types.ObjID) {
	t.Helper()
	store := dbstore.NewStore()

	wiz := dbstore.NewObjectBuilder(3)
	wiz.SetOwner(3)
	wiz.SetFlags(dbstore.FlagWizard | dbstore.FlagProgrammer | dbstore.FlagUser)
	if err := store.Add(wiz.Build()); err != nil {
		t.Fatalf("Add wizard failed: %v", err)
	}

	defineCounter := func(id types.ObjID) {
		prop := dbstore.NewProperty(types.NewInt(0), 3, dbstore.PropRead|dbstore.PropWrite, false, true)
		if errCode := store.DirectTxn().DefineProperty(id, "counter", prop); errCode != types.E_NONE {
			t.Fatalf("DefineProperty(counter) on #%d failed: %v", id, errCode)
		}
	}

	ids := make([]types.ObjID, n)
	for k := 0; k < n; k++ {
		id, errCode := store.DirectTxn().CreateObject(nil, 3, false)
		if errCode != types.E_NONE {
			t.Fatalf("CreateObject failed: %v", errCode)
		}
		defineCounter(id)
		ids[k] = id
	}

	shared, errCode := store.DirectTxn().CreateObject(nil, 3, false)
	if errCode != types.E_NONE {
		t.Fatalf("CreateObject (shared) failed: %v", errCode)
	}
	defineCounter(shared)

	return store, ids, shared
}
