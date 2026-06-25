# Fix F4 — WaifValue representation

## Toast waif semantics: REFERENCE types (proof)

ToastStunt waifs are **reference types**, not value/COW. Evidence in
`C:/Users/Q/src/toaststunt/src`:

- `include/structures.h:174` — the `Var` union holds `Waif *waif;` — a heap
  pointer, not an inline value.
- `utils.cc:282-284` — `var_ref` / `addref` for `TYPE_WAIF` does
  `addref(v.v.waif)`: aliasing a waif var shares the SAME pointer; no copy.
- `utils.cc:340-341` — `var_dup` for `TYPE_WAIF` calls `dup_waif(v.v.waif)`, and
  `waif.cc:608-614` `dup_waif` does `panic_moo("can't dup waif yet")`. Waifs are
  therefore NEVER value-copied; every var-copy is a shared reference.
- `waif.cc:742-833` `waif_put_prop` mutates `w->propvals` in place through the
  shared `Waif *w`. A property SET is visible to every holder of that waif.
- `new_waif` (`waif.cc:270-295`) `mymalloc`s a fresh `Waif` per call, so two
  separately created waifs are independent references.

So: `x = w; x.p = 5` makes `w.p == 5`, and two `new_waif()` results are
independent. The review's "broken copy-on-write" framing was a mis-stated
expectation — like the earlier `is_member` case.

### F14 note for the next coder (NOT fixed here)
Waif EQUALITY in Toast is **reference identity**: `utils.cc:431`
(`lhs.v.waif == rhs.v.waif ? 0 : 1`) and `utils.cc:477-478`
(`return lhs.v.waif == rhs.v.waif;`). Barn's `WaifValue.Equal` still uses
structural map comparison — that is finding F14 and was deliberately left red.
With the new pointer representation, the F14 fix is now trivial: compare
`w.data == otherWaif.data` (pointer identity).

## Outcome implemented: REFERENCE (made explicit and robust)

Barn's old `WaifValue` was a value struct whose `properties map[string]Value`
aliased on copy — accidentally reference-ish, but fragile and labelled
"copy-on-write". I made the reference semantics explicit.

### `types/waif.go`
- `WaifValue` is now a thin handle wrapping a single pointer:
  `type WaifValue struct { data *waifData }` where `waifData` holds
  `class`, `owner`, `properties`. Every Go copy of a `WaifValue` copies only the
  handle, so all copies reference the SAME underlying `waifData` — true Toast
  reference semantics, never an accidental fork.
- Replaced the misleading "copy-on-write" comments with a citation-backed
  explanation that waifs are reference types and `SetProperty` mutates in place.
- Added `nil`-data guards to `Class/Owner/GetProperty/PropertyNames/Equal/
  SetProperty` for zero-value safety.
- `Equal` left structural (F14), now via `w.data` fields, with a nil guard.

### Callers — no changes needed
The pointer representation preserves the exact API. Two callers intentionally
discard `SetProperty`'s return and rely on shared mutation; both still work:
- `vm/op_property.go:249` `_ = waif.SetProperty(propName, value)`
- `db/format/reader_helpers.go:252` `wd.waif.SetProperty(name, val)`
`go build ./...` is clean.

## Tests corrected (and why)

The three F4 red tests asserted value/COW behavior, which is WRONG for a
reference type. Corrected to assert Toast reference semantics, citing
`waif.cc:742` / `utils.cc:282-284,340-341` / `waif.cc:612`:

1. `types/review_test.go::TestReview_WaifSetPropertyMutatesOriginal` — now
   asserts an alias sees the set (`w2 = w1; w2.x = 42` ⇒ `w1.x == 42`) and that
   two separately created waifs are independent.
2. `vm/review_bugs_test.go::TestReview_WaifPropertyMutationAliasesAcrossStructCopies`
   — now asserts `localB` DOES see `localA`'s mutation.
3. `vm/review_bugs_test.go::TestReview_WaifSetPropertyMutatesOriginalNotCopy` —
   now asserts the original handle observes the in-place write.

## Test output

`go test ./types/ ./vm/ -run 'Waif' -v` (F4 tests pass; F14/F15 stay red):
```
--- PASS: TestReview_WaifSetPropertyMutatesOriginal
--- FAIL: TestReview_WaifEqualUsesDeepequalNotIdentity   (F14 — left red)
--- PASS: TestReview_WaifPropertyMutationAliasesAcrossStructCopies
--- PASS: TestReview_WaifSetPropertyMutatesOriginalNotCopy
--- FAIL: TestReview_ContainsWaifFalsePositive_...        (F15 — left red)
```

`go test -race ./types/ ./vm/` — no DATA RACE. Only failures are the
pre-existing intentionally-red set (unchanged before/after):
- `barn/types`: `TestReview_ObjEqualIgnoresAnonFlag`,
  `TestReview_WaifEqualUsesDeepequalNotIdentity` (F14)
- `barn/vm`: `TestReview_MapInChecksValuesNotKeys`,
  `TestReview_MapInValueFoundAsKey_ReturnsZero`,
  `TestReview_ContainsWaifFalsePositive_...` (F15)

### Before/after failure list
- Before (F4 portion): 3 FAIL — WaifSetPropertyMutatesOriginal,
  WaifPropertyMutationAliasesAcrossStructCopies, WaifSetPropertyMutatesOriginalNotCopy.
- After: 0 of those fail. No new failures introduced; the remaining red tests
  (ObjEqual, F14 equality, B1 map-in ×2, F15 containsWaif) are unrelated
  pre-existing findings.

## Commit
`COMMIT_HASH_PLACEHOLDER`
