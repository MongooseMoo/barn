# Blind Implementor Audit: Maps

**Date:** 2025-12-24
**Feature:** Maps (associative arrays)
**Spec Files:** `spec/builtins/maps.md`, `spec/types.md`, `spec/statements.md`, `spec/operators.md`

---

## Executive Summary

The Maps specification has **significant gaps** that would prevent accurate implementation:

1. **CRITICAL CONFLICT**: `types.md` states "Keys: INT or STR only" but `maps.md` explicitly lists 7+ hashable types including LIST, MAP, BOOL, FLOAT, ERR, OBJ
2. **Missing key equality semantics** for floats (NaN, -0.0 vs +0.0)
3. **No mutation-during-iteration behavior** specified
4. **Unclear iteration order** guarantees (contradictory statements)
5. **Missing error behavior** for several edge cases

---

## Gap Analysis

### GAP-001: Critical Type Conflict - Valid Key Types

**Feature:** Map key types
**Spec Files:** `spec/types.md` line 181-184 vs `spec/builtins/maps.md` lines 203-215
**Gap Type:** CONFLICT (must resolve)

**Question:**
Which types are valid map keys? There is a CRITICAL CONTRADICTION:

- `types.md` (line 182): "Keys: INT or STR only"
- `maps.md` (lines 203-215): Lists 7 hashable types: INT, FLOAT, STR, OBJ, ERR, BOOL, LIST, MAP

The spec provides examples in `maps.md` lines 217-221:
```moo
[1 -> "one", 2 -> "two"]           // Integer keys
[#0 -> "system", #1 -> "wizard"]   // Object keys
[{1,2} -> "pair"]                  // List key
```

But `operators.md` line 103 states: "E_TYPE: Invalid key type (maps require INT or STR)"

**Impact:**
An implementor CANNOT proceed without knowing which types are actually allowed. This affects:
- Type checking for map indexing operations
- Hash function implementation
- Error handling

**Suggested Resolution:**
Based on the weight of evidence (`maps.md` has detailed table + examples, while `types.md` appears to be summary), the likely intent is that ALL hashable types are valid. Therefore:

1. Update `types.md` line 182 from "Keys: INT or STR only" to "Keys: Any hashable type (INT, FLOAT, STR, OBJ, ERR, BOOL, LIST, MAP)"
2. Update `operators.md` line 103 from "(maps require INT or STR)" to "(maps require hashable type)"

---

### GAP-002: Float Key Equality - NaN Handling

**Feature:** Map key hashing
**Spec File:** `spec/builtins/maps.md` line 208
**Gap Type:** GUESS

**Question:**
How are FLOAT keys hashed and compared for equality? Specifically:

1. The spec says "NaN not recommended" (line 208) but doesn't say what happens if used
2. Are +0.0 and -0.0 considered the same key or different keys?
3. Do floats use bitwise equality or semantic equality?

**Examples:**
```moo
// What happens here?
m = [0.0 / 0.0 -> "NaN key"];  // Is this allowed?
m[0.0 / 0.0]                   // Does this retrieve it?

// Are these the same key?
m = [0.0 -> "zero"];
m[-0.0]                        // Returns "zero" or E_RANGE?

// Bitwise vs semantic
m = [0.1 + 0.2 -> "a"];
m[0.3]                         // E_RANGE because not bitwise equal?
```

**Impact:**
Float hashing is notoriously tricky. Implementors could:
- (a) Allow NaN, use bitwise equality (NaN != NaN means never retrievable)
- (b) Reject NaN at creation time with E_INVARG
- (c) Use semantic equality (but contradicts `operators.md` lines 417-420)

**Suggested Addition:**
Add to `maps.md` section 6:
```
**Float key equality:**
- Uses bitwise comparison (same as `==` operator)
- NaN values are allowed but use NaN != NaN semantics (cannot be retrieved)
- +0.0 and -0.0 are distinct keys (bitwise different)
- Recommended: Avoid float keys due to precision issues
```

---

### GAP-003: List/Map Key Equality

**Feature:** Composite key hashing
**Spec File:** `spec/builtins/maps.md` lines 213-214
**Gap Type:** ASSUME

