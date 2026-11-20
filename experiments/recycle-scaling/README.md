# Numbered-object recycle scaling

The candidate removes the numbered-object recycle reservation and keeps ordinary
cleanup in the existing transaction. Baseline and candidate use the identical
`BenchmarkRecycleScaling` implementation in
[engine/object_lifecycle_bench_test.go](../../engine/object_lifecycle_bench_test.go).

## Evidence and method

- [Baseline raw output](baseline.txt), [final candidate raw output](candidate-final.txt).
  Each file includes the measured revision and the complete benchmark output.
- Existing isolated `barn-compute` Linux guest: eight vCPUs, Intel Xeon Platinum
  8260 host CPU, Go 1.27.1, GOMAXPROCS=8. No production service or database was used.
- Both runs: `go test -p 2 ./engine -run '^$' -bench '^BenchmarkRecycleScaling$' -benchtime=100ms -count=10`.
  The same root toolchain/cache and working directory
  were used; the checkout advanced from the baseline to the candidate between runs.
- Every sample executes 16 commands per worker. Each command retires one placed
  object, creates another numbered object, retires it, and checks both are invalid.
  The precommitted variant prepares placed objects before timing. The same-slice
  variant creates and places its first object in the measured task.
- Shared and separate rooms and inherited pure MOO recycle hooks are measured
  independently. Setup, runtime shutdown and final relationship checks are outside
  the timed interval. All tasks finish before counters are read. Every cohort
  requires empty room contents and no surviving prototype children.
- Both sides enable the existing diagnostics. Gate time, shared wait, retries,
  wasted ticks, bytes and allocations are in the raw output. Standard ns/op and
  allocation values are per batch; divide by `16 * workers` for per-command values.
- Each table entry is the median of ten samples. Ratios divide the two throughput
  medians. These are descriptive medians, not significance-test results.

Earlier complete candidates remain in [candidate.txt](candidate.txt) and
[candidate-repaired.txt](candidate-repaired.txt) as investigation history. The
table uses the final candidate, including all three conformance repairs and
replacement-error tuple normalization.

## Results

All measured candidate cohorts acquired zero exclusive commit-gate holds. The
baseline acquired at least one per command, with extra acquisitions on retries.
The eight-worker throughput medians improved by 1.79x to 3.47x across this matrix.
One-worker medians improved by 1.09x to 1.32x. Precommitted objects still incur
ordinary MVCC retries; the raw counters and table retain those costs.

