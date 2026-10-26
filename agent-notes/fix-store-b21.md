# fix/store-b21

## Goal
B21: a WAIF's value for a class property must follow `set_property_info`
renames of that property, and go away when the property is deleted, as Toast
does. Toast-first; rows in mct `builtins/waif_class_property_rename_delete.yaml`.

## Toast source facts (read, /root/src/toaststunt/src)
- WAIF values are positional against the class's `waif_propdefs` (waif.cc
  find_propval_offset 300-349). A name the class does not define is E_PROPNF
  on read and write (waif_get_prop 666-735, waif_put_prop 737-830); an invalid
  class is E_INVIND. A clear slot reads the class's value.
- Rename: db_rename_propdef (db_properties.cc:251-280) ->
  rename_waif_prop_recursively (233-249) -> waif_rename_propdef (waif.cc
  143-176): when old and new are both ":" names it renames the propdef entry
  in place, so up-to-date WAIFs keep the value under the new name. Otherwise it
  drops the class's waif_propdefs.
- add/delete property null the class's (and every descendant's) waif_propdefs
  (insert_prop2 / remove_prop2). A WAIF reconciles lazily on its next access
  (update_waif_propdefs waif.cc 412-575) by comparing propdef NAME POINTERS:
  same pointer keeps the value, anything else is deleted/inserted (clear).

## Oracle results (all 16 probe rows passed on Toast, 2026-10-26)
See the YAML. Two rows show Toast's lazy pointer-identity quirk, which its own
comment calls "an ugly bug" (waif.cc:153-160):
- p_delete_readd_same_literal: delete ":foo", re-add ":foo" with the same
  program literal, no WAIF access between -> the OLD value 5 comes back
  (the literal is one string, so the name pointer matches).
- p_add_then_rename_no_access: add ":x", then rename ":foo"->":bar" with no
  WAIF access between -> the value is LOST (class propdefs were null, so the
  in-place rename was skipped and the lazy reconcile sees a new pointer).
With a WAIF access in between, or a computed name, both give the clean
behaviour (rename keeps, delete clears, re-add is default).

## Decisions and why
- Approach (a): re-key the values of every live WAIF of the class and its
  descendant classes. (b) (key by propdef identity) changes WaifImage's key and
  db/format's index<->name mapping -> the brief says stop; and it would add a
  class lookup to every WAIF read, which is hot.
- Barn implements the clean semantics (rename moves, delete drops, add leaves
  values alone). Matching the two quirk rows needs Toast's lazy propdef cache
  and string-pointer identity; NOT built, left out of the committed YAML, and
  reported as an open decision.
- In-task: StoreTxn.RenameProperty / DeleteDefinedProperty stage the move on
  every registered WAIF of the definer's subtree (task view), through the
  ordinary staged WAIF images, so the task sees it and the WAIF reads are
  validated.
- Applier: publishStagedLocked also moves the values of registered WAIFs of
  the affected classes that the txn did not stage (a WAIF another task
  published after this task enumerated), on every route. One move function
  serves both.
- WAIF writes look up the class's ":name" (E_INVIND bad class, E_PROPNF
  undefined), as Toast does. Barn wrote any name before. The lookup is a read
  dependency, so a writer racing a rename conflicts and retries.
- Registry holds types.WeakWaif per identity (needs the value; weak so the
  registry still roots nothing).

## Surprises
- Barn reads a WAIF's own map before consulting the class, so a stale key
  stays readable after delete (p_delete_access_readd gave [5,5]).
- Barn master: 11 of 16 probe rows fail.

- Write-path refinement: the class lookup runs only when the WAIF's image does
  not already hold the key. A held key already proves the class defines it
  (load maps by class propdefs; writes check; renames/deletes re-key), and a
  racing rename republishes that WAIF, so the writer's WAIF read conflicts.
  This keeps repeated writes of one property free of the lookup (its
  ":"+name concat allocates). The fast path is StoreTxn.ReplaceWaifProperty
  (one store call, as master's SetWaifProperty), which detects "held" from the
  map length around the assignment, so it adds no map lookup. A separate
  WaifProperty pre-check cost +22% on write_100k; a lookup inside the store
  call still cost ~+9%.

## State
Code done: registry WeakWaif (store_core.go, store_lifecycle.go);
newWaifMove / moveWaifProperties / StoreTxn.moveWaifValues /
moveUnstagedWaifValuesLocked (store_txn_waif.go), called from RenameProperty,
DeleteDefinedProperty and publishStagedLocked; VM setWaifProp class check.
Tests: db/store/waif_property_move_test.go (all routes; late WAIF moved by
the applier, mutation-checked; racing writer conflicts);
vm/waif_property_bench_test.go (new benchmark); containment test fixture now
defines :child on #0.
Final YAML (13 rows): Toast 14 passed, Barn branch 14 passed, Barn master
11 failed / 3 passed. K="waif or property": Toast and Barn both 438 passed,
18 skipped. Gates: build, vet, gofmt, staticcheck, go test ./db/...
./builtins ./engine ./vm ./types, -race ./db/store ./engine all ok; Elle Go
generation seed 412 x40 PASS (no checker verdict; jar not built).
Benchmarks: allocs/op equal on every row; B/op equal at fixed N; write_100k
time ~+1.5-3% (24.6 vs 25.0-25.5 ms) at -benchtime=40x.

## Open decision (for Q)
Toast's two quirk rows (p_delete_readd_same_literal, p_add_then_rename_no_access
above) are not in the YAML and Barn does not match them.

## Seen, not fixed
- WAIF reads/writes do not check class property perms (Toast
  waif_property_allows, waif.cc:650-657).
- A WAIF of a recycled class: Toast E_INVIND on read and write; Barn still
  reads a held value and writes a held name.
- chparent of a class: Toast drops WAIF values for ":" props no longer
  inherited (lazy reconcile); Barn keeps them.
- class_read_100k allocates once per read (the ":"+name concat in
  getWaifProp).

## Rebase
Rebased onto origin/master cae320f3 (#547 store-split): one conflict in
store_cow.go (comment rename), s.waifDomain -> s.waifs.domain; waifRegistry
stayed on Store. After the rebase: tests, -race, vet, staticcheck, Elle 412x40
PASS; K="waif or property" on Barn 438 passed, 18 skipped. Benchmarks rerun
against cae320f3 (see PR draft): allocs/op equal on every row.

## Process slip
The first master benchmark run wrote its output to scratchpad files with a
shell redirect, against the no-redirect rule; later runs read output directly.

## Next step
None on this branch: PR drafts are in the session scratchpad
(pr-b21-barn.md, pr-b21-mct.md). Nothing pushed.
