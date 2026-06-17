# Object Assignment Store Cleanup Fixed Point

Date: 2026-06-17

## Target

`db.Store` owns direct object field assignment for built-in object properties and player flag mutation.

## Deleted Call-Site Logic

- Removed direct VM writes to object `Name`, `Owner`, raw `Location`, and object permission flags.
- Removed direct `FlagUser` mutation from `set_player_flag`.

## Store-Owned Operations Added

- `SetObjectName`
- `SetObjectOwner`
- `SetObjectLocationRaw`
- `SetObjectFlag`

## Phase 3 Search Gate

Command:

```text
rg -n "obj\\.(Parents|Children|Location|Contents|AnonymousChildren|Anonymous|Recycled|Flags)\\s*=|append\\(.*\\.(Parents|Children|Contents|AnonymousChildren)" builtins server vm db
```

Result classification:

- Remaining `db/store.go` hits are store owner methods.
- Remaining `db/reader_object.go`, `db/reader_v4.go`, and `db/startup_repair.go` hits are loader/repair boundaries.
- Remaining `server/scheduler_login_test.go` hit is test setup.
- Remaining `server/matcher.go`, `server/verbs.go`, `builtins/objects_hierarchy.go`, `builtins/objects_movement.go`, and `builtins/protected.go` hits are read/traversal code, not mutation.

## Gates

```text
go test ./db ./builtins ./vm
go test -timeout 120s ./builtins -run "Test.*(Create|Recycle|Recreate|Move|Parent|Child|Location|Object|Player)"
go test -timeout 120s ./vm -run "Test.*Property"
git diff --check
```

Results:

- `db`, `builtins`, and `vm` passed.
- Targeted builtins lifecycle/player pattern passed with no matching tests.
- Targeted VM property pattern passed with no matching tests.
- Diff hygiene passed.

## Commit

Pending.

## Next Slice

Phase 4: separate snapshot persistence from live store.
