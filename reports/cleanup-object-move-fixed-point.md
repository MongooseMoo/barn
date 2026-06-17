# Object Movement Store Cleanup Fixed Point

Date: 2026-06-17

## Target

`db.Store` owns object movement mutation: removing an object from its old location contents, setting the new location, and inserting it into the new location contents with MOO position semantics.

## Deleted Call-Site Logic

- Removed direct `Contents` removal from `builtinMove`.
- Removed direct `Location` assignment from `builtinMove`.
- Removed direct destination `Contents` insertion from `builtinMove`.

## Store-Owned Operations Added

- `MoveObject`
- `removeObjID`
- `insertObjIDAtMOOPosition`

The builtin still owns argument validation, recursive move checks, and `accept` verb behavior.

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

`2e33025 Move object movement mutation into store`

## Next Slice

Continue Phase 3 with recycle and chparent/chparents relationship mutation.
