# Barn performance prior art

Date: 2026-07-15

This is a read-only prior-art gate. No benchmark baseline, candidate experiment,
server, conformance suite, or Toast oracle was run while producing it. Numbers
below are existing repository evidence, checked against current files, Git, and,
where available, safe raw summary/profile artifacts. They are not new results.

## 1. Repository, branch, and worktree state

| Repository | Branch and HEAD | Initial state relevant to this task |
|---|---|---|
| `C:\Users\Q\code\barn` | `master` at `864de996a111674adfe15c330f8e85813f4641f0`, tracking `origin/master` at the same commit | No tracked modification was shown. There were extensive pre-existing untracked probes, databases, notes, plans, prompts, reports, and profiles. This task later explicitly authorized only this report and `notes-barn-performance-campaign.md`. |
| `C:\Users\Q\code\moo-conformance-tests` | `main` at `e3d8dbd28c600490b4a1529619d29273ee29b30f`, tracking `origin/main`, ahead by 13 commits | Pre-existing tracked modification: `src/moo_conformance/_tests/audit/connection_lifecycle_toast_oracle.yaml`. There were extensive pre-existing untracked `.tmp`, log, prompt, report, and generated files. Nothing there was changed. |

The Barn report commit must preserve all that dirt. Key evidence status matters:

- tracked Barn authority: `vm/perf_bench_test.go`,
  `scripts/perf-compare.ps1`, `scripts/benchmark-mongoose.ps1`,
  `experiments/2026-06-27-perf-baseline.md`, its raw baseline,
  `experiments/2026-06-22-builtin-call-performance.md`,
  `experiments/2026-07-14-mongoose-performance-baseline.md`, and
  `plans/barn-toast-mongoose-convergence-workstreams.md`;
- pre-existing untracked Barn context: `notes/perf-execution-log.md`, both
  `plans/blazing-fast-barn*.md`, `plans/byecode-vm-performance-plan.md`,
  `experiments/2026-06-24-commit-dominated-concurrency-ledger.md`, and
  `scripts/run-bottleneck-finder.ps1`;
- tracked sibling benchmark authority: `bench/bench.py`, `bench/run_bench.sh`,
  `reports/benchmark-barn-vs-toast-20260619.md`, and
  `reports/barn-vm-optimization-20260619.md`;
- untracked sibling precursors: `.tmp/bench-notes.md` and `.tmp/bench.py`.
  The latter differs from tracked `bench/bench.py` by two small current safety
  initializations, so it is not the authority.

## 2. Locations and terms searched

### Barn

- Instructions and workflows: `AGENTS.md`, `README.md`, `Makefile`,
  `plans/`, `prompts/`, `notes/`, `reports/`, `experiments/`, and `scripts/`.
- Definitions and raw evidence: `vm/perf_bench_test.go`,
  `scripts/perf-compare.ps1`, `scripts/benchmark-mongoose.ps1`,
  `scripts/run-bottleneck-finder.ps1`, `experiments/*.md`,
  `experiments/*.txt`, `experiments/*.cpu`, and
  `.tmp/mongoose-convergence/perf-*` summaries/profile inventories.
- Git: `git log` on `master` and `--all`; commit shows; graph around the July
  deployment campaign; merged/unmerged performance branches; all five commits
  named by the task plus adjacent pin/result commits.
- Search terms included: `Benchmark`, `-bench`, `benchmem`, `benchstat`,
  `benchmark`, `performance`, `perf`, `profile`, `pprof`, `RSS`, `WorkingSet64`,
  `latency`, `throughput`, `allocation`, `GC`, `noise`, `CV`, `median`, `min`,
  `baseline`, `threshold`, `reject`, `revert`, `restore`, `defer`, `abandon`,
  `blocked`, `ToastStunt`, `upstream`, and the five requested commit prefixes.

### `moo-conformance-tests`

- Instructions and lifecycle: `CLAUDE.md`, `README.md`, `docs/TOASTSTUNT.md`,
  `src/moo_conformance/server.py`, `plugin.py`, and `profile_gate.py`.
