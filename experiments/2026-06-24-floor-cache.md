# Cache per-shard min for historyFloor (Track B2)

Date: 2026-06-24

Status: measured on experiment branch; source change is a real SINGLE-THREAD win but NEUTRAL on
32-core throughput. NOT promoted. Most important output: a metric lesson (below).

Experiment branch: `exp/floor-cache` (worktree C:/Users/Q/code/barn-exp-floorcache), stacked on
B1 `exp/lazy-txn` @ 10ac487 (which is stacked on A1 floor-shard @ c09bcae).

Hypothesis: after B1 relieved GC, a CPU profile showed historyFloor's per-commit map iteration
(maps.Iter.Next ~15% flat, historyFloor ~18% cum) was the new binding cost. Replacing the per-commit
16-map iteration with a read of a cached per-shard min should raise 32-worker commit-dominated speedup.

Single variable: add `readTSShard.min` (0 = empty), maintained under the shard lock by
register (lower it) / deregister (rescan shard via shardMinLocked only when the removed entry IS the
min). historyFloor reads the 16 cached shard mins instead of iterating the maps. Floor stays EXACT,
so prompt pruning is preserved.
- Rejected sub-idea first: a stale-low cached floor + stride recompute. That would break the tracked
  test TestHistoryGCKeepsLongReaderSnapshotThenPrunes (it requires the next commit after the LAST
  reader releases to prune). Never weaken a correctness test. Used the exact-min design instead.

Fast contracts (all PASS):
- go build ./... ; go test ./db/store ; go test ./db/store -run 'HistoryGC|Disjoint|Floor|COW|Cow'
  -race ; go test ./scheduler  — all ok, no race. The prompt-prune GC test passes (floor is exact).

Metric result (commit-dominated, 32 workers):
- SERIAL (1-worker) baseline: 18-20 us/commit  ->  ~10 us/commit  (NEARLY HALVED — big single-thread win)
- POOL (32-worker): ~9.5 us/commit (A1) -> ~10 us/commit (unchanged within noise)
- speedup RATIO (serial/pool): ~1.98x -> ~1.1x  (DROPPED — but only because serial got faster)

Failure analysis / interpretation (the lesson):
- The "speedup = serial(1) / pool(P)" metric measures PARALLELISM and moves ONLY when worker-vs-worker
  CONTENTION is removed. B2 reduced PER-COMMIT WORK, which speeds the serial baseline and the parallel
  pool by the SAME factor, so the ratio cannot improve — and here serial improved more, so it fell.
- The 32-worker ABSOLUTE throughput did NOT improve (~10 us/commit before and after): at 32 workers
  historyFloor's cost was hidden behind the real binding constraints (GC + scheduler coordination +
  the 16 shard locks), so removing it helped only the uncontended 1-worker path.
- This is exactly why the prior commit-dominated ledger churned through dozens of per-task-work
  reductions and never moved the ratio past ~2-3x: per-task-work reduction cannot move a parallelism
  ratio. Only contention removal (like A1's floor-lock sharding) can.

Outcome: NEGATIVE on the speedup-ratio gate; POSITIVE on single-thread absolute latency (~2x);
NEUTRAL on 32-core absolute throughput. Correct and low-risk.

Worker recommendation: do NOT promote for the parallelism goal. It is a legitimate single-thread
latency improvement worth keeping for general server throughput, but it is not the 32-core lever.
The 32-core lever is GC (GOGC=off turns the read-path ratio 4x->10x) plus remaining scheduler
contention — attack those, measured by ABSOLUTE 32-worker throughput + parallel efficiency, NOT by
the serial-relative ratio alone.

Generated diagnostics: none committed.
