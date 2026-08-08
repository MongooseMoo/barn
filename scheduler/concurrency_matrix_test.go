package scheduler

import (
	"runtime"
	"testing"
	"time"

	dbstore "github.com/MongooseMoo/barn/db/store"
)

// commitCounterDelta is a snapshot of the four MVCC commit observability counters
// on a Store (Phase A instrumentation). It is used to report the commit activity
// of a single benchmark run as a delta (after - before), which avoids any need
// for a counter reset method (and the reset races that would come with one).
type commitCounterDelta struct {
	attempts  uint64
	successes uint64
	conflicts uint64
	retries   uint64
}

// sampleCommitCounters reads all four commit counters off the store atomically.
func sampleCommitCounters(s *dbstore.Store) commitCounterDelta {
	return commitCounterDelta{
		attempts:  s.CommitAttempts(),
		successes: s.CommitSuccesses(),
		conflicts: s.CommitConflicts(),
		retries:   s.CommitRetries(),
	}
}

// sub returns the element-wise difference a-b (after minus before).
func (a commitCounterDelta) sub(b commitCounterDelta) commitCounterDelta {
	return commitCounterDelta{
		attempts:  a.attempts - b.attempts,
		successes: a.successes - b.successes,
		conflicts: a.conflicts - b.conflicts,
		retries:   a.retries - b.retries,
	}
}

// TestConcurrencyContentionMatrix is the Phase A contention-matrix benchmark. For
// a fixed task count it sweeps worker counts across three workloads and prints,
// per row, the serial vs pool time, the speedup, and the MVCC commit counter
// deltas (attempts / successes / conflicts / retries) observed over the pool run.
//
//	read-only:       arithmetic-only tasks, no commits (read ceiling; commit
//	                 counters stay ~0, itself a useful signal).
//	write-disjoint:  each task writes its OWN object (distinct == n) — no real
//	                 data conflict, the ideal-parallel case.
//	write-contendK:  the n tasks share K objects (distinct == K) — partial
//	                 contention.
//	write-contend1:  all tasks share ONE object (distinct == 1) — maximal
//	                 contention; this row MUST show nonzero conflicts and retries,
//	                 which proves the instrument actually observes aborts/retries.
//
// This is a measurement harness; the t.Logf table (run with -v) is the
// deliverable. The change under test is observation-only — it does not alter any
// control flow.
func TestConcurrencyContentionMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("contention matrix skipped in -short")
	}

	const n = 32            // fixed task count
	const readLoop = 200000 // arithmetic body for the read-only ceiling
	// Write workloads keep the body small (commit-dominated) but NONZERO: the loop
	// sits between the early property read and the write+commit, widening the window
	// so concurrent contended tasks overlap on the same read version and actually
	// abort/retry. loop==0 collapses that window and conflicts never materialize.
	const writeLoop = 40000
	workerCounts := []int{1, 2, 4, 8, 16, 32}

	t.Logf("contention matrix: n=%d tasks, readLoop=%d, writeLoop=%d", n, readLoop, writeLoop)
	t.Logf("%-15s %7s %12s %12s %9s %10s %10s %10s %9s",
		"workload", "workers", "serial", "pool", "speedup",
		"dAttempt", "dSuccess", "dConflict", "dRetry")

	row := func(workload string, workers int, serial, pool time.Duration, d commitCounterDelta) {
		speedup := float64(serial) / float64(pool)
		t.Logf("%-15s %7d %12s %12s %8.2fx %10d %10d %10d %9d",
			workload, workers,
			serial.Round(time.Microsecond), pool.Round(time.Microsecond), speedup,
			d.attempts, d.successes, d.conflicts, d.retries)
	}

	// 1. read-only workload (no commits) — reuse the arith harness verbatim.
	readSerial := measureConcurrencyWorkers(t, n, readLoop, false /*useVerb*/, false /*pool*/, 1)
	for _, w := range workerCounts {
		pool := measureConcurrencyWorkers(t, n, readLoop, false, true, w)
		row("read-only", w, readSerial, pool, commitCounterDelta{})
	}

	// Write workloads share the same shape: a serial baseline (distinct same as
	// the pool run) plus a pool run per worker count, capturing commit deltas.
	runWriteWorkload := func(label string, distinct int) commitCounterDelta {
		// Serial baseline uses the same dynamic-property workload as the pool runs
		// (so the speedup compares like with like); its counter delta is discarded.
		serial, _ := measureWriteWorkersDistinctCounted(t, n, distinct, writeLoop, false /*pool*/, 1)
		var maxDelta commitCounterDelta
		for _, w := range workerCounts {
			pool, d := measureWriteWorkersDistinctCounted(t, n, distinct, writeLoop, true /*pool*/, w)
			row(label, w, serial, pool, d)
			if d.conflicts > maxDelta.conflicts {
				maxDelta.conflicts = d.conflicts
			}
			if d.retries > maxDelta.retries {
				maxDelta.retries = d.retries
			}
		}
		return maxDelta
	}

	// 2. write-disjoint (distinct == n): no real conflict expected.
	runWriteWorkload("write-disjoint", n)
	// 3a. write-contended, K-way representative (distinct == 4).
	runWriteWorkload("write-contend4", 4)
	// 3b. write-contended, fully contended (distinct == 1): the gate row.
	contendedMax := runWriteWorkload("write-contend1", 1)

	// The gate: the fully-contended pool run MUST produce real MVCC conflicts and
	// retries, otherwise the instrument is not actually observing aborts. Real
	// concurrency requires >=2 schedulable threads; skip the assertion only when
	// the machine cannot provide it.
	if cores := runtime.GOMAXPROCS(0); cores < 2 {
		t.Logf("GOMAXPROCS=%d (<2): skipping nonzero-conflict assertion (no real concurrency)", cores)
		return
	}
	if contendedMax.conflicts == 0 {
		t.Fatalf("fully-contended (distinct=1) workload produced zero commit conflicts across the worker sweep; instrument not observing aborts")
	}
	if contendedMax.retries == 0 {
		t.Fatalf("fully-contended (distinct=1) workload produced zero commit retries across the worker sweep; instrument not observing retries")
	}
}
