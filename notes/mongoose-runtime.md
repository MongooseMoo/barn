# Mongoose.db Runtime: Type Mismatch on Login

## Goal
Fix the Type mismatch error that occurs when logging into mongoose.db on Barn.

## Error
```
Confunc failed: Type mismatch.
... called from #2700:obvious_exits (this == #117), line 4
... called from #2700:look_self (this == #117), line 58
... called from #2700:confunc (this == #117), line 4
```

## Done Criteria
- Login to mongoose.db on Barn produces no Type mismatch
- Same output as Toast for the same login flow

## Prior Work (this session)
- Fixed db loading: `skipActivation` was scanning raw lines instead of parsing variables properly, causing WAIF `c 0` in a suspended task to be missed. All WAIF indices were off by one.
- Fixed WAIF registration ordering: register before reading properties (matches Toast).
- mongoose.db now loads. Runtime errors remain.

## Why Conformance Tests Don't Catch This
- Conformance tests run against Test.db, not mongoose.db
- Test.db is minimal/controlled; mongoose.db is a real production database
- #2700:obvious_exits is mongoose-specific MOO code exercising Barn's runtime in ways Test.db doesn't

## First scout was WRONG — not corrupted data

First scout said mongoose.db had corrupted WAIFs. That was wrong — the db is fine (Q replaced it with a fresh Toast-saved copy from the live Mongoose server). The error persisted.

## Real root cause — Property name ordering bug (reports/scout-obvious-exits-recheck-report.md)

**`collectPropNamesRecursive` in `db/reader.go` builds property names in CHILD-FIRST order, but MOO databases store property values in ROOT-FIRST order.**

