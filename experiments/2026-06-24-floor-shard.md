# Shard the live-readTS floor registry (Track A1)

Date: 2026-06-24

Status: measured on experiment branch; source change RECOMMENDED for promotion (worker recommendation only — not self-promoted).

Experiment branch: `exp/floor-shard` (worktree C:/Users/Q/code/barn-exp-floor), off `work/mvcc-concurrent-moo` @ 0bb17f0.

Hypothesis: the global `floorMu` mutex guarding `activeReadTS` (the live-readTS multiset for COW
history GC) is the dominant serialization on the commit-dominated disjoint write path. A prior mutex
profile attributed 88% of all lock contention to it (historyFloor 52.7%, registerReadTS 26%,
deregisterReadTS 9%) — every transaction takes it 3x (begin/commit/release). Sharding it by readTS
should cut that contention and raise 32-worker speedup.

Single variable: replace `floorMu sync.Mutex` + `activeReadTS map[uint64]int` with
`readTSShards [16]readTSShard` (each = padded {mu, counts map}). register/deregister/historyFloor
shard by `readTS % 16`. Files: db/store/store_core.go, db/store/store_history_gc.go,
db/store/cow_history_gc_test.go (activeFloorCount helper sums shards).

Correctness:
- A given readTS always maps to one shard, so each shard's multiset is self-consistent.
- historyFloor scans all shards non-atomically. Safe because (a) new readers register at
  readTS=clock (the newest time), so a concurrently-registering reader's readTS >= the floor that
  would be returned — the scan can never return a floor that is too HIGH; (b) a floor that is too LOW
  is always safe (conservative: retains more history, never frees a live-needed version — per the
  existing store_core.go invariant comment).
- Existing GC/race tests are the safety net.

Baseline:
- Command: `go test -C <wt> ./scheduler -run 'TestConcurrencyCommitDominatedDisjoint$' -count=5 -v`
- Result (32 workers x5): 1.15x, 1.13x, 1.18x, 1.27x, 1.14x (avg ~1.17x; ~16-17 us/commit).

Experiment result:
- Same command after change.
- Result (32 workers x5): 1.87x, 1.97x, 2.02x, 2.22x, 1.84x (avg ~1.98x; ~9.0-9.9 us/commit).
- ~1.7x relative improvement; per-commit wall ~halved.

Fast contracts:
- `go build ./...` — OK.
- `go test ./db/store -count=1` — ok.
- `go test ./db/store -run 'HistoryGC|Disjoint|Floor|COW|Cow' -race -count=1` — ok (no data race).
- `go test ./scheduler -count=1` — ok (full suite, no regression).

Failure analysis (gate beat baseline but far below the 10x ambition -> profile for next target):
- Command: `-mutexprofile` on TestConcurrencyCommitDominatedDisjoint, `go tool pprof -top`.
- Dominant cost BEFORE (main branch): sync.Mutex via store floorMu = 88% (historyFloor 52.7%).
- Dominant cost AFTER: `runtime.unlock` 74.7% (runtime-internal locks) routed through
  `Scheduler.workerLoop` 35.6% / `runTask` 20.9%. sync.Mutex contention down to ~20%
  (historyFloor 13.8% residual 16-shard scan; commitDecentralized 14%).
- Interpretation: the floor bottleneck SHRANK as intended; the next wall is the single shared
  `s.taskWork` channel (hchan.lock) + per-batch barrier in runTaskBatch — 32 workers + dispatcher
  serialize on one channel, one tiny task at a time.
- Next target: Track A2 — replace single shared work channel + per-batch barrier with lower-contention
  dispatch (per-worker queues / direct batch execution / work-stealing). Secondary: per-task txn
  allocation (7 eager maps + finalizer) and BeginReadOnly's s.mu.RLock cacheline.

Metric gate: TestConcurrencyCommitDominatedDisjoint 32w, count=5, default GC. Pass = clearly and
repeatably above the ~1.17x baseline. PASS (avg ~1.98x).

Outcome: positive.

Worker recommendation: recommend promotion. (Separate verifier/parent must recompute the gate and
confirm branch cleanliness before merging to work/mvcc-concurrent-moo.)

Generated diagnostics: .probes/mu2.prof, .probes/s2.test.exe (not committed).
