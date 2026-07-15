# Barn performance campaign notes

## 2026-07-15 prior-art researcher checkpoint

- Controlling task read: `prompts/barn-performance-prior-art.md`.
- Required role instructions read: `protocols:researcher`.
- Barn instructions read: `C:\Users\Q\code\barn\AGENTS.md`.
- Initial recursive search found no `AGENTS.md` under `C:\Users\Q\code\moo-conformance-tests`; applicable sibling-repository instructions still need to be identified from its other instruction surfaces.
- No repository benchmark, history, or source research has begun.
- No production files have been modified and no benchmarks, experiments, baselines, servers, or oracle runs have been performed.
- The checkpoint initially lacked an authorized notes-file path. The manager has now explicitly authorized this file and expanded the allowed changed paths to this notes file and the required report.
- Current blocker: none.
- Next required action: verify and record the current branch and tracked/untracked state of both repositories before writing research findings.

## 2026-07-15 prior-art researcher checkpoint 2

- Barn state verified before research: branch `master`, HEAD `864de996a111674adfe15c330f8e85813f4641f0`, tracking `origin/master`, with no tracked modifications shown and extensive pre-existing untracked files. This notes file is one of the newly authorized untracked files.
- `moo-conformance-tests` state verified before research: branch `main`, HEAD `e3d8dbd28c600490b4a1529619d29273ee29b30f`, tracking `origin/main` and ahead by 13 commits; pre-existing tracked modification `src/moo_conformance/_tests/audit/connection_lifecycle_toast_oracle.yaml`; extensive pre-existing untracked files.
- No sibling `AGENTS.md` exists. Read sibling `CLAUDE.md`; it defines ToastStunt as the authority, names the WSL Barn oracle `/root/src/toaststunt/build-release/moo`, documents the managed `moo-conformance --server-command=...` path, and identifies `Test.db` as authoritative.
- Memory was searched only as a non-authoritative locator. It pointed to the five commit IDs and the convergence plan, but every claim remains subject to independent repository/Git verification.
- Filename searches in both repositories found Barn performance scripts, plans, reports, experiment ledgers, VM benchmarks, and sibling `bench/`, benchmark reports/notes, and profile-gate code/tests.
- First substantiated sibling benchmark launcher: `moo-conformance-tests/bench/run_bench.sh`. From WSL it builds Barn to `/tmp/barn_linux`, selects an existing Toast binary, copies bundled `src/moo_conformance/_db/Test.db` to `/tmp`, starts Toast on 7801 and Barn on 7802, and runs `uv run python bench/bench.py "toast=$TOAST_PORT" "barn=$BARN_PORT"`.
- No benchmark baseline, candidate experiment, production modification, server, or oracle has been run by this worker.
- Current blocker: none.
- Next action: read `bench/bench.py`, then continue evidence collection from the identified repo-local artifacts and Git history.

## 2026-07-15 prior-art researcher checkpoint 3

- `moo-conformance-tests/bench/bench.py` is a tracked synthetic, single-connection sequential eval benchmark using the suite `SocketTransport`; ten fixed workloads, one untimed warm-up, five timed repeats, wall-clock `perf_counter`, and reported min/median milliseconds. It raises server task limits identically and checks expected values where supplied. Its source docstring has a stale `.tmp/bench.py` usage path, while the launcher uses `bench/bench.py`.
- `reports/benchmark-barn-vs-toast-20260619.md` records the original WSL socket results: noop ~0.3 ms parity; Barn/Toast ratios of 9.4x int, 9.1x float, 12-13x string concat, ~17x list append, 8.7x list index, 4.2x `tostr`, 7.4x property access, 7.7x `abs`, and 9.3x nested loop. It says values came from two consecutive runs and were stable within noise, but does not record raw per-repeat samples.
- `reports/barn-vm-optimization-20260619.md` substantiates direct Go benchmark command `go test ./vm -run='^$' -bench=BenchmarkVM -benchmem` plus CPU-profile/pprof commands. It records allocation and CPU-profile evidence: arithmetic dispatch ~55-60%, allocation/GC ~20-25%; list append 1.67 GB/10k and whole-list `ValueBytes` plus copy loops as compounding O(n) work.
- Barn's tracked `scripts/benchmark-mongoose.ps1` is a managed deployment runner. It always copies the requested source DB to the run directory and verifies hashes; requires exactly three non-empty secret login lines in `MONGOOSE_LOGIN_SCRIPT`; uses the fixed post-login sequence; measures database load/listen, PROXY/banner/login/command latencies, checkpoint reply/file, settled CPU/RSS; records Barn debug vars and forced-GC heap profile; and stops both client and server in `finally`.
- `experiments/2026-07-14-mongoose-performance-baseline.md` pins fixture SHA-256 `b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`, WSL Mongoose Toast hash `a748a93644fe2b973cc85dfed902454a0a56c8b368afdc8104161ec76154d098`, client timings, 180-second settle, Toast baseline, and Barn thresholds before Barn measurement.
- Toast deployment baseline includes load 6392 ms, login 5015 ms, checkpoint file 9429 ms, CPU 3.6%, RSS 311640064 bytes. Initial Barn measurement passed every non-memory threshold and failed RSS at 1882996736 bytes versus 467460096 threshold. A profile-bearing repeat used 2010251264 bytes RSS, with forced-GC heap owners led by `types.NewMap` 247.06 MB, `ObjectBuilder.ResetProperties` 222.61 MB, and `types.NewStr` 131.01 MB.
- The current tracked convergence plan restates the benchmark rules: Toast first; thresholds recorded before optimization; one metric/hypothesis and one production surface per slice; same benchmark plus conformance gate; keep a measured improvement or fully restore; no performance change kept on profiler appearance alone. Current milestone 5 is stopped after two consecutive rejected RSS slices, unfinished at 1295065088 bytes versus 467460096.
- Independent Git verification so far: `c3c7f8748dfd60a5b732b166312c18ac85787e0d` is `perf: store resolved properties by value`; `4cba9daa95bb890518d2fae795c8a19daac38fde` is `perf: delete duplicated property names`; `8f787cc30f5a5c6ac9695137cd7d06dc073b1af3` is a docs-only commit recording the rejected map-capacity slice. The first two commits include source/tests plus experiment/plan records; the third includes only the experiment and plan.
- No benchmark, candidate experiment, server, or oracle has been run. No production source has been modified.
- Current blocker: none.
- Next action: independently verify commits `563196b` and `864de99`, then inventory remaining benchmark commands/history and sibling correctness constraints.

## 2026-07-15 prior-art researcher checkpoint 4

- Independent Git verification completes the prompt's five IDs:
  - `563196b5deb689a7b777358ca1fb30dd231cb957` is docs-only despite its `perf:` prefix: it pins the resolved-map capacity slice in the experiment and plan; it does not record rejection.
  - `8f787cc30f5a5c6ac9695137cd7d06dc073b1af3` is the later docs-only rejection record for that slice.
  - `864de996a111674adfe15c330f8e85813f4641f0` is docs-only and records the second consecutive RSS rejection/stop; it is current Barn `HEAD`.
  - The intervening `873fa40` pins the ordered-property-storage slice. Graph order is `563196b` -> `8f787cc` -> `873fa40` -> `864de99`.
- Barn history has a broader performance lineage. Current/relevant tracked records include June VM microbench work, June 22 builtin-call experiment, June 27 canonical Windows baseline/statistical comparison campaign, scheduler/MVCC experiment branches, and the July 14 Mongoose deployment RSS campaign.
- `experiments/2026-06-27-perf-baseline.md` calls itself the canonical single-thread VM baseline. It records Ryzen 5950X, Go 1.26 Windows/amd64, nine `BenchmarkVM` workloads, `-count=10` medians/CV, and exact raw file. `list_append_10k` and `list_index_1M` each have 15% CV; string concat 11%; the other rows 1-10%.
- Exact baseline command recorded there: `go test ./vm -run='^$' -bench=BenchmarkVM -benchmem -count=10 | tee experiments/perf-baseline-vm-20260627.txt`. Exact comparison helper examples use `pwsh scripts/perf-compare.ps1 ...`; the script invokes the same Go benchmark with `-count=10` and `benchstat`, requiring a separately installed `benchstat`.
- `vm/perf_bench_test.go` is tracked and directly drives compiled MOO programs through a fresh VM per `b.N`, excluding sockets/protocol and reporting allocations. It contains the nine baseline workloads and comments with direct benchmark plus CPU/memory profile commands.
- `experiments/2026-06-22-builtin-call-performance.md` records promoted commit `e0cdce8` and record commit `8bc47f0`. It improved the mislabeled socket `verb_call_200k` row (actually `abs()`, hence builtin dispatch) from Barn 52.94 ms / 6.1x Toast to 31.11 ms / 3.5x, and local allocations from ~999,484 to ~799,484. A registration-time protected-builtin cache was discarded because `load_server_options()` mutates that runtime state.
- `notes/perf-execution-log.md` is the June 27 campaign ledger. It records kept C0 baseline (`78f035d`), C1 numeric-first routing (`765e426`), C2 tagged-union Value squash (`6278e38`), C5 dispatch-loop squash (`617505f`), and C5 builtin ceremony work (ledger names `ac3906f`; current history also contains source `e4ede2d`/later equivalents). It records C2's initial 3x ns/op gate as missed and explicitly waived by the user after allocation/conformance success; an initial C2 conformance blocker was fixed before promotion. C4 was skipped because the O(n^2) premise was already obsolete; C3 scheduler concurrency was deferred because its harness/code lived off master; optional string-box/struct-shrink/PGO work remained pending.
- Sibling Git history verifies tracked performance harness addition `866d33c` and benchmarking-notes update `0fd36b7`; profile-related commits are feature compatibility gates, not performance gates.
- Sibling `docs/TOASTSTUNT.md` confirms the WSL oracle binary/core but still contains manual server/`nc` and bare `pytest` examples. Those are subordinate to Barn `AGENTS.md` and sibling `CLAUDE.md`, which require/use the managed WSL `uv run moo-conformance --server-command=...` flow for this campaign.
- No current trustworthy baseline has been run by this worker. The June baseline predates current July HEAD and the July Mongoose baseline covers deployment RSS/latency rather than a general performance budget.
- Current blocker: none.
- Next action: read the remaining performance ledgers/reports and exact sibling managed/profile constraints, then verify tracked/untracked evidence status and literature/upstream references.