**Question:**
When using LIST or MAP as keys, how is equality determined?

The spec says "By value" but doesn't clarify:
1. Is this deep equality (recursive for nested structures)?
2. How are hash collisions handled for deep structures?
3. What if the list/map contains unhashable types (though all MOO types are hashable)?

**Examples:**
```moo
// Deep equality?
m = [{1, {2, 3}} -> "nested"];
m[{1, {2, 3}}]                    // Retrieves "nested"?

// Order matters for lists?
m = [{1, 2} -> "a"];
m[{2, 1}]                         // E_RANGE (different keys)?

// Map keys as keys
m = [["a" -> 1] -> "outer"];
m[["a" -> 1]]                     // Retrieves "outer"?
```

**Impact:**
Deep equality is expensive. Implementors need to know:
- Whether to pre-compute hash for composite keys
- Whether equality is shallow or deep
- Performance implications for nested structures

**Suggested Addition:**
Add to `maps.md` section 6:
```
**Composite key equality:**
- LIST keys: Deep equality, order-sensitive. {1,2} != {2,1}
- MAP keys: Deep equality, entry-set equality (order-independent)
- Hash computation: Recursive for nested structures
- Performance: O(n) equality check for n-element composite keys
```

---

### GAP-004: Iteration Order Guarantees

**Feature:** Map iteration
**Spec File:** `spec/builtins/maps.md` lines 226-230
**Gap Type:** ASSUME

**Question:**
What are the EXACT iteration order guarantees? The spec provides contradictory information:

- Line 228: "Deterministic within a session"
- Line 229: "May vary between server restarts"
- Line 230: "Not insertion order"

But it doesn't specify:
1. Is order based on hash values?
2. Does order change when entries are added/removed?
3. Are there ANY guarantees (e.g., same map iterated twice in same session = same order)?

**Examples:**
```moo
m = ["a" -> 1, "b" -> 2, "c" -> 3];
for v, k in (m)
    // What order: a, b, c? Or hash-based? Or undefined?
endfor

m["d"] = 4;
for v, k in (m)
    // Did adding "d" change order of a, b, c?
endfor
```

**Impact:**
"Deterministic within a session" is vague. Implementors need to know:
- Can they use Go's randomized map iteration?
- Must they maintain a stable ordering algorithm?
- Is iteration order observable/testable?

**Suggested Addition:**
Replace `maps.md` section 7 with:
```
## 7. Ordering

Map iteration order is **unspecified** but follows these guarantees:

1. **Stability within single iteration**: Iterating the same map object twice without modification produces the same order
2. **No insertion-order guarantee**: Order is not based on insertion sequence
3. **Session non-determinism**: Order may vary between sessions/restarts
4. **Implementation-defined**: May be based on hash values, memory layout, or other factors

**Portable code must not depend on any specific iteration order.**
```

---

### GAP-005: Mutation During Iteration

**Feature:** Map iteration safety
**Spec File:** `spec/builtins/maps.md` section 5
**Gap Type:** ASK

**Question:**
What happens when a map is modified during iteration?

```moo
m = ["a" -> 1, "b" -> 2, "c" -> 3];
for v, k in (m)
    if (k == "b")
        m["d"] = 4;     // Add entry during iteration
        m = mapdelete(m, "c");  // Remove entry during iteration
    endif
endfor
```

The spec mentions copy-on-write (COW) semantics but doesn't clarify iteration behavior:
1. Does iteration use a snapshot of the map?
2. Are additions/deletions visible to the current iteration?
3. Is assignment of the loop variable (`m = mapdelete(m, "c")`) safe?

**Impact:**
COW semantics suggest iteration should be safe (snapshot), but this must be explicit. Implementors could:
- (a) Snapshot map before iteration (safe but memory overhead)
- (b) Iterate live map (faster but concurrent modification issues)
- (c) Detect modification and raise E_TYPE or E_INVARG

**Suggested Addition:**
Add to `maps.md` section 5:
```
### 5.1 Mutation During Iteration

Due to copy-on-write semantics, map modifications during iteration are SAFE:

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

**Reassignment is visible:**
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
```

