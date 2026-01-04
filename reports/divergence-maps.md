# Divergence Report: Maps

**Spec File**: `spec/builtins/maps.md`
**Barn Files**: `builtins/maps.go`
**Status**: divergences_found
**Date**: 2026-01-03

## Summary

Tested map builtins (mapkeys, mapvalues, maphaskey, mapdelete) and map syntax against both Toast (reference) and Barn. Found **8 critical divergences**:

1. **MAJOR:** Toast's map indexing is completely broken - returns entire map instead of value
2. **Key ordering differs** between Toast and Barn (ERR/FLOAT position swapped)
3. **Spec incorrectly documents mapdelete** behavior for missing keys
4. **Four "ToastStunt" builtins don't exist in Toast:** mapmerge, mapslice, mklist, mkmap
5. **`in` operator doesn't work** with maps in either implementation (spec claims it does)
6. **List keys not supported** (spec says they are)
7. **Map keys not supported** (spec says they are)

Core builtins (mapkeys, mapvalues, maphaskey) work correctly in both implementations, but multiple spec claims are false.

## Divergences

### 1. Map Indexing - Toast Returns Entire Map

| Field | Value |
|-------|-------|
| Test | `m = ["a" -> 1]; m["a"]` |
| Barn | `1` |
| Toast | `["a" -> 1]` (returns entire map!) |
| Classification | likely_toast_bug |
| Evidence | Toast's map access operator is completely broken. Instead of returning the value (1), it returns the entire map unchanged. Barn correctly returns the value. |

**Second test:**

| Field | Value |
|-------|-------|
| Test | `m = ["a" -> 1]; m["b"]` |
| Barn | `E_RANGE` |
| Toast | `["a" -> 1]` (returns entire map!) |
| Classification | likely_toast_bug |
| Evidence | Toast returns the map unchanged even for missing keys, instead of raising E_RANGE. Barn correctly raises E_RANGE. |

### 2. Key Ordering Divergence

| Field | Value |
|-------|-------|
| Test | `mapkeys([1 -> "one", "a" -> "letter", #0 -> "obj", E_NONE -> "err", 2.5 -> "float"])` |
| Barn | `{1, #0, 2.5, E_NONE, "a"}` |
| Toast | `{1, #0, E_NONE, 2.5, "a"}` |
| Classification | likely_barn_bug |
| Evidence | Key ordering differs. Toast: INT < OBJ < ERR < FLOAT < STR. Barn: INT < OBJ < FLOAT < ERR < STR. Toast is the reference implementation. Barn's implementation in `compareMapKeys()` has FLOAT (2) and ERR (3) in wrong order. |

### 3. mapdelete Missing Key Behavior

| Field | Value |
|-------|-------|
| Test | `mapdelete(["a" -> 1], "x")` |
| Barn | `E_RANGE` |
| Toast | `E_RANGE` |
| Classification | likely_spec_gap |
| Evidence | Spec (line 103-112) claims mapdelete should return the original map unchanged if key doesn't exist. BOTH implementations return E_RANGE. Conformance test `mapdelete_missing_key` expects E_RANGE. The spec is wrong, not the implementations. |

### 4. mapmerge Doesn't Exist in Toast

| Field | Value |
|-------|-------|
| Test | `mapmerge(["a" -> 1], ["b" -> 2])` |
| Barn | `["a" -> 1, "b" -> 2]` |
| Toast | Parse error: "Unknown built-in function: mapmerge" |
| Classification | likely_barn_bug |
| Evidence | Spec marks this as "(ToastStunt)" but Toast doesn't have it. Barn implemented a builtin that doesn't exist in the reference. |

### 5. mapslice Doesn't Exist in Toast

| Field | Value |
|-------|-------|
| Test | `mapslice(["a" -> 1, "b" -> 2, "c" -> 3], {"a", "c"})` |
| Barn | (not tested) |
| Toast | Parse error: "Unknown built-in function: mapslice" |
| Classification | likely_spec_gap |
| Evidence | Spec documents mapslice as "(ToastStunt)" but it doesn't exist. |

### 6. mklist Doesn't Exist in Toast

| Field | Value |
|-------|-------|
| Test | `mklist(["a" -> 1, "b" -> 2])` |
| Barn | (not tested) |
| Toast | Parse error: "Unknown built-in function: mklist" |
| Classification | likely_spec_gap |
| Evidence | Spec documents mklist as "(ToastStunt)" but it doesn't exist. |

### 7. mkmap Doesn't Exist in Toast

| Field | Value |
|-------|-------|
| Test | `mkmap({{"a", 1}, {"b", 2}})` |
| Barn | (not tested) |
| Toast | Parse error: "Unknown built-in function: mkmap" |
| Classification | likely_spec_gap |
| Evidence | Spec documents mkmap as "(ToastStunt)" but it doesn't exist. |

### 8. `in` Operator Doesn't Work With Maps

| Field | Value |
|-------|-------|
| Test | `"a" in ["a" -> 1, "b" -> 2]` |
| Barn | `0` |
| Toast | `0` |
| Classification | likely_spec_gap |
| Evidence | Spec section 9 claims `in` operator tests for key presence in maps. BOTH implementations return 0 (false) even when key exists. The `in` operator works with lists but not maps. Spec is wrong. |

