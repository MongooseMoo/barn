# MOO List Built-ins

## Overview

List manipulation functions. All list indices are 1-based. Lists are copy-on-write.

---

## 1. Basic Operations

### 1.1 length

**Signature:** `length(list) → INT`

**Description:** Returns number of elements.

**Examples:**
```moo
length({})           => 0
length({1, 2, 3})    => 3
length({{1}, {2}})   => 2
```

**Errors:**
- E_TYPE: Not a list

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

**Signature:** `is_member(value, list) → INT`
**Operator:** `value in list → INT`

**Description:** Returns 1-based index if found, 0 otherwise.

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

### 2.3 indexc [Verb]

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

**Description:** Returns sorted list.

**Parameters:**
- `list`: List to sort
- `keys`: List of indices to sort by (for list of lists)
- `natural`: Natural string sorting (default: false)
- `reverse`: Reverse order (default: false)

**Examples:**
```moo
sort({3, 1, 2})                  => {1, 2, 3}
sort({"b", "a", "c"})            => {"a", "b", "c"}
sort({3, 1, 2}, {}, 0, 1)        => {3, 2, 1}  (reversed)
sort({"a10", "a2", "a1"}, {}, 1) => {"a1", "a2", "a10"}  (natural)
```

**For list of lists:**
```moo
data = {{"Alice", 30}, {"Bob", 25}, {"Carol", 35}};
sort(data, {2})                   => sorted by age
sort(data, {1})                   => sorted by name
```

**Errors:**
- E_TYPE: Not a list

---

## 4. Transformation

### 4.1 reverse (ToastStunt)

**Signature:** `reverse(list) → LIST`

**Description:** Returns list in reverse order.

**Examples:**
```moo
reverse({1, 2, 3})       => {3, 2, 1}
reverse({})              => {}
```

---

### 4.2 slice [Verb]

**Signature:** `slice(list, indices) → LIST`

**Description:** Extracts elements at specified indices.

**Note:** Implemented as MOO verb, not a builtin.

**Examples:**
```moo
slice({10, 20, 30, 40}, {2, 4})      => {20, 40}
slice({10, 20, 30}, {1, 1, 1})       => {10, 10, 10}
```

**Errors:**
- E_RANGE: Index out of bounds

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

### 8.3 unique (ToastStunt)

**Signature:** `unique(list) → LIST`

**Description:** Removes duplicate elements, preserving order.

**Note:** This is a builtin in ToastStunt (implemented in server).

**Examples:**
```moo
unique({1, 2, 2, 3, 1})   => {1, 2, 3}
```

---

### 8.4 count [Verb]

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
