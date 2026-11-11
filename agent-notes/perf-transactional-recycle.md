# Transactional recycle scalability

## Goal

Resolve the source-only report from
`mongoose@mongoose.world:~/agent-workspace/tasks/issue-795-cpu-for-bugs/barn-recycle-commit-gate.md`,
measure scaling, publish an independent PR, and normal-merge after required
checks. Q authorized the corrected design with "superb, make it so! worktree!".

## State

- Branch `perf/transactional-recycle`, worktree
  `C:/Users/Q/code/barn-transactional-recycle`, based on fetched `origin/master`.
- The scaling benchmark was published before the production change so the
  remote compute checkout can measure the unchanged implementation.
- Numbered-object recycle now avoids the anonymous reservation and uses existing
  transaction-owned moves and deletion. Anonymous reservations retain their
  irreversible boundary. Persistence restores only anonymous reservations.
- Focused authored Go regressions passed: transactional numbered recycle,
  anonymous reservation retained on abandoned attempts, anonymous boundary abort,
  and numbered/anonymous continuation restore behavior. Full checks belong to PR CI.
- No production deployment, production database mutation or service restart.
- The corrected candidate conformance file is included in the existing CI
  managed-regression command. No runner or workflow scheduling policy was changed.
- PR: https://github.com/MongooseMoo/barn/pull/738 (draft during verification).
- Initial production-tree CI build/unit, pinned Elle and checkpoint checks passed.
  Full conformance found three failures on both published heads. Their fixes are
  in this worktree with focused regressions; required CI must check the repaired
  source before ready/merge.

## Decisions and why

- `builtins/objects.go` previously acquired the exclusive gate before reserving
  every recycled object, then flushed topology and moved through `DirectTxn()`.
  The gate was retained for the attempt, including later creates.
- The documented installed Toast oracle's `src/objects.cc:803-812` checks and
  sets its recycle flag only for anonymous values. Numbered hooks may overlap.
  Public primary source:
  https://github.com/lisdude/toaststunt/blob/master/src/objects.cc
- The initial proposal to move numbered reservations into object MVCC was wrong.
  It was stopped and handed off; Q then approved the corrected deletion-first
  design. No reservation field, extra lock, validation protocol or fault for a
  nonexistent numbered-object guard was introduced.
- Numbered cleanup reuses `StoreTxn.MoveObject` and `StoreTxn.RecycleObject`.
  Anonymous deletion, child reparenting, player connection effects and actual
  irreversible callback effects retain their existing coarse boundaries.
- A numbered callback's speculative instructions are protected by normal commit
  validation/retry. The existing early-abort regression specifically protects an
  irreversible reservation boundary, so its fixture now uses an anonymous
  object. Its abort and no-caller-progress assertions remain intact; no extra
  validation step was added to numbered recycle merely to retain a boundary
  that has been removed.
- Existing anonymous guard restoration and duplicate-restoration rejection remain
  tested. A separate test requires repeated numbered-continuation restoration to
  succeed without introducing a reservation.
- The allocator is already atomic (`s.ids.highWater.Add(1)`); no allocator change.
- Keep existing metrics owners and the existing benchmark file. Every benchmark
  cohort drains before reading counters and checks final contents/children.

## Oracle evidence

- Initial selected packaged Toast scenarios and capability admission completed
  successfully. Raw terminal summary:
  `7 passed, 13813 deselected, 1 warning in 260.43s (0:04:20)`.
  Artifacts are `/tmp/barn-recycle-plan-oracle-admission.json` and
  `/tmp/barn-recycle-plan-oracle.xml` inside WSL Debian.
- The rejected probe expected a second numbered recycle to return E_INVARG
  immediately while the first hook waited. It timed out on Toast:
  `1 failed, 2 passed, 1 warning in 11.01s`. Its raw evidence remains in
  `.tmp/recycle-oracle/results.xml` and `admission.json`. The timeout alone was
  not treated as an exact semantic observation.
