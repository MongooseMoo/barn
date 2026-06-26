# Fix F14 — waif equality must be reference identity, not structural

## Toast authority (confirmed by reading source)

ToastStunt waif equality is **reference identity** — it compares the `Waif *`
pointers, never the property contents. Evidence in
`C:/Users/Q/src/toaststunt/src/utils.cc`:

- `utils.cc:478` — `equality()` for `TYPE_WAIF`:
  `return lhs.v.waif == rhs.v.waif;`
- `utils.cc:431` — the `compare()` path for `TYPE_WAIF`:
  `return lhs.v.waif == rhs.v.waif ? 0 : 1;`

Both compare the raw `Waif *` pointers. No deep/structural comparison of
property maps anywhere. (Compare `TYPE_LIST`/`TYPE_MAP` at `utils.cc:471-473`,
which DO recurse via `listequal`/`mapequal` — waifs deliberately do not.)

## The change

`types/waif.go` `WaifValue.Equal`: after F4 a `WaifValue` is a thin handle over
a shared `*waifData`. Equality is now pointer identity of that handle:

```go
func (w WaifValue) Equal(other Value) bool {
    otherWaif, ok := other.(WaifValue)
    if !ok {
        return false
    }
    return w.data == otherWaif.data
}
```

- Removed the structural deep-compare (`w.data.class == ... && equalMaps(...)`).
- `equalMaps` (the property-map deep comparator) became unused and was deleted.
- nil/zero waifs handled naturally: two zero `WaifValue`s share `data == nil`
  and are equal; a zero waif vs a live waif differ by pointer.
- `Truthy`/`String`/`Type` unchanged (correctly — they do not depend on Equal).

## Test (both halves pinned)

`types/review_test.go::TestReview_WaifEqualUsesDeepequalNotIdentity` now asserts
the full reference-identity contract, with a Toast citation comment:

1. Two independently created waifs (`NewWaif(1,2)` ×2, distinct `*waifData`) are
   NOT equal.
2. ...still NOT equal after giving both the identical property `x = 7` (proves no
   deep compare).
3. Two handles sharing one `*waifData` (`w1alias := w1`) ARE equal.

The original test only had half 1; halves 2 and 3 were added.

## Hash / map-key follow-up (NOT fixed — reported)

`types/map.go:34` `keyHash` hashes a waif via `v.String()`, which is
`"<waif #class>"` (class only). So two distinct waifs of the same class produce
the same map key and collide. This disagrees with identity `Equal` (distinct
waifs are now unequal yet share a key). Note: this disagreement is **pre-existing**,
not introduced by this fix — the old structural `Equal` already disagreed with
the class-only `keyHash` (e.g. same-class waifs with different props were unequal
but collided). Waifs-as-map-keys is an untested edge case and the fix belongs in
the map subsystem (would need a per-waif identity token such as the `*waifData`
address in `keyHash`). Left as a follow-up per the prompt's scope guidance.

## Test output

`go test ./types/ ./vm/ -run 'Waif' -v`:
```
--- PASS: TestReview_WaifSetPropertyMutatesOriginal
--- PASS: TestReview_WaifEqualUsesDeepequalNotIdentity   (F14 — now green)
--- PASS: TestReview_WaifPropertyMutationAliasesAcrossStructCopies
--- PASS: TestReview_WaifSetPropertyMutatesOriginalNotCopy
--- FAIL: TestReview_ContainsWaifFalsePositive_...        (F15 — pre-existing red)
```

## Before/after failure list (`go test ./types/... ./vm/...`)

- Before: `types`: ObjEqualIgnoresAnonFlag, **WaifEqualUsesDeepequalNotIdentity (F14)**.
  `vm`: MapInChecksValuesNotKeys, MapInValueFoundAsKey_ReturnsZero, ContainsWaifFalsePositive (F15).
- After: `types`: ObjEqualIgnoresAnonFlag. `vm`: MapInChecksValuesNotKeys,
  MapInValueFoundAsKey_ReturnsZero, ContainsWaifFalsePositive (F15).
- Delta: F14 fixed (one fewer failure). No new failures introduced.

## Commit

`de565d4054072df0b639d7dbea046ab7536dcf64`
