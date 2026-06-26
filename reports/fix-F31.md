# Fix F31 — Renumber must rewrite verb/property owners (Toast parity)

## Origin
NEW finding from the F9 verifier (`reports/verify-F9.md`). No pre-existing red test.

## The bug
`db/store/store_lifecycle.go` `Renumber(oldID, newID)` rewrote structural refs
(parents/children/location/contents) and the object `.owner` field, but did NOT
rewrite ownership references stored on verbs or properties, nor owners on
anonymous objects. After a renumber, verbdef/propval owners that pointed at the
old id were left dangling at the now-recycled old id.

## Authority: ToastStunt `db_renumber_object`
`C:/Users/Q/src/toaststunt/src/db_objects.cc`, owner-fix block at lines 653-705.
The exact set of `.owner` fields rewritten, each with the dual rule
`owner == new → NOTHING; else owner == old → new`:

- Numbered objects (loop 657-684):
  - object `o->owner` — **db_objects.cc:666-669**
  - each verbdef `v->owner` — **db_objects.cc:671-675**
  - each propval `p[i].owner` — **db_objects.cc:679-683**
- Anonymous objects (loop 687-705):
  - object `o->owner` — **db_objects.cc:692-695**
  - each propval `p[i].owner` — **db_objects.cc:699-703**
  - (anonymous objects carry no verbdefs in this loop)

Structural handling confirmed complete vs Toast: the `FIX` macro (591-619) only
rewrites refs equal to `old` for parents/children and location/contents; anon
children's parent slots (624-641) and `all_users` (643-652) are likewise old→new
only. Barn already does parents/children/location/contents/chparentChildren and
keeps anon-children parent handling elsewhere, so only the owner set was missing.
Property VALUES (`p[i].var`) are never scanned — Barn correctly leaves those
alone (the negative test still holds).

## TDD — red then green
New test `TestReview_RenumberRewritesVerbAndPropOwners` in
`db/store/review_test.go`: puts a verb and a property on #0 both owned by #1,
renumbers #1→#2, asserts both owners are now #2.

Red (before fix):
```
=== RUN   TestReview_RenumberRewritesVerbAndPropOwners
    review_test.go:232: verb owner = #1 after renumber, want #2 (Toast rewrites verbdef owners)
    review_test.go:240: property owner = #1 after renumber, want #2 (Toast rewrites propval owners)
--- FAIL: TestReview_RenumberRewritesVerbAndPropOwners (0.00s)
```

Green (after fix):
```
=== RUN   TestReview_RenumberRewritesVerbAndPropOwners
--- PASS: TestReview_RenumberRewritesVerbAndPropOwners (0.00s)
=== RUN   TestReview_RenumberDoesNotUpdatePropertyValues
--- PASS: TestReview_RenumberDoesNotUpdatePropertyValues (0.00s)
PASS
ok  	barn/db/store	0.265s
```

## The Renumber change
Added a `rewriteOwner` closure implementing Toast's dual rule and applied it to:
the object owner, every verb owner (`other.verbs`), and every property owner
(`other.properties`) in the main object loop; plus a new loop over
`s.anonObjects` rewriting anon object + property owners. Structural rewrites
unchanged.

## Hardened existing test
`TestReview_RenumberDoesNotUpdatePropertyValues` now moves #0 inside #1
(`MoveObject`) before renumber and asserts `Location(0) == 2` afterward — a
positive structural-ref control — while keeping the property-VALUE negative
(`ref` still holds stale #1).

## Before/after failure list (`go test ./db/...`)
- Before fix: `TestReview_RenumberRewritesVerbAndPropOwners` FAIL; everything else PASS.
- After fix: `barn/db/store` ok, `barn/db/format` ok. `go vet ./db/store/` clean.

## Commit
COMMIT_HASH_PLACEHOLDER (branch review/branch-stocktake-2026-06-25)
