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
