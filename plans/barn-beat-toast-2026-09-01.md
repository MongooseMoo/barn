# Beat Toast where it counts — Barn strategy, 2026-09-01

Four asks from Q: find what makes Barn amazing and push it; make Barn faster
than Toast; make multicore/MVCC shine; organize the repo. This plan answers all
four with evidence gathered today (sources: `notes/barn-strategy-2026-09-01.md`,
`experiments/perf-review-20260831/`, `experiments/2026-09-01-prop-access/`,
`experiments/2026-07-14-mongoose-performance-baseline.md`).

## 0. What was not obvious

**The scoreboard was wrong.** Every performance campaign so far optimized
`BenchmarkVM` tight loops (int arithmetic, list index). On those Barn is
already within 1.5–2.0x of Toast, and real MOO code never runs them. Toast is
single-threaded: its ceiling on Mongoose is roughly one command every 3 ms,
about 300 commands/s, on any machine. Barn on 16 cores is at 104/s, and Barn
with ONE player is also about 100/s. Sixteen players buy nothing. That is the
whole story, and it has two causes, neither of which is "Go is slower than C":

1. **The retry policy.** Optimistic MVCC re-executes a losing task from
   scratch, immediately, with no backoff, up to 64 times, and only escalates on
   attempt 63 into a global exclusive gate. On the real workload `@who` needs
   ~250 attempts per success (91 commands, 54,959 attempts) and averages 1 s.
   The synthetic MVCC harness reaches 127,000 commits/s with 0% abort, so the
   store is not slow. The policy is.
2. **Allocation shape.** 43% of all bytes allocated on the real workload are
   per-verb-call frame setup (`startVerbCall` + `PrepareVerbFrame`: a heap
   `StackFrame` and a `make([]Value)` per call). Another 16% is
   `syncTaskLineNumbers` building a `Map` of every local in every frame on
   EVERY raise, caught or not. Toast only builds that map when
   `$server_options.include_rt_vars` is set (execute.cc:452–560). Mongoose
   raises and catches errors constantly. GC plus malloc is 40% of CPU.

Both are fixable without touching semantics, and both are invisible to
`BenchmarkVM`.

**Heap size is a GC cost, not just an RSS cost.** Barn holds ~800 MB of
pointer-dense heap for a 96 MB Mongoose database (Toast: 311 MB RSS total).
Every GC cycle marks that whole graph. Property names are re-resolved and
duplicated per object at load; string values are not interned.

**MVCC has unclaimed gifts.** A checkpoint today takes the commit gate
EXCLUSIVE for the whole object walk (`db/store/store_snapshot.go:71`). A
versioned store can dump a consistent snapshot while writers keep committing.
Toast forks the process to checkpoint; Barn can do better than both with code
it already has.

## 1. What makes Barn amazing (and what to push)

