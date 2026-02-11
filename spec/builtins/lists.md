# MOO List Built-ins

## Overview

List manipulation functions. All list indices are 1-based. Lists are copy-on-write.

---

## 1. Basic Operations

### 1.1 length

**Signature:** `length(value) → INT`

**Description:** Returns the length of a list, map, or string.

**Examples:**
```moo
length({})                 => 0
length({1, 2, 3})          => 3
length("hello")            => 5
length(["a" -> 1])         => 1
```

**Errors:**
- E_TYPE: Not a list, map, or string

---

### 1.2 listappend

**Signature:** `listappend(list, value [, index]) → LIST`

**Description:** Returns new list with value inserted **after** the specified position.

**Parameters:**
- `list`: Original list
- `value`: Value to insert
- `index`: Position **after which** to insert (default: appends to end)

**Index semantics:**
- Valid range: **0 to length(list)**
- Index 0: insert at beginning (before first element)
- Index 1: insert after first element (before second)
- Index length(list): append to end
- Omit index: append to end (same as index = length(list))

**Examples:**
```moo
listappend({1, 2}, 3)        => {1, 2, 3}     // Appends (default)
listappend({1, 2}, 0, 0)     => {0, 1, 2}     // Insert at beginning
listappend({1, 3}, 2, 1)     => {1, 2, 3}     // Insert after index 1
listappend({1, 2}, 5, 2)     => {1, 2, 5}     // Insert after last (appends)
```

**Errors:**
- E_TYPE: First arg not a list
- E_RANGE: Index < 0 or index > length(list)

---

### 1.3 listinsert

**Signature:** `listinsert(list, value [, index]) → LIST`

**Description:** Returns new list with value inserted **before** the specified position.

**Parameters:**
- `list`: Original list
- `value`: Value to insert
- `index`: Position **before which** to insert (default: 1)

**Index semantics:**
- Valid range: **1 to length(list)+1**
- Index 1: insert at beginning (before first element)
- Index 2: insert before second element (after first)
- Index length(list)+1: append to end
- Indices > length(list)+1 are **clamped** to length(list)+1 (appends)
- Indices <= 0 are **clamped** to 1 (inserts at beginning)
- Omit index: insert at beginning (same as index = 1)

**Examples:**
```moo
listinsert({2, 3}, 1)        => {1, 2, 3}     // Insert at beginning (default)
listinsert({1, 3}, 2, 2)     => {1, 2, 3}     // Insert before index 2
listinsert({1, 2}, 3, 4)     => {1, 2, 3}     // Index 4 = length+1, appends
listinsert({1, 2}, 0, 0)     => {0, 1, 2}     // Index 0 clamped to 1
```

**Errors:**
- E_TYPE: First arg not a list
- Note: **No E_RANGE** - out of bounds indices are clamped

---

### 1.4 listdelete

**Signature:** `listdelete(list, index) → LIST`

**Description:** Returns new list with element removed.

**Index semantics:**
- Valid range: **1 to length(list)**
- Index must be valid 1-based index

**Examples:**
```moo
listdelete({1, 2, 3}, 2)     => {1, 3}
listdelete({1}, 1)           => {}
listdelete({}, 1)            => E_RANGE  // Empty list
```

**Errors:**
- E_TYPE: First arg not a list
- E_RANGE: Index <= 0 or index > length(list)

---

### 1.5 listset

**Signature:** `listset(list, value, index) → LIST`

**Description:** Returns new list with element replaced.

**Examples:**
```moo
listset({1, 2, 3}, 9, 2)     => {1, 9, 3}
```

**Errors:**
- E_TYPE: First arg not a list
- E_RANGE: Index out of bounds

---

### 1.6 setadd

**Signature:** `setadd(list, value) → LIST`

**Description:** Adds value if not already present.

**Examples:**
```moo
setadd({1, 2, 3}, 4)     => {1, 2, 3, 4}
setadd({1, 2, 3}, 2)     => {1, 2, 3}  (no change)
```

**Errors:**
- E_TYPE: First arg not a list

---

### 1.7 setremove

**Signature:** `setremove(list, value) → LIST`

**Description:** Removes first occurrence of value using `==` operator for comparison.

**Equality semantics:**
- Uses `==` operator (see operators.md section 9.1)
- Type-strict comparison (INT != FLOAT)
- Deep structural comparison for nested lists
- Bitwise comparison for floats (no epsilon tolerance)

