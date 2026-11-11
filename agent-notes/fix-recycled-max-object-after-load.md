# fix/recycled-max-object-after-load

## Goal

After dump and restart, recycled trailing slots must stay counted: Toast (managed WSL oracle, run by another agent) gives `{valid(#10), valid(#11), max_object(), connect-created player}` = `[0, 0, 12, 12]` after creating #10/#11, recycling both, dumping and restarting; Barn gave `[1, 0, 10, 10]`.

## State

Fixed in `db/format/reader.go` (`NewStoreFromDatabase` calls `Store.AddRecycledSlot` for each `Database.RecycledObjs` id) and `db/store/store_core.go` (`AddRecycledSlot` CAS-raises `ids.highWater` and `ids.max`). Regression test `db/format/recycled_slots_test.go` failed before the fix (`max_object after reload = #2, want #4`) and passes after.

## Decisions and why

- The reader already collected recycled slot ids into `RecycledObjs` and nothing read them; using that list is the smallest change.
- Recycled slots are not materialized as objects. `valid(#10) == 1` in Barn came from create() reissuing #10 via `allocateID` (highWater + 1), not from a loaded object.
- The ids are not added to `ids.recycled`: `LowestFreeID` already treats a nil slot at or below `ids.max` as free, so renumber sees them once `ids.max` covers them.

## Surprises

None.

## Next step

CI on the PR; the matching conformance test lives on a local moo-conformance-tests branch kept for Q.
