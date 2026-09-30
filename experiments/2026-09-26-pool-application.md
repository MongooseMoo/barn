# Initial combined application measurement

Disposition: application throughput is inconclusive, with concerning losses.
This run does not establish application speedup or no regression. Preserve the
entire fixed five-pair dataset; no observations were excluded or rerun. Candidate
contains the three independently passing pools (unique, file read, pending
finalization), not either rejected conversion-capacity experiment.

## Reproduction and scope

Frozen plan: 2026-09-26-pool-campaign.md, Application measurement plan.
Runner: experiments/pool-application-measure.ps1, evaluator blob
dc78471a52cb7c385ffdd5c7c762b6ecaf7a76b2. Workload evaluator:
engine/mongoose_real_bench_test.go blob 3ac942cb3d5589b7a588dfb354ec0d0ac0627045.
Baseline source 3c0f510; candidate production deltas 7e0363d, 0e5fdba, 6b72d0a.
The runner used .tmp/engine-baseline.exe and .tmp/engine-candidate.exe in the
campaign worktree, fresh private runtime directories per process, and the same
.tmp/mongoose.db disposable input. Recorded fixture SHA256:
489FF8D14884392DCFBA6CD88D407E53A2140C03D4F0CC4B1B19CF2AFD8A031E.

GOMAXPROCS=4; players=1,4,16; warmup=1s; measure=3s; default command mix;
repair/promote enabled; CPU/memory profiling disabled. Five pairs alternating
AB/BA, ten fresh processes. All terminal exit codes are zero, and all 30 player
rows report failed=0 and uncaught=0. This is evidence for these exercised
commands, not full MOO conformance.

Raw logs and independent analysis JSON are in the adjacent
2026-09-26-pool-application-evidence directory. Analysis uses paired
candidate/baseline ratios, median and full range; percentile bootstrap of the
paired median, 20,000 resamples with seed 20260928, endpoints 499 and 19499.
Five pairs provide weak inference: wide intervals are not evidence of safety.

## Observations

| Players | Baseline goodput median [range] | Candidate median [range] | Paired ratio median | 95% interval |
| --- | --- | --- | --- | --- |
| 1 | 144 [127,208]/s | 144 [139,159]/s | 1.09449 | [0.69231,1.11450] |
| 4 | 281 [212,410]/s | 241 [174,419]/s | 0.89324 | [0.76508,1.02195] |
| 16 | 250 [218,364]/s | 214 [73,240]/s | 0.90000 | [0.20055,0.98165] |

The paired median ratio is not the ratio of unpaired medians. Candidate goodput
is lower in four of five four-player pairs and all five sixteen-player pairs.
Sixteen-player paired ratios are 0.20055, 0.96000, 0.90000, 0.98165, 0.85463.
The very slow first candidate remains included.

The frozen stop-for-diagnosis criterion was a statistically clear >5% goodput
loss (ratio interval upper bound below 0.95), or new correctness failures.
Neither criterion is established by this small dataset. That is not a pass for
application performance: intervals permit substantial losses, and sixteen-player
directionality is consistently unfavorable. Do not replace this evidence with
the passing focused benchmarks or describe it as proven host noise.

Paired allocated-byte ratio median [95% interval]:
```
players=1  0.983295 [0.963851,0.996768]
players=4  1.011244 [0.960418,1.026274]
players=16 0.995694 [0.970625,1.062673]
```
At four players p50 latency ratio median is 1.15, interval [0.99088,1.26404].
Other latency ratios/spreads and all raw values are in the JSON. Short-window
p99 is diagnostic only. No application-wide allocated-byte or latency improvement
is claimed.

## Environment evidence and attribution limits

During the fixed run the coordinator observed the following Windows counters:
```
Win32_PerfFormattedData_PerfOS_Processor _Total PercentProcessorTime=99
Everything#1 PercentProcessorTime=1122
vmmemWSL PercentProcessorTime=191
System PercentProcessorTime=117
several python processes approximately 100 each
```
Process percentages can exceed 100 across cores. This establishes competing
CPU load during the experiment, not its causal share of any measured slowdown.
The coordinator also observed 33.6 GB free RAM of 134 GB. Neither observation
justifies dropping rows or declaring the source innocent. Causal attribution
remains unknown. A separate controlled confirmation needs its own fixed plan
before data and must retain this original dataset.

## Read-only workload comparability audit

The unchanged harness has relevant limitations:

- Each player's RNG is seeded identically (0xBEEF+index), but runWindow stops
  by wall time. Different speeds produce different-length command prefixes,
  counts and realized command proportions. Identical seeds are not identical
  completed work. For example first one-player baseline look/say counts are
  221/189, while candidate counts are 144/149.
- Warmup is also time-limited and mutates the world. The same store/runtime
  then runs player levels 1,4,16 in sequence. Faster warmups/earlier levels can
  leave a different state and background queue for later measurement windows.
- The deadline stops starting commands; wg.Wait drains in-flight commands
  afterward. Goodput divides by the full actual elapsed duration, including
  that drain. Multi-second tails can dominate a nominal three-second window.
- Mallocs/TotalAlloc cover the entire process, including background runtime
  work/forks. The denominator is successful foreground commands, not total
  task work. Bytes/op therefore need not compare the same work under changed
  scheduling, retry counts, background activity or command mix.
- Per-shape attempt/retry deltas overlap under concurrency, as the harness
  explicitly documents. They are approximate at multiple players.
- runtime.GC runs after warmup and before measurement; this deliberately
  influences pooled/cache state. The source delta must tolerate pool misses,
  and the focused correctness tests cover independent scratch construction.
- The output field named max is actually pick(0.999), not a literal maximum.
  Keep the original label in raw logs, but do not interpret it as the slowest
  command.

No harness or production source was edited during this review. These limitations
explain what the measurements can establish; they do not explain away the
observed lower candidate goodput.
