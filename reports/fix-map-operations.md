# Fix Map Operations

**Date:** 2025-12-30
**Commit:** ebddef6

## Objective

Fix map operations to pass conformance tests:
- mapdelete with list keys
- Range operations on maps
- first() and last() on maps (skipped - requires call() builtin)

## Issues Found

### 1. Lists Not Allowed as Map Keys

**Problem:** `isValidMapKey()` only allowed scalar types (int, obj, str, err, float, bool). Lists were rejected with E_TYPE.

**Expected:** Lists should be valid map keys. Maps and waifs should still be rejected.

**Evidence:**
- Test `mapdelete_empty_list_key` expects success with empty list `{}`
- Test `mapdelete_list_values_key` expects E_RANGE (not E_TYPE) for missing list key `{1, 2}`
- Test descriptions explicitly state "Lists ARE valid map keys"

**Fix:** Modified `isValidMapKey()` in `builtins/maps.go` to accept `types.TYPE_LIST`.

### 2. mapdelete Behavior with Missing Empty List

**Problem:** `mapdelete()` returned E_RANGE for all missing keys, including empty lists.

**Expected:** Empty list keys should return the map unchanged (not E_RANGE), while non-empty lists and other types should still return E_RANGE.

**Evidence:**
- Test `mapdelete_empty_list_key`: `mapdelete([1 -> 2, 3 -> 4], {})` expects `{1: 2, 3: 4}` (success)
- Test `mapdelete_list_values_key`: `mapdelete([1 -> 2, 3 -> 4], {1, 2})` expects E_RANGE
- Test `mapdelete_missing_key`: `mapdelete([E_NONE -> "x"], E_ARGS)` expects E_RANGE

**Fix:** Added special case in `builtinMapdelete()` to return map unchanged when key is an empty list and not found.

### 3. Incorrect Inverted Range Detection for Maps

**Problem:** Map range assignment used `isInverted := startIdx > endIdx+1`, which meant:
- `x[2..1]` with `startIdx=2, endIdx=1`: `2 > 1+1 = false` (NOT inverted)
- This caused incorrect bounds checking

**Expected:** For maps, `startIdx > endIdx` should indicate an inverted range.

**Evidence:**
- Test `ranged_set_invalid_range_2`: `x = [1 -> 1]; x[2..1] = [...]` expects E_RANGE (startIdx 2 is out of bounds for map with 1 entry)
- Test `ranged_set_between_elements`: `x = [1 -> 1, 2 -> 2]; x[2..1] = [...]` expects success (inverted range between existing positions)

**Fix:** Changed inverted check from `startIdx > endIdx+1` to `startIdx > endIdx` for maps in `vm/indexing.go`.

### 4. Insufficient Bounds Checking for Inverted Map Ranges

**Problem:** Inverted ranges didn't validate that indices were within the map's valid positions.

**Expected:** For inverted ranges on maps:
- `startIdx` must be <= length (can't be beyond last position)
- `endIdx` must be >= 1 (can't be before first position)

**Evidence:**
- Test `ranged_set_invalid_range_3`: `x = [1 -> 1]; x[1..0] = [...]` expects E_RANGE
- Test `inverted_ranged_set_in_loop`: `x = []; x[1..0] = [...]` expects E_RANGE (can't use 1..0 on empty map)

**Fix:** Added additional bounds check for inverted map ranges: `if startIdx > length || endIdx < 1`.

## Changes

### builtins/maps.go

1. **isValidMapKey()**: Added `types.TYPE_LIST` to allowed key types
2. **builtinMapdelete()**: Added special case for empty list keys

### vm/indexing.go

1. **assignRange() - Map case**: Changed inverted detection from `startIdx > endIdx+1` to `startIdx > endIdx`
2. **assignRange() - Map case**: Simplified bounds checking and added inverted range validation

## Test Results

```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -k "map::" -v
```

**Result:** 80 passed, 1 skipped (first_last_index - requires call() builtin)

### Passing Tests

All mapdelete tests:
- ✅ mapdelete_removes_entry
- ✅ mapdelete_chain
- ✅ mapdelete_missing_key
- ✅ mapdelete_empty_list_key
- ✅ mapdelete_list_key
- ✅ mapdelete_map_key
- ✅ mapdelete_list_values_key

All range tests:
- ✅ ranged_set_invalid_range_1
- ✅ ranged_set_invalid_range_2
- ✅ ranged_set_invalid_range_3
- ✅ ranged_set_single_element
- ✅ ranged_set_between_elements
- ✅ ranged_set_merge_existing_key
- ✅ ranged_set_replace_range
- ✅ inverted_ranged_set_in_loop

### Skipped Tests

- ⏭️ first_last_index - Requires call() builtin for verb execution

## Notes

1. **Lists as Map Keys:** The underlying map implementation (`types/map.go`) already supported arbitrary values as keys via `keyHash()`. The restriction was only in the builtin validation layer.

2. **Empty List Special Case:** This is an unusual semantic - empty lists get different treatment than other values. The test description suggests this is intentional to allow empty lists as "placeholder" keys.

3. **Map Range Semantics:** Map ranges use positional indices (1, 2, 3...) not the actual key values. This is consistent with MOO's design where maps maintain insertion order.

4. **Reference Behavior:** Neither Toast nor cow_py currently implement lists as map keys - these are aspirational features defined by the test suite.

## Verification

```bash
# Build and start server
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9300 &

# Run map tests
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -k "map::" -v
```
