# fix/store-b10: delete_property leaves an anonymous child's overridden slot

## Goal
B10 from agent-notes/db-store-review.md (team lead's untracked note on master):
`delete_property(parent, name)` must remove the inherited slot from the parent's
ANONYMOUS descendants too. Toast (WSL /root/src/toaststunt aecc51e9):
remove_prop_recursively (src/db_properties.cc:317-345) appends anon children with
dbpriv_append_anon_list (:325-328) and calls remove_prop2 on TYPE_ANON
descendants (:340-341). Same for add: insert_prop_recursively (:151-185,
anon append :162-165, :178-179), called from db_add_propdef (:222-225).
Conformance test first, then the smallest Barn fix. No push/PR; PR bodies are
drafted in the session scratchpad (pr-b10-barn.md, pr-b10-mct.md).

## State
- moo-conformance-tests test/store-b10 (worktree C:/Users/Q/code/mct-store-b10)
  commit e7e2e60 on origin/main 2772ad4: new
  src/moo_conformance/_tests/builtins/property_definition_anonymous_descendants.yaml
  (7 rows, LF). WSL Toast: 8 passed (admission + 7). Barn 3cb7e64c: 3 failed,
  5 passed; failures are the overridden-slot rows ([7, 7] for [7, E_PROPNF];
  [7, 0] for [9, 1]; separate-task step got 7).
- barn fix/store-b10 (worktree C:/Users/Q/code/barn-store-b10), written on
  origin/master 3cb7e64c, rebased onto 3f97e9ba (#489 merged meanwhile); on the
  rebased tree go test ./db/store ok and the new conformance file 8 passed again:
  - removeInheritedProperty (store_txn_property_inheritance.go) also visits each
    visited object's anonymousChildren: removes the non-defined slot from the txn
    copy and stages a propertyDeletes entry (puts the anon in the footprint, so
    Commit routes coarse via writeFootprintHasAnon).
  - removeInheritedPropertyLocked (store_properties.go), used by the direct txn
    and by the coarse commit's deleteDefinedPropertyLocked, also strips anon
    children's non-defined slot.
  - store_cow.go commitDecentralized comment corrected (it claimed Toast leaves
    anon descendants alone).
  - Test TestDeleteDefinedPropertyRemovesAnonymousChildOverride
    (store_txn_test.go), snapshot+commit and direct: red on master (txn view 99),
    green after. Each half alone stays red: without the store walker the direct
    case keeps 99; without the txn walker the txn's own view keeps 99.
  - go test ./db/store -count=1: ok. Conformance on the fixed build: new file 8
    passed (with admission); new file + add/delete_property_call_shapes: 62 passed.

## Decisions and why
- The non-overridden rows pass on Barn master because the anon's stale slot is
  `clear`, so walkProperty (store_txn_properties.go) continues to the parent and
  finds nothing. Only an anon that set its own value shows the defect.
- add_property on Barn does not add a slot to anon children, but both add rows
  pass: reads fall through to the parent and a write creates the local slot.
  Left unchanged (no failing Toast-checked behavior).
- Paths the scenario reaches: same task (the anon create marks the txn
  liveMutated, so commit goes coarse: deleteDefinedPropertyLocked ->
  removeInheritedPropertyLocked, store_properties.go:690, children only) and
  separate tasks (decentralized commit; txn walker removeInheritedProperty,
  store_txn_property_inheritance.go:360, children only, footprint has no anon).

## Surprises
- bin/moo's in-memory Minimal.db has no `$nothing`; use #-1 there.

- An anon slot GC'd (recycled) between staging and commit makes the coarse
  propertyDeletes loop return E_INVIND (store_txn_commit.go propertyDeletes
  apply); that is the existing contract for any staged anon write. Not tested.

## Overlap
- PR #491 (fix/store-b1) edits removeInheritedProperty's children loop (one
  line); this branch appends a loop after it in the same function.
  `git merge-tree` against #491's head exits 0 (textual only, not test-run
  combined). Rebase after #491 merges.
- PR #489 (merged as 3f97e9ba) touched copyInheritedProperties,
  propagateDefinedProperty and buildImageWithPropertyDefine, not the delete
  walkers or this comment; the branch is rebased onto it.

## Next step
Team lead reviews; push and PRs only on their/Q's go. Merge order: barn PR,
then the conformance PR.
