# Fix fuzz finding #677: anonymous objects are never parents

## Goal

Fix the FuzzReadDatabase finding from native-only campaign run 42672041535
(issue #677): `round-trip verification failed: suspended task 0 frame 0 locals
differs`, input `db/format/testdata/fuzz/FuzzReadDatabase/69ffe028797b5a66`.

## State

Fix implemented on branch `fix/fuzz-677-suspended-locals`. The seed is committed
as the regression and passes. The paired conformance rows are in
moo-conformance-tests branch `fix/anonymous-parent-type` (separate PR, merged
after this one because Barn CI tests against conformance `main`).

## Decisions and why

- Cause: store ancestry walks resolved parent ids with `liveObjectLocked`,
  which falls back to `anonObjects[id]`. A loaded anonymous object whose OBJ
  parent id names no live numbered object resolved that parent to the
  anonymous object sharing the id. The writer then emitted that object's
  definitions as inherited slots, the reader (which names slots only from
  numbered `database.Objects`) reloaded them as `_inherited_N` placeholders,
  and the snapshot's backstop re-emitted the placeholders on the next write:
  one extra slot per checkpoint.
- Toast (source): parents are OBJ only. `is_obj()` is `TYPE_OBJ == type`
  (include/structures.h:222); `create` and `chparent(s)` return E_TYPE unless
  `is_obj_or_list_of_objs` (objects.cc:320, :560); the loader links an
  anonymous object only to `valid()` OBJ parents (db_file.cc:351-355).
- Fix 1: `parentObjectLocked` resolves parent ids among numbered objects only;
  used for parents in the snapshot name walk, `findPropertyLocked`,
  `HasVerbNameInAncestry` and `FindVerbInAncestry`. The starting object still
  resolves through `liveObjectLocked` (it may be anonymous).
- Fix 2: `create` parents and `chparents` elements must be TYPE_OBJ (E_TYPE
  otherwise), the second producer of anonymous parents in Barn.
  `chparent(x, anon)` already returned E_TYPE on Barn; unchanged.

## Evidence

- Toast oracle, `/root/src/toaststunt/build-release/moo`, four new rows plus
  capability admission: `5 passed, 13780 deselected`.
- Unchanged Barn origin/master b805c1db: `3 failed, 2 passed`
  (create anon parent, create list with anon parent, chparents list with anon).
- After fix: seed passes locally; `create(a, 1)` / `create({a}, 0)` with
  anonymous `a` return E_TYPE in `bin/moo`.

## Surprises

The fuzz signature hash of #677 equals closed #666 (different error); the
reporter keys on target and kind, not message.

## Next step

PR CI green, merge, then merge the conformance PR and close #677.