## 2026-07-15 prior-art researcher checkpoint 5

- Broad command search found four benchmark/measurement families:
  1. current tracked direct VM `BenchmarkVM` (`go test`, optional `-benchmem`, `-count`, CPU/memory profiles, `benchstat` helper);
  2. current tracked sibling WSL socket Barn-vs-Toast microbenchmark (`bash bench/run_bench.sh`, internally `uv run python bench/bench.py ...`);
  3. current tracked Barn Mongoose deployment runner (`scripts/benchmark-mongoose.ps1`, explicit engine/database/output plus secret env prerequisite);
  4. a scheduler concurrency bottleneck finder and repeated target test recorded in an untracked recovery ledger/script, but not runnable on current `master` because the referenced scheduler benchmark/sweep files are absent.
- `experiments/2026-06-24-commit-dominated-concurrency-ledger.md` is untracked and explicitly says it is a recovery ledger from an already-dirty `work/mvcc-concurrent-moo` integration branch, source uncommitted, protocol not followed cleanly, and target not achieved. Its target was repeated default-GC 32-worker speedup >=3.0x; fresh rows included 3.32x, 2.29x, 2.13x, 4.06x, 5.68x, so the all-repeats gate failed.
- That ledger records provisional keeps: bottleneck instrumentation; store/transaction allocation reductions; disjoint batching/chunking; write-only local property path; footprint cache/precompute; VM pool; single-property transaction slots; cap-4 chunks; ready-batch reductions; manual deadline check; lazy commit grouping; worker-side completion; running-state lock skip; child-context removal; write-only transaction constructor/pool; single-property commit path; and no-reader history-retention skip. None was cleanly promoted in that artifact.
- It records rejected/restored forms: smaller VM stacks/frames; loop/exception prealloc removal; huge-deadline shortcut; uncapped/cap-8 write chunking; opt-in contention profiles; direct goroutines; finalizer-clear skip; zero-loop harness omission; `lazySet` cap 1; lazy transaction object cache; chunk-local metrics; serial `Done` fairness; early unguarded local-property fast path; early VM pool; and a one-property decentralized commit fast path.
- Its focused command family measures serial/pool wall ns/op, bytes/op, allocations/op, commits/retries/task-slices/validation failures; profile variants capture CPU, allocation, mutex, and block profiles. `scripts/run-bottleneck-finder.ps1` is also untracked and currently references absent `TestConcurrency*`/`BenchmarkBottleneckFinder` sources; current `scheduler/` only exposes `review_concurrency_test.go` by matching filename search.
- Current `master` history, not just old ledger hashes, contains the June VM performance commits. Verified so far: `884b8e51e3db2159a6136966edcc2e07ce5f8e46` is the baseline/statistical harness commit; `c6d81e743590b294ca8133e587d8536efea008af` is numeric-first self-accumulation routing with characterization tests.
- Current `master` also contains July 1 kept `685c5f4607b1ec70e8fb02993938a042029b6a14`, batching orphan-anon and waif GC sweeps. Its commit message attributes live-world starvation to full-database sweeps costing hundreds of milliseconds and says the change made the Mongoose world playable; no reusable raw performance sample or statistical spread is embedded in the commit.
- All-branch history contains `campaign/barn-performance-20260714` commit `3d5353f71bbfb392f5dd2ee5bea52db4379b6116`, which only initializes a separate campaign worktree/report from source HEAD `864de99`; it contains no measurements or campaign framing.
- No benchmarks, profiles, servers, experiments, or oracle checks have been run by this worker.
- Current blocker: none.
- Next action: verify the remaining current-master VM performance commits/results, then capture exact conformance/profile constraints and complete literature/upstream/no-hit searches.

## 2026-07-15 prior-art researcher checkpoint 6

- Current-master June 27 promoted commits independently verified:
  - `5bf93a1fcf9022f0b2e96dcff9e7f58705a3b501`: tagged-union `Value`; commit records scalar allocations ~2.0M -> 11/op, geomean allocations -99.96%, bytes -98%, conformance unchanged. It also records modest single-thread latency, a `tostr` regression, and deferred concurrency payoff.
  - `fd17fa354bd0f31f614c8c7e7aa83212743d60d1`: dispatch-loop bounds-check collapse; commit records int -10%, nested -6% versus prior master and conformance unchanged.
  - `cf679902271c3295e5b109aa82775e9d632260b8`: slice-indexed builtin registry, lock-free protected-builtin snapshot, folded validation; commit records `tostr` -7.6% (regression resolved) and `builtin_abs` -32%, conformance/race clean.
- Earlier promoted current-master profile-driven commits:
  - `027e3585b08012cd42ba6c4ac90b7b62a61daa20`: adds the direct VM benchmark and numeric-first/format/bulk-copy wins; commit records string concat ~18%, list append ~16%, `tostr` ~15%, arithmetic ~5%.
  - `fb438f5a3211d5adbc2da1052c76d59dd2f30cb7`: O(1) cached list byte-size; list append 955 -> 608 ms (-36%), conformance unchanged.
  - `507b3b7425d6f85ccaa190f1fe7c8b7b07801caa`: cached current frame/inlined dispatch; int ~12%, float ~11%, nested ~14%, list index ~13%, conformance unchanged.
- June 21 promoted current-master opcode/structural work:
  - `6094276b6bddd6756a33f6b75c11c8393030fc6c`: fused range-for check/next; int 132 -> 60 ms, nested 100 -> 58 ms, list index 186 -> 144 ms; conformance unchanged.
  - `a03f4b216c9e6424cebb9fce461a30f52fc84e37`: list/map/string iteration phase A; list iteration 164 -> 116 ms.
  - `8c43bee3a21f265860b09f7644c380f49167ea3c`: fused iteration element-load phase B; 116 -> 94 ms (164 -> 94 cumulative).
  - `32f1deb4942fd28935b38155101a4d7b2be834f5`: remove statement-assignment DUP/POP; int ~12%, nested ~8%, list iteration ~12%.
  - `30d428814179f4a3f43b16c194aef95e63b6b040`: alias-safe watermark/self-append optimization; list append 10k ~580 ms -> 1.35 ms and 1.67 GB -> 1.39 MB, with alias tests and conformance green.
  - `1ef32f18d91426ae25a3336b03fde5d2e3c4d89e`: accumulator string concatenation optimization; its terse commit message has no embedded metric, so any numeric claim must come from a separate ledger/report.
- These measurements span different commits, operating systems, counts, and workload revisions. They are prior evidence, not a directly comparable current-HEAD series.
- No current benchmark or candidate experiment has been run.
- Current blocker: none.
- Next action: inspect the string-optimization and remaining experiment records, sibling profile/managed-server code, and search for literature/upstream references and documentation/precommit checks.

## 2026-07-15 prior-art researcher checkpoint 7

- No Barn `.pre-commit-config.yaml`, markdown-lint config, CONTRIBUTING guide, or documented report-specific documentation checker was found. `Makefile` only documents build/test and stale manual conformance targets; README documents a managed `scripts/run-conformance.ps1` flow. A final `git diff --check` is still an applicable low-risk hygiene check, but it is not a documented report-specific precommit.
- Barn README's preferred managed command is `./scripts/run-conformance.ps1 -Build -Binary ./barn.exe -SourceDb ./Test_conf.db -RunDb ./Test_run.db -Port 7788`; it copies the source DB, waits for listener, runs the uv-managed conformance package, stops Barn, removes the run DB by default, and writes `reports/runs/` artifacts. This is distinct from the Mongoose deployment benchmark and from the sibling WSL Toast command.
- Sibling `ManagedServer` code proves managed mode uses bundled `Test.db` unless `--server-db` is given, copies the selected DB into a temporary directory, installs fixtures, substitutes `{port}`, `{db}`, `{manifest}`, `{server_dir}`, starts the server in that temp directory, waits up to 30s for the requested host/port, and terminates/kills plus removes the temp directory in teardown. Restart adopts common checkpoint outputs.
- Sibling plugin options prove managed `--server-command`, `--server-db`, host/port, server-dir/log, oracle/target manifest, and login-script-env contracts. When oracle manifest gating is requested, a managed target must write `{manifest}` or a target manifest must be supplied.
- Sibling `profile_gate.py` fails closed unless both profiles have accepted support status; boolean equality for `option.OUTBOUND_NETWORK` and `option.PROMOTE_NUMBERS`; and equality for `database_fixture`, `database_checksum`, and `runtime_os`. These are correctness/profile constraints, not performance metrics.
- Repo-reference search found no academic paper, DOI, arXiv item, or external performance literature cited by the performance artifacts. Do not add a web/literature dependency; report `none found` for academic literature.
- Existing upstream-runtime locations are repo-local references to ToastStunt, not literature: WSL binary/core; GitHub `lisdude/toaststunt`; `src/include/structures.h` (`Var`, type tags, refcounted complex values); `src/eval_env.cc` (TYPE_NONE initialization); and general VM/runtime files such as `src/execute.cc`, `src/tasks.cc`, `src/list.cc`, `src/map.cc`, and Ruby source tests. The performance-specific note uses Toast's refcount==1 append behavior as design prior art.
- `plans/byecode-vm-performance-plan.md` is a stale pre-harness plan: it says no VM benchmarks exist and names old `vm/operations.go` surfaces. The benchmark was subsequently added and several proposed dispatch/call/line-scan surfaces were addressed. Treat it as abandoned/superseded planning, not current baseline authority.
- No benchmark or conformance command has been executed by this worker.
- Current blocker: none.
- Next action: complete exact search-location/term inventory, verify tracked/untracked status and remaining historical statuses, then draft the report.

