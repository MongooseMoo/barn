# perf/readts-shard-pad

## Goal
Measure whether padding `readTSShard` (db/store/store_core.go:35-42) to a full
64-byte cache line helps, then fix the false comment. Q chose "measure, then fix".

## State (2026-10-26)
- Branch at origin/master cae320f3, nothing committed. Lead's rule: if 64-byte
  padding is not measurably faster, commit nothing and report options.
- 64-byte padding is NOT measurably faster. Nothing committed. Waiting on Q's
  choice via the lead.
- Uncommitted in the worktree: `db/store/readts_registry_bench_test.go`
  (`BenchmarkReadTSRegistryParallel`, sub-benchmarks BeginRelease and
  RegisterDeregister) and this note.

## Layout facts (scratch unsafe probe, deleted)
- master: Sizeof(readTSShard)=56, oldest at offset 16; Sizeof(Store)=2912;
  readers at 1584, shards at +24; &shards[0] mod 64 = 16 on 4 NewStore calls.
- 40-byte pad: 64 bytes; Store 3040 (same 3072 size class).
- no pad: 24 bytes; Store 2400.

## Measurement
Windows, Ryzen 9 5950X, Go 1.27.1, test binaries built once per layout,
10 alternating rounds, -cpu 1,4,8,16, 1s benchtime. Host load sampled
(Win32_Processor.LoadPercentage) before each run: 0-56%.
- master vs 64-byte: no significant change at any -cpu (all p >= 0.39).
- master vs no pad: BeginRelease no significant change; RegisterDeregister
  +20% at -4 (p=0.009), +16% at -8 and -16 (p=0.000).
Raw benchstat output is in the lead's report.

## Decisions and why
- Benchmark drives BeginSnapshot(ts)+Release (the real begin/end path,
  store_txn.go:135, store_history_gc.go:363), with explicit readTS so values
  spread across shards; every production caller passes 0 (clock), which with
  no commits would put all workers on one shard.
- oldestLiveReadTS not exercised: it is called only from pruneWaifHistory
  (store_txn_waif.go:205), which release() calls only when waifs.pending is set
  (store_history_gc.go:351), i.e. when WAIF history exists.

## Surprises
- The full BeginRelease path (9 allocs, 1088 B, finalizer, two global RWMutex
  RLocks: s.mu and readers.floorMu) swamps any shard-layout effect.

## Decision (lead, 2026-10-26)
Fix the comment only; keep the 56-byte layout. Q asked to measure then fix, and
the measurement settles it: 64 bytes buys nothing, removing the pad costs 16-20%
on register/deregister. The comment now states the real spacing and the
measurement; the benchmark is committed as its source. Lead re-ran benchstat on
the raw files and got the same tables.

## Next step
PR to master, merge on green CI.
