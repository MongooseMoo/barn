# Fix F27 — `in` on a map: VALUE-search is CORRECT (false finding)

## Toast's map-`in` return semantics (the headline)
`x in map` in ToastStunt searches the map's **VALUES, not its keys**, and returns
the **1-based position (in key-sorted order) of the first pair whose VALUE equals
`x`**, or `0` if no value matches. The comparison is **case-INSENSITIVE**.

So for `["a" -> 1]`:
- `"a" in ["a" -> 1]` → **0**  ("a" is the KEY, not a value)
- `1   in ["a" -> 1]` → **1**  (value `1` matches; key-sorted position 1)

### Source citations (oracle down — source is authority)
- `C:/Users/Q/src/toaststunt/src/execute.cc:1383-1408` — `case OP_IN`: for the
  list/map branch, `ans.v.num = ismember(lhs, rhs, 0)` (line 1403). The `0` is
  `case_matters` → case-insensitive.
- `C:/Users/Q/src/toaststunt/src/collection.cc:46-69` — `ismember`: the
  `TYPE_MAP` branch sets `ismember_data.i = 1`, `ismember_data.value = lhs`, then
  `return mapforeach(rhs, do_map_iteration, &ismember_data)`.
- `C:/Users/Q/src/toaststunt/src/collection.cc:31-43` — `do_map_iteration(key,
  value, ...)`: compares the iterated **`value`** (not `key`) against `lhs`
  (`equality(value, ismember_data->value, case_matters)`, line 36); returns the
  running 1-based index `i` on match, else increments `i` and returns 0.
- `C:/Users/Q/src/toaststunt/src/map.cc:809-823` — `mapforeach` walks the rbtree
  with `rbtfirst`/`rbtnext` → **key-sorted** iteration order, so the returned
  index is the position in key-sorted order.

### Conformance corroboration (same `ismember` helper, via `is_member`)
- `moo-conformance-tests .../builtins/map.yaml:126-129`:
  `is_member("FOO", ["FOO" -> "BAR"]) == 0` — "FOO" is a KEY and is NOT found.
  This alone disproves key-search.
- `map.yaml:116-119`: `is_member("5", ["3"->"3","1"->"1","4"->"4","5"->"5",
  "9"->"9","2"->"2"]) == 5` — value "5" sits at key-sorted position 5.

## Verdict: F27 is a FALSE finding
The review (`reports/review-vm.md` F27 / `REVIEW.md:344`) asserted `x in map`
should search **keys**. ToastStunt searches **values**. Barn's live `executeIn`
(`vm/op_compare.go`) already iterates `pair[1]` (values) with `.Equal`
(case-insensitive) over key-sorted pairs — i.e. it was **already correct** and
already matched Toast. No behavioral change to `executeIn` was needed or made.

## Changes made
- **`vm/op_compare.go` `executeIn`** — behavior unchanged; added a Toast-cited
  comment on the map branch documenting value-search semantics and a "do NOT
  change to key search" guard so F27 is not re-flagged.
- **`vm/operators.go` `inOp`** — DEAD path. LSP `findReferences` on the symbol
  returned exactly **1 reference (its own definition)** → no callers; the VM
  dispatches `executeIn` (`vm/vm.go:483`). Its map branch already searched values
  identically, so there was **no divergence**. Left functionally unchanged; added
  a comment marking it dead and citing Toast, kept identical to `executeIn`.
- **`vm/review_bugs_test.go`** — corrected the red tests to Toast's TRUE returns
  (the review's expected values were wrong):
  - `TestReview_MapInChecksValuesNotKeys` → `TestReview_MapInSearchesValues`:
    `"a" in ["a" -> 1]` now asserts **0** (was asserting 1).
  - `TestReview_MapInValueFoundAsKey_ReturnsZero` →
    `TestReview_MapInValueFoundReturnsKeySortedPosition`:
    `1 in ["a" -> 1]` now asserts **1** (was asserting 0).
  - Added `TestReview_MapInValueAtSortedPosition`
    (`30 in ["b"->20,"a"->10,"c"->30] == 3`) mirroring the is_member case.
  - Kept `"z" in ["a"->1] == 0` (renamed `TestReview_MapInNotPresent`).
  Each assertion carries a Toast `file:line` citation.
- `in` on lists/strings: untouched.

## Test results
Before (red): `TestReview_MapInChecksValuesNotKeys` FAIL (got 0, asserted 1);
`TestReview_MapInValueFoundAsKey_ReturnsZero` FAIL (got 1, asserted 0). These
failed precisely because Barn correctly does value-search while the tests
asserted key-search.

After:
```
go test ./vm/ -run 'MapIn|In|Member' -v   → all PASS
  TestReview_MapInSearchesValues                       PASS
  TestReview_MapInValueFoundReturnsKeySortedPosition   PASS
  TestReview_MapInValueAtSortedPosition                PASS
  TestReview_MapInNotPresent                           PASS
go test ./vm/...                          → ok  barn/vm
go vet ./vm/                              → 1 PRE-EXISTING note only:
  vm/stack.go:49 ReadByte() signature (unrelated file, not touched by F27)
```

## Commit
`<filled below>` on `review/branch-stocktake-2026-06-25`.
