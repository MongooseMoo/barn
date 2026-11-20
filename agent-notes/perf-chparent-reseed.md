# chparent performance (perf/chparent-reseed)

## Goal
Q (2026-10-25): "can we not just measure chparent, but make it fuckin fast?" Then
(2026-10-26) "Make it so!" for both steps in one PR: remove the quadratic, then apply
only the lost/gained-ancestor delta per descendant.

## Cause
barn #518 (#499) made the reseed walk every descendant of the moved object. The txn
reseed (`StoreTxn.ReseedInheritedProperties`) then scanned all of tx.propertyReads /
propertyWrites / propertyDeletes once per descendant while queueing a write for each
of its slots: O(descendants^2 x slots). Mongoose #6 (1569 descendants): 5.8 s + 12.9 s
per chparents pair, Toast 59-81 ms. Profile: reseedObjectInheritedProperties 17.7 of
18.5 s, maps Iter.Next 15 s.

## Design (what changed and why it is safe)
- Live (`reseedInheritedPropertiesLocked`): the moved object is rebuilt as before, every
  slot at ts. A descendant only loses the slots of ancestors the moved object lost and
  gains clear slots (from the moved object's slot, `inheritedSlot`) of those it gained,
  except ancestors it also reaches through a parent outside the subtree
  (`otherAncestors`). Kept slots are untouched, so the #456 shared base survives; a
  descendant with nothing to change is not republished. Safe because `walkProperty`
  (store_txn_properties.go:171-211) records a read of every found slot, so a walk
  through the moved object records its rewritten slot, and `stampObjectProperties`
  bumps a changed descendant's shape.
- Txn: the commit applies only queued writes to a clone of live
  (store_cow.go:124-130, 197-223), and live already holds the reseed. So the txn reseed
  queues nothing new: each subtree object's view takes the live table (aliasing the
  immutable live image when the txn had not loaded it or differs only in properties;
  privatize otherwise; clone for anonymous), and the txn's staged writes/deletes are
  replayed through `applyTailWrites` where the reseed kept the slot
  (`dropStagedSlotsNotKept`). Staged defines are flushed before chparent
  (HasStagedTopology, store_txn.go:190). Subtree walked on live without caching.

## Tests
- `TestChangeParentsReseedMatchesToastRule` (direct, txn): 7 moves over a hierarchy
  with multiple inheritance, an other path to a lost ancestor and an anonymous child;
  every slot of the subtree checked against Toast's rule computed independently.
  Mutations checked red: ignoring other ancestors (y lost q/cq); skipping
  dropStagedSlotsNotKept (lost slots came back).
- `TestChangeParentsKeepsCommonAncestorSlots` (#499) passes.
- `BenchmarkChparentReseed`: builtin sequence, 100 props on the root, each side defines
  10 (every descendant loses 10, gains 10).

## Numbers
Benchmark (keep-all variant before the side props were added), 100 / 1000 descendants:
master 25.7 ms / 760 ms; one-pass maps 21.0 / 191; delta 0.23 / 2.43. Heavy variant
(final): 0.88 ms / 9.0 ms.
Mongoose `mongoose.db.new` (sha256 7eeb86b9...d7e), throwaway probe through the
chparents builtin in an eval session (snapshot txn), Barn moves only; Toast round trip
from bench_differ includes create/recycle; CPU load 36-57% from other sessions:
| class | descendants | Toast | Barn master | Barn now (empty new parent) | Barn now (parent defines 1 prop) |
| #6 | 1569 | 59-81 ms | 5.8 + 12.9 s | 1.6-2.7 + 1.6-2.3 ms | - |
| #2700 | 10083 | 146-252 ms | timed out | 13-16 + 11 ms | - |
| #5 | 22345 | 392-437 ms | timed out | 52-76 + 44-56 ms | 97-166 + 106-167 ms |
Remaining cost in the heavy case: republishForMutation (COW clone) of every changed
descendant.

## Not done
- Toast heavy-case timing (parent with a property) not measured; Toast rewrites every
  slot either way.
- No conformance change: behaviour is unchanged (#499 tests cover it); CI runs the suite.

## Next step
PR, CI, Q's merge decision.