## 2026-07-15 prior-art researcher checkpoint 8

- Tracked/untracked audit of key Barn evidence:
  - tracked: `vm/perf_bench_test.go`, `scripts/perf-compare.ps1`, `scripts/benchmark-mongoose.ps1`, June 27 baseline/raw file, June 22 builtin experiment, July 14 Mongoose experiment, and convergence plan;
  - untracked/pre-existing: `notes/perf-execution-log.md`, both `plans/blazing-fast-barn*.md`, `plans/byecode-vm-performance-plan.md`, concurrency recovery ledger, and `scripts/run-bottleneck-finder.ps1`.
- Existing performance-related branches include current-master-equivalent campaign branches plus unmerged `work/mvcc-concurrent-moo`, `exp/floor-cache`, `exp/floor-shard`, `exp/lazy-txn`, `perf/unbox-value`, remote string-append ordering, and the new campaign worktree. Only `perf/c0-baseline`, `perf/c1-add-routing`, and `experiment/verb-call-performance` are directly reported by `git branch --merged master`; C2/C5 have squash/equivalent commits on master while their branch tips retain extra report/fix history.
- Independently verified unmerged scheduler/store branch results:
  - `c09bcaedc12b2065050b9af9636b4da3e0ddac47` (`exp/floor-shard`): 16-way sharded live-read floor registry; commit records mutex profile at 88% contention and 32-worker speedup ~1.17x -> ~1.98x, still below the campaign target. Not on master.
  - `071210c0ed887222c3dcedd24e72f99eae805c68` (`exp/lazy-txn`): lazy transaction allocation reduced GC share ~35% -> 24%, but 32-worker result averaged 1.86x versus ~2.04x baseline, within noise; explicitly HOLD for combination, not standalone promotion. Not on master.
  - `4ec7bd53b75098ad9cab121aca2f43b711c656de` (`exp/floor-cache`): cached exact per-shard minimum reduced single-thread commit ~18-20 us -> ~10 us while 32-worker absolute throughput stayed unchanged; kept on experiment branch as latency win, explicitly not the parallelism lever. Not on master.
- Evidence line-location searches have been captured for all core benchmark commands/metrics, noise, Mongoose slices, managed WSL commands, managed-server lifecycle, and profile-gate keys. These will appear in the report's explicit locations/terms section.
- No current measurements were produced; no blocker exists.
- Next action: finish the no-hit/search-term audit and draft the final report from the verified evidence.

## 2026-07-15 prior-art researcher checkpoint 9

- Holdout/budget search terms used across both repositories: `holdout`, `evaluation set`, `performance budget`, `latency budget`, `memory budget`, `throughput budget`, `SLO`, `p95`, `p99`, `percentile`, `confidence interval`.
- No designated holdout or evaluation surface, percentile/tail-latency target, or confidence-interval protocol was found. The old untracked blazing-fast plan has ratio targets and the tracked Mongoose experiment has acceptance thresholds, but both use the same named benchmark workload for selection and validation; neither defines a holdout.
- Current Barn `master` has exactly one Go benchmark entry point by `func Benchmark` search: `vm/BenchmarkVM`. Historical conformance-loader benchmark claims in the stale byecode plan are not present in current Go sources.
- Sibling `.tmp/bench-notes.md` is an untracked working precursor to the tracked socket report. `.tmp/bench.py` is a near-duplicate of tracked `bench/bench.py` but lacks two small current assertions/initializations; tracked `bench/bench.py` is the authority.
- Old untracked `reports/test-performance-report.md` measures conformance harness overhead, not Barn engine execution. It proposed `time uv run pytest ...` against an externally running server and expected rather than demonstrated 10-15x improvements. Sibling commit `480931541d4fbb35529dd62c8434225ede1750d0` did land session-scoped connection/timeout changes, but its message also reports expected rather than sampled speedup. This is historical harness prior art, not a current Barn baseline, and its manual timing command is not the campaign's managed conformance path.
- The `.tmp/mongoose-convergence/` raw deployment directories still exist for Toast baseline, Barn baseline/profile repeat, map slice, and property slices 2-5. They contain summary JSON, event logs, transcripts, disposable DB/checkpoints, and Barn debug vars/heap profiles.
- Directly read safe `summary.json` files (not credential-bearing transcripts). They independently confirm the pinned DB and engine hashes, 180-second settle, all tracked metrics, and RSS sequence: Toast 311640064; Barn baseline 1882996736; profile repeat 2010251264; map slice 2133549056; kept property slices 1837264896 then 1295065088; rejected property slices 1469816832 then 1387020288 bytes.
- Raw socket-benchmark per-repeat samples were not found; only the tracked report's min/median summary and claim of two stable consecutive runs.
- No blocker exists. The research evidence is sufficient to draft the required report without running a baseline or experiment.
- Next action: draft the report, verify its scope/content, run applicable hygiene checks, stage only the two authorized files, and commit.
## Checkpoint 10: report validation and commit

- The staged index contained exactly `notes-barn-performance-campaign.md` and `reports/barn-performance-prior-art-2026-07-15.md`; `git diff --cached --check` passed.
- The report contains all eight required sections and explicitly leaves future profiling surfaces unranked.
- No benchmark, oracle, conformance, baseline, candidate experiment, or holdout run was performed.
- Commit `a374673220455c2031edc9c42e42922fb2a58853` was created with subject `docs: record Barn performance prior art`; it contained only the two authorized documentation files.
- The tracked worktree was clean immediately after that commit, and the branch was one commit ahead of `origin/master`; unrelated untracked files remained untouched.
- Current blocker: none. The checkpoint note itself must be folded into the same documentation commit before final handoff.

## 2026-07-15 frame worker checkpoint

- Control prompt read: `prompts/barn-performance-frame.md`.
- Current knowledge: this worker is authorized only to establish the exact seven-row `BenchmarkVM` development baseline, create the campaign ledger, update these notes, and commit the four named authorized artifacts.
- Observed so far: no repository inspection, benchmark run, protocol read, source edit, holdout access, candidate work, or commit has occurred.
- Current blocker/state: prerequisite verification remains pending: applicable protocol instructions, prior-art commit/artifacts, branch/HEAD/tracked and untracked state, authorized-path conflicts, and `benchstat` availability must be checked in the prompt's required order.

## 2026-07-15 frame worker checkpoint 2

- Read and obeyed Barn `AGENTS.md` plus the available `protocols:campaign` and `protocols:experiment` instructions; the named `protocols:experiment-worker` skill was not separately available.
- Read the prior-art report and campaign notes at commit `86f1580f360ec33a755f0c0ff58bc8d165794de7` before repository work.
- Verified branch `master` at exact HEAD `86f1580f360ec33a755f0c0ff58bc8d165794de7`; no tracked source differs from that report commit. Extensive unrelated untracked state remains untouched.
- Confirmed existing `benchstat` at `C:\Users\Q\go\bin\benchstat.exe`; nothing was installed or updated.
- The exact seven-row `-count=10 -cpu=1` baseline command completed successfully with exit code 0. Neither `nested_1k` nor `list_iter_1M` ran.
- The exact `benchstat` command completed successfully. Development geomean is `11.98m sec/op`; row medians are int `31.91m`, float `32.94m`, string concat `840.1µ`, list append `1.021m`, list index `80.34m`, builtin abs `13.56m`, and `tostr` `36.12m`.
- Noise concern: `list_append_10k` has a `±16%` timing interval; every other development row is `±0–4%`. No row was rerun and no warmup was added.
- Created `experiments/INDEX.md` with the immutable frame, baseline and artifact links, authority boundary, substantiated correctness/conformance commands, an empty candidate table, and a Frame log stating no candidate or holdout work occurred.
- Worker commit: this Frame commit containing the ledger, raw baseline, summary, and this notes update; its exact immutable hash will be reported after Git creates it.
- Current campaign status: Frame complete; 0/8 triage probes and 0/3 full experiments consumed, no candidate proposed or ranked, and the holdout remains untouched.
- Check result: `go test ./...` exited 1 only at the pre-existing tracked scheduler regression `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`; every other listed package, including `barn/vm`, passed. The campaign correctness gate is therefore currently red and must pass before any full experiment can satisfy the keep threshold.
- Hygiene result: authored Markdown passes `git diff --cached --check`. The two exact Windows command-output artifacts retain CRLF and the Go/benchstat-emitted padded `cpu:` line; a whole-staged-diff whitespace check reports those generated bytes, so they were not rewritten.
- Current blocker: none.
- Next required action: none for this Frame worker; return the exact commit identity to the campaign manager without pushing.

## 2026-07-15 profile worker checkpoint

- Control prompt read: `prompts/barn-performance-profile.md`.
- Current knowledge: this worker is authorized only to collect the exact seven-row development CPU and allocation profiles, write the evidence-only profile report, update the Instrumentation 0 ledger and these notes, and commit the nine named authorized paths.
- Observed so far: no repository identity or worktree verification, protocol or Go skill read, profile command, holdout access, source/test/benchmark edit, conformance/oracle work, candidate work, or commit has occurred.
- Current blocker/state: prerequisite verification remains pending: applicable protocol and profiling instructions, committed campaign artifacts, exact framing HEAD `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`, tracked state, pre-existing untracked state, authorized-path conflicts, and pre-existence of `vm.test` must be checked before profiling.