**Verification test:**

| Field | Value |
|-------|-------|
| Test | `"a" in {"a", "b", "c"}` (list) |
| Barn | `1` |
| Toast | `1` |
| Classification | - |
| Evidence | `in` operator works correctly with lists, confirming it's specifically broken for maps. |

### 9. List Keys Not Supported

| Field | Value |
|-------|-------|
| Test | `[{1, 2} -> "pair"]` |
| Barn | `E_TYPE` |
| Toast | `E_TYPE` (Type mismatch: expected string, integer, object, error, float, anonymous object, waif or bool; got list) |
| Classification | likely_spec_gap |
| Evidence | Spec section 6 claims "LIST | Yes | By value" in the key types table. BOTH implementations reject list keys with E_TYPE. The spec is wrong. |

### 10. Map Keys Not Supported

| Field | Value |
|-------|-------|
| Test | `[["inner" -> 1] -> "outer"]` |
| Barn | `E_TYPE` |
| Toast | `E_TYPE` (Type mismatch: expected string, integer, object, error, float, anonymous object, waif or bool; got map) |
| Classification | likely_spec_gap |
| Evidence | Spec section 6 claims "MAP | Yes | By value" in the key types table. BOTH implementations reject map keys with E_TYPE. The spec is wrong. |

## Test Coverage Gaps

Behaviors documented in spec but NOT covered by conformance tests:

- `in` operator with maps (spec claims it works, but it doesn't)
- Map indexing syntax `m[key]` returning values (Toast's implementation is broken)
- Map assignment syntax `m[key] = value`
- List keys (spec claims they're valid, implementations reject them)
- Map keys (spec claims they're valid, implementations reject them)
- `mapslice()` - marked as ToastStunt but doesn't exist
- `mklist()` - marked as ToastStunt but doesn't exist
- `mkmap()` - marked as ToastStunt but doesn't exist
- `mapmerge()` - marked as ToastStunt but doesn't exist in Toast (but Barn has it!)
- Map iteration with `for` loops (spec section 5)
- Float key equality and NaN behavior (spec section 6.1)
- Composite key equality (spec section 6.2)

## Behaviors Verified Correct

### Basic Operations (Match)
- `[]` - Empty map creation works in both
- `["a" -> 1]` - Single entry map creation works in both
- `mapkeys(["a" -> 1, "b" -> 2])` → `{"a", "b"}` - Correct in both
- `mapvalues(["a" -> 1, "b" -> 2])` → `{1, 2}` - Correct in both
- `mapkeys([])` → `{}` - Empty map handling correct in both
- `maphaskey(["a" -> 1], "a")` → `1` - Correct in both
- `maphaskey(["a" -> 1], "b")` → `0` - Correct in both
- `mapdelete(["a" -> 1, "b" -> 2], "a")` → `["b" -> 2]` - Correct in both
- `mapdelete(["a" -> 1], "x")` → `E_RANGE` - Both return E_RANGE (spec is wrong)

### Type Errors (Match)
- `mapkeys("not a map")` → `E_TYPE` - Correct in both

## Critical Issues Summary

**For Barn:**
1. Fix key ordering: ERR should come before FLOAT (line 49-66 in `builtins/maps.go`)
2. Remove `mapmerge()` builtin (doesn't exist in Toast)
3. Map indexing works correctly (better than Toast!)

**For Spec:**
1. Section 2.3: Remove claim that mapdelete returns map unchanged for missing keys (it returns E_RANGE)
2. Section 3.1-3.2: Mark mapmerge and mapslice as "NOT IN TOAST" or remove them
3. Section 4.1-4.2: Mark mklist and mkmap as "NOT IN TOAST" or remove them
4. Section 6: Remove LIST and MAP from key types table (not supported)
5. Section 9: Remove or mark as broken - `in` operator doesn't work with maps
6. Section 1.2: Document that Toast's map indexing is broken (returns entire map)

**For Toast:**
1. Map indexing operator `m[key]` is completely broken (returns entire map instead of value)
2. `in` operator doesn't work with maps
3. Many builtins marked as "ToastStunt" don't actually exist

## Conformance Tests Status

Checked conformance tests in `~/code/moo-conformance-tests/src/moo_conformance/_tests/builtins/map.yaml`:

- `mapkeys_sorted` - EXISTS (tests sorting)
- `mapvalues_sorted` - EXISTS (tests sorting)
- `mapdelete_removes_entry` - EXISTS
- `mapdelete_chain` - EXISTS
- `mapdelete_missing_key` - EXISTS (expects E_RANGE, confirms spec is wrong)
- `mapdelete_empty_list_key` - EXISTS (tests E_RANGE behavior)
- `mapdelete_list_key` - EXISTS (tests list key rejection)
- `mapdelete_map_key` - EXISTS (tests map key rejection)
- `maphaskey` - Found in gc.yaml tests

No conformance tests found for:
- Map indexing `m[key]`
- `in` operator with maps
- `mapmerge`, `mapslice`, `mklist`, `mkmap`
- Map iteration
- Key ordering specifics
