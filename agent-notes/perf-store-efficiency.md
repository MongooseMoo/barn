# Store efficiency

## Goal

Find substantial, measured db/store efficiency gains; supervise the requested Sol 6.1 coding agent and publish successful results as a PR. Q authorized this plan with "Make it so!" on 2026-11-04.

## State

Branch perf/store-efficiency in C:/Users/Q/code/barn-store-efficiency; baseline C:/Users/Q/code/barn-store-efficiency-base. All three bounded changes implemented by Sol 6.1. Parent checked source and focused live-definition regression (`ok github.com/MongooseMoo/barn/db/store 1.019s`). No PR yet; performance runs precede CI because Q-barn-wsl runners may share the host.

Completed ten paired exploratory rounds for maps.Clone, lazy maps, and wide publication; raw output in experiments/store-efficiency. Lazy maps saved six allocations per small transaction. Earlier wide scalar 1024-slot update fell 34.567us to 3.757us, but that experiment included now-rejected bulk map cloning. Final source only skips empty overflow-map allocation, retains original map compaction, lazily creates dependency maps, and shares unchanged facets in simple commits.

Final frozen binaries: C:/Users/Q/code/barn/.tmp/store-delivery.exe and store-delivery-base.exe, with identical benchmark fixtures via overlay. Linux engine binaries /tmp/barn-store-efficiency-delivery-engine and /tmp/barn-store-efficiency-base-engine. Final targeted micro pairs completed: selected-benchstat.txt shows 128-slot scalar/property publications 40.68%/26.81% faster, 1024-slot 84.27%/37.71% faster, n=10. Allocated bytes fell 24.80-91.42% across these four cases. Sparse map timing unchanged. Public report drafted in experiments/store-efficiency/README.md. All macro measurements completed: three alternating pairs each of full-mix single-player and look 1/4/8 players, all with zero command failures/unsettled commands. Mixed median goodput 44.24 to 45.80/s; look medians 29.12 to 28.35, 69.46 to 68.64, 170.72 to 178.26/s. Every throughput range overlaps. No whole-world throughput improvement established. Full-mix median process-wide bytes divided by completed commands fell 6866248 to 6601526 (-3.86%); those ratios include background work. Eight-player look median bytes ratio increased 3282514 to 3333271. Median load time across six worlds per lane 14.560 to 14.535s; post-load heap 544 to 541 MiB, later heap varies widely. Original database/catalog hashes are unchanged after measurement. Candidate and baseline binaries hashed for report provenance. Source edits unchanged throughout these final runs.

First real mixed-command BASELINE completed but had E_NACC for player71 home (3 failures at four players, 2 at eight). Exit/test PASS does not validate those cohorts; paired timing stopped and raw logs retained privately at C:/Users/Q/code/barn/.tmp/store-efficiency-real-0-base.txt, public metric summary experiments/store-efficiency/real-0-base.txt. One-player cohort had 319 completed, zero failed/unsettled. Exact dump inspection: #71.home=#62, #62 is Generic gun, inherited acceptable returns 0; home calls teleport then moveto, native move raises the observed error. No Barn behavioral defect inferred and no fixture edits. Next macro experiments are full mix at one player and look at 1/4/8 players, explicitly narrower than the failed mixed-player corpus. No comparative real-workload gain claimed yet.

Primary-source ownership audit checked ChangeParents copied inputs, CreateObject private children/parents, renumber clone-before-write, property mutation ownership, definition propagation, loader shared-base ownership, and relationship read copies. No defect found in that audit. Sol agent stopped; reuse for in-scope fixes if needed.

## Decisions and why

- Refined candidate 1 after inspecting Go 1.27 runtime Map.Clone: it duplicates existing map tables/capacity, unlike Barn's original len-sized reinsertion. Real property deletion/removal produces sparse maps. Retain only the demonstrated empty-overflow-map allocation elimination: original manual compaction loops stay, over map allocation is conditional on len>0, gone keeps its original len>0 guard. No maps.Clone remains in this change. Added sparse/empty map-after-delete benchmark layouts using actual remove/put operations. Only selected-* files measure the corrected delivered source; earlier wide variants remain exploratory evidence.