- Replaced it with two forked recyclers and a separate controller that releases
  suspended hooks. Numbered objects enter both hooks; anonymous objects enter
  one. The test checks validity while suspended, hook completion counts and
  both native outcomes after release. It cleans up its own tasks and objects.
- Corrected managed Toast result: `4 passed, 1 warning in 8.24s`, exit zero.
  `.tmp/recycle-oracle/results-corrected.xml` and `admission-corrected.json`.
- Unchanged Barn result: `1 failed, 3 passed, 1 warning in 8.56s`, exit one.
  The numbered case returned native E_INVARG instead of the expected successful
  result. The anonymous case passed. Evidence is
  `.tmp/recycle-oracle/results-barn-baseline.xml` and
  `admission-barn-baseline.json`.
- Runs used the documented managed candidate flow with
  `--moo-suite-path=builtins/recycle_unrelated_tasks.yaml`, canonical capability
  admission, the packaged Test.db/startup fixtures copied under
  `.tmp/recycle-oracle/db`, and the stock WSL Toast executable
  `/root/src/toaststunt/build-release/moo`.
- Linux-only uv environment: `/root/.cache/barn-recycle-conformance-linux`.
  The JUnit warning is the existing record_property/xunit2 incompatibility.

## Measurement

- `BenchmarkRecycleScaling` is in `engine/object_lifecycle_bench_test.go`.
  It runs 16 settled commands per worker for 1, 2, 4 and 8 workers, with
  same-slice/precommitted objects, shared/separate rooms, and hooks off/on.
  Each command retires a placed object, then creates and retires another.
- Setup, fixture teardown and final relationship checks are outside the timer.
  Both sides enable the same existing conflict/gate diagnostics. Reported metrics
  are commands/s, gate holds/time, shared wait, retries, wasted ticks and standard
  allocation metrics. No process-wide deltas are attributed to concurrent work.
- Measurement host is the existing isolated `barn-compute` guest (eight CPUs),
  reached using the documented jump route and existing known-hosts file. The
  ordinary `barn` CI pool is on Q's WSL workstation; do not call it the separate
  measurement host. Existing fuzz work was not interrupted.
- Go is the existing guest Actions tool-cache version 1.27.1. GOMAXPROCS is 8.
  Command: `go test -p 2 ./engine -run '^$' -bench '^BenchmarkRecycleScaling$'
  -benchtime=100ms -count=10`.
- The first baseline run completed but its capture was truncated; it is excluded.
  A second baseline capture and the candidate capture each retain all 32 cases
  with ten complete samples and exit zero. Raw files and the complete median
  table are in `experiments/recycle-scaling/`.
- Final eight-worker median throughput improved 1.79x to 3.47x across the matrix. Candidate
  exclusive gate holds are zero in every retained sample. Precommitted objects
  still incur ordinary MVCC retries. These are diagnostic-enabled synthetic
  results on a shared host, not production goodput or linear-scaling evidence.
- The benchmark's Go code is identical between measured baseline and candidate.
  The final evidence/metadata commit does not change the measured Go code.

## Isolation and checkpoint evidence

- Pinned Elle run: https://github.com/MongooseMoo/barn/actions/runs/42927123080 .
  Both healthy models returned literal true with exit zero for seeds 412, 395
  and 396. Injected isolation faults returned false with exit one and have saved
  anomaly witnesses. `results.json` has `errors: []`.
- Healthy seed 412 coverage: parallel=806, exclusive=677, exclusive_gate=860,
  hot_renewal=2, verb_memo=478, suspend=481, fork=14. The other healthy seeds also
  recorded nonzero counts for all required paths.
- Checkpoint run: https://github.com/MongooseMoo/barn/actions/runs/42927122982 .
  All eight generation histories have explicit true verdicts, with no recorded
  durability violations. Native outcomes remain visible: ok=1974,
  info:transport=104, fail:no-live-generation=61. Unknown transport outcomes were
  not counted as successful transactions.
