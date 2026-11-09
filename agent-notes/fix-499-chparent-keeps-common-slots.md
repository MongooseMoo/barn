# #499: chparent keeps slots of common ancestors, and reaches descendants

## Goal
Q (2026-10-26, "Make it so!"): fix barn#499. `chparent`/`chparents` must:
1. keep an object's slot (value, owner, perms, clear flag) for every property whose
   defining ancestor is in both its old and new ancestor lists;
2. do the same for every descendant of the moved object, and give descendants new
   slots for new ancestors' properties (a `c` property's slot owned by the
   descendant's owner; moved here from #485).

## Toast source (read)
`dbpriv_fix_properties_after_chparent` (`toaststunt/src/db_properties.cc:902-1047`):
- `:950-962` for each new ancestor also in old_ancestors, copy the object's old
  pval (var, owner, perms) for that ancestor's propdefs.
- `:963-976` otherwise a clear slot, perms from the parent's slot, owner = object
  owner if `PF_CHOWN` else the parent slot's owner.
- `:997-1046` recurse to each child (and anon kids); a child's old/new ancestor
  lists are built from its parents, substituting obj's old/new ancestors where the
  parent is obj.
The test is ancestor identity, not property name: a new parent can define its own
property with the same name as an old ancestor's.

## Barn before (284c86b3)
- Live: `Store.ChangeParents` (`db/store/store_relationships.go:300-341`) reseeds
  only obj via `reseedInheritedPropertiesLocked` (`store_properties.go:60-71`), which
  keeps only `defined` slots.
- Txn: the chparent/chparents builtins (`builtins/objects_hierarchy.go:222-233`,
  `:351-362`) call `StoreTxn.ReseedInheritedProperties` (`store_txn_property_inheritance.go:135-196`)
  after the live change; it rebuilds the txn copy and queues a write for every
  non-defined slot, so it must make the same keep decision. `beforeCoarse` flushes
  only staged topology (`builtins/store_reads.go:55-95`), so a property write earlier
  in the same task lives only in the txn.
- Recreate also calls the txn reseed (`objects_hierarchy.go:692`); a recreated object
  has no old ancestors and no descendants.

## Design
- Old ancestors of any object in the moved subtree = walk of the current graph with
  the moved object's parents replaced by its old parents (only its parents change).
- An object keeps its existing slot for a name iff an ancestor in both its old and
  new ancestor sets defines that name. Every slot is still versioned at ts.
- Reseed the moved object, then its descendants (children and anonymous children),
  parents before children.
- Txn reseed takes the old parents; recreate passes nil.

## Tests
mct branch `tests/chparent-keeps-common-slots` (worktree `C:/Users/Q/code/mct-chparent-keep`),
5 tests appended to `builtins/properties.yaml`: steps-based (separate tasks, live path)
and statement-based (same task, txn path).

## Coordination
barn-1d (db-store-review lead) 2026-10-25 ~23:10: don't hold; bug fixes land first,
their refactors rebase over this. #511 (B17, anon children get own slot in
propagate*) is adjacent, rebase over it. Wave 3c (refactor/no-twins) will rebase over
#499; keep the diff in place, don't restructure the direct-path twins. Same scenario
on direct and snapshot txn paths. No `features: [anonymous]` gate on conformance rows.

## State
- Toast: `7 passed, 13221 deselected` (5 new tests + admission + one more match, to
  identify from the verbose Barn run).
- Code written: `reseedInheritedPropertiesLocked(obj, oldParents, ts)` walks obj then
  descendants (republish, rebuild, stamp each); `keptInheritedNames` and
  `keepCommonAncestorSlots` shared with `StoreTxn.ReseedInheritedProperties(objID,
  oldParents)`; builtins pass oldParents (recreate nil). Two existing tests updated
  for the signature (`store_direct_txn_test.go:67`, `store_txn_test.go:1106,1119`).
- `TestChangeParentsKeepsCommonAncestorSlots` (direct/committed/txn) passes with the
  related store tests; it cannot compile on master (new signature), so the red
  evidence is the Barn-master conformance run.

- Barn master 284c86b3: 3 failed, 4 passed (other-definer and descendant-value tests
  pass on master: coverage). Branch (pre-rebase): 7 passed. 7th match is existing
  `audit/divergence_parenting_task_lifecycle.yaml::chparent_keeps_explicit_child_value`.
- Rebased over #511/#512 (clean), new Go test passes. PRs: mct #155, barn #518.

## Next step
Q's merge decision for #155 and #518; then remove worktrees `barn-499-chparent`,
`mct-chparent-keep` and their branches.