## 2026-07-15 profile worker checkpoint 2

- Read and obeyed Barn `AGENTS.md`, the committed campaign ledger, prior-art report, full campaign notes, `protocols:campaign`, `protocols:experiment`, `golang:go-performance`, and `performant-golang:go-perf-pprof-profiling`.
- Verified branch `master` at exact framing HEAD `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`.
- The tracked worktree had no pre-existing mismatch: the only tracked modification shown is this worker's authorized checkpoint update to `notes-barn-performance-campaign.md`.
- Extensive pre-existing untracked state was inventoried and remains untouched. None of the eight new raw profile/report paths appears in that inventory; authorized-path and `vm.test` pre-existence checks remain to be completed explicitly.
- The holdout remains unopened. No profile, benchmark, source/test/benchmark edit, conformance/oracle work, candidate work, or commit has occurred.
- Current blocker: none.
- Next required action: verify authorized-path conflicts and whether `vm.test` pre-exists, then run the six exact profile commands one at a time if those checks pass.

## 2026-07-15 profile recovery checkpoint

- The first instrumentation worker's relative CPU-profile output produced no confirmed root-level profile artifact.
- The manager corrected CPU and memory collection to absolute output paths.
- No artifact path from the failed attempt was confirmed session-created, so no cleanup, deletion, move, or overwrite was authorized.
- This recovery worker has not yet run the corrected profile commands.
- Full current-state verification is the next required action.

## 2026-07-15 campaign manager checkpoint: unexpected HEAD audit pending

- Prior-art gate commit reported by the researcher: `86f1580f360ec33a755f0c0ff58bc8d165794de7`.
- Campaign frame/baseline commit reported by the frame worker: `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`; development geomean `11.98m sec/op`, `list_append_10k` timing spread `±16%`, holdout untouched, and the pre-existing scheduler ID-collision test leaves the full Go gate red.
- The first profile command used a relative CPU-profile output and produced no confirmed root-level profile artifact; the following `pprof` command failed because that artifact did not exist. No profile commit was produced.
- The durable profile prompt now uses absolute CPU and memory profile output paths and requires CPU flat/cumulative and allocation-space/object views over only the seven development rows.
- The recovery worker wrote the exact authorized checkpoint above, then found branch `master` at unexpected HEAD `c87229f66a4cf6e45234275f1eed53db8722aa77` instead of required frame commit `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`. It ran no corrected profile command and altered no profile artifact.
- The user authorized a strictly read-only scout to identify `c87229f`, compare it with `7e4f27a`, inspect this notes-only tracked diff and any authorized profile output paths, and conclude `SAFE TO ADOPT` or `DO NOT ADOPT`.
- The physical scout prompt is `prompts/barn-performance-head-audit.md`. No scout has been dispatched yet.
- Current blocker: the unexpected HEAD cannot be adopted until the authorized read-only audit establishes its ancestry and exact diff. No source, benchmark, holdout, conformance, oracle, or candidate action is authorized while that audit is pending.
- Next action: dispatch the read-only scout under the Foreman actor sequence, then decide from its exact evidence whether `c87229f` may become the campaign base.

## 2026-07-15 campaign manager correction: concurrency inference withdrawn

- The user states that nobody else is working on the Barn repository.
- The first scout's initial status showed `master` at `c87229f66a4cf6e45234275f1eed53db8722aa77` and five commits ahead of `origin/master`, while a later shorthand `@{upstream}...HEAD` count returned `0 0` even though the upstream ref resolved to `864de996a111674adfe15c330f8e85813f4641f0`.
- The manager's conclusion that refs changed concurrently was unsupported and is withdrawn. The contradictory count is treated as an invalid audit observation until reproduced with explicit commit IDs and explicit refs.
- The scout made no repository change and did not complete ancestry/diff review because its mandatory notes checkpoint conflicted with the strict no-write prompt.
- Current blocker: `c87229f` still cannot be adopted until a corrected audit proves its ancestry and exact committed diff from `7e4f27a`.
- Next action: rerun the audit with explicit commit IDs and `refs/remotes/origin/master`, record HEAD at both start and end, and use `notes-barn-performance-campaign.md` as the sole permitted checkpoint-write path if the scout's mandatory checkpoint fires.

## 2026-07-15 unexpected HEAD audit checkpoint

- Read the corrected control prompt `prompts/barn-performance-head-audit.md` and repository `AGENTS.md`; the named `protocols:scout` skill is not installed, so the prompt's read-only scout contract is being followed directly.
- Initial identity commands all exited 0: `git rev-parse --show-toplevel`, `git branch --show-current`, `git rev-parse HEAD`, `git rev-parse refs/heads/master`, and `git rev-parse refs/remotes/origin/master`.
- Initial repository identity is root `C:/Users/Q/code/barn`, branch `master`, HEAD and explicit local master `c87229f66a4cf6e45234275f1eed53db8722aa77`, and explicit remote master `864de996a111674adfe15c330f8e85813f4641f0`.
- `git status --short --branch --untracked-files=all` exited 0 and showed `master` five commits ahead of `origin/master`, this notes file as the sole tracked modification, and extensive pre-existing untracked state. No listed untracked path was changed.
- An explicit-ID ancestry/metadata/diff command batch has been issued for `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644..c87229f66a4cf6e45234275f1eed53db8722aa77`; its completion output has not yet been retrieved because this mandatory checkpoint fired.
- No test, benchmark, holdout, profile, source edit, staging, commit, ref mutation, cleanup, or other repository write was performed. This mandatory append is the sole checkpoint write.
- Current blocker: none; the audit evidence is incomplete.
- Next action: retrieve the pending explicit-ID command results, inspect the exact committed and notes diffs plus authorized profile output paths, then recapture repository identity and refs for byte-for-byte comparison.

## 2026-07-15 unexpected HEAD audit checkpoint 2

- `git merge-base --is-ancestor 7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644 c87229f66a4cf6e45234275f1eed53db8722aa77` exited 0.
- `git rev-list --reverse --ancestry-path --parents 7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644..c87229f66a4cf6e45234275f1eed53db8722aa77` exited 0 and showed a linear single-parent chain: `7e4f27a` -> `6c031a7` -> `b06e048` -> `c87229f`.
- The three intervening commits modify only seven `spec/*.md` files. Their substantive changes correct command-dispatch execute permission, task-local default/inheritance behavior, and WAIF semantics. No campaign artifact, production source, test, benchmark, or holdout path changes.
- Target commit `c87229f66a4cf6e45234275f1eed53db8722aa77` has parent `b06e0488d6aebcb7aebd47ba699b493f07e72a6e`, author Christopher Toth `<q.alpha@gmail.com>`, author date `2026-07-15T10:34:06-06:00`, and subject `docs: correct WAIF semantics against Toast`.
- An explicit `git diff --quiet` comparison exited 0 for all four frame artifacts: `experiments/INDEX.md`, `experiments/2026-07-15-vm-dev-baseline.txt`, `experiments/2026-07-15-vm-dev-baseline-summary.txt`, and `notes-barn-performance-campaign.md`.
- The current notes diff is not limited to the profile prompt's five fixed recovery facts; it also contains two earlier profile checkpoints, manager audit/correction entries, and mandatory audit checkpoints.
- All seven fresh profile/report output paths are absent. `experiments/INDEX.md` exists tracked and clean; `notes-barn-performance-campaign.md` exists tracked and modified. Neither holdout benchmark was opened.
- `git rev-list --left-right --count refs/remotes/origin/master...refs/heads/master` exited 0 with `0 5`; the explicit-ref log lists prior-art, frame, and the three documentation commits in that order.
- No tests, benchmarks, profiles, holdouts, source edits, staging, commits, ref mutations, cleanup, or writes other than the two mandatory checkpoint appends were performed.
- Current blocker: none. All adoption criteria observed so far are satisfied.
- Next action: refresh the exact notes diff after this mandatory append, then perform the required final root/branch/HEAD/local-master/remote-master reread and byte-for-byte comparison.

## 2026-07-15 adopted-base profile recovery checkpoint

- The corrected read-only audit concluded `SAFE TO ADOPT` for `c87229f66a4cf6e45234275f1eed53db8722aa77` as a linear descendant of frame commit `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`.
- The intervening committed delta is specification Markdown only and preserves campaign ledger, baseline, production/test/benchmark paths, and holdout state.
- All fresh profile/report output paths were absent at the audit.
- The existing tracked notes diff contains authorized campaign checkpoints and is the only expected pre-existing tracked modification.
- The manager corrected CPU and memory collection to absolute output paths.
- This worker has not yet run the corrected profile commands.
- Full current-state verification is the next required action.

## 2026-07-15 adopted-base profile verification checkpoint

- Read and obeyed the committed campaign ledger, prior-art report, full campaign notes, Barn `AGENTS.md`, `protocols:campaign`, `protocols:experiment`, `golang:go-performance`, and `performant-golang:go-perf-pprof-profiling`.
- Verified branch `master` at exact adopted HEAD `c87229f66a4cf6e45234275f1eed53db8722aa77`.
- Verified with exit status 0 that `c87229f66a4cf6e45234275f1eed53db8722aa77` descends from frame commit `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`.
- The tracked-state check shows only the authorized modification to `notes-barn-performance-campaign.md`.
- No corrected profile command, holdout, source/test/benchmark edit, conformance/oracle action, candidate work, staging, or commit has occurred.
- Current blocker: none.
- Next required action: complete the pre-existing untracked-state, frame-artifact, fresh-output-path, and `vm.test` verification before profiling.

## 2026-07-15 profile process/artifact recovery audit checkpoint

