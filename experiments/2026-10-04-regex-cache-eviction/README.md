# Regex-cache eviction evaluation (#362)

## Preregistered comparison

Baseline: `0ad7f10a404db84aed116110439648082c900459`, which clears the complete
1024-entry map on the next insertion. Evaluate a bounded second-chance clock:
new entries start unreferenced; repeated hits set an atomic reference bit only
when it is clear. Eviction scans the fixed ring under the existing write lock,
clearing marked bits and removing the first unmarked entry. All three compiler
paths keep their complete keys, negative results and caller-owned references.
Concurrent duplicate publication reuses the existing immutable result.

Use frozen baseline/candidate binaries with identical benchmark source and Go
1.24.6, GOMAXPROCS=4, affinity 8-11, twelve randomized paired samples (seed 362),
200ms per workload, followed by two holdouts. Stable 64-pattern hot sets,
sixteen hot reads per one-off insertion, and cyclic working sets of 1023/1025
patterns run serially and concurrently. Pattern generation/warmup is untimed;
concurrent workers use local sequences to avoid a per-lookup atomic counter.
All outputs and bounded retention are checked. Profiles run separately.

Adopt only with a useful churn benefit and acceptable stable-hit cost: target
at least 5% churn improvement with a paired interval below zero, no more than
10% serial stable-hit regression or 15% concurrent stable-hit regression, and
no material (>10%) above-capacity regression. If timing is inconclusive, retain
the old policy and report the evaluation. Compile counts use separate overlay
binaries, never production counters in timed code. Capture concurrent churn
block and mutex profiles with explicit nonzero rates, interpreting cumulative
waiter time per lookup rather than as wall-clock duration.

## Results

**Retain the existing wholesale policy.** The clock candidate passes the desired
retention tests and reduces recompilation, but fails the preregistered timing
gates. Production `builtins/regexcache.go` is unchanged. The rejected source and
original test/benchmark source are archived as `.go.txt` files, compiled only
through explicit Go overlays. This evaluation does not justify adding reference
tracking or a different production eviction policy.

| Workload | Paired time change (95% interval) | Holdout |
| --- | --- | --- |
| Hot serial | +30.977% [+29.591%, +32.559%] | +30.213% |
| Hot parallel | +61.101% [+57.192%, +64.171%] | +63.136% |
| Churn serial | +12.399% [+10.970%, +14.231%] | +11.905% |
| Churn parallel | +41.922% [+30.678%, +53.226%] | +43.396% |
| Below serial | +31.488% [+30.727%, +32.313%] | +31.653% |
| Below parallel | +45.382% [+38.662%, +52.358%] | +40.214% |
| Above serial | +12.396% [+5.642%, +18.695%] | +14.214% |
| Above parallel | -44.915% [-50.187%, -39.193%] | -41.937% |

All fourteen runs show zero bytes/allocations for stable hot/below-capacity
workloads on both policies. Serial churn drops from 155 to 141 B/op, with Go
reporting one allocation/op on both. Serial above-capacity drops from
2470-2471 to 2408 B/op but increases from 24 to 25 allocations/op. Parallel
above-capacity allocation ranges are 798-817 B/op and 7-8 allocations/op for
baseline versus 203-389 B/op and 2-4 allocations/op for the candidate. Parallel
churn varies with worker scheduling; raw ranges and all observations are in
`results.json`. A benefit in the parallel above-capacity workload does not
offset the measured stable-hit/churn regressions.

Separate instrumented binaries counted actual `compileMOOPattern` calls across
65,536 lookups, three repeats. They do not add counters to timed binaries:

| Workload | Baseline compiles | Candidate compiles |
| --- | --- | --- |
| Hot/below, serial/parallel | 0 | 0 |
| Churn serial | 4112 | 3856 |
| Churn parallel | 4122-4152 | 3711-3856 |
| Above serial | 65536 | 65536 |
| Above parallel | 21553-21833 | 6032-9526 |

