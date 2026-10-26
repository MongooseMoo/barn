# fix/store-b20

## Goal
Three Barn-vs-Toast divergences ("B20"), Toast-first:
1. `set_property_info(obj, name, {owner, perms, new_name})` renames the property
   (Toast set_prop_info property.cc:149-188 -> db_rename_propdef
   db_properties.cc:251-280). Barn returned E_INVARG always.
2. Anonymous object to the verb builtins that use `is_obj()` gives E_TYPE
   (verbs.cc:192 add_verb, 249 delete_verb, 334 set_verb_info, 392 verb_args,
   433 set_verb_args, 520 set_verb_code).
3. `add_property` with a 3-element info list gives E_TYPE (property.cc:218-221).

## Toast source facts (read, toaststunt aecc51e)
- db_rename_propdef renames only a propdef in obj's OWN propdefs; an inherited
  name fails -> E_INVARG. Case-only change skips the clash check. Clash check:
  db_find_property(obj, new) (built-in names, obj, ancestors) or
  property_defined_at_or_below(new, obj) (obj and descendants). Descendant slots
  are positional, so their values/owners/perms stay. Renames waif propdefs too.
- After the rename set_prop_info sets owner/flags on obj's slot only.
- E_PERM check (property write + owner change) is before the rename.
- add_prop order: validate_prop_info, then new_name or !is_obj -> E_TYPE, then
  !is_valid -> E_INVARG, then perms E_PERM, then db_add_propdef clash E_INVARG.

## State
Done and committed; reported to team-lead. Branches:
- barn fix/store-b20 (worktree C:/Users/Q/code/barn-store-b20), base
  origin/master f137f23a: 4f76be42 store rename, e21a246c builtins, then this
  note.
- mct test/store-b20 (worktree C:/Users/Q/code/mct-store-b20), base origin/main
  3d51b08: a7019cd property_rename_and_numbered_object_checks.yaml.
- PR drafts: scratchpad pr-b20-barn.md, pr-b20-mct.md. Nothing pushed.
What changed:
- db/store: StoreTxn.RenameProperty + propertyRenames staged kind (definer only,
  keyed by new name). applyHeadWrites renames (renameSlots) between defDeletes
  and defines; walkLive routes move live descendants
  (renameInheritedPropertiesLocked); task txns stage descendants as
  delete-old + write-new (renameInheritedProperty). Renames are read from
  tx.propertyRenames (renamesOf), not stored in objectWrites. Preflight, clear,
  forget/move, HasStagedTopology, hot-read decline, read-mark skip;
  DeleteDefinedProperty of a renamed name deletes the published name; a define
  of a renamed-away name passes preflight (renamedAway).
- builtins: set_property_info renames; add_property in Toast's order;
  verb_args/set_verb_code TYPE_OBJ only; delete_verb/set_verb_info/
  set_verb_args descriptor first, then TYPE_OBJ only.

## Decisions and why
- Renames on the definer are a new staged kind; descendants use the existing
  delete/write kinds so the shared route needs no store walk (validation covers
  the children through relationship reads), as defines do.
- renameSlots removes all renamed slots of an object before putting any back,
  so a swap inside one txn keeps both. On the exclusive route the walk and the
  staged descendant writes both apply; the staged write carries the full slot,
  so the final state is the same either way.
- First version kept renames in objectWrites: +32 B/op on every commit and
  direct write (CommitScratch/objects1 4816 -> 4848). Moved them out; all B/op
  rows now at or below master, allocs equal.
- docs/elle-isolation.md:153-156 asks for a fault+workload only for a new
  validation/publication protection; the rename adds none, so none was added.

## Runs
- Toast oracle, new file: 45 passed (44 rows + capability_admission).
- Barn origin/master f137f23a, new file: 23 failed, 22 passed.
- Barn fix/store-b20, new file: 1 failed (rename_waif_class_property), 44 passed.
- K="property or verb_": Toast 3777 passed/25 skipped; Barn 3776 passed/25
  skipped/1 failed (the WAIF row).
- Gates: build, vet, gofmt, go test ./db/... ./builtins ./engine, -race
  ./db/store all ok. Elle Go generation seed 412: PASS (no checker verdict; jar
  not built).
- WSL build needs GOFLAGS=-buildvcs=false (WSL git cannot read the Windows
  worktree's .git pointer).

## Open question (sent to team-lead, no answer at report time)
WAIF values on rename: Toast re-keys (positional), Barn keys by name
(vm/op_property.go:225 reads the WAIF's own map first). Recommended leaving it
out of B20 with the Toast-green row kept.

## Surprises
- "a" is a Toast built-in property (db.h:374-385: name owner programmer wizard
  r w f a location last_move contents). My first rows defined "a" and Toast's
  add_property raised E_INVARG; rows now use other names, and "A" is a clash row.
- MSYS grep with a $'\r' pattern counted every line as CR; `tr -cd '\r' | wc -c`
  showed the YAML is LF-only.

## Next step
None in this branch: the lead decides on the WAIF question and on pushing.
