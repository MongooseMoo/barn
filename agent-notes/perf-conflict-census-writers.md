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
   WastedTicks (each lost transaction's sampled loss ticks, once, not per key),
   and WastedShare(); the harness attempts_per_slice line prints total_ticks=,
   wasted_ticks=, wasted_share=%. Mutation-checked: wasted counted per key,
   total ticks dropped, reset not clearing the total, share guard removed (NaN),
   WastedTicks not copied out. Each failed a store or app test and was restored.
   Not unit-tested: the engine passing executedTicks (vs one attempt's ticks).

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

## Next step
Team lead opens the PR. Not unit-tested: the engine wiring
(installConflictSite, the NoteSliceAttempts call); it is exercised by the
Mongoose workload harness only.
