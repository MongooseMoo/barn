# Object Hierarchy Store Cleanup Fixed Point

Date: 2026-06-17

## Target

`db.Store` owns parent/child relationship mutation for `chparent` and `chparents`, including chparent tracking and inherited-property reseeding.

## Deleted Call-Site Logic

- Removed old-parent `Children` removal and `ChparentChildren` cleanup from `builtinChparent` and `builtinChparents`.
- Removed direct `Parents` replacement and new-parent `Children` appends from `builtinChparent` and `builtinChparents`.
- Removed direct inherited-property reset/reseed from `builtinChparent` and `builtinChparents`.
- Deleted dead builtins helpers: `copyInheritedProperties`, `insertObjIDAtMOOPosition`, and `resetInheritedProperties`.

## Store-Owned Operations Added

- `ChangeParents`
- `reseedInheritedPropertiesLocked`

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

Pending.

## Next Slice

Continue Phase 3 with recycle and object flag/location assignment mutation.