- Benchmark evidence: `bench/`, `notes-benchmark-barn-vs-toast.md`,
  `reports/benchmark-barn-vs-toast-20260619.md`,
  `reports/barn-vm-optimization-20260619.md`, `.tmp/bench-*`, and Git history.
- Search terms additionally included: `server-command`, `managed`, `Test.db`,
  `oracle-profile-manifest`, `PROMOTE_NUMBERS`, `OUTBOUND_NETWORK`, `runtime_os`,
  `database_checksum`, `structures.h`, `eval_env.cc`, and `refcount`.

### Concise no-hit record

- Sibling `AGENTS.md`: none found; sibling `CLAUDE.md` is the applicable local
  instruction surface.
- Academic performance literature: none found. No paper, DOI, or arXiv source is
  cited by the performance records; this gate therefore adds no web dependency.
- Designated performance holdout/evaluation set: none found. Searches included
  `holdout`, `evaluation set`, `SLO`, `p95`, `p99`, `percentile`, and
  `confidence interval` in both repositories.
- Barn report-specific pre-commit/Markdown checker: none found. There is no
  `.pre-commit-config.yaml` or Markdown-lint config.
- Other current Barn Go benchmark entry points: none found. Current `master`
  has only `func BenchmarkVM` (`vm/perf_bench_test.go:47`).
- Current scheduler bottleneck benchmark sources: none found. Only the untracked
  recovery ledger/runner remain; their referenced `BenchmarkBottleneckFinder`
  and `TestConcurrency*Sweep` sources are absent from current `master`.

## 3. Benchmark and metric inventory

### A. Direct single-thread VM microbenchmarks

Authority: `vm/perf_bench_test.go:3-9,47-73`,
`experiments/2026-06-27-perf-baseline.md:18-70`, and
`scripts/perf-compare.ps1`.

Substantiated commands:

```text
go test ./vm -run='^$' -bench=BenchmarkVM -benchmem -count=10 | tee experiments/perf-baseline-vm-20260627.txt
pwsh scripts/perf-compare.ps1 BenchmarkVM/int_arith_1M c1-after
pwsh scripts/perf-compare.ps1 BenchmarkVM c1-after
go test ./vm -run=^$ -bench=BenchmarkVM/int_arith -cpuprofile=/tmp/cpu.prof -memprofile=/tmp/mem.prof
go tool pprof -top -nodecount=25 /tmp/cpu.prof
```

Metrics are `ns/op`, `B/op`, and `allocs/op`; profiles expose CPU and allocation
ownership. Workloads are integer and float arithmetic (1M), string concat (10k),
list append (10k), list index (1M), `abs` builtin (200k), `tostr` (200k), nested
1k x 1k loops, and list iteration (1M). Each `b.N` run compiles before the timer,
then constructs a task context and VM and runs the already-compiled program.

Reproducibility: tracked and automated. The locked raw baseline is Windows,
Ryzen 5950X, Go 1.26, `GOMAXPROCS=32`, `-count=10`. `perf-compare.ps1` requires
`benchstat` installed separately. It is not a current-HEAD baseline: the capture
is dated 2026-06-27 and later production/performance commits exist.

### B. WSL socket Barn-versus-Toast microbenchmark

Authority: sibling `bench/run_bench.sh`, `bench/bench.py:20-61,72-125`, and
`reports/benchmark-barn-vs-toast-20260619.md:20-64,100-106`.

Substantiated commands:

```text
bash bench/run_bench.sh
wsl --cd /mnt/c/Users/Q/code/moo-conformance-tests --exec bash -lc 'bash bench/run_bench.sh'
```

The launcher builds Barn for Linux, copies the bundled conformance `Test.db` to
Linux-local `/tmp`, starts Toast and Barn on fixed ports 7801/7802, and invokes
`uv run python bench/bench.py "toast=7801" "barn=7802"`. The Python harness uses
the sibling `SocketTransport`, raises the same task limits on both engines, does
one untimed warm-up, then five timed sequential round trips. It reports minimum
and median milliseconds and Barn/Toast minimum-time ratio for ten workloads:
noop, 5M integer/float arithmetic, 50k string concat, 30k list append, 1M list
index, 1M `tostr`, 1M property access, 200k `abs`, and 2500 x 2500 nested loops.

