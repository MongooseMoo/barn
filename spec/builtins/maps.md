# MOO Map Built-ins

## Overview

Map (associative array) manipulation functions. Maps are copy-on-write.

---

## 1. Map Syntax

### 1.1 Literal Syntax

```moo
[]                           // Empty map
["key" -> value]            // Single entry
["a" -> 1, "b" -> 2]        // Multiple entries
[1 -> "one", 2 -> "two"]    // Integer keys
```

### 1.2 Access Syntax

```moo
map[key]                    // Get value
map[key] = value            // Set or create entry
```

**Access behavior:**
- Returns the value associated with `key`
- Raises **E_RANGE** if key not present (regardless of key type)
- No type checking on keys - all MOO types are hashable

**Assignment behavior:**
- If `key` exists: Updates value (copy-on-write creates new map)
- If `key` does not exist: Creates new entry with `value`

**Examples:**
```moo
m = ["a" -> 1];
m["b"] = 2;              // Creates entry
// m is now ["a" -> 1, "b" -> 2]

m["a"] = 99;             // Updates entry
// m is now ["a" -> 99, "b" -> 2]

m[1]                     // E_RANGE (key 1 not found, not E_TYPE)
```

### 1.3 First/Last Markers and Ranges

Inside a map's index brackets, `^` is the map's first key and `$` its last
key in key order (see §5), not positions: after `x = [1 -> 1, 5 -> 5, 8 -> 8]`,
`x[^..$]` uses `^ == 1` and `$ == 8`.

`map[from..to]` selects by key: both endpoints are keys, and the result is the
map of every entry whose key lies from `from` through `to` in key order. When
`from` orders after `to` the result is `[]`; otherwise, if either endpoint is
not a key of the map, `E_RANGE`. Integer endpoints are keys too, never
positions.

`map[from..to] = newmap` requires both endpoints to be keys of the map
(otherwise `E_RANGE`, whatever their order); it removes the entries from `from`
through `to` and adds the pairs of `newmap`.

```moo
x = [1 -> "a", 5 -> "b", 8 -> "c"];
x[5..8]          => [5 -> "b", 8 -> "c"]
x[8..5]          => []
x[2..8]          => E_RANGE   (2 is not a key)
```

---

## 2. Basic Operations

### 2.1 mapkeys

**Signature:** `mapkeys(map) → LIST`

**Description:** Returns list of all keys in map iteration order.

**Order:** Returns keys in map iteration order. Keys are ordered consistently with `mapvalues()` - for a given map `m`, `mapkeys(m)[i]` corresponds to `mapvalues(m)[i]`.

**Examples:**
```moo
mapkeys(["a" -> 1, "b" -> 2])    => {"a", "b"}
mapkeys([1 -> "x", 2 -> "y"])    => {1, 2}
mapkeys([])                       => {}
```

**Errors:**
- E_TYPE: Not a map

---

### 2.2 mapvalues

**Signature:** `mapvalues(map [, key1, key2, ...]) → LIST`

**Description:** Returns list of values from map.

**Single argument form:** Returns all values in map iteration order. For a given map `m`, `mapvalues(m)[i]` corresponds to `mapkeys(m)[i]`.

**Multiple argument form:** Returns values for the specified keys, in the order the keys are provided.

**Examples:**
```moo
// All values
mapvalues(["a" -> 1, "b" -> 2])   => {1, 2}
mapvalues([])                      => {}

// Order correspondence with mapkeys
m = ["x" -> 10, "y" -> 20];
keys = mapkeys(m);
vals = mapvalues(m);
// keys[1] -> vals[1], keys[2] -> vals[2]

// Specific keys
mapvalues(["a" -> 1, "b" -> 2, "c" -> 3], "a", "c")   => {1, 3}
mapvalues(["x" -> 10, "y" -> 20], "y", "x")           => {20, 10}
```

**Errors:**
- E_TYPE: Not a map
- E_RANGE: Any specified key not found in map (multi-argument form only)

---

### 2.3 mapdelete

**Signature:** `mapdelete(map, key) → MAP`

