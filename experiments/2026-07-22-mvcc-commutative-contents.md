# Experiment: MVCC Redesign — Commutative Contents (Phase 5 hot-cell) 2026-07-22

Plan: `plans/mvcc-concurrency-redesign-2026-07-21.md` Phase 5 hot-cell plan,
triggered by the `room.contents` retry storm Phase 3a exposed and measured.
Branch: `mvcc-concurrency-redesign`.

## What changed

`move()` no longer stages a whole new `contents` list for each room (which forced
a read dep on the room and made two moves into the same room conflict). It now
stages COMMUTATIVE DELTAS — "remove `what` from the old room" / "add `what` to the
new room at position P" — applied IN ORDER to each room's CURRENT live contents at
commit (`applyContentsDeltas`), and it records NO read dep on either room.

Consequences (verified):
- Two moves into the SAME room both commit — setadd/setremove commute — and merely
  serialize on that room's slot mutex at publish time instead of one aborting and
  re-running the whole verb.
- Two moves of the SAME object still conflict (each reads `what.location`, a read dep
  on `what`), so there is no double-add / lost location update.
- A task that READ a room's contents and then writes still conflicts with a move into
  that room: the move still bumps the room's `relationshipVersion`; only blind
  commutative appenders skip the read dep.

## Verification

- `db/store/move_mvcc_test.go`: `TestTxnMoveSameRoomCommutes` (NEW) — two moves into
  one room both commit and both objects land there (no lost update); disjoint-room and
  maintains-contents tests still green. Scheduler mixed-builtin regression test green.
- `go test ./db/store ./builtins ./scheduler -race` green.
- Managed conformance: differential vs the Phase 2/3a baseline (same 4 pre-existing
  `dump_persistence` failures, no new).

## Results (median of 5, 2s window, `-count=1`, GOGC=400) — realistic mix

| players | rooms | goodput/s | abort% |
|--------:|------:|----------:|-------:|
| 1  | 16 |  56,203 | 0.00 |
| 16 | 16 | 126,977 | 0.00 |
| 32 | 16 | 104,909 | 0.00 |
| 16 | 64 | 100,976 | 0.00 |
| 32 | 64 |  94,117 | 0.00 |

### Comparison (realistic, GOGC=400)

| cell | Phase 3a g/s (abort) | Commutative g/s (abort) |
|------|---------------------:|------------------------:|
| 16p/16r | 102,055 (44.5%) | **126,977 (0.0%)** |
| 32p/16r |  90,976 (52.3%) | 104,909 (0.0%) |

## Interpretation

The realistic-mix abort rate collapsed from ~44–52% to **0%** and goodput rose
another ~1.24x, because the only remaining write-write interactions on the hot path
are now genuinely non-conflicting: reads skip commit, disjoint moves are independent,
and same-room moves commute (serializing cheaply on the slot mutex). Churn-stress
abort also dropped (≈41% → ≈22%), and its residual is the *ancestor-property* write
contention (the `#root.churn` writes) — that is Phase 4's target (shape-version split),
not move contention.

## Cumulative (realistic 16p/16r, vs Phase 0 master baseline 41,920/s)

**Phase 0 → here: 41,920 → 126,977 = 3.03x goodput at default (GOGC=400) config, with
0% abort on the realistic workload.** The realistic-mix ceiling is now raw throughput
(and the coarse `build`/create path + slot-mutex serialization on the hottest rooms),
not the GC wall, not a global move lock, and not a conflict/abort storm.
