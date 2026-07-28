# Mongoose multi-player contention: census, harness fidelity, escalation gate

Date: 2026-07-27. Follow-up to `2026-07-27-mongoose-real-workload.md` (verb
map-key fix). Question from Q: "do we know how to attack the multi-player
contention?" Answer required measurement first; the measurement rewrote the
plan twice.

## 1. Conflict census (BARN_DEBUG_RETRY + new DEBUG-PROPWRITE)

16-player real-mongoose run, all 8,836 validation conflicts attributed:

- **100% property conflicts** (7,562 per-property + 1,274 property-scan).
  Zero scalar, zero relationship, zero verb conflicts. The Phase 2/3 MVCC
  work holds; **Phase 4 precise-ancestry-deps would have fixed nothing** —
  these are real concurrent writes, not false conflicts.
- Shape-isolated attribution (DEBUG-PROPWRITE logs committed write keys):
  - `look`/`home`/`say`: dominated by `#24` ($wiz_utils) `traceback_log` +
    `next_perm_index` — written by `#0:handle_uncaught_error` per uncaught
    error, RMW append + round-robin cursor on ONE global object.
  - `@who`: writes **every connected player's** `.aliases` per invocation,
    plus `#80.<player>pdata` (@paranoid line store) — all @who tasks
    collide with each other.
  - Mongoose's output chain does several shared-property RMW appends per
    told line (`notifies`, `command_log`, `messages`, pdata). Free under
    Toast's global lock; structurally abort-prone under optimistic MVCC.

## 2. `home` failures at 16p: not a bug

147/200 `home` failures at 16 players were players parented on `#410`,
whose ancestry genuinely lacks a `home` verb (barn_eval verified; rooms
`#15`/`#1117` lack it too). Production serves those players the `huh`
fallback (input_processor.go:654). The harness counted legitimate huh
dispatch as failure and only met those players at the 16p roster size — a
cohort effect wearing a contention costume. Harness now mirrors the huh
fallback: **failed=0 at every level**.

## 3. Harness fidelity: protected builtins (the big twist)

The per-look uncaught E_TYPE (`#2700:title` calling `valid(INT)`) chased
through three reversals:

1. ToastStunt *source* (`bf_valid`, objects.cc) raises E_TYPE for
   non-objects — and the conformance suite pins exactly that
   (`builtins/valid_call_shapes.yaml`, 5/5 Toast-green, re-verified today).
2. Yet emergency-mode oracle on mongoose.db returned `valid(1)` → 0.
3. Resolution: **mongoose protects builtins** — `$server_options.
   protect_valid` + `#0`-reachable `bf_valid` wrapper (also bf_create,
   bf_match, bf_random, bf_set_verb_code). Toast redirects protected
   builtins to `#0:bf_<name>`; the wrapper makes valid() lenient.
   Barn implements the same redirect (builtins/protected.go,
   registry.go maybeProtectedRedirect) — **but only server boot loads the
   protect flags**. The bench harness (and barn_eval) never did, so it ran
   raw builtins and manufactured an E_TYPE storm production Barn does not
   produce. Harness now mirrors server boot.

After the fix, `#2700:title` errors vanish. The remaining E_TYPE errors
(`#10104:hour_of_day`, `#9501:cluster_by_proximity` — mixed INT/FLOAT
arithmetic) were initially misdiagnosed here as "genuine mongoose db bugs."

**CORRECTION (2026-07-28):** they are nothing of the kind. The Mongoose
deployment runs the mongoose toaststunt fork's **PROMOTE_NUMBERS** mode
(automatic numeric promotion; second pinned oracle
`/root/src/toaststunt-mongoose/build-release/moo`, commit 72e3c7f9 — see
plans/barn-toast-mongoose-convergence-workstreams.md and
notes-mongoose-promote-and-login.md, which verified this on three engines
on 2026-06-22 and named `cluster_by_proximity` specifically). That db code
is intentional promote-mode code. Barn already implements the mode
(vm/op_arith.go, `--promote-numbers`, default off for stock conformance);
the harness was running strict — a third fidelity gap, now fixed
(BARN_MONGOOSE_PROMOTE, default on).

Fully-faithful 16p/15s baseline (protect + huh + promote, gate k=63):
goodput 82/s, p50 11.2ms, p99 3361ms, abort 67.3%. #24 writes drop
3,060 → 989 per run. The residual uncaught errors are **E_INVARG, not
E_TYPE**, and are the next Rule Zero targets against the *mongoose*
oracle (per-run counts): `say` → `#3882::execute` line 10 (450); `@who` →
`#55:map_builtin` line 12 — `call_function` of some builtin (104); `look`
→ `#2700:process_players` line 21 (62); `home` → `#20:regexp_quote` line 5
— `rmatch` on the legacy `[][$^.*+?%]` class idiom, a likely Barn
regex-translation divergence (10). Stock-mode conformance arithmetic pins
(E_TYPE) remain correct for stock mode; promote mode is a separate lane
with its own oracle.

## 4. Bounded escalation commit gate