- Read-only audit commands completed with exit status 0: `Get-Content -Raw -LiteralPath 'C:\Users\Q\code\barn\prompts\barn-performance-profile-process-audit.md'`, `Get-Content -Raw -LiteralPath 'C:\Users\Q\code\barn\AGENTS.md'`, `Get-Content -Raw -LiteralPath 'C:\Users\Q\code\barn\prompts\barn-performance-profile.md'`, `git rev-parse --show-toplevel`, `git branch --show-current`, `git rev-parse HEAD`, and `git status --short --branch --untracked-files=all`.
- Repository identity is root `C:/Users/Q/code/barn`, branch `master`, HEAD `c87229f66a4cf6e45234275f1eed53db8722aa77`; short status reports `master...origin/master [ahead 5]`, modified `notes-barn-performance-campaign.md`, many pre-existing untracked paths, and untracked `experiments/2026-07-15-vm-dev-cpu.prof` plus `experiments/2026-07-15-vm-dev-cpu`.
- `Get-CimInstance -ClassName Win32_Process -Filter "Name = 'go.exe'" | Select-Object ProcessId, CreationDate, ExecutablePath, CommandLine | Format-List` exited 0 with no output: no running `go.exe` process was observed.
- `Get-CimInstance -ClassName Win32_Process -Filter "Name = 'vm.test.exe' OR Name = 'vm.test'" | Select-Object ProcessId, CreationDate, ExecutablePath, CommandLine | Format-List` exited 0 with no output: no running `vm.test.exe` or `vm.test` process was observed.
- No test, benchmark, profile reader, conformance command, oracle, process-control action, staging, or commit occurred.
- Current blocker: none; this mandatory notes checkpoint temporarily paused the still-incomplete metadata audit.
- Next action: inspect only the exact authorized output-path metadata/Git statuses and repository-root/`vm` test-binary metadata, then return the required binary conclusion and exact evidence.

## 2026-07-15 campaign manager checkpoint: isolated profile rerun authorized

- The completed recovery audit concluded `INACTIVE - ARTIFACT PRESENT`: no `go.exe`, `vm.test.exe`, or `vm.test` process was running.
- The incomplete CPU attempt created untracked `experiments/2026-07-15-vm-dev-cpu.prof` (28,943 bytes) and untracked extensionless `experiments/2026-07-15-vm-dev-cpu`.
- Pre-existing untracked root `vm.test.exe` was created on 2026-06-22 and its last-write time changed to the failed profile attempt's completion time, proving that the attempt overwrote that disposable binary.
- The user explicitly authorized deleting all three residues: root `vm.test.exe`, extensionless `experiments/2026-07-15-vm-dev-cpu`, and unverified `experiments/2026-07-15-vm-dev-cpu.prof`.
- The durable profile prompt now requires exact one-path-at-a-time cleanup, then isolates all Go profile/test-binary output under `C:\Users\Q\AppData\Local\Temp\barn-performance-profile-c87229f-20260715`.
- Each long-running `go test` call must retain its execution-session identifier and poll that same session to a real exit code. Only verified `.prof` files are copied into `experiments`; root `vm.test.exe` must remain absent.
- After pprof extraction and report/ledger/notes updates, the worker must verify the exact resolved temporary directory and remove only that directory recursively.
- Current blocker: none; the cleanup and isolated rerun have not yet been dispatched.
- Next action: dispatch an experiment worker on the updated profile prompt, complete the source-free profile artifact commit, then use the committed evidence to select targeted Go optimization skills and hypotheses.

## 2026-07-15 adopted-base isolated profile rerun checkpoint

- The corrected read-only audit concluded `SAFE TO ADOPT` for `c87229f66a4cf6e45234275f1eed53db8722aa77` as a linear descendant of frame commit `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`.
- The intervening committed delta is specification Markdown only and preserves campaign ledger, baseline, production/test/benchmark paths, and holdout state.
- All fresh profile/report output paths were absent at the audit.
- The existing tracked notes diff contains authorized campaign checkpoints and is the only expected pre-existing tracked modification.
- The user explicitly authorized deleting the disposable overwritten root `vm.test.exe`, the session-created extensionless CPU test binary, and the unverified CPU profile before the isolated rerun.
- The manager corrected CPU and memory collection to absolute output paths.
- This worker has not yet run the corrected profile commands.
- Full current-state verification is the next required action.

## 2026-07-15 isolated profile rerun verification checkpoint

- Read and obeyed the campaign ledger, prior-art report, full campaign notes, Barn `AGENTS.md`, `protocols:campaign`, `protocols:experiment`, `golang:go-performance`, and `performant-golang:go-perf-pprof-profiling`.
- Verified branch `master` at exact adopted HEAD `c87229f66a4cf6e45234275f1eed53db8722aa77`.
- Verified with exit status 0 that `c87229f66a4cf6e45234275f1eed53db8722aa77` descends from frame commit `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`.
- No cleanup or corrected profile command has run. No holdout, source/test/benchmark edit, conformance/oracle action, candidate work, staging, or commit has occurred.
- Current blocker: none; full tracked-state, pre-existing untracked-state, frame-artifact, fresh-output-path, cleanup-residue, and isolated-directory verification remains incomplete.
- Next required action: finish the current-state verification before any cleanup or profile write.

## 2026-07-15 isolated profile rerun verification checkpoint 2

- The tracked-state check shows only the authorized modification to `notes-barn-performance-campaign.md`; its diff consists of authorized campaign/profile/audit checkpoints and contains no conflicting pre-existing change.
- The full untracked inventory matches the extensive pre-existing state recorded by the campaign plus the exact expected cleanup residues; no unrelated path was changed.
- Explicit commit-to-commit checks exited 0 and show no change from frame commit `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644` to adopted HEAD `c87229f66a4cf6e45234275f1eed53db8722aa77` for `experiments/INDEX.md`, both committed development baseline artifacts, or the committed notes artifact.
- Exact residue checks show that root `vm.test.exe`, `experiments/2026-07-15-vm-dev-cpu`, and `experiments/2026-07-15-vm-dev-cpu.prof` all exist, matching the three user-authorized cleanup paths.
- `experiments/2026-07-15-vm-dev-cpu-top.txt` and `experiments/2026-07-15-vm-dev-cpu-cum.txt` are absent.
- No cleanup or corrected profile command has run.
- Current blocker: none; the remaining fresh-output paths and exact isolated-directory nonexistence still require verification before cleanup.
- Next required action: finish those exact checks, then remove the three authorized residues one at a time if verification remains clean.

## 2026-07-15 isolated profile cleanup checkpoint

- Verified all remaining fresh output paths and `reports/barn-vm-current-profile-2026-07-15.md` are absent.
- Verified the exact isolated directory `C:\Users\Q\AppData\Local\Temp\barn-performance-profile-c87229f-20260715` was absent before creation.
- Removed only the three user-authorized residues, one at a time, with the required `Remove-Item -LiteralPath ... -Force` calls; each exited 0.
- Verified separately that root `vm.test.exe`, `experiments/2026-07-15-vm-dev-cpu`, and `experiments/2026-07-15-vm-dev-cpu.prof` are all absent after cleanup.
- The attempted directory creation `New-Item -ItemType Directory -LiteralPath 'C:\Users\Q\AppData\Local\Temp\barn-performance-profile-c87229f-20260715'` failed because this PowerShell reports that `New-Item` has no `-LiteralPath` parameter.
- No profile command has run, and the exact isolated directory remains absent.
- Current blocker: none; directory creation must be retried with `New-Item -Path` using the same exact fixed path before profiling.
- Next required action: create the exact isolated directory, then run the exact CPU profile command.

## 2026-07-15 verified CPU continuation checkpoint

- The isolated CPU command completed with real exit code 0 in retained session `73512`.
- `go test -cpuprofile` recreated root `vm.test.exe` despite `-outputdir`.
- The root binary was absent immediately before the run and is therefore confirmed session-created and disposable.
- The verified isolated CPU profile has not yet been copied or read.
- No holdout, source, conformance, oracle, candidate, staging, or commit action occurred.
- Exact state verification and root-binary cleanup are next.

## 2026-07-15 verified CPU continuation state checkpoint

- Read and obeyed Barn `AGENTS.md`, the Campaign and Experiment protocols, `experiments/INDEX.md`, `reports/barn-performance-prior-art-2026-07-15.md`, `prompts/barn-performance-profile.md`, `golang:go-performance`, and `performant-golang:go-perf-pprof-profiling`.
- Verified branch `master` at exact adopted HEAD `c87229f66a4cf6e45234275f1eed53db8722aa77`.
- Verified with exit status 0 that `c87229f66a4cf6e45234275f1eed53db8722aa77` descends from frame commit `7e4f27a9c8efd6338f2440e58e6ab2f8fe3e9644`.
- The tracked-state check shows only the authorized modification to `notes-barn-performance-campaign.md`.
- No cleanup, profile copy/read, allocation collection, holdout, source/test/benchmark edit, conformance/oracle action, candidate work, staging, or commit has occurred in this continuation.
- Current blocker: none; process, root-binary, isolated-directory, isolated CPU-profile, fresh-output, and holdout-seal verification remain incomplete.
- Next required action: finish the exact required-state checks before root-binary cleanup.

## 2026-07-15 verified CPU continuation artifact checkpoint

- Verified root `vm.test.exe` exists and no `go.exe` or `vm.test.exe` process is running.
- Verified the exact isolated directory `C:\Users\Q\AppData\Local\Temp\barn-performance-profile-c87229f-20260715` exists.
- Verified isolated `vm-dev-cpu.prof` exists and is non-empty at 28,577 bytes.
- Verified the destination CPU profile, both CPU pprof text outputs, the destination memory profile, and both allocation pprof text outputs are absent.
- No cleanup, profile copy/read, allocation collection, holdout, source/test/benchmark edit, conformance/oracle action, candidate work, staging, or commit has occurred in this continuation.
- Current blocker: none; the fresh report path and holdout-seal evidence still require verification.
- Next required action: finish those two required-state checks before root-binary cleanup.