- Implemented lazy dependency maps: initialize seven read-dependency maps on first insert through existing lazySet instead of allocating all seven in BeginSnapshot. Preserve every dependency and renewal behavior. About 35-60 changed implementation lines.
- Implemented publication sharing: copy only changed facets at bounded simple commit publication. Transaction staging already owns facets lazily; the prior ownImage always called cloneObjectSharingVerbs, which copies all nonverb facets. Admission now detaches loader-supplied arrays once.
- Parent verified Store.Add, Builder setters/ResetProperties, the borrowed-facet publication regression, commitImages.own and publication propagation. Detach nonverb backing once at numbered-object ingestion, then share unchanged owned facets only in the bounded simple-publication path. Full cloning remains required for live inheritance walks: another object's definition change can mutate a scalar-only staged image through commitImages.own. Eligibility must include !walkLive, not just a per-object write mask.
- Do not change transaction boundaries, remove commit exclusion, or introduce hot-record lock/repair machinery without measured evidence. Read MOCC page image 1 and MV3C page image 2: contention and dependency repair solve distinct costs, not automatic store wins.
- Use fresh paired baseline/candidate runs, allocation counts, profiles and real Mongoose workload results. Existing scripts/perf-compare.ps1 compares against an old VM baseline and is unsuitable as the store baseline.
- Required isolation workflow is docs/elle-isolation.md: preserve healthy and deliberate-fault verdicts, raw histories, manifests, witnesses, and nonzero path coverage. CI runs on PR; do not duplicate its test suite locally before push.

## Surprises

- Parent measurement orchestration incorrectly treated the process-wide `uncaught` field as a failed command and stopped after look baseline round 0. Source engine/mongoose_real_bench_test.go:744,830-831 reads metrics.UncaughtExceptions independently of command failed counts. Raw log identifies the one exception as forked s_run/E_FLOAT, while all 1432 eight-player commands completed with failed=0 and unsettled=0. Correct the existing result classifier by removing the uncaught-counter condition; preserve/report that background error separately. Do not rerun or discard this successful command cohort. Continue look round 0 candidate, then remaining alternating pairs. All six full-mix single-player cohorts completed without command failures; median goodput 44.24 to 45.80/s overlaps ranges, bytes/command 6866248 to 6601526 (-3.86%).

- Q pointed out the password-free harness path. Verified scripts/bench_players.py bootstrap_control_account creates a disposable random-password control account offline; engine/mongoose_real_bench_test.go connects selected players through stub connections without login. No real credentials are needed. Use the in-process real-command harness for paired macro measurements, and the existing TCP bootstrap for network measurements if needed. The matrix-only credential check is not a blocker.

- Existing BenchmarkTxnPropertyWriteCommit and BenchmarkTxnReadOnlyCommit omit explicit Release, unlike the production runtime and BenchmarkCommitScratch. Use the latter for the current lifecycle baseline; repair focused benchmark lifecycle as part of implementation evidence.
- Initial unquoted dotted profile arguments did not produce the expected artifacts. Fully quoted absolute Go arguments produced .tmp/store-plan.test.exe, .tmp/store-plan-cpu.pprof and .tmp/store-plan-heap.pprof; pprof read these successfully. Use fully quoted profile/output flags in subsequent PowerShell commands.

## Concrete implementation and experiment plan

Work in one isolated branch from fresh integrated master. Keep each experiment independently removable; do not stack PRs. Sol 6.1 owns implementation, focused tests and benchmark additions; supervisor owns hypotheses, evidence review, result selection and publication.

1. Property-table copying: skip allocating an empty overflow map, retaining the original len-sized reinsertion for populated maps, set-slice copy and shared immutable base. About 8 changed lines. The initial maps.Clone candidate was rejected because it retains capacity after deletion. Test loaded shared bases, overrides, removed slots and independent mutations.
2. Lazy dependency maps: remove eager allocation of the seven read maps in BeginSnapshot; use existing lazySet on first inserts in store_txn_snapshot.go, store_txn_live.go, store_txn_mapkeys.go and store_txn_property_inheritance.go. Keep renewal copies and conditional updates correct. About 35-60 implementation lines. This preserves precisely the same validation dependencies. objects cache stays unchanged in this experiment.
3. Bounded publication sharing: establish owned nonverb backing once at Store.Add with the existing clone helper. For existing numbered objects on !walkLive publications with only scalar/property-value writes, shallow-copy the Object, clone properties only when written, retain sharedVerbList's borrowed-verb handling, and share untouched relationships/order. Existing full cloning handles every other route, created/anonymous objects, joined descendants, definitions, renames, deletes, relationships, recycling and verb edits. About 50-80 implementation lines plus roughly 100 regression/benchmark lines. Check retained builder inputs, old-reader stability, mixed ancestor-definition/descendant-scalar commits and loaded-object admission cost. This intentionally does not reuse staged property tables: other commits can update disjoint properties, so publication must still apply writes to current live state.

