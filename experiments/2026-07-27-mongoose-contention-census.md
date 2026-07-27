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

After the fix, `#2700:title` errors vanish. Remaining uncaught errors are
**genuine mongoose db bugs** (E_TYPE: `#10104:hour_of_day` line 3 —
`day_length_hours` is INT 24 multiplied by 3600.0; `#9501:
cluster_by_proximity` line 21 — same mixed-arithmetic class; both recently
authored in-db). Toast-identical by pinned facts: conformance pins mixed
INT/FLOAT arithmetic as E_TYPE and MOO operators have no override
mechanism. ~1.3 uncaught errors **per look** feed the #24 handler on the
live server too.

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
- Biggest remaining lever is not MVCC machinery — it is that **mongoose.db
  errors ~1.3×/look in recently-authored verbs** (`hour_of_day`,
  `cluster_by_proximity`), and every error is a global serialized write.
  Fixing two in-db verbs would cut the #1 conflict source at the origin.
- Barn-side levers, profile-ranked, still open: GC/alloc wall (~31% CPU),
  verb-dispatch ToLower/matchVerbName (~25%), regexp cache for match(),
  CompileMOO sourceKey rehash, callstack snapshot allocs.

## Kept observability (env `BARN_DEBUG_RETRY=1`)

- DEBUG-RETRY (retry loop), DEBUG-CONFLICT (validator mismatch detail) —
  from the previous arc.
- DEBUG-PROPWRITE (committed property-write keys, Commit success path).
- DEBUG-UNCAUGHT (error + top frame at handle_uncaught_error invocation).
