# Blind Implementor Audit: Lists Feature

## Summary
Audited list operations focusing on 1-based indexing, slicing semantics, copy-on-write behavior, mutation during iteration, and nested list handling. Found 23 specification gaps requiring clarification.

---

## Gaps Found

```yaml
- id: GAP-001
  feature: "list indexing"
  spec_file: "spec/types.md"
  spec_section: "5.1 1-Based Indexing"
  gap_type: guess
  question: |
    What happens with list[0]? The spec shows it raises E_RANGE, but doesn't
    explicitly state that index 0 is invalid. Does 0 always raise E_RANGE, or
    is there any special behavior?
  impact: |
    An implementor might: (a) treat 0 specially, (b) allow 0 as an alias for
    "insert before first", (c) reject it. The example shows E_RANGE but this
    should be stated as a rule.
  suggested_addition: |
    Add to types.md section 5.1: "Index 0 is always invalid and raises E_RANGE.
    Valid indices are 1 through length(list)."

- id: GAP-002
  feature: "list indexing"
  spec_file: "spec/types.md"
  spec_section: "5.1 1-Based Indexing"
  gap_type: guess
  question: |
    What happens with negative indices? The spec shows list[-1] would raise
    E_RANGE but doesn't state whether: (a) all negative indices are invalid,
    (b) negative indices count from end (Python-style), (c) some negative
    values have special meaning.
  impact: |
    Python users will expect list[-1] to mean "last element". MOO differs but
    this must be explicit. An implementor might accidentally implement Python
    semantics.
  suggested_addition: |
    Add to types.md section 5.1: "Negative indices are not supported and
    always raise E_RANGE. Use list[$] to access the last element."

- id: GAP-003
  feature: "list slicing"
  spec_file: "spec/types.md"
  spec_section: "5.2 Range Indexing"
  gap_type: test
  question: |
    For list[2..4], are the bounds checked before or after evaluating the range?
    What if start > length? What if end > length? Are they clamped or is it an
    error?
  impact: |
    Affects edge case behavior. list[2..100] on a 5-element list could:
    (a) raise E_RANGE, (b) clamp to list[2..5], (c) pad with zeros to index 100.
  suggested_addition: |
    Add to types.md section 5.2: "For range indexing list[start..end]:
    - If start < 1 or start > length(list), raise E_RANGE
    - If end < 1 or end > length(list), raise E_RANGE
    - If start > end, return empty list {}
    - Both bounds are checked; no clamping occurs"

- id: GAP-004
  feature: "list slicing"
  spec_file: "spec/types.md"
  spec_section: "5.2 Range Indexing"
  gap_type: ask
  question: |
    What is the result of list[3..1]? The spec says ranges are "inclusive on
    both ends" but doesn't specify behavior when start > end. Does this:
    (a) raise E_RANGE, (b) return empty list, (c) reverse the slice?
  impact: |
    Different semantics change behavior. Some languages reverse, some return
    empty, some error. Must be explicit.
  suggested_addition: |
    Add to types.md section 5.2: "If start > end in list[start..end], returns
    empty list {}. Example: list[3..1] => {}"

- id: GAP-005
  feature: "list slicing"
  spec_file: "spec/types.md"
  spec_section: "5.2 Range Indexing"
  gap_type: assume
  question: |
    The spec shows list[^..$] returns the entire list. But what does list[2..$]
    return when the list has only 1 element? Does $ evaluate to the length first,
    then the range is checked, or is this a special case?
  impact: |
    If $ is evaluated as length=1, then list[2..1] would be start > end.
    Need to know if this raises E_RANGE or returns {}.
  suggested_addition: |
    Clarify in types.md section 5.3: "Special markers ^ and $ are substituted
    before range bounds checking. $ always equals length(list). If substitution
    results in start > end, return empty list {}."

- id: GAP-006
  feature: "list slicing"
  spec_file: "spec/types.md"
  spec_section: "5.2 Range Indexing"
  gap_type: test
  question: |
    Is list[1..1] valid? The example shows it returns {1} but this should be
    stated explicitly. Single-element ranges could be treated specially.
  impact: |
    Edge case that should be documented. Implementor might special-case this.
  suggested_addition: |
    Add example to types.md section 5.2: "Single-element ranges are valid:
    list[3..3] returns a list containing only the element at index 3."

- id: GAP-007
  feature: "copy-on-write"
  spec_file: "spec/types.md"
  spec_section: "5.4 Copy-on-Write"
  gap_type: ask
  question: |
    When does the copy happen? The example shows b[1] = 99 triggers a copy.
    But what about b = {@a, 5}? Does splicing a list into a literal trigger
    COW, or is the new list always independent?
  impact: |
    Performance and memory semantics. Implementor needs to know when to check
    refcount and when to always copy.
  suggested_addition: |
    Add to types.md section 5.4: "Copy-on-write triggers on mutation operations:
    indexed assignment (list[i] = x), range assignment (list[i..j] = x), and
    list modification builtins (listappend, listset, etc.). List literals and
    splice operations always create new lists."

- id: GAP-008
  feature: "copy-on-write"
  spec_file: "spec/types.md"
  spec_section: "5.4 Copy-on-Write"
  gap_type: guess
  question: |
    For nested lists, is COW shallow or deep? If a = {{1, 2}, {3, 4}}, b = a,
    then b[1][1] = 99, what happens? Does the inner list copy? Does the outer
    list copy? Both?
  impact: |
    Critical for correctness. Shallow COW means b[1][1] = 99 modifies a's inner
    list. Deep COW means full independence. Most likely shallow but must specify.
  suggested_addition: |
    Add to types.md section 5.4: "Copy-on-write is shallow. When a list is
    copied due to mutation, only the top-level list structure is duplicated.
    Nested lists continue to share references until individually mutated.
    Example: a = {{1, 2}}; b = a; b[1][1] = 99; // a[1] and b[1] now differ,
    but initially shared."

- id: GAP-009
  feature: "list mutation"
  spec_file: "spec/builtins/lists.md"
  spec_section: "1.2 listappend"
  gap_type: ask
  question: |
    The signature says `listappend(list, value [, index])` with index meaning
    "position after which to insert". What happens with index=0? The spec says
    E_RANGE for "out of bounds" but doesn't clarify if 0 is valid meaning
    "insert at beginning".
  impact: |
    The example shows listappend({1,2}, 0, 0) => {0,1,2}, which suggests index=0
    means "before first element". But the description says "after which" which
    would make index=0 confusing. This is contradictory.
  suggested_addition: |
    Fix builtins/lists.md section 1.2: Change "position after which to insert"
    to "position before which to insert" OR clarify that index=0 is special-cased
    to mean "insert at beginning". Update description to match examples.

- id: GAP-010
  feature: "list mutation"
  spec_file: "spec/builtins/lists.md"
  spec_section: "1.2 listappend"
  gap_type: test
  question: |
    What is the valid range for the index parameter? The spec says E_RANGE for
    "out of bounds" but doesn't define bounds. Is valid range [0..length(list)]
    or [1..length(list)] or something else?
  impact: |
    Without bounds definition, implementor must guess. Examples suggest
    [0..length(list)] but this should be stated.
  suggested_addition: |
    Add to builtins/lists.md section 1.2: "Valid index range is 0 to length(list).
    Index 0 inserts at beginning. Index length(list) appends to end."

- id: GAP-011
  feature: "list mutation"
  spec_file: "spec/builtins/lists.md"
  spec_section: "1.3 listinsert"
  gap_type: ask
  question: |
    The description says "inserts before index" with default index=1. But what
    happens with listinsert({1,2}, 3, 4)? The example shows it returns {1,2,3}
    (at end), which suggests index=4 is valid even though length=2. Is index=length+1
    allowed?
  impact: |
    Contradicts usual bounds checking. Either the example is wrong or the bounds
    are [1..length+1] not [1..length]. Must clarify.
  suggested_addition: |
    Add to builtins/lists.md section 1.3: "Valid index range is 1 to length(list)+1.
    Index length(list)+1 appends to end. Indices > length(list)+1 raise E_RANGE."

- id: GAP-012
  feature: "list mutation"
  spec_file: "spec/builtins/lists.md"
  spec_section: "1.4 listdelete"
  gap_type: test
  question: |
    What is the behavior of listdelete on an empty list? Does listdelete({}, 1)
    raise E_RANGE (no index 1) or is there special handling?
  impact: |
    Edge case that should be documented explicitly.
  suggested_addition: |
    Add example to builtins/lists.md section 1.4: "listdelete({}, 1) => E_RANGE"

- id: GAP-013
  feature: "list mutation"
  spec_file: "spec/builtins/lists.md"
  spec_section: "1.7 setremove"
  gap_type: guess
  question: |
    The description says "removes first occurrence of value". But what equality
    semantics are used? The note on is_member says "uses value equality, not
    identity", but what does "value equality" mean for nested lists or floats?
  impact: |
    For floats: does 0.1+0.2 == 0.3 (bitwise or epsilon)? For lists: does
    {1, {2, 3}} compare deeply? Must reference equality semantics.
  suggested_addition: |
    Add to builtins/lists.md section 1.7: "Uses == operator for comparison.
    See operators.md section 9.1 for equality semantics (type-strict, deep
    comparison for collections, bitwise for floats)."

- id: GAP-014
  feature: "list slicing"
  spec_file: "spec/builtins/lists.md"
  spec_section: "2.1 Indexing"
  gap_type: ask
  question: |
    The indexing section shows list[2..$] but doesn't specify: when is the $
    evaluated? If the list is mutated during the expression evaluation (e.g.,
    in a function argument), does $ reflect the original or new length?
  impact: |
    Matters for: x[process(x)..$] where process mutates x. Must specify evaluation
    order.
  suggested_addition: |
    Add to builtins/lists.md section 2.1: "In range expressions list[start..end],
    both start and end are evaluated before the list is accessed. $ evaluates to
    the length of the list at the time of range evaluation."

- id: GAP-015
  feature: "list slicing"
  spec_file: "spec/operators.md"
  spec_section: "2.3 Range Assignment"
  gap_type: test
  question: |
    For range assignment list[start..end] = replacement, what happens if
    replacement is not a list? The section says "Replacement can be different
    length" implying it must be a list, but doesn't state E_TYPE for non-list.
  impact: |
    Edge case error handling. list[1..2] = 5 could: (a) raise E_TYPE, (b) treat
    as list[1..2] = {5}, (c) replace range with single element.
  suggested_addition: |
    Add to operators.md section 2.3: "Replacement must be a LIST. If replacement
    is not a list, raise E_TYPE."

- id: GAP-016
  feature: "list slicing"
  spec_file: "spec/operators.md"
  spec_section: "2.3 Range Assignment"
  gap_type: ask
  question: |
    For list[2..4] = {10, 20, 30}, the spec says replacement can be different
    length and the list grows/shrinks. But what about list[2..4] = {}? Does
    assigning empty list delete elements 2..4?
  impact: |
    This is a deletion operation via assignment. Should be explicit that empty
    list removes the range.
  suggested_addition: |
    Add example to operators.md section 2.3: "Assigning empty list deletes range:
    list[2..4] = {} removes elements 2 through 4, shrinking the list by 3."

- id: GAP-017
  feature: "iteration mutation"
  spec_file: "spec/statements.md"
  spec_section: "4.1 List Iteration"
  gap_type: test
  question: |
    What happens if the list is mutated during iteration? The spec says "Evaluate
    list_expression once" which suggests the list is captured, but doesn't
    explicitly state what happens if the original list variable is reassigned or
    mutated.
  impact: |
    Critical for correctness. Example: for x in (list) { list = {@list, x}; }
    Does this create infinite loop or iterate over original snapshot?
  suggested_addition: |
    Add to statements.md section 4.1: "The list is evaluated once before iteration
    begins. A snapshot is taken; subsequent mutations to the list variable do not
    affect the iteration. Example: for x in (list) { list = {}; } continues
    iterating over the original list."

- id: GAP-018
  feature: "iteration mutation"
  spec_file: "spec/statements.md"
  spec_section: "4.1 List Iteration"
  gap_type: ask
  question: |
    After a for loop completes normally, what is the value of the loop variable?
    Does it retain the last iterated value, or become undefined, or 0?
  impact: |
    Affects variable state post-loop. Some languages leave it as last value,
    some make it undefined. Must specify.
  suggested_addition: |
    Add to statements.md section 4.1: "After the loop completes, the loop variable
    retains the value of the last element iterated. If the list was empty, the
    variable remains unchanged from its pre-loop value (or undefined if newly
    declared)."

- id: GAP-019
  feature: "iteration with index"
  spec_file: "spec/statements.md"
  spec_section: "4.2 List Iteration with Index"
  gap_type: guess
  question: |
    For `for value, index in (list)`, does the index variable get updated to
    length+1 after loop completion, or does it retain the last valid index?
  impact: |
    Similar to GAP-018 but for index variable. Must specify post-loop state.
  suggested_addition: |
    Add to statements.md section 4.2: "After the loop completes, the index variable
    retains the value of the last index iterated (equal to length(list)). If the
    list was empty, index remains unchanged."

- id: GAP-020
  feature: "nested lists"
  spec_file: "spec/types.md"
  spec_section: "2.6 LIST"
  gap_type: test
  question: |
    What is the maximum nesting depth for lists? Can you have {{{{...}}}} to
    arbitrary depth, or is there a limit?
  impact: |
    Affects implementation strategy (recursion vs iteration). Also affects error
    handling for deeply nested operations.
  suggested_addition: |
    Add to types.md section 2.6: "Lists can be nested to arbitrary depth, limited
    only by available memory. Operations on deeply nested lists may raise E_QUOTA
    if resource limits are exceeded."

- id: GAP-021
  feature: "splice operator"
  spec_file: "spec/builtins/lists.md"
  spec_section: "9. Splice Operator"
  gap_type: ask
  question: |
    What happens with @non_list in a list literal? {1, @5, 3} - does this raise
    E_TYPE, or is @5 treated as just 5?
  impact: |
    Error handling for invalid splice. Spec shows splice only on lists but
    doesn't specify error for non-list.
  suggested_addition: |
    Add to builtins/lists.md section 9: "The @ operator requires a LIST operand.
    {@non_list} raises E_TYPE if the expression is not a list."

- id: GAP-022
  feature: "list equality"
  spec_file: "spec/operators.md"
  spec_section: "9.1 Equality"
  gap_type: ask
  question: |
    For list equality, the spec says "deep comparison", but how deep? If two
    lists contain references to the same large nested structure, is comparison
    O(1) (reference equality) or O(n) (recursive value equality)?
  impact: |
    Performance and semantics. {} == {} is clearly true, but for large nested
    lists, must specify if comparison is structural or referential.
  suggested_addition: |
    Add to operators.md section 9.1: "List equality is recursive structural
    comparison. Two lists are equal if they have the same length and all
    corresponding elements are equal (using == recursively). Comparison is
    O(n) in total element count including nested lists."

- id: GAP-023
  feature: "list bounds"
  spec_file: "spec/types.md"
  spec_section: "2.6 LIST"
  gap_type: test
  question: |
    What is the maximum length of a list? The spec mentions E_QUOTA for resource
    limits but doesn't specify a maximum list size.
  impact: |
    Implementation must know when to raise E_QUOTA. Is it 2^31 elements? 2^63?
    Memory-based? Must specify.
  suggested_addition: |
    Add to types.md section 2.6: "List length is limited only by available memory
    and quota settings. Implementations should raise E_QUOTA when list operations
    would exceed configured resource limits. Minimum supported length is 2^31-1
    elements on 64-bit systems."
```