**Description:** Returns new map with key removed. If `key` is a LIST, every
element of it is deleted as a key.

Any key type that can be a map key is accepted, including anonymous objects.
A LIST or MAP key (or list element, in the multi-key form) raises `E_TYPE`.

**Examples:**
```moo
mapdelete(["a" -> 1, "b" -> 2], "a")          => ["b" -> 2]
mapdelete(["a" -> 1, "b" -> 2], {"a", "b"})   => []
mapdelete(["a" -> 1], "x")                    => E_RANGE  (key not found)
```

**Errors:**
- E_TYPE: First arg not a map; key (or a list element) is a LIST or MAP
- E_RANGE: Key not found in map. In the multi-key form the error is raised
  with message `"Key <literal> not found in map"` (the key as `toliteral()`
  would print it; an anonymous object prints as `*anonymous*`) and the missing
  key as its value.

---

### 2.4 maphaskey (ToastStunt)

**Signature:** `maphaskey(map, key [, case_matters]) → INT`

**Flag arguments:** `case_matters` accept an INT or a BOOL and are read by truth value (`function_info()` reports the position as -3); any other type raises `E_TYPE`.

**Description:** Tests if key exists in map.

**Arguments:**
- `map`: The map to search
- `key`: The key to look for
- `case_matters` (optional): If provided and true (non-zero), string key matching is case-sensitive. If false (zero) or omitted, string keys match case-insensitively. Non-string keys always use exact matching.

Any key type that can be a map key is accepted, including anonymous objects.

**Examples:**
```moo
// Basic usage
maphaskey(["a" -> 1], "a")     => 1
maphaskey(["a" -> 1], "b")     => 0

// Case-insensitive matching (default or case_matters=0)
maphaskey(["a" -> 1], "A", 0)  => 1

// Case-sensitive matching (case_matters=1)
maphaskey(["a" -> 1], "A", 1)  => 0
```

**Errors:**
- E_TYPE: First arg not a map; `key` is a LIST or MAP

---

## 3. Iteration

Maps can be iterated with for loops:

```moo
// Value only
for value in (map)
    // value receives each value
endfor

// Value and key
for value, key in (map)
    // value receives value, key receives key
endfor
```

**Examples:**
```moo
ages = ["Alice" -> 30, "Bob" -> 25];
for age, name in (ages)
    notify(player, name + " is " + tostr(age));
endfor

// Empty map - body does not execute
for value in ([])
    notify(player, "never printed");
endfor
```

### 3.1 Mutation During Iteration

Due to copy-on-write semantics, map modifications during iteration are **SAFE**:

**Iteration uses a snapshot:** The loop iterates over the map value at loop start. Modifications create new map objects and do not affect the iteration.

**Example:**
```moo
m = ["a" -> 1, "b" -> 2, "c" -> 3];
for v, k in (m)
    m["new"] = 99;        // Creates new map, doesn't affect loop
    notify(player, k);
endfor
// Prints: a, b, c (not "new")
// After loop: m contains a, b, c, new
```

**Reassignment is visible after loop:**
```moo
m = ["a" -> 1, "b" -> 2];
for v, k in (m)
    if (k == "a")
        m = [];           // m is now empty, but loop continues on original
    endif
    notify(player, k);
endfor
// Prints: a, b (loop uses snapshot)
// After loop: m is []
```

---

## 4. Key Types

Maps support the following types as keys:

| Type | Hashable | Notes |
|------|----------|-------|
| INT | Yes | |
| FLOAT | Yes | NaN not recommended |
| STR | Yes | Most common |
| OBJ | Yes | Uses object ID |
| ERR | Yes | |
| BOOL | Yes | |
| ANON | Yes | By reference |
| WAIF | Yes | By reference |

**Note:** LIST and MAP values cannot be used as map keys. Attempting to use them will result in E_TYPE.

**Examples:**
```moo
[1 -> "one", 2 -> "two"]           // Integer keys
[#0 -> "system", #1 -> "wizard"]   // Object keys
[E_NONE -> "error key"]            // Error keys
```

### 4.1 Float Key Equality

Float keys use **bitwise comparison** (same as `==` operator):