Reproducibility: automated but not the managed conformance lifecycle. It starts
servers itself on fixed ports and its Toast path selection (`~/src/toaststunt/moo`
or `build/moo`) differs from the current Barn oracle path
`/root/src/toaststunt/build-release/moo`. The `bench.py` docstring also names the
obsolete `.tmp/bench.py` path. Those must be reconciled before treating a new run
as a campaign baseline.

### C. Mongoose deployment benchmark

Authority: `scripts/benchmark-mongoose.ps1:1-31,56-72,127-247`,
`experiments/2026-07-14-mongoose-performance-baseline.md`, and safe raw summaries
under `.tmp/mongoose-convergence/perf-*`.

The tracked script has a parameter contract, but no complete credential-free,
fully instantiated invocation was found. It requires `-Engine Toast|Barn`,
`-Database`, `-OutputDir`, optional executable/port values, and exactly three
non-empty login commands supplied through `MONGOOSE_LOGIN_SCRIPT`. Inventing a
command here would violate the evidence boundary.

It copies the source database to the output directory and verifies both hashes;
runs fixed `look`, `west`, `@who`, liveness, and checkpoint commands; uses a
3000 ms banner wait, 2500 ms inter-command delay, 15 s idle timeout, 40 s total
client duration, and 180 s settle by default; and measures:

- database load-to-listen;
- connect-to-banner and the causal PROXY-to-first-output metric;
- complete login, startup command, `look`, movement, and liveness latency;
- checkpoint reply and observed checkpoint-file completion;
- settled CPU and RSS.

Barn runs also save profile manifest, expvar/debug counters, and a forced-GC heap
profile. The runner uses a disposable copy and stops client/server in `finally`.
It is the strongest managed deployment measurement surface, but reproduction is
blocked until the exact authorized fixture path, executable builds, and secret
environment are supplied outside the report.

### D. Historical scheduler concurrency bottleneck finder

Authority is weak: untracked
`experiments/2026-06-24-commit-dominated-concurrency-ledger.md` and untracked
`scripts/run-bottleneck-finder.ps1` from `work/mvcc-concurrent-moo`.

Recorded commands include:

```text
go test ./scheduler -run "TestConcurrencyCommitDominatedDisjoint$" -count=3 -v -timeout 120s
go test ./scheduler -run "^$" -bench "BenchmarkBottleneckFinder/commit_dominated_disjoint_(serial|pool)$" -benchmem -count=3 -timeout 120s
```

Profile variants add `-benchtime=3s -count=1`, CPU/memory profiles, and a test
binary. Metrics were serial/pool wall time, speedup, bytes/allocations, commits,
retries, task slices, and validation failures; the runner also requested CPU,
allocation, mutex, and block profiles under default GC and `GOGC=off` sweeps.

Current reproducibility: no. The ledger says the branch was already dirty,
source was uncommitted, and the protocol was not followed cleanly; current
`master` lacks the benchmark/test sources. These commands are prior art only.

### E. Historical conformance-harness timing

Untracked `reports/test-performance-report.md` proposed timing an arithmetic
pytest slice against an already-running server. Sibling commit
`480931541d4fbb35529dd62c8434225ede1750d0` then added session-scoped connection
reuse and shorter timeouts, but its 10-15x claim is explicitly expected, not a
recorded sample. This measures harness overhead rather than Barn execution and
uses an external-server workflow, so it is not a valid current campaign baseline.

## 4. Existing measurements, noise, and profiles

### Locked 2026-06-27 VM baseline

| Workload | Median ns/op | CV | B/op | allocs/op |
|---|---:|---:|---:|---:|
| `int_arith_1M` | 59,330,000 | 6% | 16,005,000 | 1,999,740 |
| `float_arith_1M` | 68,670,000 | 3% | 16,005,000 | 1,999,762 |
| `string_concat_10k` | 1,331,000 | 11% | 608,100 | 19,784 |
| `list_append_10k` | 2,225,000 | 15% | 1,391,200 | 29,799 |
| `list_index_1M` | 139,200,000 | 15% | 28,022,500 | 3,491,533 |
| `builtin_abs_200k` | 30,150,000 | 1% | 6,402,830 | 799,484 |
| `tostr_200k` | 57,570,000 | 1% | 16,003,330 | 999,541 |
| `nested_1k` | 55,650,000 | 10% | 13,987,100 | 1,747,504 |
| `list_iter_1M` | 95,150,000 | 10% | 40,010,200 | 2,999,490 |

