# Go server-performance review notes (2026-08-31)

Task: Q asked "what are we not doing right" for high-perf Go server apps. Read-only review, no changes.

## Established so far
- Value = 24B tagged union (done well). GOGC=400 default. VM pool for nested VMs only.
- Verb dispatch ToLower now hoisted; regex cache exists; program cache exists (July 28 levers addressed).
- StackFrame is ~30 fields, heap-allocated per verb call ([]*StackFrame), Locals make() per call.
- Every task start takes global lifecycle.VMStartMu + Runtime.mu (acquireTaskExecution).
- TCPTransport.WriteLine: string concat + Flush per line -> one syscall per notify line.
- Scheduler: unbuffered work chan, results chan per item; heap under one mutex.
- Commit path: commitGate RWMutex + decentralized COW or store.mu exclusive.
- Real workload (07-28): 16p goodput ~77-111/s, abort 63-68% — optimistic MVCC vs Mongoose's global-object RMW writes. Retry loop re-executes up to 64x with no backoff; escalation at 63.
- Latest profiles are July 28: GC ~31%, dispatch 25% (since fixed), regexp, sourceKey (fixed).

## Next
- Inspect scheduler dispatch loop (polling ticker?), checkpoint (stop-world?), read faithful cpu profile top.

## Confirmed 2026-08-31 (2nd pass)
- Scheduler = fork-join: input loop 10ms ticker -> ProcessReadyTasks -> Plan batches (<=workers) -> Run sends to unbuffered chan and JOINS whole batch -> flushDeferredGC (needs zero executing tasks). Batch latency = slowest task; idle workers between batches; new input during batch waits.
- Read txns alias immutable images (no clone). privatizeCached deep-clones whole object incl. verb code on FIRST WRITE (7GB in July profile).
- July profile: ToLower 8.7% flat (since hoisted into lowerNames), GC ~25% (gcDrain), map iteration 9.5% (property maps), NewVM 18GB alloc (pool since), regexp 17GB (cache since), findVerb 17.5% cum.
- Fresh 16p profile running: experiments/perf-review-20260831/{cpu,mem}.prof, run.txt

## Fresh 16p profile (experiments/perf-review-20260831/)
- goodput 0/s, p50 11.7s, p99 26s, 6 commands hit deadline. July 28 same harness: 77/s, p50 9.9ms. ~1000x regression.
- CPU: 57% flat waifRep.equal; PersistentWaifRoots 55% cum (objectDir.forEach over ALL objects), called from vm.CollectPendingFinalizationValues (93%) + engine.liveWaifs; dedupe via finalizationValueInList = linear scan with deep Value.Equal => O(waifs^2) per call, per task.
- Alloc: dominated by DB load (resolvePropertyNames, SetProperty) — runtime alloc is now small; alloc is not the problem anymore.
- Commit ae7f529 (today) "Restore VM hot-path performance lost to finalization scans" — check whether HEAD includes it and whether it addressed this path.
- Errors: 157 fmt.Errorf("E_...") sites in vm+builtins; HandleError string-parses; traceback built eagerly on every raise.
- Checkpoint: Snapshot() holds commitGate EXCLUSIVE during full object copy.
- Scheduler fork-join batches on 10ms ticker (commands themselves run sync on conn goroutine).
- WriteLine: msg+"\r\n" alloc + Flush syscall per line.

## Fix in progress (Q: "closest thing that gets us back to fixed")
- Root cause: deferPendingWaifs (engine/waif_lifecycle.go:89) calls vm.CollectPendingFinalizationValues unconditionally (whole-DB PersistentWaifRoots walk) for a shutdown case; plus every waif walk dedupes by linear list scan (O(W^2)).
- Plan: types.WaifSet (identity-keyed, insertion-ordered); set-based walks in db/store/store_reachability.go, store_pending.go, vm/waif_gc.go, vm/anonymous_gc.go(canonicalWaifRoots); split CollectPendingFinalizationValues into CollectDirectFinalizationRoots (cheap, VM-only, at defer time) + CanonicalizePendingFinalizationValues (store walk, only on shutdown); PendingWaif.ShutdownRoots -> DirectRoots. Empty direct roots => skip store walk entirely.
- Verify: go build, go vet, go test ./vm ./engine ./db/store ./server ./types, -race on db/store+engine, then rerun 16p harness (goal: back to >=77/s).
- Edits applied: types/waifset.go (new), vm/waif_gc.go (rewritten set-based, CollectWaifsFromVMInto), vm/anonymous_gc.go (DirectFinalizationRoots split, canonicalWaifRoots sets), db/store/store_reachability.go + store_pending.go (set-based), finalization/coordinator.go (DirectRoots), engine/waif_lifecycle.go (lazy canonicalize, live set). Remaining: engine/runtime.go two sibling loops -> Into; then build/vet/staticcheck/tests/harness.
- All edits done incl. engine/runtime.go sibling loops; types/waifset_test.go added. go build/vet/staticcheck OK; go test types vm db/store engine/... server OK. Harness rerun in progress -> experiments/perf-review-20260831/run-after.txt. Still to do: -race on db/store+engine, full go test ./..., then report.
