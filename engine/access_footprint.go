package engine

import (
	"github.com/MongooseMoo/barn/task"
)

// Access-footprint analysis is currently stubbed: analyzeTaskAccessFootprint
// returns "unknown" for every task, so the batch scheduler leans entirely on the
// OPTIMISTIC co-scheduling path (see candidateCanJoinBatch).
//
// Why. The runtime can run ready tasks in parallel two ways. PROVEN-commutative
// co-scheduling needs a static (object, property) read/write footprint per task so
// it can guarantee two tasks never touch the same cell and thus never need to
// retry. OPTIMISTIC co-scheduling runs conflict-retryable tasks in parallel and
// lets the commit-time read/write-set validation catch a real collision, re-running
// the loser. Correctness lives entirely in that commit-time validation — the
// footprint fast path only avoids the retry COST for the subset it can prove
// disjoint.
//
// The original analyzer walked the verb AST. Master's front-end refactor removed
// that AST from the task (tasks now carry a compiled *bytecode.Program, not source
// statements), so the walker lost its input. Rather than port it — or rebuild it
// against bytecode — before knowing whether it pays off, we return "unknown" for
// every task. For a read-mostly command workload the optimistic path is where the
// concurrency comes from anyway: read-and-tell verbs have empty write sets and
// never conflict, and notify() output is buffered outside MVCC (PendingEffects)
// and flushed at commit, so the socket is never a contended store resource.
//
// This is the measurable null hypothesis. The store already counts real conflict
// retries (Store.NoteCommitRetry, surfaced in the commit observability counters).
// Watch that on a representative workload: if retries are negligible, the fast path
// was never needed. If they pile up, reintroduce it as compile-time footprint
// metadata on the Program (the front-end-agnostic seam) — not an AST walk here.
type accessFootprint struct {
	unknown bool
}

func unknownAccessFootprint() accessFootprint {
	return accessFootprint{unknown: true}
}

// analyzeTaskAccessFootprint currently returns "unknown" for every task; see the
// package comment above for the rationale and the path back to real analysis.
func analyzeTaskAccessFootprint(t *task.Task) accessFootprint {
	return unknownAccessFootprint()
}

// accessFootprintsCommute reports whether two known footprints provably never
// collide. With every footprint currently "unknown", candidateCanJoinBatch takes
// its unknown branch before reaching this — it is retained so the proven-commute
// path can be reinstated by making analyzeTaskAccessFootprint return real footprints.
func accessFootprintsCommute(left, right accessFootprint) bool {
	if left.unknown || right.unknown {
		return false
	}
	return true
}