| Previously committed | Shared room | Hook | Workers | Baseline commands/s | Candidate commands/s | Ratio | Baseline retries/cmd | Candidate retries/cmd |
|---|---|---|---:|---:|---:|---:|---:|---:|
| false | false | false | 1 | 3277.0 | 4323.0 | 1.32x | 0.0000 | 0.0000 |
| false | false | false | 2 | 3320.5 | 7500.5 | 2.26x | 0.0325 | 0.0000 |
| false | false | false | 4 | 2878.0 | 7326.0 | 2.55x | 0.2797 | 0.0000 |
| false | false | false | 8 | 2641.0 | 7389.0 | 2.80x | 0.5123 | 0.0000 |
| false | false | true | 1 | 3445.5 | 4331.5 | 1.26x | 0.0000 | 0.0000 |
| false | false | true | 2 | 2977.5 | 6928.5 | 2.33x | 0.0531 | 0.0000 |
| false | false | true | 4 | 2609.5 | 6898.0 | 2.64x | 0.3453 | 0.0000 |
| false | false | true | 8 | 2263.0 | 7853.0 | 3.47x | 0.6425 | 0.0000 |
| false | true | false | 1 | 3524.0 | 4454.5 | 1.26x | 0.0000 | 0.0000 |
| false | true | false | 2 | 3230.5 | 7425.5 | 2.30x | 0.0333 | 0.0000 |
| false | true | false | 4 | 2730.0 | 6797.0 | 2.49x | 0.2906 | 0.0000 |
| false | true | false | 8 | 2683.5 | 7884.5 | 2.94x | 0.4206 | 0.0000 |
| false | true | true | 1 | 3632.0 | 4236.5 | 1.17x | 0.0000 | 0.0000 |
| false | true | true | 2 | 3279.0 | 6852.5 | 2.09x | 0.0541 | 0.0000 |
| false | true | true | 4 | 2691.5 | 6911.0 | 2.57x | 0.4184 | 0.0000 |
| false | true | true | 8 | 2481.5 | 7435.0 | 3.00x | 0.5000 | 0.0000 |
| true | false | false | 1 | 4642.0 | 5327.5 | 1.15x | 0.0000 | 0.0000 |
| true | false | false | 2 | 3893.5 | 5679.5 | 1.46x | 0.0425 | 0.1950 |
| true | false | false | 4 | 3218.5 | 5130.0 | 1.59x | 0.1230 | 0.4238 |
| true | false | false | 8 | 3172.0 | 5847.0 | 1.84x | 0.1602 | 0.4195 |
| true | false | true | 1 | 4535.5 | 5120.0 | 1.13x | 0.0000 | 0.0000 |
| true | false | true | 2 | 3846.0 | 5498.0 | 1.43x | 0.0584 | 0.2439 |
| true | false | true | 4 | 3088.5 | 4754.5 | 1.54x | 0.2813 | 0.4788 |
| true | false | true | 8 | 3125.0 | 5606.5 | 1.79x | 0.3252 | 0.4734 |
| true | true | false | 1 | 4737.0 | 5182.5 | 1.09x | 0.0000 | 0.0000 |
| true | true | false | 2 | 3956.5 | 5522.0 | 1.40x | 0.0097 | 0.1953 |
| true | true | false | 4 | 3213.5 | 4961.5 | 1.54x | 0.1442 | 0.4444 |
| true | true | false | 8 | 3235.0 | 6206.5 | 1.92x | 0.1497 | 0.4142 |
| true | true | true | 1 | 4358.0 | 5205.5 | 1.19x | 0.0000 | 0.0000 |
| true | true | true | 2 | 3533.5 | 5196.5 | 1.47x | 0.0323 | 0.2466 |
| true | true | true | 4 | 2858.0 | 5114.0 | 1.79x | 0.4078 | 0.4677 |
| true | true | true | 8 | 2738.5 | 5857.0 | 2.14x | 0.7357 | 0.4360 |

## Limits

This is a small synthetic, fully settled in-process workload with diagnostics on.
Baseline and candidate were measured sequentially on a shared host, not interleaved
or on dedicated physical cores. Baseline load averages were 0.54/0.70/0.64 before
and 1.48/1.05/0.79 after; final candidate averages were 0.00/0.32/0.41 before and
1.39/1.05/0.73 after. No production goodput, command-tail latency, retained-heap,
CPU-utilization or linear-scaling claim follows from these runs. The benchmark
includes normal admission, VM execution and commit costs; it is not a store-only
microbenchmark. Anonymous destruction, child reparenting, player disconnection and
real irreversible callback operations retain their existing coarse paths.

The first baseline command completed but its tool output capture was truncated.
It was excluded from this comparison. Both raw files linked above were checked
for all 32 cases with ten complete samples each and terminal exit zero.

## Correctness evidence

The managed oracle scenarios distinguish overlapping numbered recycle hooks from
the anonymous guard. They pass on the documented WSL Toast oracle and fail on
unchanged Barn for the numbered-object scenario. The PR adds the same scenarios
to the existing managed CI command, plus Go regressions for gate-free numbered
cleanup, anonymous reservation/retry behavior and continuation restoration.

The pinned [Elle run](https://github.com/MongooseMoo/barn/actions/runs/42927123080)
returned literal true with exit zero for both healthy models at seeds 412, 395
and 396, and false with exit one and saved anomaly witnesses for its injected
isolation faults. Healthy seed 412 recorded parallel=806, exclusive=677,
exclusive_gate=860, hot_renewal=2, verb_memo=478, suspend=481 and fork=14. The
results file has no checker errors. Raw histories, manifests, logs and witnesses
are in the run's `elle-evidence` archive; these bounded cases are not a general
proof of isolation or liveness.

The [checkpoint campaign](https://github.com/MongooseMoo/barn/actions/runs/42927122982)
retains native uncertainty: ok=1974, info:transport=104 and
fail:no-live-generation=61. Its eight generation histories have explicit true
verdicts and no recorded durability violations. Transport outcomes were not
reclassified as successful transactions.
