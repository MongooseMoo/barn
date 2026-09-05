# Retry-policy experiments for issue 266

Date: 2026-09-04. [Issue 266](https://github.com/MongooseMoo/barn/issues/266).

## Decision

Do not integrate these runtime prototypes. The tested A+C combination has median goodput of 81/s at 16 players versus 90/s at 1 player, 37.30% aborts, and 1995.50 ms p99. It fails all three performance criteria.

This is the issue's negative-result fallback, not a performance fix. Keep issue 266 open and retain the current runtime. The delivery contains this report, a reproducible prototype archive, and two standalone instruction commits recording defects found during experimentation.

These results reject the tested parameterizations; they do not establish that every possible retry policy or combination will fail.

## Revisions and archive

Every isolated mechanism starts from baseline `16c71c8b870976e8d71eb9a2ba5b355dbaa1b7dc`.

| Configuration | Candidate commit |
| --- | --- |
| A: retry reservations, corrected | `4c424f8aa79b3c533dc9ae8d6acc6cfe24d89816` |
| B: early write-time abort | `08bd7df156891c9e7a7e63ee528cc452364d329e` |
| C: jittered backoff | `b0e49a916141d21b3fc50363d68a64ea15d10d96` |
| A+C: combined, not isolated | `a57e2dadd7e74fd8ae15874bd278c87ad4da7c13` |

[The incremental Git bundle](2026-09-04-retry-policy.bundle) preserves these heads and their experiment history. It requires the baseline commit and does not contain the database fixture. The archived code is experimental and is not part of the runtime tree.

- Bundle bytes: `12514`.
- Bundle SHA256: `85EAF62EAED00DA32152CBDC170673C40047F0FDB843EDEE8966513012C04E96`.
- `git bundle verify experiments/2026-09-04-retry-policy.bundle` returned exit code 0.

To import the experimental refs into a clone that contains the baseline:

```powershell
git fetch experiments/2026-09-04-retry-policy.bundle 'refs/heads/*:refs/remotes/issue266/*'
```

Create a separate worktree at the desired candidate commit. Do not replace a working checkout with a prototype.

## Mechanisms

- A reserves typed conflicting object IDs before re-execution. Commit arbitration checks the complete staged footprint. A shared admission guard remains held through publication; disjoint commit guards can coexist. Synthetic conflicts require a live runtime retryability predicate.
- Non-retryable, live-mutated, and irreversible-effect transactions bypass A's synthetic rejection. A reservation is therefore not an unconditional promise that its owner wins against every transaction; attempt-63 escalation remains the backstop.
- B checks recorded facets against immutable numbered-object images at mutation/staging boundaries. A private unwind signal abandons execution and enters normal retry cleanup. Anonymous targets retain commit-time validation, and final validation still handles changes after an early check.
- C uses full jitter over an exponentially growing window, capped at 32 ms. It waits after failed-attempt cleanup and transaction release, respects the task context, and adds no delay to the first attempt or attempt-63 escalation.
- A+C combines A's abort/tail improvement with C's throughput improvement. B was excluded: its isolated goodput, abort rate, and p99 were all worse than corrected A's.

`const escalateAfterAttempts = 63` is unchanged in all four archived heads.

## Measurement protocol

- CPU: AMD Ryzen 9 5950X, 16 physical cores / 32 logical processors.
- RAM: 137344585728 bytes.
- OS: Microsoft Windows 11 Pro, version 10.0.26200, build 26200.
- Go: `go1.26.0 windows/amd64`; measured `GOMAXPROCS=32`.
- Fixture: `mongoose.db.new`, 95638161 bytes.
- Fixture SHA256: `27EF0023B79395C315905D7BA4DE1EAC93E01405438FE5A3CF7C2F6E6306E8CD`.
- Harness: `engine/mongoose_real_bench_test.go`, `TestMongooseRealWorkload`.
- Mix: look 35%, say 30%, inventory 10%, who 10%, home 15%; normal numeric promotion enabled.
- Warmup 2 seconds; measurement 8 seconds; three separate-process samples per configuration/player count.
- Own benchmark processes ran sequentially. This was a shared workstation, not a controlled dedicated benchmark host.
- Tables retain the harness's reported values. Aggregation is the median of each metric across the three runs, not an average or a single selected run.

Run from the candidate worktree with the same fixture. Set the player count to 1 or 16 as appropriate:

```powershell
$env:BARN_MONGOOSE_BENCH = '1'
$env:BARN_MONGOOSE_DB = 'C:\path\to\mongoose.db.new'
$env:BARN_MONGOOSE_PLAYERS = '16'
$env:BARN_MONGOOSE_WARMUP = '2s'
$env:BARN_MONGOOSE_MEASURE = '8s'
@('run-1.log','run-2.log','run-3.log') | ForEach-Object {
  go test ./engine -run '^TestMongooseRealWorkload$' -count=1 -v *> $_
  if ($LASTEXITCODE -ne 0) { throw ('benchmark failed: ' + $_) }
}
```

An initial baseline `-count=3` invocation produced one valid timed sample, then two fixture-load failures after the harness changed the process working directory. Those failures have no performance metrics. Baseline runs 2 and 3 were collected in separate processes; every later sample also used a new process.

## Raw timed samples

Latency columns are milliseconds. Goodput is successful commands/second. Allocations/op and bytes/op are the harness's per-operation values; GCs is its reported collection count. Every row below reported `failed=0`. Accepted prototype logs also had zero ERROR/panic lines.

| Configuration | Players | Run | Goodput/s | Abort % | p50 ms | p99 ms | Max ms | Allocs/op | Bytes/op | GCs |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Baseline | 16 | 1 | 91 | 62.83 | 8.64 | 3919.36 | 4413.94 | 204787 | 53134381 | 42 |
| Baseline | 16 | 2 | 88 | 63.31 | 8.95 | 4016.23 | 4642.93 | 203621 | 52961456 | 41 |
| Baseline | 16 | 3 | 94 | 61.60 | 8.07 | 3776.93 | 4375.14 | 203719 | 52828301 | 43 |
| A | 16 | 1 | 80 | 39.68 | 26.63 | 2207.71 | 3859.45 | 74522 | 19336118 | 8 |
| A | 16 | 2 | 82 | 39.84 | 28.28 | 1860.97 | 4127.60 | 74334 | 19166943 | 7 |
| A | 16 | 3 | 82 | 40.29 | 29.79 | 2246.84 | 5008.44 | 73783 | 18889157 | 8 |
| B | 16 | 1 | 81 | 73.43 | 9.71 | 2959.77 | 5183.95 | 212458 | 55065782 | 12 |
| B | 16 | 2 | 79 | 72.66 | 10.75 | 2824.84 | 5031.08 | 213487 | 55426399 | 12 |
| B | 16 | 3 | 83 | 73.21 | 10.02 | 2688.44 | 4978.91 | 208005 | 53879149 | 13 |
| C | 16 | 1 | 115 | 52.07 | 6.71 | 4139.86 | 4806.84 | 141538 | 36712739 | 40 |
| C | 16 | 2 | 101 | 52.87 | 7.45 | 4104.26 | 4811.54 | 149034 | 38638026 | 36 |
| C | 16 | 3 | 111 | 51.46 | 7.09 | 4102.94 | 4698.33 | 144340 | 37377106 | 39 |
| A+C | 16 | 1 | 81 | 38.83 | 19.06 | 1832.54 | 3778.35 | 73984 | 19011956 | 12 |
| A+C | 16 | 2 | 81 | 37.30 | 17.32 | 2502.35 | 3577.81 | 73296 | 18830438 | 12 |
| A+C | 16 | 3 | 83 | 35.07 | 14.01 | 1995.50 | 4637.56 | 71761 | 18598010 | 12 |
| A+C | 1 | 1 | 93 | 0.00 | 11.54 | 25.54 | 92.09 | 27908 | 7000835 | 4 |
| A+C | 1 | 2 | 88 | 0.00 | 11.98 | 29.01 | 92.80 | 28053 | 7048183 | 4 |
| A+C | 1 | 3 | 90 | 0.00 | 11.86 | 29.61 | 100.40 | 28081 | 7053555 | 4 |

## Per-metric medians

| Configuration | Players | Goodput/s | Abort % | p50 ms | p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| Baseline | 16 | 91 | 62.83 | 8.64 | 3919.36 |
| A | 16 | 82 | 39.84 | 28.28 | 2207.71 |
| B | 16 | 81 | 73.21 | 10.02 | 2824.84 |
| C | 16 | 111 | 52.07 | 7.09 | 4104.26 |
| A+C | 16 | 81 | 37.30 | 17.32 | 1995.50 |
| A+C | 1 | 90 | 0.00 | 11.86 | 29.01 |

| Final performance criterion | A+C result | Decision |
| --- | --- | --- |
| 16-player abort rate < 5% | 37.30% | Reject |
| 16-player goodput > same candidate at 1 player | 81/s versus 90/s | Reject |
| 16-player p99 < 50 ms | 1995.50 ms | Reject |

A reduces aborts and tail latency but loses throughput. C improves throughput but not tail latency. A+C does not retain C's throughput gain and remains far outside the latency/abort targets.

## Invalid preliminary A trials

These trials are preserved for the audit trail and excluded from selection and medians. All were at 16 players under the same timing protocol; their `failed=0` counters concealed MOO-visible errors.

`c0da7889bd2b8ff3d22ac3d88ab5893d2cdc1a5d` released reservation admission before publication. A deterministic regression showed that a new reservation could overtake an admitted commit.

`df46edc1c34878ebcaada05dbcae74e8db390733` repaired that gap but still injected synthetic conflicts into non-retryable `do_command` calls. Its logs contained `E_INVARG` exceptions. The live retryability predicate was added before the corrected A series above.

| Candidate | Run | Goodput/s | Abort % | p50 ms | p99 ms | Max ms | Allocs/op | Bytes/op | GCs | ERROR/panic lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| c0da788 | 1 | 91 | 37.95 | 20.75 | 2089.45 | 3402.04 | 70623 | 18248362 | 12 | 114 |
| c0da788 | 2 | 94 | 37.55 | 22.00 | 2134.63 | 2520.50 | 67211 | 17460184 | 12 | 109 |
| c0da788 | 3 | 89 | 37.98 | 23.02 | 2123.36 | 4184.87 | 68967 | 17839111 | 12 | 109 |
| df46edc | 1 | 92 | 37.39 | 22.05 | 1992.63 | 3655.72 | 67974 | 17704618 | 12 | 85 |
| df46edc | 2 | 95 | 38.94 | 27.48 | 2059.38 | 4812.73 | 68866 | 17688064 | 12 | 127 |

The third publication-only trial was not run after the error-log defect was identified. Do not interpret the preliminary apparent gains as valid performance evidence.

## Verification and limits

Focused regression-first checks covered the internal stale-write unwind, reservation admission/publication ordering, non-retryable commit behavior, same-object/disjoint commit arbitration, backoff bounds/canceled contexts, and optimistic writer serializability.

Commands run on the relevant archived candidates:

```text
go test ./db/store -run '^TestStalePropertyWriteSignalsInternalEarlyAbort$' -count=1
go test ./db/store -run '^TestRetryReservationCannotOvertakeAdmittedCommit$' -count=1
go test ./db/store -run '^TestReservationDoesNotRejectNonRetryableCommit$' -count=1
go test ./engine -run '^(TestRetryBackoff.*|TestOptimisticConflictingWritersAreSerializable)$' -count=1
```

The new defect regressions were observed failing before repair and passing afterward. Combined focused store/engine checks also passed. Bundle verification and path-scoped whitespace checks cover the report delivery.

The prototypes did not reach the performance gate, so they were not promoted to full functional qualification. In particular, this report does not claim complete mixed-footprint, cancellation/suspension/panic-path coverage, a passing `go test -race ./db/store ./engine`, or a passing managed conformance run.

Those remain mandatory before any future runtime integration. CI for this report must not be interpreted as qualification of code stored inside the archive.
