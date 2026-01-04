# Divergence Report: List Builtins

**Spec File**: `spec/builtins/lists.md`
**Barn Files**: `builtins/lists.go`
**Status**: clean
**Date**: 2026-01-03

## Summary

Tested 100+ behaviors across all core list builtins and ToastStunt extensions. **No divergences found** between Barn and Toast implementations. All implemented builtins produce identical outputs for:
- Basic operations (length, listappend, listinsert, listdelete, listset, setadd, setremove)
- Search operations (is_member, indexing, slicing)
- Transformations (reverse, sort, unique)
- Edge cases (empty lists, out-of-bounds indices, type mismatches)
- Deep equality comparisons for nested lists

**Key findings:**
1. All core list builtins match Toast reference behavior
2. ToastStunt extensions (intersection, union, diff, sum, avg, etc.) are **not implemented as builtins** in either server - they return E_VERBNF
3. Type strictness is correctly enforced (INT != FLOAT in comparisons)
4. Index clamping behavior matches spec for listinsert
5. Error handling is consistent across all edge cases

## Divergences

**None found.** All 100+ tests passed with exact output match.

## Test Coverage Gaps

The following behaviors are documented in spec but have **limited or no conformance test coverage**:

### 1. Index Clamping Edge Cases
- `listinsert({1, 2}, 0, -100)` - extreme negative clamping
- `listinsert({1, 2}, 3, 1000)` - extreme positive clamping
- **Evidence**: Tested manually, both servers clamp correctly, but no conformance test

### 2. Deep Equality in Nested Structures
- `setremove({{1, 2}, {3, 4}}, {1, 2})` - nested list removal
- `is_member({1, 2}, {{1, 2}, {3}})` - nested list membership
- **Evidence**: Tested manually, both implement deep equality correctly

### 3. Type Strictness Boundaries
- `is_member(2, {1, 2.0, 3})` => 0 (INT != FLOAT)
- `is_member(2.0, {1, 2, 3})` => 0 (FLOAT != INT)
- `setremove({1, 2, 3}, "2")` => {1, 2, 3} (no match)
- **Evidence**: Both servers enforce type-strict equality

### 4. Slice Operator Edge Cases
- `{1, @{}, 2}` => {1, 2} (empty splice)
- `{@{@{1, 2}, @{3, 4}}}` => {1, 2, 3, 4} (nested splice)
- **Evidence**: Works correctly, minimal test coverage

### 5. Range Indexing Boundaries
- `{1, 2, 3}[3..1]` => {} (backwards range)
- `{1, 2, 3}[1..-1]` => {} (negative end)
- `{1, 2, 3}[1..10]` => E_RANGE (out of bounds)
- **Evidence**: Correctly implemented, limited conformance tests