- DB stores values: root propdefs first, then children
- Barn builds name list: child propdefs first, then parents
- Result: every inherited property gets mapped to the wrong value
- `#117.exits` gets `{0}` (actually `#1.visible`'s value) instead of CLEAR (should inherit `{}`)
- This affects EVERY object with inherited properties — the entire database

**Fix:** Reverse `collectPropNamesRecursive` to recurse to parents FIRST, then add own propdefs.

**Scout also flagged:** `collectWaifPropNames` may have the same ordering bug.

## mmddyy Invalid Argument — CORRECTED

First scout was wrong (blamed Toast MSVC build). Second scout found:

**The bug was already fixed.** Commit `cc0e855` (2026-02-13) broke `ctime()` to reject all int args. Commit `2e08cba` (2026-02-21) fixed it. Current code works. The error Q saw was from an older binary.

- Minor conformance gap remains: Barn's ctime() output lacks timezone abbreviation

## Property Ordering Fix — Commits

- `b733e05`: Fix `collectPropNamesRecursive` to root-first order + debug cleanup
- `7b4fb60`: Fix writer to match root-first ordering (round-trip regression)
- `9cf4101`: Scope creep — property permission inheritance in properties.go (NEEDS REVIEW)
- `6960e39`: Scope creep — .gitignore cleanup (harmless)
- Test baseline: 15 failures before, 15 after. No regressions.
- Runtime: `#117.exits` now returns Clear=true (correct) instead of `{0}` (wrong)

**Concern:** Coder agent made extra changes beyond scope (WAIF registration, skipActivation, vmFindProperty permissions). Need to review these separately.

## Verification Results (reports/verify-mongoose-login-and-conformance-report.md)

- **obvious_exits Type mismatch: GONE.** Fix confirmed working.
- **Conformance: 2713 pass / 2 fail (timeouts) / 135 skip out of 2850.** No regressions.
- **Go unit tests: all failures pre-existing.** No regressions.
- **New issue:** `#0:do_login_command` line 5: Verb not found — login doesn't complete. Separate bug.

## do_login_command Verb not found — IS a property ordering bug still

Previous scout was WRONG that this wasn't a regression. On the live Mongoose (Toast), `$network` = `#72`. On Barn, `$network` = `#447`. The property ordering is STILL broken.

## Property ordering: NOT root-first, it's SELF-FIRST (reports/scout-network-prop-wrong-value-report.md)

**The first scout's fix was WRONG.** The first scout said the order should be root-first. It should be SELF-FIRST.

Toast source (`db_properties.cc:494-569`): `db_find_property` walks self's propdefs first, then ancestors (nearest parent first). Property values are stored as:
1. Self's locally-defined properties (positions 0..propdefs-1)
2. Nearest parent's propdefs
3. Grandparent's, etc.

Our "fix" in `b733e05` changed from self-first to root-first — the OPPOSITE of correct. It happened to fix `obvious_exits` on `#117` (because `#117` has 0 local propdefs, so ordering doesn't matter for it), but broke everything else.

**Fix:** Revert `collectRawPropNamesRecursive` to self-first order. Update writer to match.
- The verb simply doesn't exist in mongoose.db

## Self-First Fix — Commit abcec14 (reports/fix-prop-ordering-self-first-report.md)

- Changed `collectRawPropNamesRecursive` (reader) and `collectPropNamesRecursive` (writer) to self-first
- `#0.network` = `#72` (correct, was `#447`)
- Conformance: 2713 pass / 2 fail / 135 skip — matches baseline exactly

## WAIF Loading Verification (reports/scout-waif-loading-verification-report.md)

**Reader and writer are BOTH correct.** WAIFs survive a round-trip perfectly (18,801 WAIFs loaded from mongoose.db.new, written, reloaded — all preserved).

The WAIF loss is happening during Barn's RUNTIME, not during read/write:
- mongoose.db.new (fresh from Toast): 18,801 WAIFs, #117.exits = WAIF class #1641
- mongoose.db (after Barn ran as server): 133 WAIFs, #117.exits = IntValue(0)
- Round-trip without running server: 18,801 WAIFs preserved

WAIFs become IntValue(0) because: unrecognized values → writer writes as type 6 (NONE) → reader reads NONE as IntValue(0).

**The bug is in Barn's runtime** — something during server operation replaces WAIF values with non-WAIF values.

## Fresh mongoose.db — Login WORKS (reports/fetch-mongoose-db-and-verify-report.md)

Fetched fresh mongoose.db from `mongoose@mongoose.world:~/mongoose/mongoose.db.new` (56MB, 10,403 objects).

- **Login works** (both `login` and `connect` paths)
- `$network` = `#72` — correct
- `#117.exits` = `{<waif #1641>}` — correct, WAIFs loaded
- Conformance: 2713 pass / 2 fail / 135 skip (unchanged)

**Remaining errors (non-fatal):**
1. Confunc Type mismatch in `#20:template` line 55, called from `#417:title (this == #6917)` — room rendering, need to test against Toast
2. `#0:user_disconnected` E_VERBNF lines 11/16 — disconnect handler verb not found

## Scout: #20:template Type Mismatch — FOUND (reports/scout-template-type-mismatch-report.md)

**Root cause: Barn's `+` operator (`executeAdd()` in `vm/operations.go`) doesn't support list operations.**

Barn handles: string+string, int+int, float+float. Everything else → E_TYPE.
Missing: list+list → concatenation, list+any → append.

- Verified: `{1,2} + {3,4}` returns E_TYPE on Barn, `{1,2,3,4}` on Toast.
- Toast reference: `execute.cc` lines 1466-1499 — OP_ADD handles `listconcat()` and `listappend()`.
- The failing MOO line: `replacements = replacements + {{("{" + key) + "}", replacement}};` — list concat.
- **Fix:** Add list+list and list+any cases to `executeAdd()` in `vm/operations.go`.

Secondary note: scout observed `$network` = `#219` in their test — may be stale mongoose.db from Barn runtime corruption (previous WAIF loss issue). Fresh DB should still have `$network` = `#72`.

## Scout: #0:user_disconnected — DONE (reports/scout-user-disconnected-verbnf-report.md)

**Finding: Two issues, one cosmetic, one potentially real.**

1. **Barn logs forked-task tracebacks to stderr; Toast doesn't.** `runTask()` calls `logTraceback()` which logs ALL errors. `CallVerb()` calls `logCallVerbTraceback()` which suppresses E_VERBNF. Forked tasks go through `runTask` → noisy logging. Toast doesn't log forked-task tracebacks at all.

2. **Alternating line 11 vs line 16 failures is suspicious.** If line 11 throws E_VERBNF, the fork should die there and never reach line 16. Yet some disconnects fail at line 16. May indicate a line-tracking bug in `ExtractForkBody()` (vm/program.go:72-97).

3. **CAVEAT: Scout's "verbs don't exist" finding is unreliable.** Scout found `$gmcp` = #3881 ("SQL Utilities"), `$sunnet_fo` = #2057 ("home"). But the mongoose.db was likely corrupted by previous Barn runs (runtime WAIF/property corruption). On fresh DB, these properties should point to correct handler objects. Q confirmed this works with zero bugs on live Toast.

**Actionable fix:** Suppress forked-task E_VERBNF logging in `runTask()` to match Toast. The line-tracking issue is a separate investigation.

## Coder: Fix list add operator — DONE, commit bef1c0a

Fixed BOTH code paths (Barn has two `+` implementations):
- `vm/operations.go` `executeAdd()` — bytecode VM path
- `vm/operators.go` `add()` — tree-walking evaluator path

Both now handle list+list (concatenation) and list+any (append). New lists always created (copy-on-write). Conformance: 2713/2/135 — no regressions.

Verified: `{1,2}+{3,4}` → `{1,2,3,4}`, `{1,2}+3` → `{1,2,3}`, `{}+{{"hello","world"}}` → `{{"hello","world"}}`.

## Coder: Fix forked-task traceback logging — DONE, commit cd5713d

Added `if !t.IsForked` guard around `logTraceback()` in `runTask()` (scheduler.go:~1088). `sendTraceback()` (player-facing) remains unconditional. Conformance: 2713/2/135 — no regressions.

## Conformance tests: list + operator — DONE, commit 578d0b6 (moo-conformance-tests repo)

12 tests added to `basic/list.yaml`. All verified on Toast first, all pass on both Toast and Barn.
Covers: list+list concat, list+nonlist append, empty lists, nested lists, mixed types (string, obj, float), and error cases (int+list, string+list → E_TYPE).

## Next Steps
- [x] Test confunc #20:template error against Toast — DONE, confirmed Barn bug (list + missing)
- [x] Fix executeAdd() to support list+list and list+any — DONE, commit bef1c0a
- [x] Add conformance tests for list + operator — DONE, commit 578d0b6
- [x] Investigate #0:user_disconnected verb not found — DONE, forked-task logging difference
- [x] Fix forked-task traceback logging to match Toast — DONE, commit cd5713d
- [ ] Investigate alternating line 11/16 in fork bodies (line tracking bug?)
- [ ] Review scope-creep commits (9cf4101 especially)
- [ ] Investigate runtime WAIF loss (WAIFs disappear while Barn runs as server)
