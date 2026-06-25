# Data-Type Builtins Review

**Scope:** `builtins/lists.go`, `maps.go`, `strings.go`, `math.go`, `json.go`, `url.go`, `ansi.go`, `pcre.go`, `limits.go`, `runtime_options_test.go`

**Baseline:** `go test ./builtins/... -count=1` → PASS (0 failures before this review)

**Test file written:** `builtins/review_data_test.go` (prefix `TestReview_Data_`)

**Toast oracle:** unavailable (toast_moo.exe absent from expected path; ELF build cannot run on Windows). Behaviors marked CONFIRMED are confirmed by red test output. Behaviors marked SUSPECTED require oracle verification.

---

## Architecture Summary

The data-type builtins are a mixed bag. Limit checking (`limits.go`) is well-structured with proper RWMutex guards and sensible defaults. The JSON implementation (`json.go`) has a thoughtful ordered-map design. URL and ANSI builtins are minimal and correct.

The list/string layer has systemic inconsistency in string-equality semantics: three different comparison functions (`Equal` case-insensitive, `strictEqual` case-sensitive, `String()` as key) are used inconsistently across builtins that should all follow MOO's case-insensitive default. The PCRE layer uses Go RE2 instead of the Spencer ERE engine ToastStunt uses, which is an accepted known limitation, but has an additional early-exit bug for empty subjects. Math is mostly fine but has a hard overflow on `abs(MinInt64)`.

---

## CONFIRMED Bugs (red tests, all in `builtins/review_data_test.go`)

### CRITICAL

**BUG-1: `abs(MinInt64)` returns a negative value (silent overflow)**

`builtins/math.go` line 28: `-v.Val` on `math.MinInt64` overflows in two's-complement and returns `math.MinInt64` unchanged.

```
=== RUN   TestReview_Data_AbsMinInt64Overflow
    review_data_test.go:32: abs(MinInt64) = -9223372036854775808 (negative!), want an error
    review_data_test.go:35: abs(MinInt64) returned a value -9223372036854775808 instead of an error
--- FAIL: TestReview_Data_AbsMinInt64Overflow (0.00s)
```

Toast raises `E_FLOAT` on integer overflow. Barn silently returns a mathematically impossible negative absolute value.

---

### HIGH

**BUG-2: `is_member()` uses case-SENSITIVE string comparison**

`builtins/lists.go` line 240 calls `strictEqual` for list search. The code comment says explicitly "case-sensitive for strings." But MOO string equality is case-insensitive (`StrValue.Equal` uses `EqualFold`). `is_member("HELLO", {"hello"})` must return 1.

```
=== RUN   TestReview_Data_IsMemberStrCaseSensitiveBug
    review_data_test.go:69: is_member("HELLO", {"hello"}) = 0, want 1 (MOO string equality is case-insensitive)
--- FAIL: TestReview_Data_IsMemberStrCaseSensitiveBug (0.00s)
```

**BUG-3: `unique()` uses `String()` as dedup key — case-sensitive, wrong**

`builtins/lists.go` line 338: `seen[elem.String()]`. `StrValue.String()` returns the MOO-quoted representation (e.g. `"\"hello\""`), which is case-sensitive. `unique({"hello","HELLO"})` keeps both elements.

```
=== RUN   TestReview_Data_UniqueStrCaseInsensitive
    review_data_test.go:53: unique({"hello","HELLO"}) = 2 elements, want 1 (MOO strings are case-insensitive)
--- FAIL: TestReview_Data_UniqueStrCaseInsensitive (0.00s)
```

BUG-2 and BUG-3 combine into a semantic coherence failure: `setadd`/`setremove` use `Equal` (case-insensitive) while `is_member` and `unique` use case-sensitive comparison. A value that `setadd` considers "already present" is invisible to `is_member`, and `unique` will not remove it.

```
=== RUN   TestReview_Data_SetaddUniqueConsistency
    review_data_test.go:99: setadd sees them as equal (len=1) but unique keeps both (len=2) — inconsistent
--- FAIL: TestReview_Data_SetaddUniqueConsistency (0.00s)
```

**BUG-4: `sort()` silently ignores `keys`, `natural`, and `reverse` arguments**

`builtins/lists.go` line 276 has a `// TODO: Implement full sort with all parameters` comment. The function accepts 1–4 args without error but ignores args 2–4. `sort({1,2,3}, {}, 0, 1)` with `reverse=1` returns `{1,2,3}` instead of `{3,2,1}`.

```
=== RUN   TestReview_Data_SortReverseIgnored
    review_data_test.go:120: sort({1,2,3}, {}, 0, 1) first element = 1, want 3 (reverse flag ignored)
--- FAIL: TestReview_Data_SortReverseIgnored (0.00s)
```

No error is returned — callers who pass `reverse=1` get silently wrong results.

**BUG-5: `pcre_match()` early-returns `{}` for empty subject without attempting match**

`builtins/pcre.go` lines 22–24 unconditionally return an empty list when `subject == ""`. Patterns like `.*` and `^$` match the empty string; Toast returns a match result. The guard must be removed.

```
=== RUN   TestReview_Data_PcreMatchEmptySubject
    review_data_test.go:140: pcre_match("", ".*") = {} (empty), want a match result for the empty string
--- FAIL: TestReview_Data_PcreMatchEmptySubject (0.00s)
```