**Examples:**
```moo
setremove({1, 2, 3}, 2)      => {1, 3}
setremove({1, 2, 2, 3}, 2)   => {1, 2, 3}  (only first)
setremove({1, 2, 3}, 4)      => {1, 2, 3}  (not found)
setremove({{1, 2}, {3}}, {1, 2})  => {{3}}  (deep equality)
```

**Errors:**
- E_TYPE: First arg not a list

---

## 2. Search and Access

### 2.1 Indexing

Lists use 1-based indexing:

```moo
list[index]         // Get element
list[start..end]    // Get sublist (inclusive)
list[index] = val   // Set element (statement)
```

**Evaluation order:**
- In range expressions `list[start..end]`, both `start` and `end` are evaluated **before** the list is accessed
- `$` evaluates to `length(list)` at the time of range evaluation
- Mutations to the list during expression evaluation do not affect the already-evaluated indices

**Examples:**
```moo
{1, 2, 3}[1]        => 1
{1, 2, 3}[2]        => 2
{1, 2, 3}[1..2]     => {1, 2}
{1, 2, 3}[2..$]     => {2, 3}  // $ = 3 before access
```

**Errors:**
- E_RANGE: Index out of bounds
- E_TYPE: Non-integer index

---

### 2.2 is_member / `in` operator

**Signature:** `is_member(value, list [, case_matters]) → INT`
**Operator:** `value in list → INT`

**Description:** Returns 1-based index if found, 0 otherwise.

**Parameters:**
- `value`: Value to search for
- `list`: List to search in
- `case_matters`: Optional, for case-sensitive string comparison (default: case-insensitive)

**Examples:**
```moo
is_member(2, {1, 2, 3})      => 2
is_member(4, {1, 2, 3})      => 0
2 in {1, 2, 3}               => 2
"x" in {"a", "b"}            => 0
```

**Note:** Uses value equality, not identity.

**Errors:**
- E_TYPE: Second arg not a list

---

### 2.3 all_members (ToastStunt)

**Signature:** `all_members(value, list) → LIST`

**Description:** Returns list of all indices where value appears. Unlike is_member which returns only the first match, all_members returns all matching positions.

**Note:** This is a builtin in ToastStunt implemented as a background thread operation.

**Examples:**
```moo
all_members(2, {1, 2, 3, 2, 4})           => {2, 4}
all_members(5, {1, 2, 3})                 => {}
all_members("x", {"a", "x", "b", "x"})    => {2, 4}
```

**Comparison with is_member:**
- `is_member(value, list)` → returns first index or 0
- `all_members(value, list)` → returns list of all indices or {}

**Errors:**
- E_TYPE: Second arg not a list

---

### 2.4 indexc [Verb]

**Signature:** `indexc(list, value [, start]) → INT`

**Description:** Case-insensitive search in list of strings.

**Note:** Implemented as MOO verb, not a builtin.

**Examples:**
```moo
indexc({"Hello", "World"}, "hello")   => 1
indexc({"Hello", "World"}, "WORLD")   => 2
```

---

## 3. Sorting

### 3.1 sort (ToastStunt)

**Signature:** `sort(list [, keys] [, natural] [, reverse]) → LIST`

**Description:** Returns a list sorted by its own values or by a parallel list of keys.

**Parameters:**
- `list`: List to return in sorted order
- `keys`: Optional list of sort keys. If provided and non-empty, it must be the same length as `list`. Ordering is based on `keys` but the returned list is from `list`.
- `natural`: Natural string ordering (default: false). Comparisons are case-insensitive in both modes.
- `reverse`: Reverse order (default: false)

**Type constraints:**
- The list used for comparisons (`keys` if non-empty, otherwise `list`) must contain only one scalar type: INT, FLOAT, OBJ, ERR, or STR.
- Lists, maps, waifs, and anonymous objects are not sortable and raise E_TYPE.

**Examples:**
```moo
sort({3, 1, 2})                    => {1, 2, 3}
sort({"b", "a", "c"})              => {"a", "b", "c"}
sort({3, 1, 2}, {}, 0, 1)          => {3, 2, 1}  (reversed)
sort({"a10", "a2", "a1"}, {}, 1)   => {"a1", "a2", "a10"}  (natural)

data = {{"Alice", 30}, {"Bob", 25}, {"Carol", 35}};
keys = {30, 25, 35};
sort(data, keys)                   => {{"Bob", 25}, {"Alice", 30}, {"Carol", 35}}
```