---

## Critical Gaps Summary

**Highest Priority (affects correctness):**
1. GAP-009: listappend index semantics contradiction
2. GAP-003: Range bound checking behavior
3. GAP-008: Shallow vs deep copy-on-write
4. GAP-017: List mutation during iteration

**Medium Priority (affects edge cases):**
5. GAP-002: Negative index behavior
6. GAP-004: Reverse range behavior
7. GAP-011: listinsert bounds with length+1
8. GAP-016: Range assignment with empty list

**Low Priority (documentation clarity):**
9. GAP-001: Index 0 explicit rejection
10. GAP-018-019: Post-loop variable state
11. GAP-020: Nesting depth limits
12. GAP-023: Maximum list length

---

## Testing Checklist for Implementor

Based on gaps found, an implementor would need to test:

- [ ] list[0] raises E_RANGE
- [ ] list[-1] raises E_RANGE (not Python-style last element)
- [ ] list[100] on 5-element list raises E_RANGE (not clamped)
- [ ] list[3..1] returns {} (not error, not reversed)
- [ ] list[2..$] evaluates $ before access
- [ ] a = {1,2}; b = a; b[1] = 99; // a unchanged (COW)
- [ ] a = {{1,2}}; b = a; b[1][1] = 99; // shallow COW
- [ ] listappend({1,2}, 0, 0) => {0,1,2} (index=0 valid)
- [ ] listinsert bounds allow length+1
- [ ] for x in (list) { list = {}; } // iterates over snapshot
- [ ] for x in ({1,2,3}) {}; // x == 3 after loop
- [ ] {1, @"string", 3} raises E_TYPE
- [ ] {1,2} == {1,2} uses recursive deep equality
- [ ] list[2..4] = {} deletes elements
