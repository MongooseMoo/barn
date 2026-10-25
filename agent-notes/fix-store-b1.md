# fix/store-b1: delete_property leaves a child's overridden slot

## Goal
B1 from agent-notes/db-store-review.md (on master, untracked): after
`delete_property(parent, name)` commits, a child that had overridden the value
still reads it. Toast removes the slot from every descendant
(toaststunt aecc51e9 src/db_properties.cc:317-348 remove_prop_recursively,
called from db_delete_propdef :390). Conformance test first, then the smallest
Barn fix. No push, no PR (team lead's instruction); PR bodies drafted in the
session scratchpad (pr-b1-barn.md, pr-b1-mct.md).

## State
Done, not pushed.
- moo-conformance-tests branch test/store-b1 (worktree C:/Users/Q/code/mct-store-b1),
  commit 67db72c on origin/main be876f5: new
  src/moo_conformance/_tests/builtins/delete_property_descendants.yaml.
  Each `run` step is one eval task (runner.py:788).
- barn branch fix/store-b1 (this worktree), on origin/master 3cb7e64c:
  removeInheritedProperty (db/store/store_txn_property_inheritance.go) stages a
  `propertyDeletes` entry per removed descendant slot; regression test
  TestTransactionDeleteDefinedPropertyRemovesCommittedChildOverride in
  db/store/store_txn_test.go.
- Evidence, managed harness from WSL Debian, narrowed with CONFORMANCE_PATHS to the
  new file and K=delete_property_descendants (admission kept):
  - WSL Toast (`make conformance-toast`): 2 passed.
  - Barn origin/master 3cb7e64c (`make conformance-k`): 1 failed, 1 passed;
    step `{child}.dp_override` expected E_PROPNF, got 99.
  - Barn fix/store-b1: 2 passed.
  - Go test: red before fix (LocalProperty ok=true), green after;
    `go test ./db/store -count=1` ok.

## Decisions and why
- Fix point: the staging side, not the applier. commitDecentralized
  (store_cow.go:415-443) applies a definition delete to the definer only and its
  comments already say descendant removals arrive as staged propertyDeletes; the
  staging side never produced them. Both appliers (buildImageWithPropertyDelete
  store_cow.go:142, coarse store_txn_commit.go:664) tolerate a slot that is absent,
  and the coarse deleteDefinedPropertyLocked already removes descendants itself.
- Narrowed runs with CONFORMANCE_PATHS: a whole-suite collection over /mnt/c with
  several other agents' runs competing had printed nothing for ~14 minutes.

## Surprises
- WSL git cannot read a worktree created from Windows (its .git file holds a
  Windows path), so an in-WSL `go build` needs GOFLAGS=-buildvcs=false.
- Toast's remove_prop_recursively also removes from TYPE_ANON descendants
  (db_properties.cc:340-341); Barn's comment store_cow.go:328-331 says anon
  descendants are never touched and calls that Toast-matching. Out of scope for
  B1; not tested.

## Next step
Team lead reviews; push and PRs only on their/Q's go.
