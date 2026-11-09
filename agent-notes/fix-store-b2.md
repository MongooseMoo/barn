# fix/store-b2: renumber() must not hand out an anonymous object's id

## Goal
Fix B2 from agent-notes/db-store-review.md (master working tree): renumber() could
give a numbered object the id of a live (or recycled) anonymous object.
Conformance test first (moo-conformance-tests worktree C:/Users/Q/code/mct-store-b2,
branch test/store-b2), then the Barn fix here (branch fix/store-b2, written on
90681354, rebased cleanly onto 3cb7e64c after #486 merged; db/store tests and the
conformance file passed again after the rebase: `3 passed in 23.73s`).
No push, no PR; PR bodies drafted in the session scratchpad (pr-b2-barn.md,
pr-b2-mct.md).

## State
Done locally, both branches committed, nothing pushed.
- Repro on master (bin/moo, Minimal.db): `a = create(#-1,1); b = create(#-1);
  n = renumber(b); return {a,b,n};` => `{*#4, #5, #4}`; adding `n.name = "x"` =>
  E_INVARG "commit conflict not retryable". Recycled-anon path is reachable:
  `a = create(#-1,1); recycle(a); b = create(#-1); n = renumber(b);` => n = #4 and
  `valid(a)` = 1 afterwards.
- Toast source (WSL /root/src/toaststunt @ aecc51e9): db_renumber_object
  (src/db_objects.cc:544-552) fills only `objects[_new] == nullptr` for _new < old;
  anonymous objects are not in `objects[]` (separate `anonymous_objects` map).
- Conformance: builtins/renumber_anonymous_identity.yaml, mct commit 933aabf.
  Stock WSL Toast 3 passed; Barn 90681354 2 failed (E_INVARG on the live case;
  `[0, 1, 1, "renumbered"]` on the recycled case); Barn with fix 3 passed.
- Store regression TestLowestFreeIDSkipsAnonymousObjectIDs (db/store/store_test.go):
  red on master for both cases (`LowestFreeID = 1, want above #2 (anon is #1)`),
  green with the fix; `go test ./db/store -count=1` ok.
- Fix (db/store/store_lifecycle.go): Recycle no longer appends an anonymous id to
  recycledID; LowestFreeID's gap scan skips ids present in s.anonObjects.

## Decisions and why
- Each conformance test starts with `restart_server: {reset_db: true}`: renumber's
  target depends on every hole in the session's database, so only a fresh fixture
  makes "no lower hole, so n == b" a Toast fact. Trailing reset leaves later tests
  a clean database, like server/signals_children.yaml.
- No `features: [anonymous]` requirement: stock Toast's server_version("features")
  lacks it, so the suite skipped on Toast (`SKIPPED [2] ... Requires feature:
  anonymous`). builtins/create.yaml and gc.yaml use anonymous objects without it.
- Fix at both sources (Recycle's append, LowestFreeID's nil-slot branch) rather
  than filtering recycledID inside LowestFreeID: recycledID is documented as ids
  for reuse, and an anon id is never reusable.

## Surprises
- `--fail-on-unexpected-skip` did not flag the feature skip because the YAML
  itself declared the requirement.
- language/anonymous.yaml declares `features: [anonymous]` too; by the same check it
  likely skips on stock Toast. Not verified (outside this task).
- WSL git cannot read a Windows worktree's .git file; WSL `make conformance-k`
  needs `GOFLAGS=-buildvcs=false`.
- s.anonObjects entries are never deleted (no `delete(s.anonObjects` in db/store),
  so the gap-scan check holds for recycled anons as well.

## Next step
Team lead: review, then push/PR both branches (mct first).