### 6. Sort with Non-String/Int Types
- `sort({#3, #1, #2})` => {#1, #2, #3} (object sorting)
- `sort({E_INVARG, E_TYPE, E_ARGS})` => {E_TYPE, E_ARGS, E_INVARG} (error sorting)
- **Evidence**: Sorting works by type code then value

### 7. setadd Duplicate Behavior
- `setadd({1, 2, 2}, 2)` => {1, 2, 2} (checks existence, doesn't dedupe list)
- **Evidence**: Only checks if value exists, doesn't remove existing duplicates

### 8. listappend vs listinsert Semantics
- `listappend({1, 2}, 3, 1)` => {1, 3, 2} (inserts AFTER index 1)
- `listinsert({1, 2}, 3, 2)` => {1, 3, 2} (inserts BEFORE index 2)
- **Evidence**: Correctly implemented but subtle difference needs more test coverage

### 9. ToastStunt Extensions as Verbs
All these return E_VERBNF in both servers (not implemented as builtins):
- Set operations: `intersection()`, `union()`, `diff()`
- Aggregation: `sum()`, `avg()`, `product()`
- Searching: `assoc()`, `rassoc()`, `iassoc()`
- Utility: `make_list()`, `flatten()`, `count()`
- Transform: `rotate()`, `slice()`, `indexc()`
- **Evidence**: Tested 43 extension functions, all return E_VERBNF
- **Spec gap**: Spec documents these as builtins but they're implemented as MOO verbs

## Behaviors Verified Correct

### Basic Operations (10 behaviors)
- ✓ `length({})` => 0
- ✓ `length({1, 2, 3})` => 3
- ✓ `length({{1}, {2}})` => 2 (nested counts outer elements)
- ✓ `length(42)` => E_TYPE
- ✓ `listappend({1, 2}, 3)` => {1, 2, 3}
- ✓ `listappend({1, 2}, 0, 0)` => {0, 1, 2}
- ✓ `listappend({1, 3}, 2, 1)` => {1, 2, 3}
- ✓ `listappend({}, 1)` => {1}
- ✓ `listappend({1, 2}, 3, -1)` => E_RANGE
- ✓ `listappend({1, 2}, 3, 5)` => E_RANGE

### listinsert Operations (7 behaviors)
- ✓ `listinsert({2, 3}, 1)` => {1, 2, 3} (default: prepend)
- ✓ `listinsert({1, 3}, 2, 2)` => {1, 2, 3}
- ✓ `listinsert({1, 2}, 3, 4)` => {1, 2, 3} (clamped to len+1)
- ✓ `listinsert({1, 2}, 0, 0)` => {0, 1, 2} (clamped to 1)
- ✓ `listinsert({1, 2}, 0, -1)` => {0, 1, 2} (clamped to 1)
- ✓ `listinsert({1, 2}, 3, 100)` => {1, 2, 3} (clamped to len+1)
- ✓ No E_RANGE errors (all out-of-bounds clamped)

### listdelete Operations (8 behaviors)
- ✓ `listdelete({1, 2, 3}, 2)` => {1, 3}
- ✓ `listdelete({1, 2, 3}, 1)` => {2, 3}
- ✓ `listdelete({1, 2, 3}, 3)` => {1, 2}
- ✓ `listdelete({1}, 1)` => {}
- ✓ `listdelete({}, 1)` => E_RANGE
- ✓ `listdelete({1, 2}, 5)` => E_RANGE
- ✓ `listdelete({1, 2}, 0)` => E_RANGE
- ✓ `listdelete({1, 2}, -1)` => E_RANGE

### listset Operations (6 behaviors)
- ✓ `listset({1, 2, 3}, 9, 2)` => {1, 9, 3}
- ✓ `listset({1, 2, 3}, 9, 1)` => {1, 9, 3}
- ✓ `listset({1, 2, 3}, 9, 3)` => {1, 2, 9}
- ✓ `listset({1, 2}, 9, 5)` => E_RANGE
- ✓ `listset({1, 2}, 9, 0)` => E_RANGE
- ✓ `listset({1, 2}, 9, -1)` => E_RANGE

### Set Operations (10 behaviors)
- ✓ `setadd({1, 2, 3}, 4)` => {1, 2, 3, 4}
- ✓ `setadd({1, 2, 3}, 2)` => {1, 2, 3}
- ✓ `setadd({}, 1)` => {1}
- ✓ `setadd({{1, 2}}, {3, 4})` => {{1, 2}, {3, 4}}
- ✓ `setadd({{1, 2}, {3, 4}}, {1, 2})` => {{1, 2}, {3, 4}} (deep equality)
- ✓ `setremove({1, 2, 3}, 2)` => {1, 3}
- ✓ `setremove({1, 2, 2, 3}, 2)` => {1, 2, 3} (first occurrence only)
- ✓ `setremove({1, 2, 3}, 4)` => {1, 2, 3}
- ✓ `setremove({{1, 2}, {3, 4}}, {1, 2})` => {{3, 4}} (deep equality)
- ✓ `setremove({}, 1)` => {}

### Search Operations (6 behaviors)
- ✓ `is_member(2, {1, 2, 3})` => 2
- ✓ `is_member(4, {1, 2, 3})` => 0
- ✓ `is_member(1, {1, 2, 3})` => 1
- ✓ `is_member(3, {1, 2, 3})` => 3
- ✓ `is_member(1, {})` => 0
- ✓ `is_member({1, 2}, {{1, 2}, {3}})` => 1 (deep equality)

### Indexing Operations (9 behaviors)
- ✓ `{1, 2, 3}[1]` => 1
- ✓ `{1, 2, 3}[2]` => 2
- ✓ `{1, 2, 3}[3]` => 3
- ✓ `{1, 2, 3}[0]` => E_RANGE
- ✓ `{1, 2, 3}[-1]` => E_RANGE
- ✓ `{1, 2, 3}[5]` => E_RANGE
- ✓ `{1, 2, 3}[1..2]` => {1, 2}
- ✓ `{1, 2, 3}[1..3]` => {1, 2, 3}
- ✓ `{1, 2, 3}[2..$]` => {2, 3}

### Range Slicing (5 behaviors)
- ✓ `{1, 2, 3}[3..1]` => {} (backwards)
- ✓ `{1, 2, 3}[2..2]` => {2} (single element)
- ✓ `{1, 2, 3}[1..-1]` => {} (negative end)
- ✓ `{1, 2, 3}[1..10]` => E_RANGE (end out of bounds)
- ✓ `{}[1..0]` => {} (empty range on empty list)

### Transformations (9 behaviors)
- ✓ `reverse({1, 2, 3})` => {3, 2, 1}
- ✓ `reverse({})` => {}
- ✓ `reverse({1})` => {1}
- ✓ `reverse({{1, 2}, {3, 4}})` => {{3, 4}, {1, 2}}
- ✓ `sort({3, 1, 2})` => {1, 2, 3}
- ✓ `sort({"c", "a", "b"})` => {"a", "b", "c"}
- ✓ `sort({})` => {}
- ✓ `sort({3, "a", 1, "b"})` => {1, 3, "a", "b"} (by type then value)
- ✓ `sort({#3, #1, #2})` => {#1, #2, #3}

### Unique (4 behaviors)
- ✓ `unique({1, 2, 2, 3, 1})` => {1, 2, 3}
- ✓ `unique({1, 2, 3})` => {1, 2, 3}
- ✓ `unique({})` => {}
- ✓ `unique({{1}, {1}, {2}})` => {{1}, {2}} (deep equality)

### Splice Operator (6 behaviors)
- ✓ `{@{1, 2}, @{3, 4}}` => {1, 2, 3, 4}
- ✓ `{0, @{1, 2}, 5}` => {0, 1, 2, 5}
- ✓ `{@{}, @{}}` => {}
- ✓ `{1, @{2, 3}, 4}` => {1, 2, 3, 4}
- ✓ `{@{@{1, 2}, @{3, 4}}}` => {1, 2, 3, 4} (nested)
- ✓ `{1, @{}, 2}` => {1, 2} (empty splice)

### Type Strictness (4 behaviors)
- ✓ `is_member(2, {1, 2.0, 3})` => 0 (INT != FLOAT)
- ✓ `is_member(2.0, {1, 2, 3})` => 0 (FLOAT != INT)
- ✓ `setremove({1, 2, 3}, "2")` => {1, 2, 3} (INT != STR)
- ✓ All equality operations use type-strict comparison

## Conformance Test Coverage

Found 66 test references to list builtins in conformance suite:
- Basic operations: Well covered (listappend, listinsert, listdelete, listset, setadd)
- Search operations: Well covered (is_member, indexing)
- Transformations: Moderate coverage (sort, reverse, unique)
- Edge cases: Limited coverage (clamping, type mismatches, nested equality)

### Tests in `basic/list.yaml`:
```yaml
tests:
  - length_of_list
  - listappend_to_end
  - listappend_at_position
  - listinsert_at_position_2
  - listinsert_at_position_1
  - listdelete_at_position_2
  - listset_at_position_2
  - setadd_new_element
  - setadd_existing_element
```

### Tests in `builtins/collection_improvements.yaml`:
- Extensive copy-on-write testing for nested lists and maps
- Reference sharing behavior
- Nested collection assignment

## Implementation Notes

### Barn Implementation Quality
The Barn implementation in `builtins/lists.go` is **correct and complete** for all tested behaviors:

1. **Index conversion**: Properly handles 1-based MOO indexing
2. **Clamping**: listinsert correctly clamps out-of-bounds indices
3. **Error handling**: Returns E_RANGE/E_TYPE appropriately
4. **Deep equality**: Uses Value.Equal() for nested structure comparison
5. **Type strictness**: Enforces INT != FLOAT distinction
6. **Copy-on-write**: All operations return new lists

### Spec Accuracy
The spec in `spec/builtins/lists.md` is **accurate** for implemented builtins:
- Index semantics correctly documented
- Clamping behavior matches implementation
- Error conditions properly specified
- Examples match actual behavior

### Spec Gap: ToastStunt Extensions
The spec documents 43 ToastStunt extension functions as builtins, but testing reveals they return E_VERBNF in both Toast and Barn:

**Not implemented as builtins:**
- Set operations: intersection, union, diff
- Aggregation: sum, avg, product
- Searching: assoc, rassoc, iassoc
- Utility: make_list, flatten, count
- Transform: rotate, slice, indexc

**Recommendation**: Either:
1. Mark these as "verb implementations" in spec
2. Implement them as builtins in both servers
3. Move them to a separate "verb library" spec section

## Test Expressions Used

### Basic Operations
```moo
length({})                          => 0
length({1, 2, 3})                   => 3
length({{1}, {2}})                  => 2
length(42)                          => E_TYPE
listappend({1, 2}, 3)               => {1, 2, 3}
listappend({1, 2}, 0, 0)            => {0, 1, 2}
listappend({1, 3}, 2, 1)            => {1, 2, 3}
listappend({1, 2}, 3, -1)           => E_RANGE
listinsert({2, 3}, 1)               => {1, 2, 3}
listinsert({1, 2}, 3, 100)          => {1, 2, 3}
listdelete({1, 2, 3}, 2)            => {1, 3}
listdelete({}, 1)                   => E_RANGE
listset({1, 2, 3}, 9, 2)            => {1, 9, 3}
listset({1, 2}, 9, 0)               => E_RANGE
```

### Set Operations
```moo
setadd({1, 2, 3}, 4)                => {1, 2, 3, 4}
setadd({1, 2, 3}, 2)                => {1, 2, 3}
setadd({{1, 2}}, {1, 2})            => {{1, 2}}
setremove({1, 2, 3}, 2)             => {1, 3}
setremove({1, 2, 2, 3}, 2)          => {1, 2, 3}
setremove({{1, 2}, {3}}, {1, 2})    => {{3}}
```

### Search and Indexing
```moo
is_member(2, {1, 2, 3})             => 2
is_member(4, {1, 2, 3})             => 0
is_member({1, 2}, {{1, 2}, {3}})    => 1
{1, 2, 3}[1]                        => 1
{1, 2, 3}[0]                        => E_RANGE
{1, 2, 3}[1..2]                     => {1, 2}
{1, 2, 3}[2..$]                     => {2, 3}
{1, 2, 3}[3..1]                     => {}
{1, 2, 3}[1..10]                    => E_RANGE
```

### Transformations
```moo
reverse({1, 2, 3})                  => {3, 2, 1}
reverse({})                         => {}
sort({3, 1, 2})                     => {1, 2, 3}
sort({"c", "a", "b"})               => {"a", "b", "c"}
sort({3, "a", 1, "b"})              => {1, 3, "a", "b"}
sort({#3, #1, #2})                  => {#1, #2, #3}
unique({1, 2, 2, 3, 1})             => {1, 2, 3}
unique({{1}, {1}, {2}})             => {{1}, {2}}
```

### Splice Operator
```moo
{@{1, 2}, @{3, 4}}                  => {1, 2, 3, 4}
{0, @{1, 2}, 5}                     => {0, 1, 2, 5}
{1, @{}, 2}                         => {1, 2}
{@{@{1, 2}, @{3, 4}}}               => {1, 2, 3, 4}
```

### Type Strictness
```moo
is_member(2, {1, 2.0, 3})           => 0
is_member(2.0, {1, 2, 3})           => 0
setremove({1, 2, 3}, "2")           => {1, 2, 3}
```

## Conclusion

Barn's list builtin implementation is **fully conformant** with Toast reference implementation. All 100+ tested behaviors produce identical outputs. No bugs or divergences detected.

**Key strengths:**
- Correct 1-based indexing throughout
- Proper index clamping for listinsert
- Type-strict equality comparisons
- Deep equality for nested structures
- Appropriate error handling

**Spec improvement opportunities:**
1. Clarify that ToastStunt extensions are verb implementations, not builtins
2. Add more conformance tests for edge cases (clamping, type mismatches, nested equality)
3. Document setadd behavior: checks existence but doesn't deduplicate existing list
4. Add examples showing listappend vs listinsert semantic difference

**No code changes needed.** Implementation is correct.
