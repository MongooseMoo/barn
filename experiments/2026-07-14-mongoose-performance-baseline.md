# Mongoose deployment performance baseline

## Fixed workload

- Fixture SHA-256: `b9bc25492bd56cb28ba0a63165f456c60417387e251391fbe8c97d7d79c9bb69`
- Toast executable: `/root/src/toaststunt-mongoose/build-release/moo`
- Toast executable SHA-256: `a748a93644fe2b973cc85dfed902454a0a56c8b368afdc8104161ec76154d098`
- Runner: `scripts/benchmark-mongoose.ps1`
- Client: existing `cmd/moo_client`, with optional timestamped events and a
  deterministic maximum-duration boundary
- Login input: exactly three newline-separated commands supplied through the
  uncommitted `MONGOOSE_LOGIN_SCRIPT` environment variable
- Post-login commands, identical on both engines: `look`, `west`, `@who`, task
  and connection liveness query, then `dump_database()`
- Client timing: 3000 ms banner wait, 2500 ms between commands, 15 seconds idle
  timeout, and 40 seconds maximum duration
- Resource sample: after a fixed 180-second post-workload settle period
- Checkpoint completion: observed from creation of the disposable
  `<run.db>.new`, not from the command reply

Raw run artifacts stay under `.tmp/mongoose-convergence/` and are not committed
because the client transcript is deployment-local. The summary below contains
no login commands or credentials.

## Pinned WSL Mongoose Toast baseline

Run directory: `.tmp/mongoose-convergence/perf-toast-20260714-03`

| Metric | Toast baseline |
|---|---:|
| Database load to listening | 6392 ms |
| Connect to first banner | 143 ms |
| PROXY send to first output | 3 ms |
| Complete login from PROXY send | 5015 ms |
| Startup `@who` response | 2 ms |
| Explicit `look` render | 3 ms |
| Open-exit movement response | 6 ms |
| Liveness query response | 1 ms |
| Checkpoint command reply | 2 ms |
| Checkpoint file completion | 9429 ms |
| Post-settle CPU | 3.6% |
| Post-settle RSS | 311640064 bytes |

The liveness query returned `{3, 1}`: three queued tasks and one connected
player. The Toast transcript independently reported checkpoint completion in
9.37 seconds, consistent with the runner's 9.429-second file observation.

## Barn acceptance thresholds

These thresholds were fixed from the Toast baseline before measuring Barn:

- database load to listening: at most 12784 ms (2x Toast);
- complete login from PROXY send: at most 10030 ms (2x Toast);
- PROXY-to-first-output, startup command, `look`, movement, and liveness response: at most
  100 ms each (Toast is 1-6 ms; the floor avoids making scheduler jitter the
  acceptance boundary);
- checkpoint file completion: at most 18858 ms (2x Toast);
- post-settle CPU: at most 7.2% (2x Toast);
- post-settle RSS: at most 467460096 bytes (1.5x Toast);
- liveness: one connected player, a nonnegative queued-task count, successful
  `look` and west movement, and a completed checkpoint file.

Barn must be measured with the unchanged runner, fixture, commands, client
timings, and settle period. A failed threshold names the first performance
target; it does not authorize widening to another metric.

Connect-to-first-banner remains an informational measurement, not an acceptance
threshold. The required causal metric in the plan is PROXY prelude to first
banner/output. The client deliberately waits 3000 ms before sending PROXY;
Barn's banner follows that prelude, while Toast emits its banner before it.
Treating Barn's 3001 ms connect-to-banner value as a performance failure would
measure protocol ordering plus the intentional wait, not latency after PROXY.

## Windows Barn baseline

Run directory: `.tmp/mongoose-convergence/perf-barn-20260714-01`

| Metric | Barn | Threshold | Result |
|---|---:|---:|---|
| Database load to listening | 5380 ms | 12784 ms | pass |
| PROXY send to first output | 1 ms | 100 ms | pass |
| Complete login from PROXY send | 5483 ms | 10030 ms | pass |
| Startup `@who` response | 4 ms | 100 ms | pass |
| Explicit `look` render | 11 ms | 100 ms | pass |
| Open-exit movement response | 4 ms | 100 ms | pass |
| Liveness query response | 2 ms | 100 ms | pass |
| Checkpoint file completion | 2341 ms | 18858 ms | pass |
| Post-settle CPU | 0.46875% | 7.2% | pass |
| Post-settle RSS | 1882996736 bytes | 467460096 bytes | **fail** |

The liveness query returned `{2, 1}` and the checkpoint file completed. Barn's
saved `/debug/vars` proves that the RSS failure belongs to the Go heap rather
than an external process-accounting artifact: `HeapAlloc=847925488`,
`HeapInuse=1089699840`, `HeapSys=1945698304`, and `Sys=2008639992` bytes after
12 garbage collections.

The sole active performance target is post-settle RSS. The first hypothesis is
that the loaded database's in-memory object/value representation dominates the
retained heap. The next evidence is a heap profile from the same fixed Barn
workload; no other metric or source surface is active.

## Heap profile and first slice

The unchanged profile-bearing repeat is under
`.tmp/mongoose-convergence/perf-barn-20260714-02`. It reproduced the failure at
2010251264 bytes RSS with all non-memory gates still passing. Forced-GC
`inuse_space` accounted for 814.77 MB, of which database load retained 803.25
MB. The leading flat owners were:

| Owner | Retained bytes | Heap share |
|---|---:|---:|
| `types.NewMap` | 247.06 MB | 30.32% |
| `ObjectBuilder.ResetProperties` | 222.61 MB | 27.32% |
| `types.NewStr` | 131.01 MB | 16.08% |
| `Database.resolvePropertyNames` cumulative | 256.68 MB | 31.50% |
| `Database.readValue` cumulative | 508.04 MB | 62.35% |

This confirms the database-representation hypothesis. The first and only
active source slice is `types.NewMap`: its current `goMap` retains an insertion
order slice of key hashes plus a Go hash map whose values duplicate the full
key/value entries. The slice will delete redundant retained storage for small
maps while preserving exact typed key identity, insertion order, copy-on-write,
and indexed lookup for larger maps. The same deployment benchmark and managed
promotion/conformance gate decide whether the slice is kept.
