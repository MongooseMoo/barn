# Fix F15 — containsWaif false positive (class+owner vs instance identity)

## Finding
`vm/collection_helpers.go` `containsWaif` decided a value "is" the target waif by
`v.Class() == waif.Class() && v.Owner() == waif.Owner()`. Two distinct waifs sharing
class+owner were treated as the same instance, producing a false `E_RECMOVE` on
legitimate property assignment (`vm/op_property.go:241` `setWaifProp`).

## Toast authority (identity comparison)
ToastStunt's recursive-containment check is `refers_to` in
`C:/Users/Q/src/toaststunt/src/waif.cc:236-268`:
- Leaf test is **WAIF POINTER IDENTITY**, not class/owner:
  `waif.cc:250` — `if (waif_self_check && target.v.waif == key.v.waif) return 1;`
- It then recurses into the waif's own property values:
  `waif.cc:252-256` (`p = target.v.waif->propvals; ... refers_to(*p++, key, true)`).
- Invoked at `waif.cc:816` (`if (refers_to(val, me, true)) return E_RECMOVE;`).

## Change
`vm/collection_helpers.go` `containsWaif`:
- Leaf comparison changed from class+owner to instance identity via
  `WaifValue.Equal` (data-pointer equality, F14 / *waifData identity, F4).
- Added recursion into the waif's own property values to match Toast
  (`waif.cc:252-256`), with a visited set keyed on waif identity
  (`map[types.WaifValue]bool`) to guard aliasing cycles and guarantee termination.
- Existing list/map traversal preserved (now threading the visited set through a
  `containsWaifVisited` helper; public `containsWaif` signature unchanged).

## Tests (both halves)
`TestReview_ContainsWaifFalsePositive_SameClassOwnerDistinctInstances` now asserts:
- False-positive gone: `containsWaif(waifB, waifA)` is false for distinct
  instances with same class+owner.
- True positives detected: a waif contains itself; target nested in a list;
  target stored in another waif's property; and a distinct same-class instance in
  that property still does NOT match.

## Verification
```
go test ./vm/ -run 'TestReview_ContainsWaifFalsePositive|Waif' -v
--- PASS: TestReview_WaifPropertyMutationAliasesAcrossStructCopies
--- PASS: TestReview_WaifSetPropertyMutatesOriginalNotCopy
--- PASS: TestReview_ContainsWaifFalsePositive_SameClassOwnerDistinctInstances
PASS
```

### Full vm suite — before vs after
- Before: `TestReview_ContainsWaifFalsePositive_*` FAILED, plus the two
  pre-existing intentionally-red B1 map-`in` tests
  (`TestReview_MapInChecksValuesNotKeys`, `TestReview_MapInValueFoundAsKey_ReturnsZero`).
- After: F15 test PASSES. The only remaining failures are the two B1 map-`in`
  red tests (separate finding, unchanged). No NEW failures.

## Commit
COMMIT_HASH_PLACEHOLDER
