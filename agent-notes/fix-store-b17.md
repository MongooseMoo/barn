# fix/store-b17: add_property gives anonymous descendants no slot of their own

## Goal
B17: `add_property(parent, ...)` must give the parent's ANONYMOUS descendants
their own clear slot, as it does permanent ones. Toast (WSL /root/src/toaststunt
aecc51e): insert_prop_recursively (src/db_properties.cc:151-185) appends the anon
children of the root and of every permanent descendant (:162-166) and calls
insert_prop2 on TYPE_ANON entries (:176-179); insert_prop2 gives a `c` slot the
receiving object's owner (:133-134). Barn's add walkers followed `children` only,
so an anon child had no slot and property_info reported the definer's slot
(owner of the definition, not the anon's owner). Mirror of B10 (delete side).
No push/PR; PR bodies drafted in the session scratchpad (pr-b17-barn.md,
pr-b17-mct.md).

## State
- moo-conformance-tests test/store-b17 (worktree C:/Users/Q/code/mct-store-b17)
  on origin/main 80eb0b6: new
  src/moo_conformance/_tests/builtins/property_definition_anonymous_slot_owner.yaml
  (8 rows: c / plain property x anon child of root / of a permanent child x one
  task / separate tasks; checks property_info owner and perms, is_clear_property,
  value). WSL Toast: 9 passed (admission + 8). Barn origin/master 4449b485:
  4 failed (every `c` row, `[0, 'rc', 1, 5]` for `[1, 'rc', 1, 5]`), 5 passed.
- barn fix/store-b17 (this worktree) on origin/master 4449b485:
  - propagatePropertyToDescendantsLocked (db/store/store_properties.go) also
    gives each visited object's anonymousChildren a fresh inherited slot. It
    serves the direct txn (definePropertyLocked) and the exclusive commit route's
    live walk (publishStagedLocked walkLive, store_cow.go).
  - propagateDefinedProperty (db/store/store_txn_property_inheritance.go) does
    the same in the txn copy and stages a propertyWrite for the anon; the anon
    in the write set routes the commit exclusive (writesTouchAnon,
    store_txn_commit.go).
  - Test TestDefinePropertyGivesAnonymousDescendantsOwnSlot (store_txn_test.go),
    snapshot (in txn and after commit) and direct, anon child of the root and of
    a permanent child: red on master (snapshot in txn: no slot), green after.
  - go test ./db/store ./builtins -count=1: ok. Conformance on the fixed build:
    new file + B10's file 16 passed; 12 builtins/*prop*.yaml files with
    K=property 164 passed, 14 deselected; 8 anonymous/create/recycle/gc/objects
    files 263 passed, 58 skipped.

## Decisions and why
- Plain (non-c) rows pass on master: reads fall through to the parent's slot,
  which has the same owner and perms. Kept as coverage.
- Additive loops mirroring B10's delete loops rather than a shared helper, so
  the diff only inserts lines (wave 3c rebases over it).

## Surprises
- B10's store_cow.go comment and `writeFootprintHasAnon` are gone after 3b
  (#508); routing is now writesTouchAnon in store_txn_commit.go.

## Not verified
- That each half alone stays red (B10 checked this; here only the combined fix
  was run). By construction the direct case goes through the store walker only
  and the in-txn check through the txn walker only.

## Next step
Team lead reviews; push and PRs only on their/Q's go. Merge order: barn PR,
then the conformance PR.