The ledger explicitly warns that 15% CV list rows and 11% string concat make
small raw deltas untrustworthy; use `benchstat` p-values. No current-HEAD repeat
exists, so these are historical comparison data, not today's baseline.

### 2026-06-19 WSL socket comparison

| Workload | Toast ms | Barn ms | Barn/Toast |
|---|---:|---:|---:|
| noop | 0.3 | 0.3 | about 1.0x |
| integer 5M | 66 | 626 | 9.4x |
| float 5M | 69 | 644 | 9.1x |
| string concat 50k | 20 | 277 | 12-13x |
| list append 30k | 785 | 13,300 | about 17x |
| list index 1M | 28 | 240 | 8.7x |
| `tostr` 1M | 149 | 638 | 4.2x |
| property access 1M | 60 | 450 | 7.4x |
| `abs` 200k | 9 | 70 | 7.7x |
| nested 2500 x 2500 | 85 | 800 | 9.3x |

The report says two consecutive runs were stable within noise, but raw per-repeat
samples, CVs, confidence intervals, and the exact tested Barn commit are absent.
Later performance commits make this comparison stale.

### 2026-07-14 deployment baseline and RSS sequence

Pinned database SHA-256:
`b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`.
Pinned Toast executable SHA-256:
`a748a93644fe2b973cc85dfed902454a0a56c8b368afdc8104161ec76154d098`.

| Metric | Toast | Initial Barn | Barn threshold |
|---|---:|---:|---:|
| load to listen | 6,392 ms | 5,380 ms | 12,784 ms |
| PROXY to first output | 3 ms | 1 ms | 100 ms |
| complete login | 5,015 ms | 5,483 ms | 10,030 ms |
| startup command | 2 ms | 4 ms | 100 ms |
| `look` | 3 ms | 11 ms | 100 ms |
| movement | 6 ms | 4 ms | 100 ms |
| liveness query | 1 ms | 2 ms | 100 ms |
| checkpoint file | 9,429 ms | 2,341 ms | 18,858 ms |
| settled CPU | 3.6% | 0.46875% | 7.2% |
| settled RSS | 311,640,064 B | 1,882,996,736 B | 467,460,096 B |

Connect-to-banner is intentionally informational: Barn's roughly 3,001 ms
includes the configured 3,000 ms pre-PROXY wait and cannot explain latency after
PROXY. Every causal/non-memory gate passed. RSS alone failed.

The unchanged profile-bearing repeat was 2,010,251,264 B RSS. Forced-GC
`inuse_space` was 814.77 MB, 803.25 MB cumulative under database load. Recorded
flat/cumulative evidence included:

- `types.NewMap`: 247.06 MB flat (30.32%);
- `ObjectBuilder.ResetProperties`: 222.61 MB flat (27.32%);
- `types.NewStr`: 131.01 MB flat (16.08%);
- `Database.resolvePropertyNames`: 256.68 MB cumulative;
- `Database.readValue`: 508.04 MB cumulative.

Safe raw `summary.json` files remain on disk and independently match the tracked
ledger. They confirm this RSS sequence on the same fixture and 180-second settle:

| Run | RSS | Decision |
|---|---:|---|
| profile-bearing baseline repeat | 2,010,251,264 B | baseline/profile authority |
| adaptive small-map slice 1 | 2,133,549,056 B (+6.13%) | rejected/restored |
| property-by-value slice 2 | 1,837,264,896 B (-8.61% vs repeat) | kept |
| delete duplicate property names slice 3 | 1,295,065,088 B (-29.51% vs slice 2) | kept |
| exact map-capacity slice 4 | 1,469,816,832 B (+13.49% vs slice 3) | rejected/restored |
| ordered property storage slice 5 | 1,387,020,288 B (+7.10% vs slice 3) | rejected/restored; stop |

The last kept value remains 827,604,992 B above the 467,460,096 B threshold.
There are single observations per named deployment slice, not repeated samples or
spread estimates. The fixed workload and large deltas support the recorded slice
decisions, but do not define general RSS variability.