**Notes:**
- This runs in a background thread when threading is enabled; the task may suspend.

**Errors:**
- E_TYPE: Not a list, mixed element types, or unsupported element type
- E_INVARG: `keys` length does not match `list`

---

## 4. Transformation

### 4.1 reverse (ToastStunt)

**Signature:** `reverse(value) → VALUE`

**Description:** Reverses a list or string.

**Examples:**
```moo
reverse({1, 2, 3})       => {3, 2, 1}
reverse("abc")           => "cba"
```

**Errors:**
- E_INVARG: Not a list or string

---

### 4.2 slice (ToastStunt)

**Signature:** `slice(list [, index] [, default_value]) → LIST`

**Description:** Extracts elements from each item in a list of lists, strings, or maps.

**Parameters:**
- `list`: Source list whose elements will be indexed
- `index`: Optional index specification (INT, LIST of INT, or STR). Default is 1.
- `default_value`: Only used when `index` is STR and a map key is missing

**Semantics:**
- If `index` is INT: for each element, return the item at that index (1-based). Strings yield 1-character strings.
- If `index` is LIST of INT: for each element, return a list of those positions (1-character strings for string elements).
- If `index` is STR: each element must be a MAP. For each map, return the value for that key. If the key is missing and `default_value` is provided, append `default_value`. If the key is missing and no default is provided, nothing is appended for that map.

**Type requirements:**
- When `index` is INT or LIST, each element must be a LIST or STR.
- When `index` is STR, each element must be a MAP.

**Index validation:**
- Empty index list returns E_RANGE
- Zero or negative indices in index list return E_RANGE
- Non-integer indices in index list return E_INVARG
- Any out-of-range index for any element returns E_RANGE

**Examples:**
```moo
slice({{10, 20}, {30, 40}}, 2)          => {20, 40}
slice({"ab", "cd"}, {2, 1})            => {{"b", "a"}, {"d", "c"}}
slice({["a" -> 1], ["a" -> 2]}, "a")   => {1, 2}
slice({["a" -> 1], ["b" -> 2]}, "a", 0) => {1, 0}
```

**Errors:**
- E_TYPE: First arg not a list
- E_INVARG: Element types do not match index type, or non-int in index list
- E_RANGE: Empty index list, zero/negative indices, or out-of-range index

---

### 4.3 rotate [Verb]

**Signature:** `rotate(list [, count]) → LIST`

**Description:** Rotates list elements.

**Note:** Implemented as MOO verb, not a builtin.

**Examples:**
```moo
rotate({1, 2, 3, 4})         => {2, 3, 4, 1}
rotate({1, 2, 3, 4}, 2)      => {3, 4, 1, 2}
rotate({1, 2, 3, 4}, -1)     => {4, 1, 2, 3}
```

---

## 5. Set Operations (MOO Verbs - Not Builtins)

**Note:** The following are implemented as MOO verbs in standard databases, not as server builtins. They return E_VERBNF if called directly without the verb implementations.

### 5.1 intersection [Verb]

**Signature:** `intersection(list1, list2) → LIST`

**Description:** Returns elements common to both lists.

**Examples:**
```moo
intersection({1, 2, 3}, {2, 3, 4})   => {2, 3}
intersection({1, 2}, {3, 4})         => {}
```

---

### 5.2 union [Verb]

**Signature:** `union(list1, list2) → LIST`

**Description:** Returns combined unique elements.

**Examples:**
```moo
union({1, 2, 3}, {2, 3, 4})   => {1, 2, 3, 4}
```

---

### 5.3 diff [Verb]

**Signature:** `diff(list1, list2) → LIST`

**Description:** Returns elements in list1 not in list2.

**Examples:**
```moo
diff({1, 2, 3}, {2, 3, 4})   => {1}
diff({1, 2, 3}, {4, 5})      => {1, 2, 3}
```

---

## 6. Aggregation (MOO Verbs - Not Builtins)

**Note:** The following are implemented as MOO verbs in standard databases, not as server builtins.

### 6.1 sum [Verb]

**Signature:** `sum(list) → INT|FLOAT`

**Description:** Returns sum of numeric elements.

**Examples:**
```moo
sum({1, 2, 3, 4})        => 10
sum({1.5, 2.5})          => 4.0
sum({})                  => 0
```

