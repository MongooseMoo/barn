# Object Create/Recreate Store Cleanup Fixed Point

Date: 2026-06-17

## Target

`db.Store` owns object creation and recreate relationship setup: allocation, parent assignment, parent child-list attachment, anonymous-child tracking, and inherited property seeding.

## Deleted Call-Site Logic

- Removed direct object construction and `Parents`/`Anonymous`/`Flags` mutation from `builtinCreate`.
- Removed direct parent `Children` and `AnonymousChildren` appends from `builtinCreate`.
- Removed direct inherited-property seeding and parent `Children` appends from `builtinRecreate`.

## Store-Owned Operations Added

- `CreateObject`
- `attachChildToParentsLocked`
- `copyInheritedPropertiesLocked`

`Recreate` now seeds inherited properties and attaches the recreated object to its parent inside the store.

## Search Gate

Command:

```text
rg -n "obj\\.(Parents|Children|Location|Contents|AnonymousChildren|Anonymous|Recycled|Flags)\\s*=|append\\(.*\\.(Parents|Children|Contents|AnonymousChildren)" builtins server vm db
```

Result classification:

- `builtinCreate` and `builtinRecreate` no longer contain relationship/object setup writes.
- Remaining runtime hits are recycle, move, chparent/chparents, object/player flag mutation, and read traversal helpers for later Phase 3 sub-slices.
- Reader/startup-repair hits are load/repair boundaries.
- Store hits are owner methods.

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

Continue Phase 3 with recycle, movement, and chparent/chparents relationship mutation.
