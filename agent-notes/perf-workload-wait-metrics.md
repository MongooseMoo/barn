# perf/workload-wait-metrics

## Goal
Measurement only, no behaviour change: make `TestMongooseRealWorkload`
(engine/mongoose_real_bench_test.go) print where command wall time goes, per
measured player level: runtime phase sums, exclusive commit gate holds by task
verb, shared commit gate wait, admission preemptions/readmission, and for each
unsettled command the stuck task's readier (id, state, lease). Q approved.

## State
- Done: store shared-gate counters + test (db/store/escalation_gate_test.go),
  metrics `PhaseCount`/`PhaseStats.Sub` + test, engine `gateHoldStats`,
  `Runtime.GateHolds`, sliceGate timing.
- Done also: engine gate-hold test (irreversible_effect_boundary_test.go),
  `Runtime.ResetGateHolds` (window = reset before measure, like
  ResetConflictCensus, so per-verb max is exact for the window), task
  `Readier()` accessor + test (exec_ready_test.go).
- Done also: harness prints four lines per level after "commits:" (wall
  phases, exclusive gate holds, shared gate wait, admission) and the readier
  in unsettled messages. Mutation-checked all four new pieces (each test went
  red, restored). vet/gofmt clean on engine, metrics, db/store, task.
- 1-player harness run done once (BARN_MONGOOSE_DB pointed at the main
  checkout's mongoose.db.new); all four lines printed. Committed on the branch.
  No push, no PR (per brief).

## Results of the one 1-player run (8s window)
- Exclusive gate held only by #1007:s_run: 286 holds, 745 ms total, max 137 ms.
  Its acquire wait (commit_gate phase) 286 / 1.22 s.
- Shared gate wait: 1907 commits, 653 ms total, mean 343 us.
- retry_waste 205 / 9.49 s of vm_attempt 23.1 s; slice total 29.9 s (background
  s_run tasks run alongside the one player).
- admission preemptions=6 but Readmission phase count 0.

## Decisions and why
- Phase window deltas: `metrics.RuntimePhaseStats` already snapshots each phase
  (metrics/runtime.go:95). Added only `PhaseCount` and `PhaseStats.Sub`. The
  cumulative max cannot be windowed, so the harness prints the upper bound of
  the highest bucket that grew in the window instead.
- `metrics.CommitGate` records only the wait to ACQUIRE the exclusive gate
  (engine/task_runtime.go:221 escalation, :305 irreversible-effect boundary).
  The shared-gate wait at commit (db/store/store_txn_commit.go:250) is only fed
  to `scope.Waited` (admission gateWait, mixed with exclusive waits), never
  counted on its own. Added two store atomics at that site.
- Exclusive holds are recorded in `sliceGate.drop` (the one place every engine
  hold ends), keyed by task this/verb, mutex-guarded map on the Runtime. Only
  runs when a gate was held. The checkpoint snapshot's exclusive hold
  (db/store/store_snapshot.go:73) is outside the engine and not counted.

## Surprises
- The brief says to print after the "read checkpoints:" line; no such line
  exists in the harness. New lines go after the "commits:" line.

- Preemptions (6) with zero Readmission observations: engine/task_runtime.go
  observes Readmission only when `scope.Yield()` returns > 0. Why every wait
  was 0 here is not established (possibly the yielding task is redispatched
  synchronously inside preempt and Windows' coarse monotonic clock reads 0);
  unverified. The harness label now says readmission_nonzero_waits.

## Next step
- Lead decides whether to run 16 players with these lines (not done here).
