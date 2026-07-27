# Real-Mongoose workload harness: the 64-retry storm (2026-07-27)

## Question

Q: Barn "feels slow" running real Mongoose workloads, but the synthetic MVCC
curve says ~35us/command. What is actually slow, and why?

## Instrument

`scheduler/mongoose_real_bench_test.go` (`TestMongooseRealWorkload`, gated
`BARN_MVCC_BENCH`-style behind `BARN_MONGOOSE_BENCH=1`). Loads the real
`mongoose.db.new` (96.5 MB, 17.7k objects, 185 usable players), then drives
real command lines (`look` / `say ...` / `i` / `@who` / `home`) through the
production per-line path — `command.ParsePlayerCommand` → `#0:do_command` via
`CallVerbWithArgstr` → fallback `command.FindVerb` → `ExecuteVerbTaskSync` —
one goroutine per simulated player, exactly like `server/input_processor.go`.
A stub ConnectionManager makes `notify()` a successful no-op and reports the
simulated players as connected. Knobs: `BARN_MONGOOSE_{DB,PLAYERS,WARMUP,
MEASURE,ONLY,CPUPROFILE,MEMPROFILE}`. `-count=1` required (go test caches).

## Baseline result (merged master, pre-fix)

1 player, 10s: **goodput 2-3 commands/s**; look avg 294ms, say 684ms,
@who 788ms; **1.7M allocs and 210MB allocated per command**; abort rate
**94.7% with a single player**. Per-shape commit counters: look = exactly 64
retries/command, say/@who/home/inventory = exactly 128 (the
`maxConflictRetryAttempts = 64` cap, hit by 1-2 tasks per command).
Idle probe: 0 commits in 2s with no commands → no background writer; the
storm is deterministic self-conflict, not concurrency.

## Root cause (proven with BARN_DEBUG_RETRY instrumentation)

1. Every command's uncaught error runs `#0:handle_uncaught_error`
   synchronously (`RunServerVerbTask`).
2. That task — and any real task with a staged write — records verb reads
   keyed by `Verb.name`, which the loader stores as the FULL space-separated
   alias string (`"mail_forward mail_notify"`,
   `db/format/reader_object.go:186-192`), while the live `Object.verbs` map
   is keyed by `names[0]` (`db/store/builder.go:71-72`).
3. `validateVerbReadsLocked` looks up `live.verbs[key.name]`, finds nil for
   every multi-alias verb, returns E_INVARG → `ValidationFailed()` → the
   scheduler retries → same nil lookup → **every commit re-runs to the
   64-attempt cap**. All conflicts logged were `kind=verb want=0 live=0` on
   utility objects (#20 `explode split`, #52 `has_callable_verb hcv`,
   #56 `suspend_if_needed sin`, mail verbs on #40/#46/#57).
4. The synthetic curve never saw it: its fixture verbs have single-word
   names, so read key == map key.

## Fix

`Verb.mapKey()` (= `names[0]`, fallback `name`) in `db/store/object.go`;
used by the three txn key producers (`markVerbRead`, `stageVerbCode`) and
unified across the live-map insert sites (`Store.AddVerb`, `SetVerbInfo`,
`DeleteVerb` repair) — `Store.AddVerb` had keyed the map by the full string,
the loader by `names[0]`; both now use `mapKey()`.
Red-green regression: `db/store/verb_read_alias_test.go`
(`TestAliasedVerbReadCommits`) — E_INVARG before, commits cleanly after.

## Result after fix

| metric (1 player) | before | after |
|---|---:|---:|
| goodput | 2-3/s | **78-133/s** |
| look avg | 294ms | **7.5-11ms** |
| say avg | 684ms | **17-23ms** |
| allocs/op | 1.7M | **19-40k** |
| bytes/op | 210MB | **3-5.5MB** |
| abort (1p) | 94.7% | **0.00%** |

Full `go test ./...` green; `go test -race ./db/store` green.

## What remains slow (next targets, in evidence order)

1. **Real contention at 4-32 players**: abort 50.7% @4p → 71.65% @32p;
   @who avg 1.17s @32p; `home` commands FAIL at 16/32p (retry cap exhaustion
   surfaces E_INVARG to the user — correctness-adjacent). Real players
   cluster in hot rooms and share ancestors → Phase 4 (precise ancestry
   deps) is now THE target, measurable with this harness.
2. **CPU profile** (`experiments/mongoose-real-2026-07-27.{cpu,mem}`,
   pre-fix but shape-representative): GC ~31% cum; verb dispatch
   `FindCallableVerb` 18.6% cum with `strings.ToLower` 10% flat and linear
   `matchVerbName` 15%; `executeGetProp` 10.3%; regexp compiled per
   `match()` call (~4GB alloc, no cache); `CompileMOO` `sourceKey` re-hashes
   full verb source per verb call (1.5GB); `GetCallStack`/
   `snapshotActivationFrames` ~3GB.
3. **Conformance lead (Toast-first before any claim)**: on this DB,
   `#0:do_command` raises uncaught E_INVARG under Barn on every command
   (single frame, line 9); production Barn silently serves all commands via
   the native-parser fallback. Suspects: `verb` intrinsic set to
   "do_command" instead of the command verb; nil-capability `force_input`.
   Needs the managed WSL Toast oracle + a conformance test.

## Diagnostics kept in tree

`BARN_DEBUG_RETRY=1` now logs each conflict retry
(scheduler/task_runtime.go: task/verb/attempt/error) and each validation
failure with its exact dependency (db/store/store_txn.go `debugConflict`:
kind/obj/name/want/live). Near-zero cost when unset.
