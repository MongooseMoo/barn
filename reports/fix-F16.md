# Fix F16 — setadd/unique equality coherence

## Toast's true per-builtin equality (cited)
- **setadd → case-INSENSITIVE.** `toaststunt/src/list.cc:149-156` `setadd()` calls
  `ismember(value, list, 0)`; `toaststunt/src/collection.cc:45-69` `ismember` evaluates
  `equality(lhs, rhs.v.list[i], case_matters)` with `case_matters == 0`, so strings
  compare case-insensitively. (Barn `setadd` already uses `.Equal`, which matches.)
- **is_member → case-SENSITIVE by default.** `toaststunt/src/collection.cc:84`
  `case_matters = arglist.v.list[0].v.num < 3 || is_true(arglist.v.list[3])`; with 2 args
  count<3 so `case_matters` is TRUE. (Separate finding, not F16; left untouched.)
- **`unique` does NOT exist in ToastStunt.** A grep of `toaststunt/src` finds no `unique`
  symbol, `function_info("unique")` aborts, and the oracle eval of `unique(...)` produces
  no result. `unique` is a Barn-only extension with no Toast authority.

## Oracle outputs (bare exprs; oracle.sh prepends `;`)
```
setadd({"hello"},"HELLO")  => {"hello"}    # not added → case-insensitive
is_member("HELLO",{"hello"}) => 0          # case-sensitive
"HELLO"=="hello"           => 1            # == case-insensitive
unique({"hello","HELLO"})  => (no output)  # function absent in Toast
```

## Verdict on F16
The review framed setadd vs unique as "incoherent". setadd's true rule (case-insensitive)
is correct and unchanged. unique has no Toast rule, so the "legitimately different
equality → fix the test" branch does not apply by Toast authority. unique is a set-dedup
operation; MOO set operations (setadd/setremove) and `==` use case-insensitive value
equality. The coherent and correct fix is to make unique dedup with the SAME value
`.Equal` setadd uses.

## Change
`builtins/lists.go` `builtinUnique`: replaced the `elem.String()` map-key dedup
(case-SENSITIVE, included quotes) with first-occurrence dedup using value `.Equal`
(case-insensitive for strings), matching setadd. No test was weakened; both F16 red tests
assert the now-correct behavior, so they were kept and now pass.

## Tests
Green (F16 targeted):
```
--- PASS: TestReview_Data_UniqueStrCaseInsensitive
--- PASS: TestReview_Data_SetaddUniqueConsistency
```
Full `go test ./builtins/...`: the remaining failures are pre-existing intentionally-red
tests for OTHER findings — `IsMemberStrCaseSensitiveBug`, `SortReverseIgnored`,
`PcreMatchEmptySubject`, `CapitalizeDeprecatedTitle`, `FileReadlinesBinaryMode`,
`QueuedTasksSortOrder`, `VerbCodeAllowsOwnerWithoutReadBit`, `AddVerbUsesProgNotPlayerForPerm`.
None call `unique`; no NEW failures introduced.

### Before → after (F16 only)
- Before: `UniqueStrCaseInsensitive` FAIL, `SetaddUniqueConsistency` FAIL.
- After:  both PASS. Other red tests unchanged.

## Commit
<filled below>