| Already true | Push |
|---|---|
| Only MOO server with Toast semantics on N cores (MVCC, per-task snapshots, suspend = commit) | Make it actually scale on real code (§3) and publish the numbers |
| Amortized O(1) list/string append: `list_append_30k` 0.003x Toast, `string_concat_50k` 0.14x | Extend the same discipline to map updates and `setadd`, add to bench_differ corpus |
| Observability: structured JSON logs, tracebacks with source lines, pprof, expvar, `barn_logs` | Zero-pause checkpoints; Prometheus (#208); per-verb cost profiling exposed to wizards |
| Modern builtins already ported: sqlite, fileio, crypto/argon2, HTTP, JSON, maps, waifs, anonymous objects, multiple inheritance | Bytes type (`docs/design/bytes-type.md`), native TLS/websocket listeners (plan exists) |
| A real conformance oracle loop (Toast-first YAML, managed harness) | Add a *performance* oracle: Toast-vs-Barn on the same multi-player Mongoose mix |
| Federation design (Grange, `plans/moo-federation.md`) built on suspend-commits semantics | Ship after the multicore story is proven; do not start before |
| bench_differ: in-process Barn vs Toast microbench with value comparison | Add realistic shapes: verb calls, try/except, inherited props, `$prop`, builtins, maps |

Not yet an advantage: **Unicode**. Barn strings are Go strings but conformance
(#67, PR #210) pulls indexing toward Toast's byte semantics. Decide explicitly:
byte-faithful `TYPE_STR` for conformance plus a real `bytes`/text story from
the bytes design doc. Do not market Unicode until that decision is made.

## 2. Scoreboard

Headline metric: **committed commands/s on the real Mongoose database with a
realistic mix (look 35 / say 30 / i 10 / @who 10 / home 15) at 1, 4, 16 players**,
via `engine/mongoose_real_bench_test.go`, plus **p99 latency** and **abort rate**.
Secondary: bench_differ ratio on a realistic corpus; RSS after load; checkpoint
pause time.

| Metric | Toast | Barn today | Target |
|---|---:|---:|---:|
| Real mix, 1 player, goodput | ~300/s (est. from 3 ms/cmd; MEASURE) | ~100/s | ≥ 300/s |
| Real mix, 16 players, goodput | ~300/s (serial ceiling) | 104/s | ≥ 1,500/s |
| Real mix, 16 players, p99 | n/a | 3.9 s | < 50 ms |
| Real mix, abort rate | 0 | 42% | < 5% |
| bench_differ tight loops | 1.0x | 1.5–2.0x | ≤ 1.2x |
| bench_differ realistic corpus | 1.0x | unmeasured | ≤ 1.0x |
| RSS after Mongoose load | 311 MB | 1,880 MB | < 600 MB |
| Checkpoint commit pause | fork() pause | full walk under exclusive gate | 0 |

Toast's real-mix number does not exist yet. Phase 0 measures it.

## 3. Workstream A — make multicore shine (the headline)

A0. **Toast ceiling measurement.** Drive the same mix against the pinned
mongoose Toast oracle over TCP with N simulated players and record goodput.
Toast serializes, so N barely matters; record 1 and 16 anyway. This is the
number Barn must beat. Extend `../moo-conformance-tests/bench/` or a new
`scripts/bench_players.py`.

A1. **Retry policy (biggest lever, smallest change).** Three independent,
individually measurable pieces, each an experiment on the 16p harness:
  - *Escalate early.* Today's experiment (`escalateAfterAttempts` 63 → 2,
    results appended to `notes/barn-strategy-2026-09-01.md`) tells us whether
    the existing global gate is enough once the harness is faithful.
  - *Lock on retry, per object.* After the first loss, the retry acquires
    per-object write-intent locks for the objects it conflicted on (the
    validator already knows them: `debugConflict` in `store_txn.go`), sorted
    by id, before re-executing. Other retriers wait instead of colliding.
    Fresh tasks stay optimistic. This is standard OCC-with-lock-on-retry.
  - *Early abort at write time.* On staging a write, compare the object's
    live version to the txn's read version; abort immediately instead of
    running the whole verb to a doomed commit.
  Gate: 16p abort < 5%, goodput up, `go test -race ./db/store ./engine`,
  managed conformance green.

A2. **Property-granular copy-on-write.** `privatizeCached` deep-clones the
whole object (properties map, verb list, verbs map, chparent map) on the first
write. `@who` writes one property on 188 players and clones 188 whole objects
per attempt. Share the immutable verb structures always; clone only the
properties map (or better: a persistent map with path copying).

A3. **Stop manufacturing contention.** The 07-27 census showed most global
writes are `#0:handle_uncaught_error` appending to `#24.traceback_log` because
Barn raises errors Toast does not: `say → #3882::execute` line 10 (450/run),
`@who → #55:map_builtin` line 12, `look → #2700:process_players` line 21,
`home → #20:regexp_quote` (`rmatch` on `[][$^.*+?%]`). Each is a Rule Zero
conformance target against the mongoose oracle. Fewer Barn bugs = fewer
global writes = fewer conflicts. Track the uncaught-error count per run as a
harness metric so it cannot regress silently.

A4. **Zero-pause checkpoint.** Replace the exclusive-gate walk in
`SnapshotWithRoots` with a read transaction at a fixed `readTS` that walks
published images through the existing history mechanism. Commits proceed
during the dump. Measure pause time before/after; conformance pins
`dump_database()` output and the `CHECKPOINTING` log line, so both stay
byte-identical.

A5. **Scheduler batches.** The forked/suspended-task scheduler is fork-join on
a 10 ms ticker with unbuffered hand-off; batch latency is the slowest task.
Interactive commands bypass it, so this is lower priority, but `fork`-heavy
Mongoose code (output chains) will hit it once A1 lands. Replace with a
per-worker queue and no barrier.

## 4. Workstream B — serial speed on real code

B1. **Frame slab.** Allocate `StackFrame` structs from a per-VM slab (reuse on
pop; only escape when a task suspends and its VM is snapshotted) and locals
from a contiguous per-VM value stack (`Locals = stack[base:base+n]`). Expected:
roughly −40% bytes/op on the real workload. Guard: frames must not be retained
after pop anywhere (`snapshotActivationFrames` copies today; audit
`GetCallStack`, traceback, task_stack, fork).

B2. **Lazy traceback variables.** `HandleError` must not build per-frame
variable maps unless `$server_options.include_rt_vars` is set (Toast's
`SVO_INCLUDE_RT_VARS`) or the traceback is consumed by `task_stack(...,
include_variables)`. Also defer `buildTraceback` until a handler is found or
the error is uncaught. Expected: −16% bytes/op; every `try/except` idiom in
Mongoose gets cheaper.

B3. **Error values without `fmt.Errorf`.** 157 `fmt.Errorf("E_...")` sites in
vm+builtins; `HandleError` parses the code back out of the string. Raise a
typed `MooError{Code, Detail}`; format only when a message is displayed.

B4. **Verb lookup index.** `walkVerb` scans `verbList × lowerNames` with
`strings.Index` per compare on every ancestor. Precompute per object: an
exact-name map (name → earliest index) plus an ordered slice of wildcard
verbs only; a lookup is one map hit and a short wildcard scan, taking the
earliest match. Then promote the per-txn resolution memo to a store-level
cache keyed by `(obj, name, verbShapeEpoch)`; the memo already replays the
read-set marks, so validation stays exact.

B5. **Finalization scan gating.** `popFrame` walks every local, arg and
pending error through `collectPendingFinalizationsFromFrame` on every return
(≈10% CPU incl. linear `pendingFinalizationValueInList`). Keep a per-frame
"may hold finalizable" bit set on store of a waif/anon value; skip the walk
when clear. Replace the linear dedupe list with the identity set from
`types.WaifSet`.

B6. **Realistic bench_differ corpus.** Add `experiments/corpus-real.txt`:
verb call chain 6 deep, `` `x.p ! E_PROPNF' `` loop, inherited property read
through 6 ancestors, `$prop`, `for x in (list)`, map get/put, `index/strsub/
match/tostr` mixes, `valid()` through a protected-builtin wrapper verb. Run it
with every change. This is the corpus that predicts Mongoose.

B7. **Free wins.** Profile-guided optimization: commit a `cmd/barn/default.pgo`
from the real-workload CPU profile (Go applies it automatically); expect
2–10% on an interpreter. Build linux/amd64 with `GOAMD64=v3`. Record both in
bench_differ.

B8. **Tight loops (last).** Only after B1–B7: superinstructions for
`local op const`, `cmp + jump`, `x = x + 1`; count ticks per backward jump
and call rather than per opcode. Target ≤ 1.2x Toast; parity is not required
to win the scoreboard.

## 5. Workstream C — memory and GC

C1. Intern property names at load: share the name slice with the defining
ancestor instead of `resolvePropertyNames` per object (256 MB).
C2. Intern string values at load by content (131 MB of `NewStr`, mostly
duplicates); optional runtime intern for short strings.
C3. Pointer-sparse property storage: values in a flat `[]Value` per object
with a shared name→index map per definer, not `map[string]Property` per object.
C4. Measure: heap after load, GC mark time per cycle (`GODEBUG=gctrace=1`),
and real-workload goodput; target RSS < 600 MB and a measurable GC CPU drop.

## 6. Workstream D — repository organization

Working tree is 12 GB excluding `.git`, with 394 untracked root files: ~330
`notes-*.md` agent scratch files, ~60 `test_*/tmp_*/toast_*` transcripts,
15 scratch database copies of 65–458 MB each, stray `nul.#N#` files. Tracked
cruft: three `*.exe~`, four root `notes-*.md`, `PLAN.md` and
`REMEDIATION_PLAN.md` (obsolete pre-implementation docs), `out`,
`zen_generated.code`, `review-mvcc-transaction-findings.md`.

Target layout (all reversible, one PR):
- `notes/` — agent scratch and checkpoints. Move the four tracked root
  `notes-*.md` there; add `/notes-*.md` to `.gitignore` so the hook-driven
  scratch convention keeps working without polluting root. Sweep the ~330
  untracked ones into `notes/archive/2026/` (they are untracked, so this is a
  move on disk only; Q approves deletion instead if preferred).
- `docs/` — durable prose: keep `design/`, `reports/`, move `TRACING.md`,
  `operator-http.md` stays. Retire `PLAN.md` and `REMEDIATION_PLAN.md` to
  `docs/history/`.
- `plans/` — active plans only; move finished ones to `plans/done/`.
- `experiments/` — keep; add the missing `INDEX.md` rows for 08-31 and 09-01.
- `scripts/` — add `scripts/clean-scratch.ps1` that deletes root scratch
  databases, transcripts and stray `nul.#N#` files by pattern (dry-run
  default). Q runs it; the plan does not delete 10 GB on its own.
- `.gitignore` — add `/nul.#*`, `/notes-*.md`, `/*.cpu`, `/*.mem`,
  `/*.stackdump`, `/main.py`, `/hg.html`, `/compare.sh`, and untrack `*.exe~`.
- `repository_hygiene_test.go` — replace the hard-coded obsolete list with a
  rule: no tracked root file matching the scratch patterns.
- Windows runtime DLLs (`argon2.dll`, `pcre.dll`, `sqlite3.dll`, `nettle`,
  `libstdc++`) stay at root until the loader path is verified; note in README.

## 7. Sequencing

1. A0 (Toast ceiling) and B6 (realistic corpus) first: they are the two
   measurements every later claim depends on. One session.
2. A1 retry policy. Three experiments, one branch each. This is the day the
   16p number stops equalling the 1p number.
3. B2 + B3 (lazy tracebacks, typed errors) and B1 (frame slab): serial
   allocation roughly halves; GC share falls; 1p goodput should pass Toast's
   ~300/s. B7 free wins ride along.
4. A2 property-granular COW and B4 verb index: the remaining big allocators
   and the biggest CPU item outside GC.
5. A4 zero-pause checkpoint: the first visible "MVCC feature" users notice.
6. C1–C3 memory; B5; A5; B8 tight loops last.
7. D runs in parallel as one hygiene PR plus a cleanup script for Q.

Every step: experiment branch, before/after on the 16p harness and
bench_differ, `go test -race ./db/store ./engine`, managed conformance green,
record under `experiments/`. Kill criteria per `protocols:experiment`.

## 8. Issues

| Item | Issue |
|---|---|
| A0 Toast multi-player ceiling | #265 |
| A1 Retry policy (per-object locks, early abort, backoff) | #266 |
| A2 Property-granular COW | #267 |
| A3 Barn-manufactured contention (uncaught errors) | #268 |
| A4 Zero-pause checkpoint | #269 |
| A5 Scheduler without fork-join batches | #270 |
| B1 Frame slab | #271 |
| B2 Lazy traceback variables | #272 |
| B3 Typed MooError | #273 |
| B4 Verb lookup index + store-level memo | #274 |
| B5 Finalization scan gating | #275 |
| B6 Realistic bench_differ corpus | #276 |
| B7 PGO + GOAMD64=v3 | #277 |
| B8 Superinstructions | #278 |
| C1 Memory: interning, pointer-sparse properties | #279 |
| D Repository organization | #280 |
