# Object Recycle Store Cleanup Fixed Point

Date: 2026-06-17

## Target

`db.Store` owns recycle-time relationship cleanup and lifecycle flag mutation.

## Deleted Call-Site Logic

- Removed recycle-time child reparenting from `builtinRecycle`.
- Removed recycle-time contents/location cleanup from `builtinRecycle`.
- Removed recycle-time property/verb clearing from `builtinRecycle`.
- Removed recycle-time parent `Children` cleanup from `builtinRecycle`.
- Deleted the now-dead builtins `removeObjID` helper.

## Store-Owned Behavior

`Store.Recycle` now:

- Reparents children to the recycled object's parents.
- Moves contained objects to `$nothing`.
- Removes the object from its old location contents.
- Clears runtime property and verb maps.
- Removes the object from parent child lists.
- Marks the object recycled/invalid and records the recycled ID.

## Gates

```text
go test ./db ./builtins ./vm
go test -timeout 120s ./builtins -run "Test.*(Create|Recycle|Recreate|Move|Parent|Child|Location|Object)"
git diff --check
```

Results:

- `db`, `builtins`, and `vm` passed.
- Targeted builtins lifecycle pattern passed with no matching tests.
- Diff hygiene passed.

## Commit

`90c9274 Move recycle cleanup into store`

## Next Slice

Continue Phase 3 with object flag/location assignment mutation.
