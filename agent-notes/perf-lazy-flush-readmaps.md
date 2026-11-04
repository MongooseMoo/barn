# Lazy flush read maps

## Goal

Fix the remaining eager read-map allocation in FlushStagedToLive identified during Q's requested Claude review of PR #598. Q asked "can we / should we fix the flush thing?" within the already-authorized store optimization work. Use Sol 6.1 for coding; parent supervises measurement, review and delivery.

## State

Independent branch perf/lazy-flush-readmaps in C:/Users/Q/code/barn-lazy-flush-readmaps, based on integrated master after PR #598. Preserve main checkout's unrelated dirty files. Q clarified: "the question being why there are 7 distinct places to nill and not just one". Sol implemented the revised ownership change in four files, with 139 insertions and 17 deletions including tests and benchmark. Parent inspected the complete source change and authored regression. Sol's focused command `go test ./db/store -run '^TestStoreTxnFlushPostReadsValidate$' -count=1` returned `ok github.com/MongooseMoo/barn/db/store 1.045s`. Sol is stopped after delivery. No PR yet.

Frozen benchmark executables were built successfully under main checkout's .tmp: lazy-flush-base.exe and lazy-flush-candidate.exe. Baseline worktree C:/Users/Q/code/barn-lazy-flush-base uses a Go overlay pointing only its commitalloc_bench_test.go at candidate's identical benchmark source. Ten sequential alternating-order pairs completed. Raw output and benchstat are in experiments/lazy-flush/. Without later reads: 7280 to 6944 B/op, 45 to 38 allocs/op. With location/property reads: 8992 to 8752 B/op, 53 to 48 allocs/op. Every observation has these allocation results. Elapsed-time differences are inconclusive (p=0.684 and p=1.000). Parent inspected the raw observations, benchmark lifecycle and complete source/test diff. One inaccurate allocation comment was corrected after the frozen build; no executable logic changed.

## Decisions and why

Smallest structural change: move the seven object-version dependency maps into one inline objectReadSet value embedded in StoreTxn, deleting their individual declarations from StoreTxn. Flush clears that value with tx.objectReadSet = objectReadSet{}. This makes reset ownership explicit without merging differently keyed dependencies, adding an allocation or duplicating fields. Existing promoted selectors can keep referring to the actual maps in that owner; no aliases, forwarding methods or compatibility wrapper. About 25–40 production lines including relocation and the corrected stale comment. Add focused regression/benchmark coverage in the existing files, about 80–140 lines. No new flag, lock, transaction policy or general cleanup.

Source checked: db/store/store_txn_live.go:345 onwards, StoreTxn field layout, CommitAndRenewCarryingReads and hot renewal. All seven maps reset together in flush and are carried together during renewal. Every insertion path was changed to lazySet in #598, except a direct assignment that updates an existing ranged verbReads key. Failed/no-op flushes return before resetting dependencies and must remain unchanged. Keep renewal copy algorithms unchanged. verbMemoSeen is separate memo bookkeeping, cleared independently during memo materialization; WAIF and map-key state also carry writes and are outside this object-version group.

The real producer is builtins/store_reads.go:flushStagedBeforeCoarse. It returns without flushing unless HasStagedTopology is true. Benchmark a staged move or equivalent actual topology path, with and without post-flush reads, rather than a scalar-only artificial flush. Preserve explicit transaction Release. Verify post-flush reads still produce correct results and dependencies; avoid tests that merely mirror nil assignments.

Measurement: one benchmark source for both lanes via Go overlay, frozen executables, at least ten alternating baseline/candidate observations, GOMAXPROCS=4/GOGC=100. Report B/op, allocations/op and elapsed time separately. This small fix does not justify claiming a server-throughput gain or repeating the noisy Mongoose macro experiment. CI runs on PR; no local full CI duplication. Authored focused tests may run after edits. Read docs/elle-isolation.md and inspect the pinned CI evidence because the changed maps track transaction reads.

## Surprises

Claude called the method FlushStagedWrites; the actual method is FlushStagedToLive. It also suggested "two lines"; the old code had seven assignments and a three-line comment. The first benchmark launch used unquoted dotted flags; PowerShell passed `-test`, and the binary rejected it with `flag provided but not defined: -test` before any measurements. Corrected the actual command to quote each complete flag, such as '-test.bench=^BenchmarkTxnTopologyFlush$'. No rejected launch is included as a measurement.

## Next step

Publish the independently based PR with concrete allocation results. Await PR CI and pinned Elle evidence, review the actual PR, then normal merge and cleanup. Do not post a GitHub review. Frozen binaries and overlay live outside both disposable worktrees and must be preserved with measurement evidence. Full local CI was not duplicated; its results are unknown until the PR checks complete.
