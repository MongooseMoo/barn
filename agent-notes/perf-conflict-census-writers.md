# Conflict census: writers, read sites, unchanged values, cost

Branch `perf/conflict-census-writers`, worktree `../barn-conflict-diag`, based
on master after #582. The team lead opens the PR; this agent never pushes.

## Goal
Under census tracking, each lost property or map-key conflict records: the
writer (label + verb/line that staged the version the loser went stale on), the
loser's first-read site with slice ticks at read and at loss, whether the live
value is identical to what was read, wasted ticks per key, and a per-slice
attempts histogram. Commit quality: zero cost when off, a server option to turn
it on, a read-only JSON debug endpoint, tests with mutation checks, docs.

## State
Done, in two commits on the branch:
1. Store and engine: db/store/store_conflict_diag.go (records), hooks in the
   store transaction files, engine/conflict_diag.go (site callback from the VM
   frame), engine/task_runtime.go (NoteSliceAttempts per slice), the workload
   harness printout (engine/mongoose_real_bench_test.go), store tests.
   ConflictKind gained MarshalText so JSON shows kind names.
2. Server option `CONFLICT_CENSUS = 0|1` (config), applied in
   server.LoadDatabase; `Server.Store()` (atomic pointer, set after load);
   debug route `/debug/conflict-census` (internal/app/census.go); tests; docs in
   CLAUDE.md and docs/operator-http.md.

3. Totals: ConflictCensus.TotalTicks (ticks of every counted slice over all
   its attempts, passed by the engine as executedTicks to NoteSliceAttempts),
   WastedTicks and WastedShare(); the harness attempts_per_slice line prints
   total_ticks=, wasted_ticks=, wasted_share=%.

4. Fix for wasted > total (Mongoose 16p run: wasted_share=104.2%). Cause,
   proven by engine/conflict_diag_test.go (after_fork case, before the fix:
   total=2, wasted=307): engine/task_runtime.go read the attempt's ticks
   (attemptTicks = bcVM.Ticks - sliceEntryTicks) right after launchSlice and
   before drainForks, which resumes the VM after every fork and runs the rest
   of the slice. Everything a task executed after its first fork statement was
   missing from attemptTicks, executedTicks and TotalTicks (and from the
   VMAttempt/RetryWaste metrics' ticks), while the site callback, sampled at
   the commit after drainForks, counted it. Fix: drainForks runs before the
   ticks are read; WastedTicks is now the engine's attemptTicks summed at the
   two retry points (rerunTicks, passed as NoteSliceAttempts' third argument),
   no longer the siteFn sample in lostValidation, so wasted <= total by
   construction. Per-key ReadTicks/LossTicks/Diag.WastedTicks still come from
   siteFn; with the reorder a final-commit loss's sample equals the attempt's
   ticks (asserted), and a boundary loss's sample is the ticks up to the
   irreversible builtin.
   Checked and ruled out by reading: retry checkpoint restoring Ticks (launchSlice
   resets bcVM.Ticks = sliceEntryTicks on attempt > 0, task_runtime.go
   launchSlice), pooled VMs (task VMs are never pooled, vm/pool.go), closure
   capturing a stale VM (installed per attempt on the VM that runs it), renew
   carrying siteFn (same VM and entry).
   Mutation-checked, each failed and was restored: ticks read before drainForks
   (after_fork fails), rerunTicks not added at the commit retry (fresh,
   after_fork, resumed fail), not added at the boundary retry (boundary fails),
   siteFn sum put back in lostValidation (engine and store tests fail),
   NoteSliceAttempts not adding rerunTicks (store and app tests fail).
   vmElapsed is still measured before drainForks (it also feeds lag reports);
   left alone, not in scope.

Verified: build; gofmt, go vet, staticcheck clean on db/store, engine, config,
server, internal/app; targeted tests pass (store Conflict/Renewal/MapKey,
internal/app, config, server ConflictCensusOption, engine Census|Conflict).
Benchmarks vs master with tracking off, count=6 interleaved, benchstat: store
TxnPropertyWriteCommit, TxnReadOnlyCommit, TxnFindPropertyAncestry and engine
ThreadedContinuations players=1: no significant change; store B/op and
allocs/op identical. Engine noise was 6 to 13 percent, so only a large engine
regression is ruled out.

Mutation checks (each failed its test, all restored): writer site dropped,
history walk skipped, read ticks zeroed, loss ticks not sampled, wasted ticks
split across keys, property and map-key unchanged forced false, histogram
overflow bucket off by one, diag allocated with tracking off, endpoint Tracking
forced false, parser ignoring CONFLICT_CENSUS, server not applying it.

## Decisions and why
- Kept the stopped agent's rework instead of dropping it: one pointer on
  StoreTxn (nil when tracking is off) makes every recording site a single nil
  test, and maxWriterSlots bounds the writer registry, which a long-running
  server needs.
- Writer/read-site/value only for property and map-key conflicts; wasted ticks
  for every kind.
- Writer = publisher of the first rewrite after the version read (history
  walk), else the live version's writer.
- Endpoint is its own path, not an expvar under /debug/vars, so /debug/vars
  stays small and the census lock is taken only when asked.
- Option is applied at load, not toggled live: SetConflictCensusTracking only
  affects transactions begun afterwards, and writers published before it are
  unknown anyway.

## Surprises
- Server-initiated hooks (engine/call_verb.go) get no site callback; their reads
  and writes show as unknown sites.
- The debug endpoint starts before LoadDatabase, so the census handler reaches
  the store through an atomic pointer set after the load.

- The census showed wasted > total because ticks run after a fork statement
  were never counted in the attempt (item 4). The VMAttempt and RetryWaste
  metrics had the same undercount before the fix.

## Next step
Team lead re-runs the Mongoose workload to confirm wasted_share < 100% and
opens the PR. The engine wiring is now covered by
TestConflictCensusWastedTicksAreTheRerunAttempts (fresh, after fork, resumed
after suspend, boundary loss). Not re-measured: the Mongoose workload numbers
after the fix.
