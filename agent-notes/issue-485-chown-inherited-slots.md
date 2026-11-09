# #485: inherited property slots ignore the c flag

## Goal
Apply Toast's `c` (PF_CHOWN) owner rule to inherited property slots (#485).

## State (2026-10-25)
Q said "make it so". Branch `fix/485-chown-inherited-slots` (worktree `C:/Users/Q/code/barn-485-chown`, from origin/master 5a61c4c8) has the fix and `TestChownFlagSlotsOwnedByEachObjectOwner` (failed before: every `cx` owner #5; passes after). Conformance branch `tests/chown-inherited-slots` (worktree `C:/Users/Q/code/mct-chown-slots`): four `chown_flag_*` tests in `builtins/properties.yaml`. Toast: 5 passed (with admission), including the descendant check later moved out. Rerun after narrowing, Barn master red and branch green runs in progress.

## Toast (source; confirmed by the oracle run)
- create / recreate / chparent: `dbpriv_fix_properties_after_chparent` (`db_properties.cc:974`): a slot from a NEW ancestor gets `owner = pv.perms & PF_CHOWN ? me->owner : pv.owner`, `pv` = the parent's slot. `bf_create` sets the object's owner before changing parents (`objects.cc:393` then `:405`; recreate `:494` then `:496`). Recurses to descendants (`db_properties.cc:997-1047`).
- chparent keeps a slot (value, owner, perms) whose ancestor is in both the old and new ancestor lists (`db_properties.cc:955-962`).
- add_property: `insert_prop2` (`db_properties.cc:133`): `if (perms & PF_CHOWN) owner = o->owner`, for every descendant AND the definer itself (`insert_prop_recursively` inserts at root first, `:153`).

## Fix
- `inheritedSlot(from, owner)` (`store_properties.go`): owner is the object's owner when `from` has `c`, else `from.owner`. Both copy functions take the new object's owner (create, txn create, Recreate, both reseeds); both propagations pass `child.owner`.
- Both define paths: with `c`, the definer's slot owner is the definer object's owner.
- No read mark for the owner: a concurrent owner change plus this txn still equals a serial order (this txn first), since nothing here writes the owner.

## Decisions (Q, 2026-10-25)
- The chparent value-loss bug is a separate issue: #499 (line citations corrected by edit, with Q's OK).
- The descendant case (a moved object's descendants get their own `c` slots) moves to #499: `Store.ChangeParents` reseeds only the moved object (`store_relationships.go:336`), and the builtin reseeds only `objVal` (`builtins/objects_hierarchy.go:222-233`). Descendants have no new slot; reads walk up to the moved object's slot, so they report its owner. Reseeding them with today's reseed would wipe their written values, which #499's keep-common-ancestors reseed fixes.

## Surprises
- `add_property(#1,"w",...)` => E_INVARG: `w` is a builtin property name (write flag). Not a bug.
- From Git Bash, `wsl --cd /mnt/c/...` fails (MSYS rewrites the path); run wsl from PowerShell.
- I filed #499 in the same batch as a draft edit that failed to match, so it went out with wrong line numbers; corrected with Q's OK.

## Next step
Read the three conformance runs; commit both branches; open the two PRs; add the descendant case to #499 (needs Q's OK to edit the issue).
