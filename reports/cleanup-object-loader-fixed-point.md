# Object Loader Store Cleanup Fixed Point

Date: 2026-06-17

## Target

`db.Store` owns loaded-object insertion and high-water/max-object bookkeeping. `Database.NewStoreFromDatabase()` no longer writes `store.objects`, `store.highWaterID`, or `store.maxObjID` directly.

## Deleted Logic

- Removed loader-side object-map insertion from `db/reader.go`.
- Removed loader-side high-water and max-object updates from `db/reader.go`.
- Reused a store-owned insertion helper from both runtime `Add` and database loading.

## Search Gate

Command:

```text
rg -n "store\\.objects\\[|highWaterID|maxObjID" db builtins server vm
```

Result classification:

- Remaining hits are `db.Store` fields and methods.
- No non-store caller writes `store.objects`, `store.highWaterID`, or `store.maxObjID`.

## Gates

```text
go test ./db ./builtins ./vm
git diff --check
```

All gates passed.

## Commit

`c844277 Consolidate loaded object insertion`

## Next Slice

Continue Phase 3 with runtime create/recycle/recreate/move/hierarchy relationship mutation.