Mechanism: `Store.commitGate` RWMutex. Ordinary commits hold it shared
(outermost; lock order commitGate → store locks). After
`escalateAfterAttempts` validation losses, the retry loop takes it
exclusive, re-snapshots, re-executes, and commits a `gateExempt` txn —
which cannot lose, because no ordinary commit can interleave. Unit tests
(`db/store/escalation_gate_test.go`): gate blocks ordinary commit; exempt
txn commits under the gate; escalated attempt never loses against hot
writers (200 iterations, race-clean).

Why: cap exhaustion surfaces a conflict-only E_INVARG to the user — a
phantom "coding error" no serial execution produces. On the (unfaithful)
harness this fired ~5/s at 16 players; on the faithful harness it is
currently zero, but remains possible under any hotter contention. With the
threshold below the cap the class is impossible by construction.

Tuning (16p faithful harness, 15s windows, single runs, ±10% noise):

| config  | goodput | p50    | p99     | max     | abort  |
|---------|---------|--------|---------|---------|--------|
| no gate | 111/s   | 8.6ms  | 2857ms  | 4323ms  | 68.5%  |
| k=48    | 96/s    | 9.0ms  | 2333ms  | 3234ms  | 71.5%  |
| k=63    | 102/s   | 8.5ms  | 2792ms  | 4021ms  | 71.5%  |

(k=8 on the earlier harness serialized the server: goodput −45%, p50
6ms→117ms. Never ship a low threshold.)

**Shipped at k=63**: escalate only on the final attempt — cannot-lose
guarantee at no measurable cost. k=48 is the documented alternative if
tail latency ever outranks ~13% throughput.

## 5. State of the "superduperfast" ledger after today

- Production-faithful 16p baseline: ~111 commands/s goodput, p50 ~9ms,
  abort ~68%. The abort rate is now driven by *genuine* Mongoose write
  patterns (global error log fed by genuine in-db bugs; @who alias cache;
  @paranoid stores), not Barn bookkeeping.
- ~~Biggest remaining lever: fix two in-db verbs~~ **WRONG — retracted
  2026-07-28** (see PROMOTE_NUMBERS correction above). The lever is running
  the Mongoose workload in promote mode, as the deployment does; done. The
  remaining error traffic is four E_INVARG classes that are suspected
  *Barn* divergences to chase via the mongoose oracle.
- Barn-side levers, profile-ranked, still open: GC/alloc wall (~31% CPU),
  verb-dispatch ToLower/matchVerbName (~25%), regexp cache for match(),
  CompileMOO sourceKey rehash, callstack snapshot allocs.

## 2026-07-28 follow-up: the four E_INVARG classes, resolved

| class | symptom | verdict | evidence |
|-------|---------|---------|----------|
| regex | `home`/`@who` chains: `rmatch(s, "[][$^.*+?%].*")` → E_INVARG | **Barn bug — fixed** (a91809f; conformance 8fcd84b) | Toast `{6,11,...}`, Barn E_INVARG; translator had no class state (`[%]` → unterminated Go class) |
| idle render | `look` → `#2700:process_players:21` `idle_seconds()` | **harness artifact — fixed** | stub ConnManager returned nil connections → E_INVARG (network.go:1150); benchConnection now models idle/connected times |
| @who cohort | `#410:@who:11` → `map_builtin` → E_PROPNF | **genuine db state** | protected `call_function` → `#1584:bf_call_function` → `bf_idle_seconds` reads `.cloaked` on wizard targets; wizard #36 lacks it; Toast: `#36.cloaked` → `*Aborted*` (quoted) |
| sqlite | `say` → `#2585:length:7` → "This database is not open" | **genuine db state** | `#2585.sql` is waif index 4035 in the dump; `$sql_utils.databases` = 6277–6279 (lambdamoo-db-py) — the registry was rebuilt and the sound handler kept an orphan. Any server booted from this snapshot errors on `say` until re-registered |

Harness fidelity added along the way: prod-mode listener shape (so
`$prod()` = 1 and `#0:server_started` activates services), `#0:server_started`
boot invocation, repo-root cwd (fileio/sqlite sandbox is cwd-relative), and
real-enough per-player connections. An earlier oracle identity probe ran on
`mongoose_fresh2.db` — wrong fixture (it still had the waifs shared); the
exact-fixture dump analysis is the decisive evidence, with the slow
exact-fixture oracle boot pending as belt-and-suspenders.

Fully-faithful 16p/15s baseline after all of it: goodput 77/s, p50 9.9ms,
p99 5530ms, abort 62.7%, zero command failures. The remaining uncaught-error
traffic (~500/run feeding `#24`) is the snapshot's own two genuine error
classes, which the production deployment presumably also pays after any
restart from this state.

**Actionable for Mongoose (db-side, not Barn):** re-register the sound
handler's SQLite waif (`#2585.sql`) with `$sql_utils`, and give wizard #36 a
`.cloaked` property. Both would cut most of the remaining `#24` global-log
contention at its origin.

## Kept observability (env `BARN_DEBUG_RETRY=1`)

- DEBUG-RETRY (retry loop), DEBUG-CONFLICT (validator mismatch detail) —
  from the previous arc.
- DEBUG-PROPWRITE (committed property-write keys, Commit success path).
- DEBUG-UNCAUGHT (error, message, top + caller frame at
  handle_uncaught_error invocation; full traceback for E_PROPNF).
- DEBUG-CALLFN (call_function name + error on exception results).
