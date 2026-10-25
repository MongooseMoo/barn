# fix/store-b1: delete_property leaves a child's overridden slot

## Goal
B1 from agent-notes/db-store-review.md (on master, untracked): after
`delete_property(parent, name)` commits, a child that had overridden the value
still reads it. Toast removes the slot from every descendant
(toaststunt aecc51e9 src/db_properties.cc:317-348 remove_prop_recursively,
called from db_delete_propdef :390). Conformance test first
(moo-conformance-tests branch test/store-b1, worktree C:/Users/Q/code/mct-store-b1),
then the smallest Barn fix here. No push, no PR (team lead's instruction).

## State
- Worktrees created: barn-store-b1 at origin/master 90681354; mct-store-b1 at
  origin/main be876f5.
- Conformance test written:
  mct-store-b1/src/moo_conformance/_tests/builtins/delete_property_descendants.yaml
  (one `run` step = one eval task, runner.py:788). Toast run in progress via
  `make conformance-toast` from WSL.
- Barn fix applied: store_txn_property_inheritance.go removeInheritedProperty
  now stages `propertyDeletes` for each removed descendant slot.
- Regression test TestTransactionDeleteDefinedPropertyRemovesCommittedChildOverride
  (db/store/store_txn_test.go): red before fix (LocalProperty ok=true), green after.
  `go test ./db/store -count=1` ok.
- origin/master moved to 3cb7e64c (#486, store_core.go alias matching, unrelated)
  after this branch was cut; rebase onto it before handing off. Red Barn run uses
  detached worktree C:/Users/Q/code/barn-store-b1-base at 3cb7e64c.

## Decisions and why
- Fix point: StoreTxn.removeInheritedProperty
  (db/store/store_txn_property_inheritance.go:375-390) removes the child slot
  only from the txn-private clone. commitDecentralized (store_cow.go:415-443)
  applies the definition delete to the definer only and expects staged
  per-descendant propertyDeletes. Staging the delete there is the fix; the
  appliers (buildImageWithPropertyDelete store_cow.go:142, coarse
  store_txn_commit.go:664) already tolerate a slot that is not present.

## Surprises
- Toast's remove_prop_recursively also removes from TYPE_ANON descendants
  (db_properties.cc:340-341); Barn's comment store_cow.go:328-331 says anon
  descendants are never touched and calls that Toast-matching. Out of scope
  for B1; not verified by test.

## Next step
Write the YAML test, run on WSL Toast (green) and Barn master (red).