## 2026-07-15 profile collection checkpoint

- Completed required state verification; the report path was absent and the holdout remained sealed under the verified exact seven-row CPU command with no later holdout action.
- Removed exactly the confirmed session-created root `vm.test.exe`, verified it absent, copied the isolated CPU profile, and ran both required CPU pprof views with exit status 0.
- CPU evidence: 28.27-second duration, 27.57 seconds of samples (97.52%). Barn-owned flat leaders include `barn/vm.(*VM).executeLoop` 22.92%, `barn/vm.(*VM).Execute` 12.40%, `barn/vm.(*VM).ReadByte` 9.03%, and `barn/builtins.(*Registry).dispatch` 5.80%.
- The exact allocation command ran in retained session `71570` and completed with real exit code 0. All seven development rows passed; reported per-row allocation rates ranged from 11 allocs/op for arithmetic and builtin abs to 599,912 allocs/op and 12,808,090 B/op for `tostr_200k`.
- The allocation run recreated root `vm.test.exe`; no matching process was running. Removed exactly that session-created root binary and confirmed it absent.
- Copied isolated `vm-dev-mem.prof` to `experiments/2026-07-15-vm-dev-mem.prof`.
- No allocation pprof read, report/ledger finalization, holdout, source/test/benchmark edit, conformance/oracle action, candidate work, staging, or commit has occurred yet.
- Current blocker: none.
- Next required action: run the two exact allocation pprof views one at a time.

## 2026-07-15 Instrumentation 0 completion checkpoint

- Evidence commit: this commit; its exact hash is returned in the worker handoff.
- CPU collection completed with real exit code 0 in retained session `73512`; allocation collection completed with real exit code 0 in retained session `71570`.
- CPU profile duration is 28.27 seconds with 27.57 seconds of samples (97.52%). Barn-owned flat leaders are `barn/vm.(*VM).executeLoop` 22.92%, `barn/vm.(*VM).Execute` 12.40%, `barn/vm.(*VM).ReadByte` 9.03%, and `barn/builtins.(*Registry).dispatch` 5.80%.
- Allocation-space leaders are `barn/types.(*sliceList).append` 4,909.26 MB (55.64%), `barn/types.(*strRep).appendRep` 2,605.40 MB (29.53%), and `barn/types.NewStr` 897.54 MB (10.17%).
- Allocation-object leaders are `barn/types.(*strRep).appendRep` 53,415,015 (41.98%), `barn/types.(*sliceList).append` 35,490,400 (27.89%), `barn/types.NewStr` 19,607,083 (15.41%), and `internal/strconv.FormatInt` 13,992,149 (11.00%).
- Created the eight-section evidence-only report and added only the `Instrumentation 0` round-log entry to `experiments/INDEX.md`; no candidate row was added.
- No candidate, holdout, source/test/benchmark edit, conformance, oracle, promotion, push, or branch-switch action occurred.
- Current status: profile collection and evidence writing are complete; exact temporary-directory cleanup, hygiene checks, staging, and commit remain.
- Current blocker: none.
- Next required action: resolve and verify the exact isolated temporary-directory path, remove only that directory recursively, and confirm it and root `vm.test.exe` are absent.

## 2026-07-15 Instrumentation 0 precommit checkpoint

- Resolved the isolated temporary directory to exactly `C:\Users\Q\AppData\Local\Temp\barn-performance-profile-c87229f-20260715`, removed only that directory recursively, and confirmed it is absent.
- Confirmed root `vm.test.exe` is absent after both profile collections and cleanup.
- Staged only the nine authorized paths. The staged path inventory contains exactly six raw profile/pprof artifacts, the profile report, `experiments/INDEX.md`, and this notes file.
- Authored Markdown passes `git diff --cached --check`.
- The whole staged hygiene check reports only CRLF line endings in the four exact PowerShell-generated pprof text artifacts; those generated bytes were preserved rather than rewritten.
- No candidate, holdout, source/test/benchmark edit, conformance, oracle, promotion, push, or branch-switch action occurred.
- Current status: collection, report, ledger, notes, cleanup, and applicable hygiene checks are complete; final staged-content verification and commit remain.
- Current blocker: none.
- Next required action: restage this authorized notes update, verify the exact staged diff, and commit descriptively without pushing.

## 2026-07-15 VM candidate scout checkpoint

- Current findings: Read the controlling scout prompt, repository `AGENTS.md`, Campaign protocol, Foreman protocol, scout role instructions, and the required Go performance, methodology, and pprof skills. No repository source, history, test, benchmark, profile, holdout result, or campaign ledger has been inspected yet.
- Current state: Required instruction loading is incomplete; four required Go skills remain unread. No probe, candidate, holdout, source execution, test, benchmark, profile, build, conformance, oracle command, or source edit has occurred.
- Blocker: Mandatory checkpoint interrupted instruction loading; there is no external blocker after this checkpoint is recorded.
- Next action: Finish reading the four remaining required Go skills, then verify branch `master`, expected HEAD `d9c9f4dfe7d9fe8ffe0168f8e6a6738ad04f7a7d`, tracked state, and all pre-existing untracked state before any further writing.

## 2026-07-15 VM candidate scout evidence checkpoint

- Current findings: Verified branch `master` at exact HEAD `d9c9f4dfe7d9fe8ffe0168f8e6a6738ad04f7a7d`. The only tracked diff is this scout's authorized checkpoint notes; the complete pre-existing untracked inventory was enumerated and remains untouched. Read `experiments/INDEX.md`, the prior-art gate, and the current aggregate development profile. The profile attributes aggregate CPU to VM dispatch/stack/bytecode/builtin paths and aggregate allocation churn chiefly to list append, string append, `NewStr`, integer formatting, and Builder growth; it explicitly cannot assign those aggregate shares to individual rows without isolated probes.
- Current state: Required instructions and mandatory campaign artifacts are fully read. No test, benchmark, profile, escape analysis, build, conformance, oracle, holdout, candidate, or source execution has occurred. No source/test/benchmark/ledger file has been edited.
- Blocker: none.
- Next action: Perform the prompt's read-only investigation of the exact source owners, directly relevant tests/benchmark/compiler consumers, and Git/report/ledger history while excluding sealed holdout results and execution.

## 2026-07-15 VM candidate scout source-owner checkpoint

- Current findings: Read every exact measured production owner named by the scout prompt. The dispatch loop performs an immediate-op test, opcode switch call, tick predicate and context-tick sync for counted opcodes, yield check, and limit check per instruction (`vm/vm.go`); stack and bytecode reads are small methods over cached frame state (`vm/stack.go`). Builtin calls read two operands, slice fixed args directly from the VM stack, separately query line-sync metadata, then call the registry, where the same entry is bounds-checked again before protected redirect and signature validation (`vm/op_misc.go`, `builtins/registry.go`). String and list append use alias-safe copy-on-write frontier watermarks and return new heap headers even on backing reuse (`types/str.go`, `types/list.go`); these header/aliasing semantics are correctness constraints. `tostr` builds through `strings.Builder`, formats each value to a Go string, checks the materialized result length, then boxes it with `NewStr` (`builtins/types.go`).
- Current state: Exact production owners are read; compiler/bytecode consumers, development benchmark definition excluding holdout result inspection, relevant correctness tests, and Git/report/ledger history remain. No prohibited execution or edit occurred.
- Blocker: none.
- Next action: Read compiler and bytecode consumers that establish emitted opcode/terminator/peephole shapes, then the development benchmark source without opening holdout results, then contract tests and adjacent history.

## 2026-07-15 VM candidate scout compiler-benchmark checkpoint

- Current findings: The compiler emits a frame-popping terminator for all compiled programs and fork extraction appends `OP_RETURN_NONE`; the dispatch loop relies on that invariant. Fixed-argument builtins compile args directly followed by `OP_CALL_BUILTIN <id> <argc>`, while splice calls build lists through list append/extend. Effect-context self-accumulation compiles `x = x + expr` through `OP_STRING_APPEND` for numeric and string rows, and self-list append compiles through `OP_LIST_APPEND`; these compiler choices couple several development rows to the same production surfaces. The seven allowed development workload definitions and benchmark harness were read without inspecting holdout results or executing anything. Each timed iteration creates a fresh task context and VM, then runs a precompiled program; compilation and store creation are outside the timer.
- Current state: Compiler/bytecode consumers and development benchmark source are read. Relevant correctness and aliasing tests plus adjacent Git/report/ledger history remain.
- Blocker: none.
- Next action: Read the focused VM/string/list/value/builtin/bytecode contract tests that constrain the measured owners, then search Git and durable reports/ledgers for every adjacent kept, rejected, skipped, superseded, or blocked optimization.

## 2026-07-15 VM candidate scout contract checkpoint

- Current findings: Focused tests require list and string aliases to retain old content across divergent appends, including aliases captured in lists, maps, properties, slices, and multi-binding chains. Numeric/string self-accumulation through `OP_STRING_APPEND` must preserve int, float, string, and strict mixed-type behavior. VM tests pin implicit return-zero and the universal terminator invariant. Builtin tests pin invalid-ID `E_VERBNF`, arg count/type validation, protected redirect/fallthrough/deny behavior, and concurrent protected-set reads. Value tests pin zero-allocation scalar int construction/read, exact scalar representation, distinct cross-type values, explicit `None`/`Unbound`, and string equality semantics. Bytecode tests pin empty-program termination plus absolute handler-IP rebasing and relative jump preservation in extracted fork bodies.
- Current state: Focused correctness/aliasing/value/builtin/bytecode contracts are read. The remaining scout work is the adjacent Git/report/ledger audit, eligibility synthesis, report writing, documentation/Git hygiene, notes finalization, and commit.
- Blocker: none.
- Next action: Search Git history and durable performance artifacts for every nearby kept, rejected, skipped, superseded, or blocked VM optimization, mapping each candidate family to its exact prior cause and current eligibility.

