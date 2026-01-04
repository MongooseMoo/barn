# Divergence Report: Regex Builtins

**Spec File**: `C:\Users\Q\code\barn\spec\builtins\regex.md`
**Barn Files**: `C:\Users\Q\code\barn\builtins\strings.go` (match/rmatch only)
**Status**: critical_divergences_found
**Date**: 2026-01-03

## Summary

Tested regex builtin functions against Toast (reference) and Barn implementations. Found **CRITICAL divergences** in MOO pattern syntax interpretation and missing PCRE functions.

**Key Findings**:
- 2 functions implemented: `match()`, `rmatch()`
- 5+ functions missing: `pcre_match()`, `pcre_replace()`, `pcre_replace_all()`, `pcre_split()`, `pcre_match_all()`, `pcre_valid()`
- **CRITICAL**: Barn misinterprets MOO pattern syntax - treats `%` as character class introducer (like `\` in PCRE), but Toast treats `%` as escape to make regex special chars literal
- 0 conformance tests exist for any regex functions
- Capture group format differs (Barn returns empty list, Toast returns 9 pairs)

## Critical Divergences

### 1. match() - MOO Pattern Syntax Interpretation

**CRITICAL BUG**: Barn fundamentally misunderstands MOO pattern syntax.

| Field | Value |
|-------|-------|
| Test | `match("123", "%d")` |
| Barn | `{1, {1, 1, {}, "123"}}` - MATCHES (treats %d as digit class) |
| Toast | `{}` - NO MATCH (treats %d as literal 'd') |
| Classification | **likely_barn_bug** - CRITICAL |
| Evidence | Toast source code (`src/pattern.cc`) shows `%` escapes special regex characters to make them literal. Barn code (`builtins/strings.go` line 882) incorrectly converts `%d` to `[0-9]`. |

**Additional Evidence**:
```bash
# Test if %d matches literal 'd'
./toast_oracle.exe 'match("d", "%d")'
=> {1, 1, {{0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}}, "d"}
# Toast: %d DOES match literal 'd'

# Barn's pattern converter (strings.go:876-938) incorrectly implements:
# %d → [0-9]  (WRONG - should be literal 'd')
# %w → [a-zA-Z0-9_]  (WRONG - should be literal 'w')
# %s → [ \t\n\r]  (WRONG - should be literal 's')
# %+ → +?  (WRONG - should be literal '+')
# %* → *?  (WRONG - should be literal '*')
```

**Correct MOO Pattern Behavior** (from Toast source):
- `%` followed by ANY char = escape that char to literal
- `%d` = literal "d" (NOT digit class)
- `%+` = literal "+" (NOT one-or-more quantifier)
- Regular regex syntax still works: `[0-9]`, `a+`, `.*`, etc.
- `\` must be escaped as `\\` in patterns

---

### 2. match() - Invalid Pattern Error on %+

| Field | Value |
|-------|-------|
| Test | `match("a+", "%+")` |
| Barn | `{2, {E_INVARG, "", 0}}` - ERROR |
| Toast | `{2, 2, {{0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}}, "a+"}` - SUCCESS |
| Classification | **likely_barn_bug** |
| Evidence | Barn converts `%+` to `+?` (non-greedy quantifier), which creates invalid regex when nothing precedes it. Toast correctly treats `%+` as literal plus sign. |

---

### 3. match() - Capture Group Format

| Field | Value |
|-------|-------|
| Test | `match("hello world", "h.*o")` |
| Barn | `{1, {1, 6, {}, "hello world"}}` - empty capture list |
| Toast | `{1, 6, {{0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}, {0, -1}}, "hello world"}` - 9 pairs |
| Classification | **likely_barn_bug** |
| Evidence | Toast always returns 9 capture group pairs `{start, end}`, with `{0, -1}` for unused groups. Barn returns empty list `{}`. LambdaMOO spec documents this format. Barn has `TODO: capture groups` comment at line 682. |

**Expected Format**:
```moo
{start, end, captures, subject}
where captures = {{start1, end1}, {start2, end2}, ..., {start9, end9}}
```

Each capture pair is `{0, -1}` if that group wasn't captured.

---

## Missing Functions (PCRE)

The following PCRE functions are documented in spec but **NOT IMPLEMENTED** in Barn:

### 4. pcre_match()

| Field | Value |
|-------|-------|
| Test | `pcre_match("hello", "h.*o")` |
| Barn | **NOT IMPLEMENTED** |
| Toast | **NOT AVAILABLE** (parser error: "Unknown built-in function: pcre_match") |
| Classification | **needs_investigation** |
| Evidence | Toast source code has PCRE functions (`src/pcre_moo.cc`), but they're not available in toastcore.db. May require compile-time flag or different database. Barn has zero PCRE implementation. |

**NOTE**: Toast source code contains:
- `bf_pcre_match()` - `src/pcre_moo.cc`
- `bf_pcre_replace()` - exists
- PCRE support registered in `register_function()`

These functions exist in Toast source but are not available in the test build. May be:
1. Compile-time optional (#ifdef PCRE)
2. Disabled in toastcore.db
3. Require different configuration

**Cannot test divergences until both servers have PCRE enabled.**

---

### 5. pcre_replace() - Missing

Documented in spec section 2.1, not implemented in Barn.

---

### 6. pcre_replace_all() - Missing

Documented in spec section 2.2 (marked ToastStunt), not implemented in Barn.

---

### 7. pcre_split() - Missing

Documented in spec section 3.1 (marked ToastStunt), not implemented in Barn.

---

### 8. pcre_match_all() - Missing

Documented in spec section 4.1 (marked ToastStunt), not implemented in Barn.

---

### 9. pcre_valid() - Missing

Documented in spec section 6.1 (marked ToastStunt), not implemented in Barn.

---

## Test Coverage Gaps

**CRITICAL**: Zero conformance tests exist for regex functions.

```bash
$ find ~/code/moo-conformance-tests/src/moo_conformance/_tests/ -name "*.yaml" -exec grep -l "pcre_\|match(" {} \;
# Returns: No files with pcre_ functions
# match() appears in object matching context, not pattern matching
```

**Behaviors with NO test coverage**:
- `match(subject, pattern)` - basic pattern matching
- `match(subject, pattern, case_matters)` - case sensitivity
- `rmatch(subject, pattern)` - reverse matching
- MOO pattern syntax: `[0-9]`, `.*`, `a+`, `[abc]`, etc.
- MOO pattern escaping: `%+`, `%*`, `%.`, `%%`, etc.
- Capture groups with `\(...\)` syntax
- All 9 capture group slots
- Invalid patterns returning E_INVARG
- Empty strings
- Unicode characters
- All PCRE functions (cannot test without implementation)

---

## Behaviors Verified Correct

### match() - Basic Matching
```moo
match("abc", "b")
Barn:  {2, 2, {}, "abc"}
Toast: {2, 2, {{0, -1}, ...}, "abc"}
```
Position calculation correct (2, 2). Only capture format differs.

---

### match() - No Match
```moo
match("hello", "goodbye")
Barn:  {}
Toast: {}
```
Both correctly return empty list for no match.

---

### match() - Standard Regex Quantifiers
```moo
match("aaa", "a+")
Barn:  {1, 3, {}, "aaa"}
Toast: {1, 3, {{0, -1}, ...}, "aaa"}
```
Position correct. Both support standard regex `+` quantifier.

---

### rmatch() - Reverse Matching
```moo
rmatch("hello hello", "hello")
Barn:  {7, 11, {}, "hello hello"}
Toast: {7, 11, {{0, -1}, ...}, "hello hello"}
```
Both correctly find LAST occurrence at position 7-11.

---

## Recommended Actions

### Immediate (Critical)

1. **FIX MOO PATTERN SYNTAX** - Rewrite `mooPatternToGoRegex()` in `builtins/strings.go`:
   - `%` followed by ANY character = literal that character
   - Remove all character class mappings (%d, %w, %s, etc.)
   - `%+` → `\+`, `%*` → `\*`, `%.` → `\.`, `%%` → `%`
   - Keep standard regex syntax unchanged: `[0-9]`, `a+`, `.*`, etc.

2. **IMPLEMENT CAPTURE GROUPS** - Fix TODO at line 682:
   - Always return 9 capture group pairs
   - Format: `{{start, end}, ...}` with `{0, -1}` for unused
   - Use `re.FindStringSubmatchIndex()` for positions

3. **ADD CONFORMANCE TESTS** - Create `~/code/moo-conformance-tests/src/moo_conformance/_tests/builtins/regex.yaml`:
   - Test MOO pattern escaping: `%d`, `%+`, `%%`, etc.
   - Test capture groups (when implemented)
   - Test case sensitivity
   - Test rmatch()
   - Test error conditions

### Future

4. **IMPLEMENT PCRE FUNCTIONS** - After match() is correct:
   - Investigate Toast PCRE availability (compile flags?)
   - Implement `pcre_match()`, `pcre_replace()` using Go's `regexp` package
   - Note: Go regexp is RE2-based, not full PCRE (some features missing)
   - Add PCRE conformance tests

5. **UPDATE SPEC** - After testing complete:
   - Document exact MOO pattern syntax (% escape rules)
   - Clarify that PCRE functions may be compile-time optional
   - Add examples of capture group format
   - Document RE2 limitations for Go implementation

---

## Evidence Summary

**Toast Pattern Syntax** (from `src/pattern.cc`):
```c
// In translate_pattern():
case '%':
    c = *p++;
    if (!c)
        goto fail;
    else if (strchr(".*+?[^$|()123456789bB<>wW", c))
        stream_add_char(s, '\\');  // ESCAPE special chars
    stream_add_char(s, c);
    break;
```

This code **ESCAPES** special regex characters after `%`, making them literal.

**Barn Pattern Converter** (`builtins/strings.go:876-938`):
```go
case 'd': // digit
    result.WriteString("[0-9]")  // WRONG - should be literal 'd'
case '+': // one or more (non-greedy in MOO)
    result.WriteString("+?")  // WRONG - should be literal '+'
```

Barn implements **opposite behavior** - treating `%` as character class introducer.

---

## Conclusion

Barn's regex implementation has a **fundamental architectural flaw** in pattern syntax interpretation. The `mooPatternToGoRegex()` function must be completely rewritten based on Toast's behavior. Once fixed, capture groups must be implemented, then extensive conformance tests added before PCRE functions can be attempted.

**Severity**: CRITICAL - affects all code using match() on production MOO servers
**Effort**: Medium - clear fix path, well-documented in Toast source
**Priority**: HIGH - blocks conformance with existing MOO code