### Earlier CPU/allocation profiles

Sibling `reports/barn-vm-optimization-20260619.md` records:

- arithmetic: about 55-60% dispatch machinery and 20-25% allocation/GC;
- `int_arith_1M`: 2,000,000 allocations before value de-boxing;
- `list_append_10k`: 1.67 GB allocated; whole-list `ValueBytes` was 26%
  cumulative, compounded with list-copy work.

Later tracked `reports/perf-c5-profiler.md` records post-de-box arithmetic/nested
at about 85% interpreter-dispatch machinery, with the per-op code-end check at
13.4%; `tostr` was dominated by builtin registry/validation/protected-check
ceremony rather than struct copying. The corresponding fixes were later kept.
These profiles are valuable attribution history, not evidence of the current
post-fix bottleneck; re-profiling is required before another source slice.

## 5. Prior experiments: results and causes of death

### Kept on current `master`

| Commit | Change | Recorded result |
|---|---|---|
| `027e3585b08012cd42ba6c4ac90b7b62a61daa20` | VM harness; numeric-first add, faster integer formatting, bulk list copy | string concat ~18%, list append ~16%, `tostr` ~15%, arithmetic ~5%; package gates passed |
| `fb438f5a3211d5adbc2da1052c76d59dd2f30cb7` | O(1) cached list byte-size | list append 955 -> 608 ms (-36%); conformance unchanged |
| `507b3b7425d6f85ccaa190f1fe7c8b7b07801caa` | cache current frame and inline dispatch layer | int ~12%, float ~11%, nested ~14%, list index ~13%; conformance unchanged |
| `eaa1d0f0dbcffc558cedd6f9627f13671da157d5` | content-addressed LRU compiled-verb cache, alongside compiler/bytecode extraction | conformance unchanged; no isolated performance sample recorded in the commit |
| `6094276b6bddd6756a33f6b75c11c8393030fc6c` | fused range-for check/next | int 132 -> 60 ms, nested 100 -> 58 ms, list index 186 -> 144 ms |
| `a03f4b216c9e6424cebb9fce461a30f52fc84e37` + `8c43bee3a21f265860b09f7644c380f49167ea3c` | fused collection iteration phases A/B | list iteration 164 -> 116 -> 94 ms |
| `32f1deb4942fd28935b38155101a4d7b2be834f5` | remove dead DUP/POP for simple assignment statements | int ~12%, nested ~8%, list iteration ~12% |
| `30d428814179f4a3f43b16c194aef95e63b6b040` | alias-safe in-place self-list append | 10k append ~580 -> 1.35 ms; 1.67 GB -> 1.39 MB; aliasing and conformance gates passed |
| `eb593138235490ed95010beec01217eb5c58d39a` | serve built-in properties before the guaranteed-failing inheritance search | `prop_access_1M` 376 -> 254 ms (~33%); conformance unchanged |
| `1ef32f18d91426ae25a3336b03fde5d2e3c4d89e` | accumulator string concatenation | kept, but the commit itself has no numeric result; do not infer one from nearby workloads |
| `e0cdce8` + record `8bc47f0` | fixed-argc builtin call fast path | socket `abs` 52.94 -> 31.11 ms (6.1x -> 3.5x Toast); local allocations about 999,484 -> 799,484 |
| `884b8e51e3db2159a6136966edcc2e07ce5f8e46` | lock C0 baseline/statistical helper | durable `-count=10` baseline and `benchstat` workflow; scheduler track explicitly deferred |
| `c6d81e743590b294ca8133e587d8536efea008af` | numeric-first self-accumulation routing | campaign ledger records int -8.64%, float -17.08%, p=0.000; allocations unchanged |
| `5bf93a1fcf9022f0b2e96dcff9e7f58705a3b501` | replace interface `Value` with tagged-union struct | int/float allocations about 2.0M -> 11/op; geomean allocations -99.96%, bytes -98%; conformance/race clean |
| `fd17fa354bd0f31f614c8c7e7aa83212743d60d1` | collapse dispatch-loop hot path | int -10%, nested -6% versus prior master; conformance unchanged |
| `cf679902271c3295e5b109aa82775e9d632260b8` | cut builtin-call ceremony | `tostr` -7.6%, `builtin_abs` -32%; conformance/race clean |
| `685c5f4607b1ec70e8fb02993938a042029b6a14` | batch orphan-anon/waif GC sweeps | commit attributes hundreds-of-ms live-world sweeps and login starvation; says Mongoose became playable, but provides no reusable spread/raw series |