---

### GAP-006: Empty Map Iteration

**Feature:** Map iteration edge cases
**Spec File:** `spec/builtins/maps.md` lines 179-189
**Gap Type:** ASSUME

**Question:**
What happens when iterating over an empty map?

```moo
for v in ([])
    // Does this body execute?
endfor

for v, k in ([])
    // Does this execute?
endfor
```

The spec shows an example for list iteration but not maps. Based on list semantics, the body should not execute, but this should be explicit.

**Impact:**
Minor, but completeness matters. Edge cases should be documented.

**Suggested Addition:**
Add to `maps.md` section 5 examples:
```moo
// Empty map
for v in ([])
    notify(player, "never printed");
endfor
// Loop body does not execute

for v, k in ([])
    notify(player, "never printed");
endfor
// Loop body does not execute
```

---

### GAP-007: Loop Variable Values After Iteration

**Feature:** Map iteration variable state
**Spec File:** `spec/statements.md` lines 129-152
**Gap Type:** GUESS

**Question:**
After a map iteration completes, what are the values of the loop variables?

```moo
m = ["a" -> 1, "b" -> 2];
for v, k in (m)
    // ...
endfor
// What are v and k now?
```

For lists, the spec doesn't clarify this either. Implementors might:
- (a) Leave v/k as last iterated values
- (b) Set to 0
- (c) Leave undefined/uninitialized

**Impact:**
This is actually a statement-level issue, not maps-specific. But the map iteration section should clarify.

**Suggested Addition:**
Add to `spec/statements.md` section 4.3:
```
**Post-loop variable values:**
After loop completion (normal or via break), loop variables retain the value from the last iteration, or remain unmodified if the collection was empty.

**Example:**
```moo
for v, k in (["a" -> 1, "b" -> 2])
    // ...
endfor
// v and k hold last iterated values

for v, k in ([])
    // ...
endfor
// If v and k were previously undefined, they remain undefined
// If they had values, those values are unchanged
```
```

---

### GAP-008: mapslice with Missing Keys

**Feature:** `mapslice` error handling
**Spec File:** `spec/builtins/maps.md` lines 120-135
**Gap Type:** ASK

**Question:**
The spec says `mapslice` raises `E_RANGE` if a key is not found (line 134). But what if the keys list contains DUPLICATES?

```moo
mapslice(["a" -> 1, "b" -> 2], {"a", "a", "b"})
// Returns ["a" -> 1, "b" -> 2]?
// Or ["a" -> 1, "a" -> 1, "b" -> 2] (invalid map)?
```

Also, does `E_RANGE` get raised on the FIRST missing key, or after checking all keys?

```moo
mapslice(["a" -> 1], {"b", "c", "d"})
// E_RANGE for "b", "c", or "d"? Or all of them?
```

**Impact:**
Duplicate handling affects return value. Error reporting affects error messages.

**Suggested Addition:**
Update `maps.md` section 3.2:
```
**Error behavior:**
- E_TYPE: First arg not a map, second not a list
- E_RANGE: Raised on FIRST missing key encountered (iteration order of keys list)

**Duplicate keys:** The keys list may contain duplicates. Each key is included only once in the result (maps cannot have duplicate keys).

**Example:**
```moo
mapslice(["a" -> 1, "b" -> 2], {"a", "a"})   => ["a" -> 1]
mapslice(["a" -> 1], {"b"})                  => E_RANGE (key "b" not found)
```
```

---

### GAP-009: mkmap with Invalid Pairs

**Feature:** `mkmap` error handling
**Spec File:** `spec/builtins/maps.md` lines 157-172
**Gap Type:** GUESS

**Question:**
The spec says `E_INVARG` for "Elements not 2-element lists" (line 171), but doesn't clarify edge cases:

1. What if an element is a list with < 2 elements? `{{"a"}}`
2. What if an element is a list with > 2 elements? `{{"a", 1, 2}}`
3. What if an element is not a list at all? `{{1, 2}, "not a list"}`
4. What about `{{}}` (empty list)?

