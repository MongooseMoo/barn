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

**Signature:** `mapvalues(map) → LIST`

**Description:** Returns list of all values in map iteration order.

**Order:** Returns values in map iteration order. For a given map `m`, `mapvalues(m)[i]` corresponds to `mapkeys(m)[i]`.

**Examples:**
```moo
mapvalues(["a" -> 1, "b" -> 2])   => {1, 2}
mapvalues([])                      => {}

// Order correspondence
m = ["x" -> 10, "y" -> 20];
keys = mapkeys(m);
vals = mapvalues(m);
// keys[1] -> vals[1], keys[2] -> vals[2]
```

**Errors:**
- E_TYPE: Not a map

---

### 2.3 mapdelete

**Signature:** `mapdelete(map, key) → MAP`

**Description:** Returns new map with key removed.

**Examples:**
```moo
mapdelete(["a" -> 1, "b" -> 2], "a")   => ["b" -> 2]
mapdelete(["a" -> 1], "x")             => E_RANGE  (key not found)
```

**Errors:**
- E_TYPE: First arg not a map
- E_RANGE: Key not found in map

---

### 2.4 maphaskey (ToastStunt)

**Signature:** `maphaskey(map, key) → BOOL`

**Description:** Tests if key exists in map.

**Examples:**
```moo
maphaskey(["a" -> 1], "a")    => 1
maphaskey(["a" -> 1], "b")    => 0
```

**Errors:**
- E_TYPE: First arg not a map

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

Map iteration order is **implementation-defined** with these guarantees:

1. **Stability within iteration:** Iterating the same map object twice without modification produces the same order
2. **No insertion-order guarantee:** Order is not based on insertion sequence
3. **Session non-determinism:** Order may vary between sessions/restarts
4. **Implementation-specific:** May be based on hash values, comparison ordering (e.g., red-black tree), or other factors

**Portable code must not depend on any specific iteration order.**

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