**Errors:**
- E_TYPE: Non-numeric element

---

### 6.2 avg [Verb]

**Signature:** `avg(list) → FLOAT`

**Description:** Returns average of numeric elements.

**Examples:**
```moo
avg({1, 2, 3})           => 2.0
avg({10, 20})            => 15.0
```

**Errors:**
- E_TYPE: Non-numeric element
- E_INVARG: Empty list

---

### 6.3 product [Verb]

**Signature:** `product(list) → INT|FLOAT`

**Description:** Returns product of numeric elements.

**Examples:**
```moo
product({1, 2, 3, 4})    => 24
product({})              => 1
```

---

## 7. Searching (MOO Verbs - Not Builtins)

**Note:** The following are implemented as MOO verbs in standard databases, not as server builtins.

### 7.1 assoc [Verb]

**Signature:** `assoc(value, alist [, index]) → LIST|0`

**Description:** Searches association list for matching element.

**Examples:**
```moo
alist = {{"a", 1}, {"b", 2}, {"c", 3}};
assoc("b", alist)        => {"b", 2}
assoc("x", alist)        => 0

// Search by second element
assoc(2, alist, 2)       => {"b", 2}
```

---

### 7.2 rassoc [Verb]

**Signature:** `rassoc(value, alist [, index]) → LIST|0`

**Description:** Searches from end of association list.

---

### 7.3 iassoc [Verb]

**Signature:** `iassoc(value, alist [, index]) → INT`

**Description:** Like assoc but returns index instead of element.

---

## 8. Utility (MOO Verbs - Not Builtins)

**Note:** The following are implemented as MOO verbs in standard databases, not as server builtins.

### 8.1 make_list [Verb]

**Signature:** `make_list(count [, value]) → LIST`

**Description:** Creates list with repeated value.

**Examples:**
```moo
make_list(3)          => {0, 0, 0}
make_list(3, "x")     => {"x", "x", "x"}
make_list(0)          => {}
```

**Errors:**
- E_INVARG: Negative count

---

### 8.2 flatten [Verb]

**Signature:** `flatten(list [, depth]) → LIST`

**Description:** Flattens nested lists.

**Examples:**
```moo
flatten({{1, 2}, {3, 4}})        => {1, 2, 3, 4}
flatten({1, {2, {3, 4}}})        => {1, 2, 3, 4}
flatten({1, {2, {3}}}, 1)        => {1, 2, {3}}
```

---

### 8.3 count [Verb]

**Signature:** `count(list, value) → INT`

**Description:** Counts occurrences of value.

**Examples:**
```moo
count({1, 2, 2, 3, 2}, 2)   => 3
count({1, 2, 3}, 4)         => 0
```

---

## 9. Splice Operator

The `@` operator splices lists:

```moo
{@list1, @list2}     // Concatenate
{1, @{2, 3}, 4}      // => {1, 2, 3, 4}
verb(@args)          // Spread arguments
```

**Examples:**
```moo
a = {1, 2};
b = {3, 4};
{@a, @b}             => {1, 2, 3, 4}
{0, @a, 5}           => {0, 1, 2, 5}
```

---

## 10. Error Handling

| Error | Condition |
|-------|-----------|
| E_TYPE | Non-list argument |
| E_RANGE | Index out of bounds |
| E_INVARG | Invalid argument value |

---

## Go Implementation Notes

```go
// MOO lists are 1-based, copy-on-write
type MOOList struct {
    data []Value
}

func (l *MOOList) Get(index int64) (Value, error) {
    // Convert 1-based to 0-based
    i := int(index) - 1
    if i < 0 || i >= len(l.data) {
        return nil, E_RANGE
    }
    return l.data[i], nil
}

func builtinListappend(args []Value) (Value, error) {
    list, ok := args[0].(*MOOList)
    if !ok {
        return nil, E_TYPE
    }

    value := args[1]

    index := len(list.data)
    if len(args) > 2 {
        idx, _ := toInt(args[2])
        index = int(idx)
        if index < 0 || index > len(list.data) {
            return nil, E_RANGE
        }
    }

    // Copy-on-write: create new list
    newData := make([]Value, len(list.data)+1)
    copy(newData[:index], list.data[:index])
    newData[index] = value
    copy(newData[index+1:], list.data[index:])

    return &MOOList{data: newData}, nil
}
```