**Examples:**
```moo
mkmap({{"a"}})              // E_INVARG?
mkmap({{"a", 1, 2}})        // E_INVARG? Or use first 2?
mkmap({{1, 2}, "oops"})     // E_INVARG?
mkmap({{}})                 // E_INVARG?
```

**Impact:**
Error handling must be precise for each case.

**Suggested Addition:**
Update `maps.md` section 4.2:
```
**Errors:**
- E_TYPE: Argument is not a list
- E_INVARG: Any element is not a list, OR any element is not exactly 2 elements long

**Example:**
```moo
mkmap({{"a", 1}, {"b", 2}})    => ["a" -> 1, "b" -> 2]
mkmap({{"a"}})                 => E_INVARG (element has 1 element, not 2)
mkmap({{"a", 1, 2}})           => E_INVARG (element has 3 elements, not 2)
mkmap({{1, 2}, 3})             => E_INVARG (element 2 is not a list)
mkmap({{}})                    => E_INVARG (element has 0 elements, not 2)
```
```

---

### GAP-010: mkmap with Duplicate Keys

**Feature:** `mkmap` duplicate handling
**Spec File:** `spec/builtins/maps.md` lines 157-172
**Gap Type:** GUESS

**Question:**
What happens when the input list contains duplicate keys?

```moo
mkmap({{"a", 1}, {"b", 2}, {"a", 99}})
// Result: ["a" -> 99, "b" -> 2] (last wins)?
// Or: ["a" -> 1, "b" -> 2] (first wins)?
// Or: E_INVARG (duplicates not allowed)?
```

**Impact:**
This determines whether `mkmap(mklist(m))` is an identity operation or not.

**Suggested Addition:**
Add to `maps.md` section 4.2:
```
**Duplicate keys:** If multiple pairs have the same key (by equality), the LAST occurrence in the list determines the value in the resulting map.

**Example:**
```moo
mkmap({{"a", 1}, {"a", 99}})      => ["a" -> 99]
mkmap({{"a", 1}, {"b", 2}, {"a", 99}})  => ["a" -> 99, "b" -> 2]

// Round-trip property
m = ["a" -> 1, "b" -> 2];
mkmap(mklist(m)) == m             => 1 (true, but order may differ)
```
```

---

### GAP-011: mklist Order

**Feature:** `mklist` output order
**Spec File:** `spec/builtins/maps.md` lines 140-154
**Gap Type:** ASSUME

**Question:**
In what order does `mklist` return the {key, value} pairs?

The spec says iteration order is "deterministic within a session" but "may vary between restarts." Does `mklist` follow the same iteration order?

```moo
m = ["a" -> 1, "b" -> 2, "c" -> 3];
mklist(m)   // {{"a", 1}, {"b", 2}, {"c", 3}}? Or some other order?
```

**Impact:**
If `mklist` has different ordering than iteration, it creates inconsistency.

**Suggested Addition:**
Add to `maps.md` section 4.1:
```
**Order:** Returns pairs in the same order as map iteration (see section 7). Order is deterministic within a session but unspecified across sessions.

**Example:**
```moo
m = ["a" -> 1, "b" -> 2];
pairs = mklist(m);
// Order matches: for v, k in (m) iteration
```
```

---

### GAP-012: mapkeys/mapvalues Order

**Feature:** `mapkeys` and `mapvalues` output order
**Spec File:** `spec/builtins/maps.md` lines 31-62
**Gap Type:** ASSUME

**Question:**
Do `mapkeys` and `mapvalues` return lists in the same order as each other?

```moo
m = ["a" -> 1, "b" -> 2, "c" -> 3];
keys = mapkeys(m);      // {"a", "b", "c"}?
vals = mapvalues(m);    // {1, 2, 3}?
// Is keys[i] guaranteed to correspond to vals[i]?
```

The spec should explicitly state whether `keys[i]` and `vals[i]` are paired.

**Impact:**
If not guaranteed, code like this breaks:
```moo
keys = mapkeys(m);
vals = mapvalues(m);
for i in [1..length(keys)]
    notify(player, tostr(keys[i]) + " -> " + tostr(vals[i]));
endfor
```

**Suggested Addition:**
Update `maps.md` sections 2.1 and 2.2:

For `mapkeys`:
```
**Order:** Returns keys in map iteration order. Keys are ordered consistently with `mapvalues()` and `mklist()` for the same map.
```

For `mapvalues`:
```
**Order:** Returns values in map iteration order. For a given map `m`, `mapvalues(m)[i]` corresponds to `mapkeys(m)[i]`.
```

---

### GAP-013: Map Access with Wrong Key Type

**Feature:** Map indexing error handling
**Spec File:** `spec/builtins/maps.md` lines 20-25
**Gap Type:** GUESS

**Question:**
What error is raised when accessing a map with the wrong key type?

```moo
m = ["a" -> 1, "b" -> 2];  // String keys
m[1]                       // E_RANGE or E_TYPE?

m = [1 -> "one"];          // Integer keys
m["1"]                     // E_RANGE or E_TYPE?
```

The spec says "E_RANGE if missing" (line 23) but doesn't distinguish between:
- (a) Key of correct type but not present
- (b) Key of incorrect type

`operators.md` line 103 suggests E_TYPE for invalid key type, but this contradicts the simple "E_RANGE if missing" in maps.md.

**Impact:**
Error type affects catch handlers:
```moo
try
    value = m[key];
except (E_RANGE)
    // Catches missing key, but also wrong type?
endtry
```

**Suggested Addition:**
Update `maps.md` section 1.2:
```
**Access Syntax:**

```moo
map[key]                    // Get value
```

**Returns:** The value associated with `key`

**Errors:**
- E_TYPE: Key type is not hashable (never happens; all MOO types are hashable)
- E_RANGE: Key not present in map (regardless of key type)

**Note:** There is NO type checking on keys. `["a" -> 1][1]` raises E_RANGE (key `1` not found), not E_TYPE (wrong key type).
```

---

### GAP-014: Map Assignment Creating Entries

**Feature:** Map indexed assignment
**Spec File:** `spec/builtins/maps.md` line 24, `spec/operators.md` lines 94-99
**Gap Type:** ASSUME

**Question:**
When assigning to a map with a key that doesn't exist, does it CREATE the entry or raise E_RANGE?

```moo
m = ["a" -> 1];
m["b"] = 2;        // m is now ["a" -> 1, "b" -> 2]? Or E_RANGE?
```

Based on the `operators.md` line 98 ("sets or adds key-value pair"), assignment CREATES entries. But `maps.md` line 23 says access raises "E_RANGE if missing," which might imply assignment would too.

**Impact:**
This is a critical operation. Implementors need confirmation.

**Suggested Addition:**
Update `maps.md` section 1.2:
```
**Access Syntax:**

```moo
map[key]                    // Get value (E_RANGE if missing)
map[key] = value            // Set or create entry
```

**Assignment behavior:**
- If `key` exists: Updates value (copy-on-write creates new map)
- If `key` does not exist: Creates new entry with `value`

**Example:**
```moo
m = ["a" -> 1];
m["b"] = 2;              // Creates entry
// m is now ["a" -> 1, "b" -> 2]

m["a"] = 99;             // Updates entry
// m is now ["a" -> 99, "b" -> 2]
```
```

---

### GAP-015: maphaskey vs `in` Operator

**Feature:** Key existence testing
**Spec File:** `spec/builtins/maps.md` lines 83-97 and section 9
**Gap Type:** GUESS

**Question:**
The spec provides TWO ways to test key existence:
1. `maphaskey(map, key)` → returns BOOL (lines 83-97)
2. `key in map` → returns 1 or 0 (section 9, lines 246-256)

Are these functionally identical? The spec says `maphaskey` returns "BOOL" (which in MOO is integers 0/1 based on types.md), and `in` returns "1 if key exists, 0 otherwise."

Are these the same, or is there a subtle difference?

**Impact:**
If identical, the spec should say so. If different, the difference must be explained.

**Suggested Addition:**
Add to `maps.md` section 2.4:
```
**Note:** `maphaskey(m, k)` is functionally equivalent to `k in m` for maps. Both return 1 (true) if the key exists, 0 (false) otherwise.

The `in` operator (see section 9) is preferred for its conciseness, but `maphaskey` may be clearer in some contexts.
```