The June 27 C2 value change did not meet its original >=3x single-thread ns/op
gate and temporarily caused 1,383 conformance failures through zero-value/None
handling. The construction-site blocker was fixed, allocations and correctness
gates passed, and the user explicitly waived the mis-specified latency gate before
promotion. This is a kept change with a recorded gate revision, not proof of the
original speed hypothesis.

### Discarded, skipped, deferred, or superseded VM work

- Registration-time caching of `IsProtectedBuiltin` was discarded in the June 22
  experiment because `load_server_options()` mutates protected-builtin runtime
  state. The later kept solution uses an atomically replaced immutable snapshot.
- The standalone C4 O(n^2) append campaign was skipped: current watermark-backed
  list/string growth had already removed that premise. Remaining per-append
  constant-factor header allocation was not promoted as a separate slice.
- C3 scheduler concurrency was deferred in the June 27 campaign because the
  required MVCC scheduler and benchmark sources were not on `master`.
- Optional string-box reduction, smaller `Value`, GC tuning, and PGO remained
  unexecuted/pending in the untracked campaign ledger. They are not current
  hypotheses without a new profile.
- `plans/byecode-vm-performance-plan.md` is superseded: it says no VM benchmarks
  exist and names old source locations. The benchmark and several proposed
  dispatch/call surfaces were subsequently implemented.
- `perf/unbox-value` is an old unmerged branch from an earlier wide migration;
  current `master` later received the separate, gated C2 tagged-union squash.

### Unmerged scheduler/store experiments

| Commit/branch | Status | Evidence |
|---|---|---|
| `c09bcaedc12b2065050b9af9636b4da3e0ddac47` / `exp/floor-shard` | unmerged partial win | 16-way live-read floor sharding; mutex profile had 88% of contention there; 32-worker speedup ~1.17x -> ~1.98x, below target |
| `071210c0ed887222c3dcedd24e72f99eae805c68` / `exp/lazy-txn` | HOLD, not standalone promotion | GC share ~35% -> 24%, but 32-worker average 1.86x vs ~2.04x baseline, within noise |
| `4ec7bd53b75098ad9cab121aca2f43b711c656de` / `exp/floor-cache` | kept only on experiment branch | exact cached floor reduced single-thread commit ~18-20 us -> ~10 us; 32-worker throughput unchanged; explicitly not the parallelism lever |
| untracked concurrency recovery ledger / `work/mvcc-concurrent-moo` | blocked/unfinished | all repeated 32-worker rows had to reach 3.0x; observed 3.32x, 2.29x, 2.13x, 4.06x, 5.68x. Goal not achieved; dirty uncommitted source and protocol violations recorded |

That recovery ledger provisionally kept instrumentation, transaction/store
allocation reductions, disjoint batching/chunking, write-only/local-property
paths, footprint caching, VM/transaction pools, ready-batch reductions, deadline
and worker-state reductions, and single-property commit/history paths. Because
the source was not cleanly committed, these are recovered ideas, not landed wins.

It explicitly rejected/restored: smaller VM stack/frame capacities; loop and
exception preallocation removal; huge-deadline shortcut; uncapped or cap-8 write
chunking; opt-in contention profiles; direct goroutines; skipping finalizer
clear; omitting benchmark loop scaffolding; `lazySet` capacity one; lazy
transaction object cache; chunk-local metrics; serial `Done` fairness; an early
unguarded local-property fast path; an early VM pool; and a one-property generic
commit shortcut. Recorded cause was failure to improve the focused pool row,
target degradation/noise, invalid benchmark semantics, or correctness risk.

### July 14 deployment RSS campaign, exact Git significance

The complete current-master ledger chain is:

