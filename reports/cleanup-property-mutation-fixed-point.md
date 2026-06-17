# Property Mutation Store Cleanup Fixed Point

Date: 2026-06-17

## Target

`db.Store` owns property mutation. VM and builtins callers no longer write property map slots, delete property map slots, or mutate property value/info fields directly for objects returned from `Store.Get`.

## Deleted Call-Site Logic

- Removed property value/override writes from `vm/op_property.go`.
- Removed property definition, deletion, clear override, inherited propagation, and descendant scans from `builtins/properties.go`.
- Moved defined-property lookup in `builtins/limits.go` behind the store.

## Store-Owned Operations Added

- `DefinedPropertyNames`
- `LocalProperty`
- `DefinedProperty`
- `HasLocalProperty`
- `IsPropertyDefinedOnObject`
- `PropertyClearState`
- `SetPropertyInfo`
- `SetPropertyValue`
- `DefineProperty`
- `DeleteDefinedProperty`
- `ClearPropertyOverride`
- `HasDefinedPropertyInDescendants`
- `ResetInheritedProperties`

## Search Gate

Command:

```text
rg -n "obj\\.Properties\\[|delete\\(obj\\.Properties|prop\\.Value =|prop\\.Clear =|prop\\.Owner =|prop\\.Perms =" builtins vm server db
```

Result classification:

- Remaining `db/store.go` hits are the new owner methods.
- Remaining `db/reader_v4.go`, `db/reader_object.go`, `db/reader_helpers.go`, and `db/writer_object.go` hits are snapshot/parser I/O boundary code, not live runtime store mutation. They stay for Phase 4 unless snapshot ownership changes earlier.
- `server/matcher.go:131` is read-only alias lookup.
- `db/reader_test.go:310` is test assertion/setup.
- `builtins/objects_hierarchy.go:784` is parent-change inherited-property reset and is deferred to Phase 3 relationship mutation so the whole parent/child/property reset operation moves together.

## Gates

```text
go test ./db ./builtins ./vm
go test -timeout 120s ./builtins -run "Test.*Property"
go test -timeout 120s ./vm -run "Test.*Property"
git diff --check
```

All gates passed.

## Commit

`01b51b6 Close property mutations through store`

## Next Slice

Phase 2: close verb mutation through `db.Store`.
