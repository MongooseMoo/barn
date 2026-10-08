# Deferred GC concurrent latency assessment (#270)

## Frozen plan

Baseline production: `34e6622`. Candidate production: `e46c433` (the engine
tree remains the previously verified `5ae0bc6` tree). No production changes
are part of this assessment. Copy the new opt-in harness unchanged to baseline.

Run five alternating baseline/candidate pairs on Windows Go 1.27.1 with
GOMAXPROCS=4. Each run has two scenarios: 1,000 and 100,000 iterations of real
MOO integer arithmetic per background invocation. Two independent background
principals continuously create one anonymous object and perform that work.
Each result must equal n*(n+1)/2. The long scenario deliberately raises the test
task quota; it explores long uninterrupted slices, not default-quota frequency.

Keep admission capacity at eight, leaving spare slots for a foreground command
and an independent admission probe. Each sends 1,200 requests on a fixed 5 ms
schedule. Record command service latency, completion lateness relative to that
schedule, and admission wait, including p99, p99.9, and maximum. Serial request
submission catches up after a stall; lateness records that backlog rather than
hiding it through coordinated omission. Background completions measure realized
work; their counts need not match when service rates differ.

Hold finalization during warmup, then seed the preceding expensive sweep at
the common start time and release the hold. Subsequent cadence and cost are the
unaltered production policy. Record cumulative sweep service and the anonymous
candidate count while load is still running (that snapshot can include active
objects). Validate every command and background result. No synthetic sleeping
builtin, database from production, manual server, or MOO semantic change is used.

Decision: investigate repeated candidate admission/command stalls over 50 ms
with more than 20 ms added maximum wait versus the paired baseline. This is an
explicit synthetic responsiveness guard, not a claimed product SLA. Compare
the short and long scenarios before attributing the delay to drain or sweep;
do not claim production throughput or a speedup from this experiment. If the
guard fails, hold merge and retain all samples for a bounded design revision.

## Results and decision

The plan/harness was committed before sampling as `cf45a6f`. Harness blob in
both worktrees: `121d034527dfa4aeef15fc330d35e731dfe64b1e`. All ten samples
finished with `sample_exit=0`; every arithmetic and foreground acknowledgement
matched. Raw samples and the complete percentile/count inventory are in
`2026-10-08-deferred-gc-concurrent-evidence/` (`paired-summary.csv`).

| Pair | Short base command max ms | Short candidate command max ms | Long base command max ms | Long candidate command max ms | Long candidate admission max ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1 | 2770.409 | 2444.234 | 0.525 | 10.128 | 5.611 |
| 2 | 2924.296 | 2101.864 | 0.556 | 11.282 | 6.324 |
| 3 | 2196.554 | 2174.251 | 0.544 | 56.740 | 23.056 |
| 4 | 770.901 | 1266.666 | 39.124 | 94.810 | 23.354 |
| 5 | 580.487 | 1274.835 | 17.799 | 98.703 | 7.710 |

The long case crosses the declared responsiveness guard in pairs 3, 4, and 5;
the short case crosses it in pairs 4 and 5. Recommendation: hold merge pending
a bounded latency improvement and a repeated unchanged-workload comparison.
These measurements do not invalidate the previously verified root safety and
idle progress, but they do not establish acceptable concurrent responsiveness.

Long-case anonymous snapshots are 1486/471, 1467/440, 1457/349, 559/109,
and 1039/208 (base/candidate). Four long baseline runs performed no sweep;
the candidate performed sweep work in every run. Some latency is the actual
cost of settling work that the baseline postponed, rather than admission wait
alone. The admission probe and command maximum measure different boundaries:
a command can already hold admission and wait behind the VM-start barrier.

Both versions have large rapid-allocation stalls. Counts and baseline latency
also varied substantially across pairs; uncontrolled host scheduling and Go GC
remain possible contributors. Do not interpret this as an application-level
regression estimate or a production SLA result. The long case uses a larger
quota than the defaults. The observed command maxima still warrant investigation
before promotion; a low service p99 alone would hide the resulting request backlog.

## Diagnostic profile and bounded next change

A separate candidate CPU-profile run (excluded from the paired samples) recorded
1856.391 ms maximum command latency and 2141.145 ms cumulative sweep service.
The focused CPU profile reports:

```text
flushDeferredGC                       cumulative 1.94 s
RecycleOrphanAnonymousBatch            cumulative 1.91 s
recycleFrozenAnonymousCandidates       cumulative 1.85 s
already-recycled map check (line 483)   cumulative 1.09 s
recycle callback (line 487)            cumulative 0.70 s
```

The router nests requests over the same immutable candidate list. Once every
candidate is handled, later requests still scan and check that list. The first
bounded improvement to evaluate is an early exit once that frozen list has been
exhausted, retaining request order, first applicable context, exactly-once
routing, and the protection for objects created by recycle hooks. This identifies
a concrete avoidable cost; it does not prove the optimization will meet the guard.
Existing VM tests exercise the frozen-candidate and hook-created-object laws.
Preserve them and compare the same harness before expanding scheduler scope.

If reducing redundant sweep work is insufficient, attribute residual delay
before changing architecture. Long admission drains require a safe way to park
active VMs at a shorter boundary. Expensive tracing/recycling requires a design
that preserves root validity and recycle-hook ordering while reducing pause work.
Simply dropping a sweep, ignoring active roots, or using repeated pause timeouts
would undermine the progress contract and is not an acceptable fix.

The first profile attempt used relative output paths; Go exited successfully but
the named artifacts were absent and pprof exited 2. Repeating with verified
absolute paths produced both artifacts and pprof exited 0. The useful profile
text is retained separately; the executable and binary CPU profile stay local.

## Verification and scope

The new opt-in harness compiles on both trees. It also ran on the candidate with
the race detector (`go test -race ./engine -run
'^TestDeferredGCConcurrentMeasurement$' -count=1 -timeout=90s -v`):

```text
PASS
ok github.com/MongooseMoo/barn/engine 13.594s
harness_race_exit=0
```

Race-instrumented measurements are diagnostic and are excluded from the paired
performance samples. Through assessment commit `cb4c607`, production engine/VM
source and the managed gate configuration remained unchanged. The earlier full
Go/race/managed evidence covers that production tree; the concurrent assessment
does not replace exact-head CI before merge. Subsequent router implementation and
verification are recorded in `2026-10-08-deferred-gc-router.md`.
No collector optimization, scheduler redesign, publication, or merge is included
in this assessment. The router optimization was subsequently authorized and
implemented in the next recorded phase.