## 2026-07-15 VM candidate scout prior-art checkpoint

- Current findings: Git and durable reports confirm the end-of-code check removal, cached frame, scalar de-box, fixed-argc stack-window args, lock-free protected snapshot, ID-indexed builtin entries, inline validation, numeric-first self-accumulation, O(1) list byte sizing, and watermark-backed list/string growth are already implemented. Switching self-accumulation from `OP_STRING_APPEND` to `OP_ADD` remains blocked/dead because its float-overflow and `PROMOTE_NUMBERS` behavior still differs and the required Toast semantic verification has not changed. The old O(n^2) append hypothesis is dead/already fixed, but per-append string/list header allocation was explicitly left unexecuted; the new current profile now measures those exact headers as dominant allocation owners, so that narrower constant-factor family is eligible. Smaller initial VM stack/frame capacities and loop/exception preallocation removal were previously rejected and current `NewVM` evidence is only 0.67% of allocation space, so they remain dead/dominated. Registration-time protected-builtin caching remains invalid because runtime options mutate the protected set; the current atomic snapshot is already implemented.
- Current state: Source, compiler, development benchmark, focused contracts, Git history, and adjacent durable performance records are substantially complete. No execution or source edit occurred.
- Blocker: none.
- Next action: Finish the eligibility synthesis against exact current line citations, choose no more than eight distinct one-surface hypotheses, rank the non-dominated set, specify four or fewer first-round probes, write the authorized report, run documentation/Git hygiene, finalize notes, and commit only the two authorized paths.

## 2026-07-15 VM candidate scout completion

- Current findings: Created `reports/barn-vm-candidate-scout-2026-07-15.md` with exact repository/search identity, measured source-shape findings and correctness constraints, an explicit prior-art eligibility table, six one-surface falsifiable hypotheses, separate CPU/allocation non-dominated rankings, first-round exclusions, the pre-existing scheduler ID-collision gate note, and a four-probe recommended triage round.
- Scout commit/status: report and campaign-note deliverables are complete and `git diff --check` passes; the commit containing this line is the scout commit whose exact hash is returned to the foreman. No push, branch switch, promotion, or ledger update is authorized.
- Execution record: no triage probe, candidate, holdout, test, benchmark, profile, escape analysis, build, conformance, oracle, or production/source execution occurred. No source, test, benchmark, or `experiments/INDEX.md` edit occurred.
- Current blocker: none.
- Next required action: stage only `reports/barn-vm-candidate-scout-2026-07-15.md` and `notes-barn-performance-campaign.md`, verify the exact staged diff, commit descriptively, and return the exact commit hash without pushing.

## 2026-07-15 VM ledger ideation checkpoint

- Current findings: Read the controlling ideation prompt, Barn `AGENTS.md`, the `protocols:researcher` role instructions, the Campaign protocol, `experiments/INDEX.md`, the candidate scout report, and the full campaign notes. The manager-authorized candidate rows and exact Round 1 preregistration text are ready to be recorded without running any candidate work.
- Current state: No branch/HEAD/worktree verification, test, benchmark, profile, build, conformance, oracle, holdout, source command, staging, or commit has occurred. This mandatory checkpoint append is the only repository write by this worker so far.
- Current blocker: none.
- Next action: Verify branch `master`, expected HEAD `aa9370312039dca9484a5180b4b30135833d9d1c`, and complete tracked/untracked state before any ledger edit.

## 2026-07-15 VM ledger ideation completion

- Ideation commit/status: The authorized ledger and campaign-note updates are complete; the commit containing this line is the ideation commit whose exact hash is returned to the campaign manager.
- Accepted Round 1 probe order: C4, C2, C3, C1. C5 and C6 remain eligible but deferred after Round 1.
- Campaign budget remains 0/8 triage probes and 0/3 full experiments consumed. No candidate source is authorized for integration by this preregistration.
- Execution status: No test, benchmark, profile, build, conformance, oracle, holdout, source command, push, branch switch, or promotion occurred.
- Current blocker: none.
- Next action: Run documentation/Git hygiene checks, stage only `experiments/INDEX.md` and `notes-barn-performance-campaign.md`, verify the exact staged diff, and commit descriptively without pushing.

## 2026-07-15 campaign manager checkpoint: C4 experiment preparation

- Ideation ledger commit `031afb2fce36729798d258bc4ff7fb1e937b442f` records candidates C1-C6 and exact Round 1 order C4, C2, C3, C1; budget remains 0/8 triage probes and 0/3 full experiments.
- The Experiment protocol is now fully read. C4 must run on a dedicated experiment branch/worktree, with evaluator paths sealed and a preregistration commit created before any source delta.
- C4 is one variable only: fast-path exactly the one-argument `tostr` case without the concatenation Builder while preserving zero/multi-argument behavior, `valueToStr`, context limit refresh, final string-size checking, and MOO value semantics.
- C4 triage is directional only and cannot promote source. Its focused development row is `BenchmarkVM/tostr_200k`; the sealed holdout remains forbidden.
- The preregistered killing threshold is fewer than 150k allocs/op removed or no directional timing improvement. A surviving triage branch still requires a separate full Experiment run, independent verification, and explicit user authorization before any holdout.
- The full Go gate remains red at the pre-existing scheduler ID-collision test; no candidate can satisfy the full keep/promotion gate until that independent prerequisite is green.
- Current blocker: no dedicated C4 branch/worktree has been prepared yet.
- Next action: dispatch a coordination worker to create a clean dedicated C4 branch/worktree from `031afb2fce36729798d258bc4ff7fb1e937b442f`, then activate one experiment worker in that worktree with exact preregistration, evaluator seal, baseline/source/probe, revert-or-commit, ledger, and record duties.

## 2026-07-15 C4 worktree preparation checkpoint

- Current Git findings: The main worktree is on `master` at exact HEAD `031afb2fce36729798d258bc4ff7fb1e937b442f`. Its only tracked modification is the authorized `notes-barn-performance-campaign.md` checkpoint; the complete unrelated untracked inventory reported by `git status --short --untracked-files=all` remains untouched. The local branch `experiment/barn-vm-c4-tostr-20260715` is absent.
- Current state: No branch or worktree has been created. Nothing has been staged, committed, fetched, pulled, pushed, merged, rebased, cherry-picked, tested, benchmarked, profiled, or implemented.
- Current blocker: This mandatory checkpoint interrupted item 2 before remote branch absence was verified; no external blocker is known yet.
- Next action: Resume item 2 by identifying configured remotes and verifying the exact branch is absent remotely, then continue the prompt's path and worktree-registration checks only if that verification passes.

## 2026-07-15 C4 worktree preparation checkpoint 2

- Current Git findings: The exact branch was absent locally and from configured remote `origin`; the exact target path was absent and unregistered. The authorized `git worktree add -b` command succeeded. Git now registers `C:/Users/Q/code/barn-worktrees/c4-tostr-20260715` on `refs/heads/experiment/barn-vm-c4-tostr-20260715` at `031afb2fce36729798d258bc4ff7fb1e937b442f`. Direct checks inside the new worktree confirm the exact branch and HEAD, and `git status --short --untracked-files=all` is empty.
- Current state: No C4 implementation or experiment execution occurred. Nothing was staged, committed, fetched, pulled, pushed, merged, rebased, cherry-picked, tested, benchmarked, or profiled. The main worktree re-verification confirms branch `master` and exact HEAD `031afb2fce36729798d258bc4ff7fb1e937b442f`.
- Current blocker: This mandatory checkpoint interrupted item 6 before the final complete main-worktree tracked/untracked status could be rechecked; no external blocker is known.
- Next action: Run the final main-worktree `git status --short --untracked-files=all`, verify only the authorized notes path is tracked-modified and unrelated untracked files remain, then return the required preparation evidence.

## 2026-07-15 C4 sample-count correction checkpoint

- Current findings: The researcher, Campaign, and Experiment protocols and `AGENTS.md` are fully read. The Experiment protocol requires at least three runs per side for a directional signal. `experiments/INDEX.md` still contains the authorized stale C4 wording `pending two-sample` for the `tostr_200k` source probe and has no pre-execution correction paragraph.
- Current Git state: The main worktree is on `master` at exact HEAD `031afb2fce36729798d258bc4ff7fb1e937b442f`. Before this checkpoint, the only tracked modification was this authorized campaign notes file; unrelated untracked state was inventoried and left untouched. No test, benchmark, profile, build, conformance, oracle, holdout, source, push, switch, or promotion command ran.
- Current blocker: This mandatory checkpoint interrupted the documentation correction before `experiments/INDEX.md` was edited; no external blocker is known. Existing campaign-note convention records a created commit and then folds the checkpoint into that documentation commit.
- Next action: Apply only the two exact authorized `experiments/INDEX.md` text changes, append the correction state to these notes, run documentation/Git hygiene checks, stage only the two authorized paths, verify the staged diff, and create the descriptive correction commit without running prohibited commands.

## 2026-07-15 C4 sample-count correction

- Correction: Before any C4 data collection, the candidate row now specifies a three-sample-per-side `tostr_200k` directional source probe, and the Round 1 preregistration records that the Experiment protocol's minimum replaced the scout's proposed two samples.
- Correction commit: `fd60567ffb70e5ef45fa169325c1d6d7a17347e6` (`docs: correct C4 directional sample count`) contains the corrected ledger wording and pre-execution protocol paragraph.
- Execution state: C4 remains unrun. No C4 benchmark, source edit, preregistration, holdout, or candidate command has run.
- Budget state: 0/8 triage probes and 0/3 full experiments remain consumed.
- Current blocker: none.
- Next action: Commit this exact correction-commit identity as a notes-only campaign record without pushing, switching, or promoting.