The clock removes 256 repeated-hot compilations from serial churn while still
compiling all 3856 cold patterns. Concurrent sequences overlap and scheduling
changes duplication and residency; these ranges are observations, not a fixed
parallel compile-count guarantee. All observed retained counts stay at or below
1024. Original policy-retention tests fail on baseline (hot eviction at cold
insertion 1023, repeated negative-result eviction) and pass on the candidate.
Those policy assertions describe an optimization goal, not a regex-language
correctness defect. The committed regressions instead preserve the retained
policy's boundedness, caller-held MOO/PCRE/rightmost patterns, unchanged failures,
adjacent negative-result reuse and concurrent mixed eviction. Existing complete
key/case/anchor/rightmost and MOO-versus-PCRE tests remain in place.

Block and mutex profiles ran separately from timing and from each other, using
100,000 parallel churn iterations, three samples, with explicit
`-test.blockprofilerate=1` or `-test.mutexprofilefraction=1`. Cache-focused block
delay was 50.08-53.55ms (median 52.75ms) on baseline and 64.12-72.18ms (median
66.07ms) on the candidate. Mutex delay was 60.54-66.79ms (median 63.07ms) versus
71.83-79.64ms (median 75.05ms). Approximate median cumulative waiter delay per
100,000 lookups is 528/661ns for block and 631/751ns for mutex. These include
calibration and untimed warmup, are diagnostic cumulative waiter time rather
than elapsed time, and must not be added together. Text profiles show the
write-lock path as the dominant cache contention stack. Profiling perturbs
scheduling; it supports the direction of the timing result, not an exact
production contention prediction. Binary profiles and executables stay in `/tmp`.

Paired bootstrap intervals use 10,000 resamples of the twelve log ratios;
holdouts are separate. Benchstat uses the twelve main samples. These synthetic
compiler-cache workloads do not measure production churn frequency or server
throughput. CPU frequency/governor and other machine work were not controlled.
No alternative policy is claimed to be universally worse or better: this
specific candidate did not earn adoption. A future policy evaluation can reuse
the workloads, and should establish a better stable-hit/churn tradeoff first.

## Validation and reproduction

The clock candidate passed full builtins tests, full builtins race tests,
`go vet ./...` and `staticcheck ./...` before timing. After restoring production,
the focused retained-policy regressions passed; final race/static verification
and overlay reproduction are recorded with the kept evaluation commit below.
The benchmark/compile-count portion of the committed test file is identical to
the frozen test source; only the two production-policy regressions differ.

Frozen test-source SHA-256:
`692f4bf85ce92104d7b1087bc66ecf8b6c50d5707da3b77a40b9c3d17271765d`.
Timed baseline:
`e4db22e6c03c0606a0aa894f21a62a7eb0ab8afbc048a6dbb81952080d0d6985`.
Timed candidate:
`cfe5be871524413b914dd39c02ac0050437fa70e5ecdf71738174f1bad1f1b5b`.
Count baseline:
`282848e61426d2f2f7eaa8d22feef114c8b54f3bbd1002c38660d5e305510c9c`.
Count candidate:
`2905d23174b122ceb23a4f23efd4d16209d807ffa2384c0c03d10a034a3b89f3`.
Go 1.24.6 built all four; benchstat uses golang/perf commit
`406019bb8b6893dd1245d31bf511c719619bb5c9`.

From the worktree in WSL, set its correct `GIT_DIR` and `GIT_WORK_TREE` if needed:

```sh
python3 experiments/2026-10-04-regex-cache-eviction/prepare_binaries.py \
  --baseline 0ad7f10a404db84aed116110439648082c900459 \
  --output /tmp/barn-362-tools
python3 experiments/2026-10-04-regex-cache-eviction/run_pairs.py \
  --baseline /tmp/barn-362-tools/baseline.test \
  --candidate /tmp/barn-362-tools/candidate.test \
  --benchstat /root/go/bin/benchstat \
  --output experiments/2026-10-04-regex-cache-eviction/results
```

Run each `*-counts.test` separately with
`-test.run=^TestRegexpEvictionCompileCounts$ -test.count=3 -test.v`, using
GOMAXPROCS=4 and taskset affinity 8-11. Profile each timed binary with
`-test.run=^$`, `-test.bench=^BenchmarkRegexpEviction$/^ChurnParallel$`,
`-test.benchtime=100000x` and one profile/rate pair at a time. Read using
`go tool pprof -top -nodefraction=0 -focus=cachedMOOPattern`.
