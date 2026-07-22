# Experiment: MVCC Redesign — Phase 3a move() off the stop-the-world path (2026-07-22)

Plan: `plans/mvcc-concurrency-redesign-2026-07-21.md` Phase 3a.
Branch: `mvcc-concurrency-redesign`.

## What changed

`move()` no longer mutates the live store under `store.mu.Lock` EXCLUSIVE and no
longer calls `markLiveStoreMutated` (which forced the coarse stop-the-world commit
with retry disabled). Instead `builtinMove` stages the move through the transaction
(`StoreTxn.MoveObject`): it records relationship reads on the moved object, its old
location, and the new location, and stages `what.location` plus the whole new
`contents` list of both rooms as `relationshipWrites`. These commit through the
decentralized path (`buildImageWithRelationship` now applies `contents`), so:

- Two moves between DISJOINT rooms touch disjoint slots and commit in parallel.
- Two moves into the SAME room conflict on that room's `relationshipVersion`, so
  the loser retries (no lost update).
- The 70% of commands that are reads (look/say) no longer block behind a global
  move lock.

The recursive-move check (`E_RECMOVE`) also went through the txn
(`StoreTxn.HasContentDescendant`) so it records read deps — preventing two
concurrent moves from each creating a containment cycle.

`enterfunc`/`exitfunc` remain an unimplemented TODO — the scout confirmed they have
ZERO conformance coverage and Toast-Barn parity does not require them.

### Mixed-builtin consistency guard

`move()` decentralizes ONLY when the task has not already mutated the live store
(`!ctx.LiveStoreMutated`). A task that ran a coarse builtin first (create/recycle/
renumber/chparent) has already written the live store directly, and those builtins
read/mutate the live store — so a decentralized (staged-only) move mixed with them
would let a later coarse op see stale live state (a `create;move;recycle` task
errored `E_INVIND` before this guard). Pure-move tasks (the hot path and the whole
benchmark workload) stay decentralized and keep the win; mixed tasks fall back to
the coarse move so live stays consistent. Regressed conformance tests
(`object_hierarchy::locations_*`, `objects::renumber_multiple_inheritance`) caught
this; a `scheduler` regression test now guards it.

## Verification

- `db/store/move_mvcc_test.go`: decentralized move maintains contents; two disjoint
  moves both commit (no false conflict); two same-room moves conflict (no lost
  update); every move asserts it did NOT set `liveMutated` (not coarse). Green.
- `go test ./db/store ./builtins ./scheduler` green; `-race` green.
- Managed conformance: 11,426 passed / 128 skipped / 4 failed — the SAME 4 pre-existing
  `dump_persistence` tests as the Phase 2 baseline, no new failures (an earlier draft of
  this phase regressed `locations_*`/`renumber`; the mixed-builtin guard below fixed them,
  re-verified by an isolated differential against the Phase 2 baseline barn).

## Results (median of 5, 2s window, `-count=1`, GOGC=400)

### realistic mix (look40/say30/move20/stamp7/build3)

| players | rooms | goodput/s | abort% |
|--------:|------:|----------:|-------:|
| 16 | 16 | 102,055 | 44.5 |
| 32 | 16 |  90,976 | 52.3 |
| 16 | 64 | 107,868 | 28.7 |
| 32 | 64 |  98,324 | 36.5 |
| 16 | 256 |  95,278 | 14.9 |
| 32 | 256 |  90,315 | 22.8 |

### Comparison to Phase 2 (realistic, GOGC=400)

| cell | Phase 2 g/s (abort) | Phase 3a g/s (abort) | speedup |
|------|--------------------:|---------------------:|--------:|
| 16p/16r | 72,728 (0%) | 102,055 (44.5%) | 1.40x |
| 32p/16r | 70,552 (0%) |  90,976 (52.3%) | 1.29x |
| churn 16p/16r | 87,126 (21.9%) | 151,256 (41.3%) | 1.74x |
| churn 32p/16r | 85,108 (26.6%) | 147,901 (48.0%) | 1.74x |

## Interpretation — a real goodput win that EXPOSES the room.contents hotspot

Goodput rose 1.29–1.74x because reads and disjoint moves no longer serialize behind
the coarse move lock. The cost: abort rate jumped from ~0% to 15–52%, and it is
**pure `room.contents` contention** — it falls monotonically as rooms spread the
load (16r 44.5% → 64r 28.7% → 256r 14.9% at 16 players), exactly the Zipfian-hotspot
signature, not a bug. Under the old coarse path these conflicts were "hidden" by
global serialization (0% abort but everything blocked); Phase 3a trades that global
block for localized retries, and nets higher committed goodput.

This is the symptom the plan earmarks for **Phase 5's hot-cell plan**: "retry storms
on `room.contents` at high player counts → commutative merge ops for setadd/setremove-
shaped writes (they commute)." `move()`'s contents edits ARE setadd/setremove on the
room's contents list — a commutative-merge contents write would let two moves into the
same room both commit without conflict, collapsing the abort rate. That is the highest-
value follow-up now that Phase 3a has made the hotspot measurable.

## Cumulative (realistic 16p/16r, vs Phase 0 master baseline 41,920/s)

Phase 0 → Phase 3a: **41,920 → 102,055 = 2.43x goodput** at default (GOGC=400) config,
with the residual ceiling now being true `room.contents` contention rather than a
global lock or the GC wall.
