# Fix F2 — runtime-created anonymous objects lost at checkpoint (data loss)

## Toast's anonymous-object serialization model (authority)

Read from `C:/Users/Q/src/toaststunt/src/db_objects.cc`:

- `db_make_anonymous` (`db_objects.cc:449-465`): when an object becomes anonymous
  at runtime, Toast sets `o->id = NOTHING` and clears its slot in the numbered
  `objects[]` array. A live anonymous object therefore has **no numbered id** and
  does **not** live in the numbered object space.
- `dbpriv_assign_anonymous_object` (`db_objects.cc:415-433`): at dump time, if the
  anon's `id == NOTHING`, Toast allocates a fresh slot
  (`objects[num_objects] = o; oid = o->id = num_objects; num_objects++;`) — i.e. it
  assigns an **above-max serialization id** on the spot.
- `db_write_anonymous` (`db_objects.cc:441-447`): calls the above and writes that
  assigned id with `dbio_write_num(oid)`.
- `db_read_anonymous` / `dbpriv_read_anonymous_object` (`db_objects.cc:376-413`):
  reads them back by that id.

**Conclusion:** in Toast, every anonymous object — however it was created (runtime
or loaded) — lives out-of-band with no numbered id while live, and is assigned an
above-max serialization id uniformly at dump time. They round-trip uniformly.
Barn's `s.anonObjects` path already mirrors this; the bug was that the runtime
`CreateObject` path did not use it. The red test asserts exactly Toast behavior, so
no test was modified.

## What changed and where

### 1. `db/store/store_lifecycle.go` — `CreateObject`
Anonymous runtime creation no longer calls `insertObjectLocked` (which lands the
object in the numbered `s.objects` map). It now stores the object in
`s.anonObjects[newID]` — the same out-of-band collection the loader's
`AddAnonymous` uses and that the field comment in `store_core.go` documents as the
sole home for anonymous objects. `highWaterID` is still bumped (the identity id
must never be reissued to a later allocation) but `maxObjID` is left untouched, so
`max_object()` is unaffected (existing invariant preserved).
`attachChildToParentsLocked` is still called; it only mutates the *parents'* child
lists and never needs the anon child in `s.objects`.

### 2. `db/store/store_reachability.go` — GC/reachability scans unified
Added `lookupAnonymousLocked(id)` and `rangeAnonymousLocked(fn)` helpers that
consider **both** backing maps, and routed the anon scans through them:
- `expandAnonymousReachabilityLocked` (was `s.objects[id]`)
- `UnreachableAnonymousValues` (was `s.objects[id]`)
- `AnonymousRecycleCandidates` (was iterating `s.objects`)

This guarantees the planner (`planAnonymousSerializationLocked`, already reading
`s.anonObjects`), the GC candidate scan, and the serializer all see one consistent
set of anonymous objects. The union keeps existing `vm/anonymous_gc_test.go`
fixtures green (those inject anon via `store.Add` → `s.objects`) while making
runtime anon (now in `s.anonObjects`) visible to GC.

## Tests corrected
None. Toast confirms the red test's expectation; the test asserts true Toast
behavior.

## Green test output
```
=== RUN   TestReview_RuntimeAnonLostAtSnapshot
--- PASS: TestReview_RuntimeAnonLostAtSnapshot (0.00s)
PASS
ok  	barn/db/store	0.248s
```
`go test -race ./db/store/` (anon tests): `ok` (race clean).

## No new regression (full-package failing-test diff, before → after)

Before (HEAD), failing across `./db/store/... ./db/format/... ./vm/...`:
```
TestReview_ContainsWaifFalsePositive_SameClassOwnerDistinctInstances
TestReview_MapInChecksValuesNotKeys
TestReview_MapInValueFoundAsKey_ReturnsZero
TestReview_RenumberDoesNotUpdatePropertyValues
TestReview_RuntimeAnonLostAtSnapshot      <-- F2
TestReview_WaifPropertyMutationAliasesAcrossStructCopies
TestReview_WaifSetPropertyMutatesOriginalNotCopy
```
After: identical list **minus** `TestReview_RuntimeAnonLostAtSnapshot`. The only
delta is F2 flipping red→green. All remaining failures are the pre-existing,
intentionally-red tests for other findings (Renumber, map-`in`, waif COW,
containsWaif).

## Follow-up (out of scope, untested, noted)
The GC's `recycle()` execution path (`store.Recycle`) still looks up `s.objects`,
so a runtime anon that becomes a recycle candidate cannot currently be recycled by
id from `s.anonObjects`. This is the same situation that already applied to loaded
anon and is beyond F2 (data-loss-at-checkpoint) scope; no test covers it.

## Commit
`COMMIT_HASH_PLACEHOLDER`
