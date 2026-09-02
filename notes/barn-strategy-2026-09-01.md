# Barn strategy session — 2026-09-01

Q's ask: (1) find what makes Barn amazing and push it, (2) make Barn faster than
Toast, (3) make multicore/MVCC shine, (4) organize the repo. Think/plan/explore;
open issues where work is not done here.

## Facts gathered (current-turn, with sources)

### Single-thread VM vs Toast (bench_differ 2026-09-01, master 2c9ce9c)
- Tight loops: Barn 1.5–2.0x slower (list_index 1.99x, prop_access 1.72x,
  int/float arith ~1.55x, nested loop 1.49x, builtin_abs 1.58x).
- tostr parity (0.98x). string_concat 0.14x, list_append 0.003x (Toast O(n^2)).
- bench_differ corpus has NO realistic shapes: verb calls, try/except raise+catch,
  inherited property fetch, `$prop`, builtins like index/strsub/match, for-in,
  maps. Mongoose lives there, not in tight arithmetic loops.

### End-to-end Mongoose (experiments/2026-07-14-mongoose-performance-baseline.md)
- Toast: look 3ms, @who 2ms, move 6ms, RSS 311MB.
- Barn: look 11ms, @who 4ms, move 4ms, RSS 1.88GB (heap 800MB retained from DB
  load: NewMap 247MB, ResetProperties 222MB, NewStr 131MB, resolvePropertyNames).

### Real workload harness (engine/mongoose_real_bench_test.go, run-after 2026-08-31)
- 16p: goodput 104/s, abort 42%, p50 32ms, p99 3.9s, 44MB + 177k allocs per
  command. @who: 91 ok for 54,959 attempts (~250 retries per command; every @who
  runs to the k=63 escalation cap). look avg 66ms, say 65ms.
- 1 player (July 27): 78–133/s. So 16 players buy ZERO goodput over 1 player.
- Retry policy: engine/task_runtime.go maxConflictRetryAttempts=64, escalate at
  63 into the GLOBAL commitGate (exclusive). No backoff. No early abort at write
  time. No per-object lock on retry.