| Commit | Significance |
|---|---|
| `5270fb131754029a1ac0908728dd4cb499b22908` | add deployment runner/baseline surface |
| `7870ef74911f0c1f5470dfcf1ef35a71f7bb6bed` | record initial Barn deployment baseline |
| `2b7d40dee177429a1f902fd06e524675675c0146` | capture deployment heap profile |
| `b04bbca80d3a13fbc8b3c8a8cff4b1b009ba294a` | pin map-memory slice 1 |
| `7b84757b6571cd56e118771d722f63a88622a784` | record slice 1 rejection/restoration |
| `92c5c208e66ddf64f03eeb38daf61e5b66a4854a` | pin property-storage slice 2 |
| `c3c7f8748dfd60a5b732b166312c18ac85787e0d` | **kept** resolved properties by value; RSS 2,010,251,264 -> 1,837,264,896 B |
| `6400748c69a2b5fe8eb1f90fbcfa28b34c0590e9` | pin compact-property slice 3 |
| `4cba9daa95bb890518d2fae795c8a19daac38fde` | **kept** duplicate property-name deletion; RSS -> 1,295,065,088 B |
| `563196b5deb689a7b777358ca1fb30dd231cb957` | pin exact map-capacity slice 4; **not** itself a rejection record |
| `8f787cc30f5a5c6ac9695137cd7d06dc073b1af3` | record slice 4 rejection/restoration at 1,469,816,832 B |
| `873fa40c4a4f307e20032d3500da174b00d94389` | pin ordered-property slice 5 |
| `864de996a111674adfe15c330f8e85813f4641f0` | record slice 5 rejection/restoration at 1,387,020,288 B and stop after consecutive rejections |

This independently verifies—and refines—the prompt's five prefixes. The current
campaign is blocked by its exact-convergence stop rule, not complete: RSS remains
above threshold and the last two slices produced no kept improvement.

## 6. Correctness and conformance constraints to preserve

- Barn implementation/diagnostics live in Barn; durable behavioral truth lives
  in `moo-conformance-tests` (`plans/barn-toast-mongoose-convergence-workstreams.md:28-35`).
- WSL Toast is the oracle. For Barn work the exact binary is
  `/root/src/toaststunt/build-release/moo`; do not substitute Windows Toast.
- The sibling's documented managed command is:

  ```text
  wsl --cd /mnt/c/Users/Q/code/moo-conformance-tests --exec bash -lc 'uv run moo-conformance --server-command="/root/src/toaststunt/build-release/moo {db} {db}.out -p {port}" -v'
  ```

  It uses bundled `Test.db`, a free port, temporary DB copy/directory, automatic
  start/stop, and auto-detected server working directory. Barn `AGENTS.md`
  forbids manual conformance server/oracle launches and tracked-DB runs.
- Mongoose fixture, profiles, login flow, accounts/passwords, and credentials
  must remain Barn-local. Any semantic issue discovered on Mongoose must reduce
  to bundled `Test.db` before becoming a sibling conformance test; if faithful
  reduction is impossible, stop rather than adding Mongoose knowledge there.
- Managed profile comparisons fail closed unless support status is accepted;
  `option.OUTBOUND_NETWORK`, `option.PROMOTE_NUMBERS`, `database_fixture`,
  `database_checksum`, and `runtime_os` match
  (`src/moo_conformance/profile_gate.py:14-66`).
- `--server-command` supports `{port}`, `{db}`, `{manifest}`, and `{server_dir}`.
  The managed server copies the selected DB, installs fixtures, waits for the
  requested host/port, captures the log, and tears down in `finally`.
- Every performance slice must name one metric/hypothesis, capture before/profile,
  change one production surface, rerun the same benchmark and conformance gate,
  then commit a measured improvement or fully restore
  (`plans/barn-toast-mongoose-convergence-workstreams.md:509-531`).
- A profile is diagnostic, not a keep decision. Functional/profile compatibility,
  liveness, persistence/checkpoint behavior, error text asserted by conformance,
  and exact MOO value/aliasing semantics remain gates.

Repo-referenced upstream runtime prior art, not academic literature:

- ToastStunt repository `https://github.com/lisdude/toaststunt`;
- `src/include/structures.h` for `Var`, type tags, and refcounted complex values;
- `src/eval_env.cc` for explicit `TYPE_NONE` initialization;
- `src/execute.cc`, `src/tasks.cc`, `src/list.cc`, and `src/map.cc` for VM,
  scheduler, list, and map behavior;
- original Toast Ruby tests under `test/tests/`.

These locations may answer semantics/representation questions later. They do not
replace live managed Toast verification.

## 7. Gaps before a trustworthy baseline, budget, or holdout

1. **No current-HEAD baseline.** The statistical VM baseline is June 27; the
   socket comparison is June 19; current HEAD is July 14 after many changes.
2. **No single benchmark contract.** Direct VM, synthetic socket, deployment,
   and historical concurrency surfaces use different OSes, fixtures, counts,
   lifecycle, and metrics. Their numbers cannot be combined into one budget.
3. **Socket harness drift.** Fixed ports/manual lifecycle and old Toast path do
   not match the managed WSL oracle contract. Raw five-repeat samples and the
   tested Barn commit are not recorded.
4. **Deployment invocation is incomplete by design.** Fixture hash and workload
   are pinned, but no complete authorized invocation, credential mechanism value,
   or executable build recipe/hash for a new Barn candidate is recorded in a
   reusable command. Do not reconstruct these from transcripts.
5. **Noise is uneven.** VM has 10 samples/CV and `benchstat`; deployment slices
   have one RSS sample each; socket results only say two runs were stable. No
   warm/cold policy, machine-load control, percentile/tail policy, or repeat-count
   rule spans the families.
6. **No holdout.** Existing workloads informed profiling and source selection and
   then served as acceptance gates. No separate unexamined workload, world,
   conformance family, or traffic trace is designated for final evaluation.
7. **No current general budget.** The tracked deployment experiment has explicit
   Toast-derived thresholds only for that fixture. The untracked blazing-fast
   plan's ratio goals are stale and partially achieved/refuted. There is no
   current CPU/RSS/latency/throughput budget for Barn as a whole.
8. **Concurrency authority is missing.** The recovery runner/ledger is untracked,
   its source harness is absent from current master, and the branch goal failed.
   It cannot supply a baseline or holdout without a separately authorized repair.
9. **Profile attribution is stale after fixes.** Old profiles successfully led to
   kept changes. They do not prove today's hottest owner. The deployment stop
   rule also forbids silently resuming the property family.
10. **Correctness baseline numbers drifted.** Historical reports cite 3,871 or
    3,988 passes with 131 skips. The sibling is now ahead 13 with a dirty tracked
    oracle YAML file, so those counts are not current gates until a managed run is
    explicitly authorized.

These gaps prevent trustworthy campaign framing today. They do not authorize a
baseline run, holdout choice, target ranking, or resumption of the blocked RSS
workstream in this prior-art task.

## 8. Performance surfaces worth profiling later (explicitly unranked)

The following are evidence-backed profiling surfaces, deliberately unranked:

- retained database representation after load: property resolution/storage,
  maps, strings, object metadata, and Go heap fragmentation/`HeapSys` versus
  live retained bytes;
- current VM opcode/dispatch cost and opcode frequency after all June loop,
  assignment, de-boxing, and dispatch changes;
- builtin call and protected-builtin lookup paths after the June registry change,
  using real call-heavy workloads rather than the misleading old `verb_call` name;
- string accumulation/representation and alias-safe header allocation after the
  kept accumulator work;
- list/map/property access on workloads that are not already optimized benchmark
  idioms, including defined properties and user verbs rather than only builtins;
- scheduler deferred GC reachability sweeps on a large live-world workload;
- scheduler/store commit, transaction, deadline/context, ready-batch, and worker
  overhead, but only after a clean current-master harness exists;
- database load/listen, checkpoint, and post-settle memory on the pinned
  deployment workload if the exact blocked-workstream authority is explicitly
  changed;
- end-to-end connection/login/command latency under managed lifecycle, preserving
  the causal PROXY ordering and liveness anchors;
- conformance-harness connection reuse only if the objective is test-cycle time,
  kept separate from Barn engine throughput.

No surface above is a selected target. Selection, budget, and holdout design
belong to the manager's campaign-framing phase after this artifact is reviewed.
