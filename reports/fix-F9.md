# Fix F9 — Renumber and ObjValue references in property values

## Verdict

**Toast renumber DOES NOT rewrite property-value object refs.** `db_renumber_object`
rewrites only structural/built-in references and owner fields; it never scans
arbitrary property VALUES for `TYPE_OBJ` references to the old id.

Outcome implemented: **(b)** — Barn was already correct. The red test encoded a
WRONG expectation. I corrected the test to assert true Toast behaviour and left
`Renumber` unchanged.

## Toast renumber semantics (authority, with file:line)

`C:/Users/Q/src/toaststunt/src/db_objects.cc` `db_renumber_object(Objid old)`,
lines 569-714:

- Lines 591-619: the `FIX(up, down)` macro, invoked as `FIX(parents, children)`
  and `FIX(location, contents)` — fixes ONLY the parents/children hierarchy and
  the location/contents hierarchy.
- Lines 624-641: fixes anonymous children's `parents` slot references.
- Lines 643-652: fixes the `all_users` list.
- Lines 653-705: walks every object (and anonymous objects) but rewrites only
  the `.owner` fields of objects (`o->owner`), verbdefs (`v->owner`), and
  propvals (`p[i].owner`). The property VALUES themselves (`p[i].var`) are never
  inspected for `TYPE_OBJ` refs to `old`.
- Lines 708-713: returns the new id; no further reference rewriting.

Entry point `bf_renumber` is `C:/Users/Q/src/toaststunt/src/server.cc:2483-2497`
(wizard-only, valid object), which just calls `db_renumber_object`.

Docs: no separate prose description of renumber semantics exists under
`C:/Users/Q/src/toaststunt/docs/` (only ChangeLog mentions). The source is the
authority and is unambiguous. This also matches the long-standing LambdaMOO
contract: renumber updates built-in property references only; references in
ordinary property values / list elements are the programmer's responsibility.

## Barn's behaviour

`db/store/store_lifecycle.go` `Renumber` (lines 332-418) updates parents,
children, chparentChildren, location, contents, and owner across all objects —
exactly the structural + owner set Toast fixes. It does not (and per Toast must
not) walk property values. **No code change to Renumber.**

## The change

`db/store/review_test.go` `TestReview_RenumberDoesNotUpdatePropertyValues`:
re-pointed the assertion to the true Toast behaviour. After `Renumber(1,2)`,
`#0.ref` (an `ObjValue`) must remain the stale `#1`, not become `#2`. Added a
header comment citing `db_objects.cc:569-714`. No other test changed.

## Test output (green)

```
=== RUN   TestReview_RenumberDoesNotUpdatePropertyValues
--- PASS: TestReview_RenumberDoesNotUpdatePropertyValues (0.00s)
PASS
ok  	barn/db/store	0.253s
```

## Before / after failure list

- Before: `db/store` had exactly one failure —
  `TestReview_RenumberDoesNotUpdatePropertyValues` (red by design).
- After: `go test ./db/...` → `ok barn/db/format`, `ok barn/db/store`. Zero
  failures. No new failures introduced. `go vet ./db/store/` clean.

## Commit

<filled in after commit>