## 2026-07-15 C4 worktree fast-forward checkpoint

- Current Git findings: Before this checkpoint, the main worktree was verified on `master` at exact HEAD `12acccaa82b3d893275d4ff9498a1ed5f8bd1468` with empty tracked status. `git worktree list --porcelain` registers `C:/Users/Q/code/barn-worktrees/c4-tostr-20260715` at `031afb2fce36729798d258bc4ff7fb1e937b442f` on `refs/heads/experiment/barn-vm-c4-tostr-20260715`; a direct check inside that worktree confirms branch `experiment/barn-vm-c4-tostr-20260715`.
- Current state: The required fast-forward has not run. No C4 implementation, test, benchmark, profile, holdout, source/report/experiment artifact, fetch, pull, push, rebase, cherry-pick, stage, or commit occurred. This checkpoint append is not staged or committed.
- Current blocker: This mandatory checkpoint interrupted the experiment-worktree precondition checks before its direct HEAD and complete cleanliness were verified; no external blocker is known.
- Next action: Verify the experiment worktree's exact HEAD, complete cleanliness, and linear ancestry from `031afb2fce36729798d258bc4ff7fb1e937b442f` to `12acccaa82b3d893275d4ff9498a1ed5f8bd1468`, then run the prompt's exact `git merge --ff-only` command only if every precondition passes.

## 2026-07-15 campaign manager checkpoint: C4 ready to launch

- The dedicated worktree `C:\Users\Q\code\barn-worktrees\c4-tostr-20260715` is registered on `experiment/barn-vm-c4-tostr-20260715`, fast-forwarded to corrected base `12acccaa82b3d893275d4ff9498a1ed5f8bd1468`, and clean.
- Main `master` remains at the same base; its only tracked modification is authorized campaign checkpoint notes.
- The physical experiment prompt is `prompts/barn-vm-c4-triage.md`.
- C4 preregistration freezes one direct `len(args) == 1` branch in `builtinTostr`; no helper, representation change, test edit, benchmark edit, or broader string change is allowed.
- Baseline and candidate each use exactly three `BenchmarkVM/tostr_200k` samples at `-cpu=1`. Primary metric is allocs/op with a minimum reduction of 150,000; median sec/op must improve directionally and B/op must not increase.
- Every tracked `*_test.go` path under `builtins` and `vm`, including `vm/perf_bench_test.go`, is sealed in a committed evaluator inventory before source work.
- Preregistration/baseline evidence must be committed before the source commit. Targeted contracts are `go test ./builtins ./vm`.
- A survivor remains only on the experiment branch as `triage-survivor; confirmation required`. A rejected source commit is explicitly reverted on that branch before the final evidence commit.
- Budget remains 0/8 triage probes until C4 actually executes and commits its result. Holdout, promotion, integration, push, and main-worktree modification remain forbidden.
- Current blocker: none; the experiment worker has not yet been activated.
- Next action: activate one native experiment worker for the prepared C4 worktree, execute the frozen preregistration/baseline/source/contracts/candidate/evaluator/outcome sequence, then independently review the committed record before selecting C2.

## 2026-07-15 campaign manager checkpoint: C4 preregistration invalidated

- C4 preregistration commit `2c06488c23ef9009562ec56ad31b22e4aa4d4669` was invalid before any source change because its frozen source variable contradicted the current source shape.
- Exact mismatch 1: current `builtinTostr` declares and runs the Builder loop before `UpdateContextLimits(ctx)`, while the frozen record required the one-argument branch after the refresh and before the Builder path.
- Exact mismatch 2: current `valueToStr` returns only `string`, while the frozen record required propagation of a nonexistent existing error.
- Closure commit `781c4fa7af10791ee125648bd46083732eb03f63` records the invalid preregistration on `experiment/barn-vm-c4-tostr-20260715`; that branch is clean.
- No source edit/commit, targeted test, candidate measurement, evaluator diff, holdout, promotion, or integration occurred. The committed baseline/preregistration artifacts are not candidate evidence.
- The C4 hypothesis remains untested. Budget remains 0/8 triage probes and 0/3 full experiments.
- Current blocker: the invalid closure evidence is not yet recorded on `master`, and no corrected C4 branch/worktree/preregistration exists.
- Next action: commit an exact record-only copy of the invalid-preregistration evidence on `master`, then prepare a separate corrected C4 branch/worktree/preregistration citing the actual source shape.

## 2026-07-15 C4 invalid-record integration checkpoint

- Current findings: Read the controlling integration prompt, repository `AGENTS.md`, `protocols:researcher`, Campaign protocol, Experiment protocol, and the current campaign notes through the C4 invalidation checkpoint.
- Authorized task: record-only integration of the invalid C4 preregistration evidence from closure commit `781c4fa7af10791ee125648bd46083732eb03f63`; no C4 implementation or measurement is authorized.
- Current state: No branch, HEAD, tracked/untracked state, experiment-worktree identity, ancestry, cleanliness, evidence artifact, or diff verification has occurred. No experiment artifact, ledger, source, test, benchmark, profile, build, conformance, oracle, holdout, staging, commit, push, branch switch, merge, rebase, cherry-pick, fetch, or pull action has occurred.
- Current blocker: none external; prerequisite repository-state verification remains pending.
- Next action: verify main branch `master`, exact HEAD `12acccaa82b3d893275d4ff9498a1ed5f8bd1468`, and complete tracked/untracked state, requiring this authorized campaign notes file to be the only tracked modification.

## 2026-07-15 C4 invalid-record integration checkpoint 2

- Main verification: branch `master`, exact HEAD `12acccaa82b3d893275d4ff9498a1ed5f8bd1468`; the authorized campaign notes file is the sole tracked modification and the complete unrelated untracked inventory remains untouched.
- Evidence-worktree verification: branch `experiment/barn-vm-c4-tostr-20260715`, exact closure HEAD `781c4fa7af10791ee125648bd46083732eb03f63`, completely clean; its parent is exact preregistration commit `2c06488c23ef9009562ec56ad31b22e4aa4d4669`.
- Evidence inspection: Read the four exact committed experiment artifacts. The closure record states the two frozen source-shape mismatches, no source/candidate/holdout execution, zero budget use, and an untested retry-eligible hypothesis.
- Ledger delta verification: Relative to base `12acccaa82b3d893275d4ff9498a1ed5f8bd1468`, the experiment branch changes only C4's status/evidence/cause and appends the Round 1 non-execution/zero-budget statement required by the integration prompt.
- Current state: No integration artifact or ledger edit, staging, commit, source/test/benchmark/profile/build/conformance/oracle/holdout execution, push, switch, merge, rebase, cherry-pick, fetch, or pull has occurred.
- Current blocker: none.
- Next action: create the four exact evidence artifacts with `apply_patch`, verify their hashes against closure commit `781c4fa7af10791ee125648bd46083732eb03f63`, then apply only the verified ledger delta and append the required completion section to these notes.

## 2026-07-15 C4 invalid-record integration checkpoint 3

- Created all four authorized evidence artifacts with `apply_patch`.
- Exact-hash result: `experiments/2026-07-15-c4-tostr-triage.md` already matches closure blob `05523e62a0c9c83b50b050468b71e7b80b77a6da` exactly.
- The three generated `.txt` artifacts initially have correct visible content but LF working-tree line endings, so their blob hashes do not yet match the closure commit's CRLF blobs. They are not accepted as exact evidence yet.
- Verified no Git attribute specifies text/EOL conversion for those paths and repository `core.autocrlf` is `false`. The installed `unix2dos` command is available for the required mechanical line-ending normalization after the `apply_patch` content edit.
- Current state: No ledger edit, staging, commit, source/test/benchmark/profile/build/conformance/oracle/holdout execution, push, switch, merge, rebase, cherry-pick, fetch, or pull has occurred.
- Current blocker: none; exact generated-artifact byte verification remains incomplete.
- Next action: mechanically convert only the three authorized generated text artifacts to CRLF, verify all four blob hashes exactly against closure commit `781c4fa7af10791ee125648bd46083732eb03f63`, and proceed only on exact match.

## 2026-07-15 C4 invalid-preregistration record integration completion

- Evidence commits: preregistration `2c06488c23ef9009562ec56ad31b22e4aa4d4669`; invalid-record closure `781c4fa7af10791ee125648bd46083732eb03f63`.
- Integrated exact experiment artifacts:
  - `experiments/2026-07-15-c4-tostr-triage.md`
  - `experiments/2026-07-15-c4-tostr-before.txt`
  - `experiments/2026-07-15-c4-tostr-before-summary.txt`
  - `experiments/2026-07-15-c4-evaluator-tree.txt`
- Ledger record: C4 is `invalid preregistration; corrected retry eligible`; the two frozen source-shape mismatches are recorded, and Round 1 records that C4 did not execute.
- Budget state: 0/8 triage probes and 0/3 full experiments consumed.
- Execution state: no source edit/commit, candidate measurement, targeted test, evaluator execution, holdout, promotion, or integration of candidate source occurred.
- Evidence verification: all four integrated experiment artifact blob hashes exactly match closure commit `781c4fa7af10791ee125648bd46083732eb03f63`; the integrated ledger is byte-identical to the closure worktree ledger.
- Hygiene result: authored Markdown passes `git diff --cached --check`. The whole staged check reports only the exact CRLF bytes in the three committed generated text artifacts, which were preserved to keep their closure-commit blob identities.
- Current blocker: none for this record-only integration.
- Next action: prepare a separate corrected C4 branch, worktree, and preregistration citing the actual source shape.