Each experiment changes one mechanism. Retain only measured improvements. Do not implement history-lock batching now: the profile does not establish it as the main cost. Do not replace MVCC, add transaction repair, alter retry policy, change finalizer behavior or weaken serializability.

### Measurement

Exploratory allocation profile, objects32, 2s, Windows default 32 CPUs:

```text
418.20MB 43.05% propTable.clone
137.99MB 14.20% (*StoreTxn).writesByObject
104.03MB 10.71% (*StoreTxn).privatizeCached
 97.03MB  9.99% cloneObjectNonVerbState
 15.51MB  1.60% (*Store).BeginSnapshot
```

These are flat sampled allocation shares from a small-object microbenchmark, not production shares. CPU samples included substantial runtime sweeping; no attribution of that cost to a particular store lock is justified.

- Use BenchmarkCommitScratch objects1/8/32 for production begin-stage-commit-release lifecycle. Repair explicit Release in the two older commit benchmarks before using them in both A/B versions. Add wide-object scalar/property cases shaped from loaded objects and retained-reader cases, keeping fixtures identical between revisions.
- Run baseline/candidate sequentially in alternating order, at least ten independent observations per variant, fixed Go version/OS/GOMAXPROCS/GOGC. Compare ns/op, B/op and allocs/op with benchstat; retain raw outputs and instrumented profiles separately. Target at least 20% lower time or allocated bytes on the affected representative store path, with a real-workload gain and no meaningful regression. This is an experiment target, not a promised result.
- Confirm through the existing password-free engine TestMongooseRealWorkload harness in Debian WSL using frozen baseline/candidate executables, identical checkpoint/sound DB, rotating lane order and fresh worlds. Full mix at multiple players produced baseline permission failures; preserve them and compare only successful full-mix single-player and look-only 1/4/8-player cohorts. Use world medians as independent samples. Measure startup/load and retained heap because admission copying shifts costs. These narrower runs cannot establish a successful multiplayer mixed-workload gain.
- Local fixtures mongoose.db.new and files/sqlite/sound.sqlite were hashed before measurement. Debian and the harness have been exercised. Recheck fixture hashes after measurement; real credentials are unnecessary.
- Use engine's real command diagnostic harness if needed for per-task retry/scheduling attribution, following docs/workload-diagnostics.md. Do not mistake wall time or process-wide counters for per-command CPU costs.

### Correctness and delivery

Run only actively authored focused tests locally. Commit/push authorized implementation and open the requested PR, then wait for hosted PR checks and repair failures. Ensure runner selection uses CI_RUNNER_LABELS as required. Publication/validation changes require the pinned Elle workflow with healthy/fault controls, explicit verdicts and nonzero coverage, including mixed-definition/scalar old-reader regressions. Managed conformance supplies any uncertain MOO behavioral oracle checks.

PR evidence must show baseline/candidate settings, raw benchmark outputs, statistics, successful workload totals, load/memory tradeoffs and CI/Elle evidence. Publish only actual retained improvements; negative experiments remain accurately recorded and out of delivered source. Follow standing normal-merge requirements once all applicable gates pass.

## Next step

Report now includes completed macro medians/ranges, load/heap tradeoffs, hashes and background exception distinction. Commit/push/open the requested PR; review that open PR and await CI/pinned Elle evidence before normal merge. CI_RUNNER_LABELS selects self-hosted Linux X64 barn. Real-world throughput improvement remains unestablished; the concrete delivered gains are the selected store benchmarks. PR body draft is C:/Users/Q/code/barn/.tmp/store-efficiency-pr.md. Preserve private raw logs and binaries under the main checkout .tmp when cleaning owned worktrees after merge.