---

### GAP-016: Map Equality with Different Ordering

**Feature:** Map equality semantics
**Spec File:** `spec/operators.md` lines 391-425
**Gap Type:** ASSUME

**Question:**
When comparing maps for equality, is order relevant?

```moo
m1 = ["a" -> 1, "b" -> 2];
m2 = ["b" -> 2, "a" -> 1];
m1 == m2                      // 1 (true)? Or 0 (false)?
```

The spec says "Lists/maps compared by value (deep)" (line 401), which suggests entry-set equality (order-independent). But it doesn't explicitly state this.

For maps, order is implementation-defined, so two maps with the same entries should be equal regardless of internal ordering.

**Impact:**
If equality is order-sensitive, maps created differently might not be equal even with same entries.

**Suggested Addition:**
Add to `spec/operators.md` section 9.1:
```
**Map equality:**
- Two maps are equal if they have the same set of (key, value) pairs
- Iteration order is IGNORED (order is implementation-defined)
- Key and value comparison uses deep equality recursively

**Example:**
```moo
["a" -> 1, "b" -> 2] == ["b" -> 2, "a" -> 1]   => 1 (true)
["a" -> 1] == ["a" -> 1.0]                     => 0 (value types differ)
```
```

---

### GAP-017: Nested Map Copy-on-Write

**Feature:** COW semantics for nested maps
**Spec File:** `spec/types.md` lines 399-410
**Gap Type:** ASSUME

**Question:**
The spec explains COW for lists (lines 403-408) but not for nested maps. Does COW apply recursively?

```moo
m1 = ["a" -> ["inner" -> 1]];
m2 = m1;                      // m2 shares storage with m1
m2["a"]["inner"] = 99;        // Does this mutate m1["a"]?
```

Based on COW semantics, `m2["a"]` should create a new copy of the inner map before modification. But this isn't explicit.

**Impact:**
Implementors need to know whether:
- (a) COW applies only at top level
- (b) COW applies recursively to nested structures

**Suggested Addition:**
Add to `spec/types.md` section 5.4:
```
**Nested collections:**
Copy-on-write applies ONLY at the top level of each collection. Accessing a nested collection returns a reference, and modifying it triggers COW for that nested collection.

**Example:**
```moo
m1 = ["a" -> ["inner" -> 1]];
m2 = m1;                      // Shares storage
m2["a"]["inner"] = 99;        // COW on m2["a"], not m2
// m1["a"]["inner"] is still 1
// m2["a"]["inner"] is 99
```

**Implementation note:** Each assignment triggers COW at that level:
1. `m2["a"]` retrieves reference to inner map
2. `["inner"] = 99` mutates the inner map
3. Inner map COW creates new instance
4. Outer map remains shared
```

---

### GAP-018: mapmerge Order

**Feature:** `mapmerge` result order
**Spec File:** `spec/builtins/maps.md` lines 102-117
**Gap Type:** ASSUME

**Question:**
When `mapmerge` combines two maps, what is the iteration order of the result?

```moo
m1 = ["a" -> 1, "b" -> 2];
m2 = ["c" -> 3, "d" -> 4];
merged = mapmerge(m1, m2);
// Iteration order: a, b, c, d? Or based on hash? Or undefined?
```

The spec says "map2 values override map1" (line 106) for duplicates, but doesn't address ordering.

**Impact:**
If order is observable, implementors need to know the rule.

**Suggested Addition:**
Add to `maps.md` section 3.1:
```
**Order:** The iteration order of the result is unspecified (consistent with section 7). Portable code must not depend on order.
```

---

### GAP-019: mapdelete on Non-Existent Key

**Feature:** `mapdelete` behavior
**Spec File:** `spec/builtins/maps.md` lines 66-80
**Gap Type:** CLARIFICATION NEEDED

**Question:**
The spec shows `mapdelete(["a" -> 1], "x")` returns `["a" -> 1]` (no change, line 75). Is this:
- (a) Returns the SAME map object (reference equality)
- (b) Returns a NEW map object with same contents (value equality)

Due to COW semantics, it likely returns the same object, but this should be explicit for optimization.