**BUG-6: `capitalize()` uses deprecated `strings.Title` — apostrophe bug**

`builtins/strings.go` line 348. `strings.Title` capitalizes the character after any Unicode word-boundary including apostrophes. `capitalize("it's a test")` returns `"It'S A Test"` (the S after the apostrophe is wrongly uppercased). The function is deprecated in Go 1.18+.

```
=== RUN   TestReview_Data_CapitalizeDeprecatedTitle
    review_data_test.go:168: capitalize("it's a test") = "It'S A Test" (strings.Title apostrophe bug: 'S capitalized)
--- FAIL: TestReview_Data_CapitalizeDeprecatedTitle (0.00s)
```

Fix: use `golang.org/x/text/cases`.

---

## ARCHITECTURAL Findings (no test; severity-ranked)

**ARCH-1 [HIGH]: Debug `fmt.Printf` left in production `builtinSlice`**

`builtins/lists.go` lines 437 and 479 contain `fmt.Printf("[SLICE DEBUG] ...")` calls. Every call to `slice()` that hits these branches will spam stdout in production. These are not behind a build tag.

**ARCH-2 [MEDIUM]: `mapkeys` comment states wrong type ordering**

`builtins/maps.go` line 17 comment: "integers < floats < objects < errors < strings." Actual `types.CompareMapKeys` implements `INT < OBJ < FLOAT < ERR < STR`. The code is internally consistent (`compareJSONKeys` in `json.go` matches the implementation), but the comment will mislead anyone reading only `mapkeys`. Oracle verification needed to confirm which order Toast actually uses.

**ARCH-3 [MEDIUM]: `rmatch()` iterates the subject byte-by-byte, not rune-by-rune**

`builtins/strings.go` line 736: `for i := 0; i <= len(subject); i++`. For multi-byte UTF-8 subjects, this attempts to anchor the pattern at positions that split a codepoint, potentially producing garbled match positions or failing to find the rightmost match.

**ARCH-4 [MEDIUM]: `explode()` silently truncates a multi-byte UTF-8 delimiter to its first byte**

`builtins/strings.go` line 372: `delim = string([]byte{delimVal.Value()[0]})`. Any delimiter whose first Unicode code point requires more than one byte is broken silently.

**ARCH-5 [MEDIUM]: `frandom()` does not validate `min <= max`**

`builtins/math.go` line 608. Two-argument form does not check `min > max`. Toast raises `E_INVARG`. Barn silently computes `rand.Float64() * (max-min)` with a negative range.

**ARCH-6 [MEDIUM — SUSPECTED]: `parse_json` uses a 32-bit integer threshold**

`builtins/json.go` line 247: integers outside `[MinInt32, MaxInt32]` become floats. Toast may use 64-bit integer range. Round-tripping `{"n": 2147483648}` would give a float instead of an integer. Needs oracle verification.

**ARCH-7 [LOW]: `CheckListLimit`/`CheckMapLimit` called after delete operations**

`builtins/lists.go` line 121, `builtins/maps.go` line 119. A delete always produces a smaller value than the input. If the input passed the limit check, the result trivially does too. Dead check on every delete path.

**ARCH-8 [LOW]: `rand.Seed` is deprecated (Go 1.20+)**

`builtins/math.go` line 625, in `builtinReseedRandom`. The global random source is auto-seeded in Go 1.20+; explicit seeding via the deprecated API is a no-op on newer Go versions.

---

## Summary Table

| ID | Severity | Kind | One-line description |
|----|----------|------|----------------------|
| BUG-1 | CRITICAL | CONFIRMED | `abs(MinInt64)` overflows, returns negative value |
| BUG-2 | HIGH | CONFIRMED | `is_member()` uses case-sensitive comparison; MOO strings are case-insensitive |
| BUG-3 | HIGH | CONFIRMED | `unique()` uses `String()` key; case-sensitive dedup, wrong |
| BUG-4 | HIGH | CONFIRMED | `sort()` silently ignores `keys`/`natural`/`reverse` args |
| BUG-5 | HIGH | CONFIRMED | `pcre_match()` early-exits on empty subject, skipping valid matches |
| BUG-6 | HIGH | CONFIRMED | `capitalize()` uses deprecated `strings.Title`; apostrophe/Unicode bugs |
| ARCH-1 | HIGH | ARCHITECTURAL | Debug `fmt.Printf` left in production `builtinSlice` |
| ARCH-2 | MEDIUM | ARCHITECTURAL | `mapkeys` comment states wrong type ordering (INT<FLOAT<OBJ vs INT<OBJ<FLOAT) |
| ARCH-3 | MEDIUM | ARCHITECTURAL | `rmatch()` byte-level iteration breaks on multi-byte UTF-8 subjects |
| ARCH-4 | MEDIUM | ARCHITECTURAL | `explode()` truncates multi-byte UTF-8 delimiter to first byte |
| ARCH-5 | MEDIUM | ARCHITECTURAL | `frandom()` accepts `min > max` without error |
| ARCH-6 | MEDIUM | SUSPECTED | `parse_json` 32-bit int threshold; Toast may use 64-bit range |
| ARCH-7 | LOW | ARCHITECTURAL | Limit check after delete is dead code |
| ARCH-8 | LOW | ARCHITECTURAL | `rand.Seed` deprecated in Go 1.20+ |