- Contention census (07-27): 100% property conflicts on global objects (#24
  traceback_log via handle_uncaught_error, @who writes every player's .aliases,
  output chain RMW appends). Part of the error traffic is Barn divergences
  (say→#3882::execute, @who→#55:map_builtin, look→#2700:process_players) — each
  uncaught error = a write to global #24 = a conflict.

### 16p CPU profile (experiments/perf-review-20260831/cpu-after.prof)
- gcDrain 26%, mallocgc 16.5% (GC+alloc ≈ 40%).
- Verb dispatch: walkVerb 8.4% cum, matchVerbNameLowered 4.8% (linear name scan).
- Finalization/waif scans still ~10%: collectPendingFinalizationsFromFrame 5%,
  collectPendingWaifsFromFrame 4.3%, MayHoldFinalizable 2%, waifRep.equal 1.3%,
  pendingFinalizationValueInList 2.1% (linear list scan on every frame pop).
- syncTaskLineNumbers 5.5%; HandleError 6%; NewMap 3.8%; maybeProtectedRedirect
  18% cum (protected builtin → MOO wrapper verb; expected for mongoose).

### Repo hygiene
- 1058 tracked files. Root has ~350 untracked cruft files (notes-*.md x ~330,
  test_*/tmp_*/toast_* transcripts, scratch DBs). Tracked cruft: 3 `*.exe~`,
  4 root `notes-*.md`, reports/patch-summary.txt. `notes/` dir already exists
  (68 files). No `default.pgo` anywhere (PGO unused).

## Working hypotheses (not yet verified)
- H1 Retry storm is the multicore killer: lock-on-retry (per-object intent
  locks after first loss) + early write-time abort + jittered backoff would
  convert @who's 250 retries into ~1-2. Global gate at low k serialized the
  server; per-object locks would not.
- H2 The GC wall is heap SIZE (800MB pointer-dense live heap scanned each
  cycle) as much as alloc rate. Interning property names/strings at load and
  pointer-sparse layouts cut both RSS and GC mark cost.
- H3 Remaining serial gap on real code is dispatch (verb lookup linear scans,
  per-call frame setup, line-number sync, eager tracebacks), not arithmetic.
- H4 PGO + GOAMD64=v3 are free single-digit % wins nobody has taken.

## Current blocker
None. Next: alloc profile, harness knobs, GC config, verb-lookup/finalization/
syncTaskLineNumbers/HandleError code reads, then write the plan + issues.

## Checkpoint 2 — alloc profile + code reads (2026-09-01)
- 16p alloc_space (53GB total over 16s): startVerbCall 15.8GB flat (30%) +
  PrepareVerbFrame 7.2GB (13.5%) = 43% of ALL allocation is per-verb-call
  frame setup (`&StackFrame{}` + `make([]Value, NumLocals)` per call).
- syncTaskLineNumbers 8.5GB cum (16%): HandleError calls it on EVERY raise
  (caught or not) and it builds a types.NewMap of all locals for EVERY frame
  (NewMap 6.3GB, NewStr 2.8GB). Toast's make_stack_list only builds the
  rt-var map when include_variables is set (execute.cc:452-512); the only Barn
  consumer is builtins/tasks.go:533 (task_stack include_variables). Pure waste.
- buildTraceback 1.7GB, GetCallStack 2.1GB, snapshotActivationFrames 1.1GB,
  ActivationFrame.ToList 0.9GB: eager traceback machinery on every raise.
- privatizeCached/cloneObjectForReadTxn 2.7GB: first write to an object deep
  clones ALL properties, verbList, verbs map, chparentChildren. @who writes
  188 players' .aliases → 188 full clones per attempt × ~250 attempts.
- Resolve memo (store_resolve_cache.go) is per-txn and disabled after the
  first staged write; verb walk is a linear scan over verbList×lowerNames per
  ancestor with strings.Index per compare (walkVerb 8.4% CPU).
- Retry loop (engine/task_runtime.go:100-330): no backoff, no early abort,
  escalation only at attempt 63 into a GLOBAL exclusive gate.
- Checkpoint (store_snapshot.go:71): holds commitGate EXCLUSIVE for the whole
  object walk — MVCC could make this a free snapshot read.
- Suspend commits the txn and begins a fresh read-only txn (task_runtime.go
  ~410-434): correct Toast semantics; federation plan relies on it.
- Value is a 24B tagged union (types/value.go) — de-box already done.
- Unicode: strings are Go strings; #67/PR #210 push toward Toast's byte
  semantics for indexing. "Unicode" is not a settled Barn advantage yet.
- End-to-end bench (../moo-conformance-tests/bench/bench.py) is single-conn
  tight-loop evals; no multi-player Toast-vs-Barn goodput number exists.
- Working tree is 12GB excluding .git; 394 untracked root files; 925 dirty
  entries. repository_hygiene_test.go pins a fixed obsolete list + ignore
  patterns. ~/.claude/hooks/note-counter.sh drives the notes-*.md convention.

## Strategic conclusion so far
Beat Toast on the metric that matters: committed commands/sec for N real
players on Mongoose. Toast's ceiling is one core (~300/s at 3ms/cmd). Barn
today: ~100/s at ANY player count (retry storm). Path: (1) retry policy
(lock-on-retry + early abort + backoff) unlocks scaling; (2) frame slab +
lazy traceback vars roughly halve serial allocation; (3) property-granular
COW; (4) verb index; (5) heap compaction for GC mark cost; (6) PGO/GOAMD64.

## Checkpoint 3 — k=2 experiment done (2026-09-01)
Recorded in experiments/2026-09-01-escalate-at-2.md. 16p: goodput 87→109/s,
abort 66→43%, p99 3.7s→363ms, bytes/op 60→21MB, but p50 8→157ms (global gate
serializes). 1p unchanged at 129/s (look 11.6ms vs Toast 3ms = 3.9x serial
gap on real code). Plan written: plans/barn-beat-toast-2026-09-01.md.
Next: open issues per plan §8, hygiene PR, commit plan+notes+experiment.