**Impact:**
Performance: Implementations can optimize by returning same reference vs creating new map.

**Suggested Addition:**
Update `maps.md` section 2.3:
```
**Efficiency:** If the key does not exist, returns the original map (same reference). No copy is created.

**Example:**
```moo
m1 = ["a" -> 1];
m2 = mapdelete(m1, "x");
// m1 and m2 are the same object (reference equality)
```
```

---

### GAP-020: Performance Table Missing mapmerge

**Feature:** Performance documentation
**Spec File:** `spec/builtins/maps.md` lines 315-325
**Gap Type:** MISSING

**Question:**
The performance table (section 11) is missing several operations:
- `mapmerge`: What is the complexity?
- `mapslice`: What is the complexity?
- `mklist`: What is the complexity?
- `mkmap`: What is the complexity?
- `maphaskey`: What is the complexity?

**Impact:**
Performance-conscious implementors need this information.

**Suggested Addition:**
Update the performance table to include:

| Operation | Complexity |
|-----------|------------|
| Access | O(1) average |
| Insert | O(1) average (O(n) copy due to COW) |
| Delete | O(n) copy due to COW |
| mapkeys | O(n) |
| mapvalues | O(n) |
| mapdelete | O(n) copy (or O(1) if key missing) |
| mapmerge | O(n + m) copy, where n = len(map1), m = len(map2) |
| mapslice | O(k * log n) where k = len(keys), n = len(map) |
| mklist | O(n) |
| mkmap | O(n) where n = len(list) |
| maphaskey | O(1) average |
| Iteration | O(n) |
| `in` test | O(1) average |

---

## Summary of Gaps

| ID | Type | Severity | Description |
|----|------|----------|-------------|
| GAP-001 | CONFLICT | CRITICAL | Contradiction: types.md says INT/STR keys only, maps.md says 7+ types |
| GAP-002 | GUESS | HIGH | Float key equality (NaN, -0.0 vs +0.0) undefined |
| GAP-003 | ASSUME | MEDIUM | List/Map key deep equality not specified |
| GAP-004 | ASSUME | MEDIUM | Iteration order guarantees vague ("deterministic" but how?) |
| GAP-005 | ASK | HIGH | Mutation during iteration behavior unspecified |
| GAP-006 | ASSUME | LOW | Empty map iteration not shown in examples |
| GAP-007 | GUESS | LOW | Loop variable values after iteration not specified |
| GAP-008 | ASK | MEDIUM | mapslice duplicate key handling and error reporting unclear |
| GAP-009 | GUESS | MEDIUM | mkmap invalid pair error cases not exhaustive |
| GAP-010 | GUESS | MEDIUM | mkmap duplicate key handling not specified |
| GAP-011 | ASSUME | LOW | mklist output order not specified |
| GAP-012 | ASSUME | MEDIUM | mapkeys/mapvalues correspondence not guaranteed |
| GAP-013 | GUESS | MEDIUM | Map access with wrong key type: E_RANGE or E_TYPE? |
| GAP-014 | ASSUME | MEDIUM | Map assignment creating entries should be explicit |
| GAP-015 | GUESS | LOW | maphaskey vs `in` equivalence not stated |
| GAP-016 | ASSUME | MEDIUM | Map equality order-independence not explicit |
| GAP-017 | ASSUME | MEDIUM | Nested map COW behavior not documented |
| GAP-018 | ASSUME | LOW | mapmerge result order not specified |
| GAP-019 | CLARIFICATION | LOW | mapdelete on missing key: same object or new? |
| GAP-020 | MISSING | LOW | Performance table incomplete |

---

## Blockers for Implementation

The following gaps MUST be resolved before accurate implementation:

1. **GAP-001 (CRITICAL):** Resolve key type contradiction
2. **GAP-002 (HIGH):** Define float key equality semantics
3. **GAP-005 (HIGH):** Specify mutation-during-iteration behavior
4. **GAP-012 (MEDIUM):** Guarantee mapkeys/mapvalues correspondence

All other gaps can be assumed/guessed with reasonable defaults, but the above 4 prevent confident implementation.