- **NaN values:** Allowed but `NaN != NaN` means keys using NaN cannot be retrieved
- **+0.0 vs -0.0:** Treated as same key (bitwise equal in IEEE 754)
- **Precision:** Uses exact bitwise equality, so `(0.1 + 0.2)` and `0.3` are different keys

**Recommendation:** Avoid float keys due to precision issues. Use string keys for decimal values.

### 4.2 Reference Key Equality

**ANON and WAIF keys:**
- Use reference equality (identity comparison)
- Two distinct anonymous objects are different keys even if their properties match
- Same object referenced twice is the same key

**Performance:** Reference keys have O(1) equality checking

---

## 5. Ordering

A map keeps its keys sorted, and iteration (`for`, `mapkeys()`,
`mapvalues()`, `toliteral()`) visits them in ascending key order, never in
insertion order. Two maps with the same pairs iterate identically however they
were built.

Keys of different types order by type: INT < OBJ < ERR < FLOAT < BOOL < STR <
ANON < WAIF. Within a type:

- INT, OBJ: numerically (full 64-bit range)
- ERR: by error code
- FLOAT: numerically
- BOOL: `false` before `true`
- STR: case-insensitively (keys differing only in case are the same key)
- ANON, WAIF: by object identity; which of two such keys comes first is
  unspecified but does not depend on insertion order

```moo
mapkeys([1 -> "int", "z" -> "str", 3.14 -> "float", "a" -> "str2"])
=> {1, 3.14, "a", "z"}
mapkeys([true -> 1, false -> 0])
=> {false, true}
```

**Note:** `mapkeys(m)[i]` corresponds to `mapvalues(m)[i]` - these functions use the same iteration order

---

## 6. Error Handling

| Error | Condition |
|-------|-----------|
| E_TYPE | Non-map argument |
| E_RANGE | Key not found (on access) |
| E_INVARG | Invalid conversion input |

---

## 7. `in` Operator [Not Working]

> **Note:** The `in` operator does not currently work with maps in ToastStunt or Barn. It always returns 0 (false) regardless of whether the key exists. Use `maphaskey(map, key)` instead to test for key presence.

**Current behavior (incorrect):**
```moo
"a" in ["a" -> 1, "b" -> 2]    => 0  (should be 1, but returns 0)
"x" in ["a" -> 1, "b" -> 2]    => 0  (correct)
```

**Workaround:**
```moo
maphaskey(["a" -> 1], "a")     => 1  (correct)
maphaskey(["a" -> 1], "x")     => 0  (correct)
```

---

## 8. Go Implementation Notes

```go
// MOO maps are copy-on-write
type MOOMap struct {
    data map[string]Value  // Simplified; real impl uses Value keys
}

func builtinMapkeys(args []Value) (Value, error) {
    m, ok := args[0].(*MOOMap)
    if !ok {
        return nil, E_TYPE
    }

    keys := make([]Value, 0, len(m.data))
    for k := range m.data {
        keys = append(keys, StringValue(k))
    }
    return &MOOList{data: keys}, nil
}

func builtinMapdelete(args []Value) (Value, error) {
    m, ok := args[0].(*MOOMap)
    if !ok {
        return nil, E_TYPE
    }

    key := args[1]

    // Copy-on-write
    newData := make(map[string]Value, len(m.data))
    keyStr := key.String()
    for k, v := range m.data {
        if k != keyStr {
            newData[k] = v
        }
    }

    return &MOOMap{data: newData}, nil
}

// For proper Value keys, use a hash-based approach:
type MOOMap struct {
    entries []mapEntry
    index   map[uint64][]int  // hash -> entry indices
}

type mapEntry struct {
    key   Value
    value Value
}
```

---

## 9. Performance Notes

| Operation | Complexity |
|-----------|------------|
| Access | O(1) average |
| Insert | O(1) average (O(n) copy) |
| Delete | O(n) copy |
| Keys/Values | O(n) |
| Iteration | O(n) |
| `in` test | O(1) average |

Copy-on-write means modifications create new maps, but unmodified maps share memory.
