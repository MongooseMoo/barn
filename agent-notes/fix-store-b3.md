# fix/store-b3: chparent() of an anonymous object lists it in the new parent's children()

## Goal
Bug B3 from agent-notes/db-store-review.md (on the main checkout): `chparent(anon, q)`
makes `children(q)` list the anonymous object. Fix conformance-test-first.

## State
- Conformance: C:/Users/Q/code/mct-store-b3, branch test/store-b3 off origin/main (be876f5).
  Row `builtins/objects.yaml::chparent_anonymous_not_added_to_parent_children`, commit ff64290.
  Managed WSL Toast (aecc51e9, binary sha256 72fb1cf9...): PASSED. Barn origin/master
  (90681354): FAILED, got `[[[], []], [], ['#11'], [], ['#11'], 1]`.
- Barn: C:/Users/Q/code/barn-store-b3, branch fix/store-b3 off origin/master (90681354).
  Fix in db/store/store_relationships.go `ChangeParents`; regression
  `TestChangeParentsAnonymousMovesAnonymousChildren` in db/store/store_test.go.
  Red before fix (children of new parent, stale old-parent anonymousChildren, missing
  new-parent anonymousChildren); green after; `go test ./db/store` ok; managed row PASSED
  on the fixed Linux build.
- Nothing pushed; PR bodies drafted in the session scratchpad (pr-b3-barn.md, pr-b3-mct.md).

## Decisions and why
- Only one chparent path exists: `Store.ChangeParents` (db/store/store_relationships.go:300),
  called from builtins/objects_hierarchy.go:222 (chparent) and :351 (chparents); the txn then
  adopts live relationships. No separate StoreTxn chparent path.
- Toast (aecc51e9, src/db_objects.cc:1086-1115): for TYPE_OBJ it edits parents' `children`;
  for TYPE_ANON it moves the anon between parents' `anonymous_objects` entries
  (dbpriv_remove_anon old, dbpriv_add_anon new). Barn's equivalent is `anonymousChildren`,
  so the fix removes the anon from old parents' anonymousChildren and attaches with
  `obj.anonymous` (which also keeps it out of chparentChildren, as createObject does).
- The row lives in builtins/objects.yaml, not language/anonymous.yaml: anonymous.yaml requires
  feature `anonymous`, which the stock Toast oracle does not advertise, so every row in it is
  SKIPPED on Toast (`Requires feature: anonymous`) and could not be proven Toast-green.

## Surprises
- `Store.InvalidateAnonymousChildren` (store_lifecycle.go:669) has no production caller.
- Recycle probes give the same answer on both engines: Toast's recycle reparents anon
  kids via db_next_child (db_objects.cc:980-990), Barn reparented the wrongly-numbered child.
  The stale old-parent anonymousChildren entry had no MOO-visible effect found; it is
  covered by the Go regression only.
- language/anonymous.yaml rows never run against the stock Toast oracle (feature gate above).

## Next step
Lead reviews; pushing and PRs are held.