- Downloaded the full raw archives under `.tmp/elle-pr-738` and
  `.tmp/jepsen-pr-738`. They include histories, manifests, logs and anomaly
  witnesses. Extracted inspection copies are in
  `/tmp/barn-recycle-738-evidence` inside WSL Debian. Preserve these outside the
  task worktree before cleanup.

## Repairs from full conformance

- Preserved the initial failed run's XML, admission record and structured logs
  under `.tmp/conformance-failed-738`. It found self-recursive recycle reporting
  a non-MOO runtime error, staged recycle/query/recreate returning E_INVARG, and
  an empty queued fork being lost at restart. No assertions were relaxed.
- Local conformance was behind CI. Fetched the exact CI-selected dependency into
  detached `.tmp/conformance-ci`, preserving the companion checkout's dirty work.
  Use `/root/.cache/barn-recycle-ci-conformance-linux` for this distinct project.
  Remove the detached dependency worktree before removing this Barn worktree.
- Managed Toast confirmed the self-recursion and empty-fork expectations. Its
  next-recycled-object result at the highest slot differs from the pre-existing
  suite assertion: actual `[1,0,1,0]`, expected `[1,1,1,0]`. That lookup convention
  was not changed. A reduced scenario ignores the lookup value and asserts only
  recreate identity/validity; it passed on Toast and reproduced Barn's E_INVARG.
- `vm/recycle_lifecycle.go` now continues cleanup when dispatch refuses a hook
  with E_MAXREC, matching Toast's missing-hook/frame-limit handling. Errors raised
  inside an already running hook still use the normal error path.
- A stronger named-handler regression then exposed a second part of that
  failure: cleanup replaced the error code but assigned its bare None value
  directly to the unwinding exception. A named handler indexing its error tuple
  raised E_TYPE. `handleErrorWithReason` now sends replacement cleanup errors
  through its existing canonical tuple construction before resuming the unwind.
  Both caught-tuple and native uncaught recursion regressions pass.
- `builtinNextRecycledObject` already flushes staged topology for its live scan;
  its live bound check now follows that flush as well. It no longer rejects an
  own newly allocated slot merely because it has not been published yet.
- `LoadQueuedTasks` no longer discards a valid ID solely for an empty source
  slice. The existing compiler/VM installation builds an executable empty fork.
- Authored Go regressions first failed on the query bound and empty fork, then
  passed with the three repairs. The self-recursion regression checks the native
  uncaught E_INVARG rather than only catching it in MOO. The authored managed
  scenarios passed on the repaired Barn and Toast with admission included.
- Evidence: `.tmp/recycle-oracle/results-ci-pinned-toast.xml`,
  `results-query-recreate-toast.xml`, and `results-repaired-candidate.xml`, with
  corresponding admission records. The final performance candidate includes
  all repairs and tuple normalization. Its 320 samples completed with exit zero;
  `candidate-final.txt` and the refreshed median table retain this measurement.
  Prior complete candidates remain retained history.

## Literature

Used the research-papers plugin's paper-reader existing-complete path for the
Kung/Robinson paper directory. Inspected page images 000-003, printed pages
213-216. Only that scope was read here. The title page dates the paper to 1981,
despite the collection directory containing 1979. The private-write,
validation/publication discussion supports reusing Barn's existing transaction
owner; it does not establish MOO lifecycle semantics or a speedup.

## Surprises

- The initial reservation, not just movement, took the gate.
- The numbered-object reservation was itself a MOO divergence.
- The apparent allocator unknown in the report was already an atomic counter.

## Next step

Publish the source repairs and complete final raw evidence together to the
existing PR branch. The temporary benchmark ref is no longer needed for measurement.
Finish required CI on that head, preserve archives outside the worktree, make
the PR ready, and normal-merge when checks are clear. Clean up task worktrees and
branches, including the temporary benchmark ref.
