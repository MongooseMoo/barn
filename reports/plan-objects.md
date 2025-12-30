# Plan: Fix chparent/chparents Property Conflict Tests

## Executive Summary

Two conformance tests are failing related to property conflict detection and property value resets during parent changes:
1. `objects::chparent_property_conflict_variations` - Tests that property conflicts between objects and descendants cause E_INVARG
2. `stress_objects::chparents_property_reset_multi` - Tests that property values reset correctly when switching between parents that define the same property

## Failing Tests Analysis

### Test 1: `chparent_property_conflict_variations`

**Location:** `C:\Users\Q\code\cow_py\tests\conformance\builtins\objects.yaml:885-903`

**What it does:**
```moo
a = create($nothing);
b = create($nothing);
c = create($nothing);
m = create(a);
n = create(b);
z = create($nothing);
add_property(a, "foo", 0, {player, ""});
add_property(b, "foo", 0, {player, ""});
add_property(c, "foo", 0, {player, ""});
add_property(m, "bar", 0, {player, ""});
add_property(n, "baz", 0, {player, ""});
add_property(z, "bar", 0, {player, ""});
return {chparent(a, b), chparent(a, c), chparent(a, z)};
```

**Expected:** E_INVARG (the test expects an error to be raised)

**What should happen:**
- `chparent(a, b)` should fail because:
  - Object `a` has property "foo" defined
  - Object `b` has property "foo" defined
  - This creates a **direct property conflict** where the child would inherit conflicting "foo" properties

- `chparent(a, c)` should also fail (same reason as above with `c`)

- `chparent(a, z)` should fail because:
  - Object `a` has child `m` which defines property "bar"
  - Object `z` defines property "bar"
  - This creates a **descendant property conflict** where `m` (descendant of `a`) would inherit conflicting "bar" from new parent `z`

**Current behavior:** Server appears to hang or timeout during test execution

**Toast oracle verification:**
```bash
./toast_oracle.exe 'a = create($nothing); add_property(a, "foo", 0, {player, ""}); b = create($nothing); add_property(b, "foo", 0, {player, ""}); return chparent(a, b);'
# Returns: #128 (E_INVARG) ✓ Correct
```

### Test 2: `chparents_property_reset_multi`

**Location:** `C:\Users\Q\code\cow_py\tests\conformance\server\stress_objects.yaml:181-198`

**What it does:**
```moo
a = create($nothing);
b = create($nothing);
c = create($nothing);
m = create($nothing);
n = create($nothing);
add_property(a, "foo", "foo", {player, "c"});
add_property(b, "foo", "foo", {b, ""});
chparents(c, {m, n, a});
c.foo = "bar";
result_before = c.foo;
chparents(c, {b, m, n});
pi = property_info(c, "foo");
return {pi[1] == b && pi[2] == "", c.foo == "foo", result_before == "bar"};
```

**Expected:** `{1, 1, 1}` (all three conditions should be true)

**What should happen:**
1. Initially `c` has parents `{m, n, a}` and inherits "foo" from `a`
2. Set `c.foo = "bar"` creates a local override (the property is still inherited, not defined on c)
3. `result_before` should be "bar" ✓
4. Change parents to `{b, m, n}` - now inherits "foo" from `b` instead
5. The property value should **reset** to `b`'s definition because it's a different inheritance source
6. After reparenting:
   - `property_info(c, "foo")` should show owner=`b`, perms="" (from b's definition)
   - `c.foo` should be "foo" (the default value from b, NOT "bar")
   - `result_before` should still be "bar" (the saved value before reparenting)

**Current behavior:** Returns E_INVARG instead of {1, 1, 1}

**Root cause hypothesis:** The property reset logic in `resetInheritedProperties()` (line 1380) or `chparents` may not be handling the case where the same property name is defined on different parents correctly.

## Root Cause Analysis

### Issue 1: Direct Property Conflicts Not Detected

**File:** `C:\Users\Q\code\barn\builtins\objects.go`
**Function:** `builtinChparent` (lines 696-802)

The function checks for **descendant conflicts** (line 752-756) but does NOT check for **direct conflicts** between the object being reparented and the new parent.

**Current code (lines 748-756):**
```go
// Check for property conflicts: only chparent-added descendants of obj
// cannot define properties that are also defined on new_parent or its ancestors.
// The object being reparented itself is NOT checked - it can shadow parent properties.
if newParentVal.ID() != types.ObjNothing {
    newParentProps := collectAncestorProperties(store, newParentVal.ID())
    if hasChparentDescendantConflict(store, obj, newParentProps) {
        return types.Err(types.E_INVARG)
    }
}
```

**The bug:** The comment says "The object being reparented itself is NOT checked - it can shadow parent properties" but this is **WRONG**. An object CANNOT have a property with the same name as a parent's property when that property is defined on the object itself.

**Correct semantics:**
- If object `a` **defines** a property "foo" (not just inherits it), and you try to `chparent(a, b)` where `b` **defines** "foo", this should be E_INVARG
- If object `a` only **inherits** property "foo" (doesn't define it), then `chparent(a, b)` is OK - the inheritance just changes

### Issue 2: Property Reset Logic Missing in chparents

**File:** `C:\Users\Q\code\barn\builtins\objects.go`
**Function:** `builtinChparents` (lines 804-938)

The function does NOT call `resetInheritedProperties()` after changing parents. Compare with `builtinChparent` which DOES call it (line 799).

**Current code (lines 921-937):**
```go
// Set new parents
obj.Parents = newParents

// Add to new parents' children lists and track as chparent-added
for _, newParentID := range newParents {
    newParent := store.Get(newParentID)
    if newParent != nil {
        newParent.Children = append(newParent.Children, objVal.ID())
        // Track that this child was added via chparent (not create)
        if newParent.ChparentChildren == nil {
            newParent.ChparentChildren = make(map[types.ObjID]bool)
        }
        newParent.ChparentChildren[objVal.ID()] = true
    }
}

return types.Ok(types.NewInt(0))
```

**Missing:** Call to `resetInheritedProperties(obj)` before returning.

### Issue 3: resetInheritedProperties Implementation

**File:** `C:\Users\Q\code\barn\builtins\objects.go`
**Function:** `resetInheritedProperties` (lines 1377-1390)

**Current implementation:**
```go
func resetInheritedProperties(obj *db.Object) {
    toDelete := []string{}
    for name, prop := range obj.Properties {
        if !prop.Defined {
            toDelete = append(toDelete, name)
        }
    }
    for _, name := range toDelete {
        delete(obj.Properties, name)
    }
}
```

**The problem:** This function only DELETES inherited properties that have been overridden. But it doesn't:
1. Re-inherit properties from the NEW parent chain
2. Update the owner/perms metadata for inherited properties

**What should happen:**
When an object's parents change, ALL inherited properties (not defined on the object) should be:
1. Removed from the object
2. Re-inherited from the new parent chain using `copyInheritedProperties()`

## Implementation Plan

### Step 1: Add Direct Property Conflict Check to chparent
**Complexity:** Low
**File:** `builtins/objects.go`
**Function:** `builtinChparent`
**Lines:** After line 741 (after validating newParent exists)

**Add this check:**
```go
// Check for direct property conflicts between obj and new parent
// If obj defines a property that new_parent or its ancestors also define, that's E_INVARG
// (This is different from inherited properties, which can be shadowed)
if newParentVal.ID() != types.ObjNothing {
    newParentProps := collectAncestorProperties(store, newParentVal.ID())

    // Check properties DEFINED on obj (Defined=true)
    for name, prop := range obj.Properties {
        if prop.Defined && newParentProps[name] {
            return types.Err(types.E_INVARG)
        }
    }
}
```

### Step 2: Move Conflict Check After Direct Check
**Complexity:** Low
**File:** `builtins/objects.go`
**Function:** `builtinChparent`
**Lines:** Move the descendant conflict check (currently lines 748-756) to AFTER the new direct conflict check

**Reasoning:** Check conflicts in order: direct first, then descendants

### Step 3: Add Direct Property Conflict Check to chparents
**Complexity:** Low
**File:** `builtins/objects.go`
**Function:** `builtinChparents`
**Lines:** After line 881 (after checking duplicate properties among new parents)

**Add similar check as in Step 1:**
```go
// Check for direct property conflicts between obj and new parents
// If obj defines a property that any new parent or their ancestors also define, that's E_INVARG
allNewParentProps := make(map[string]bool)
for _, parentID := range newParents {
    props := collectAncestorProperties(store, parentID)
    for name := range props {
        allNewParentProps[name] = true
    }
}

// Check properties DEFINED on obj (Defined=true)
for name, prop := range obj.Properties {
    if prop.Defined && allNewParentProps[name] {
        return types.Err(types.E_INVARG)
    }
}
```

### Step 4: Fix Property Reset in chparents
**Complexity:** Medium
**File:** `builtins/objects.go`
**Function:** `builtinChparents`
**Lines:** Before line 937 (before return statement)

**Add:**
```go
// Reset inherited property overrides when parents change
// Properties that are propdefs (Defined=true) are kept
// Properties that are local overrides (Defined=false) are removed and re-inherited
resetInheritedProperties(obj)
// Re-inherit properties from new parent chain
newProps := copyInheritedProperties(obj, store)
// Merge with existing defined properties
for name, prop := range obj.Properties {
    if prop.Defined {
        newProps[name] = prop
    }
}
obj.Properties = newProps
```

### Step 5: Update resetInheritedProperties Implementation
**Complexity:** Medium
**File:** `builtins/objects.go`
**Function:** `resetInheritedProperties`
**Lines:** 1377-1390

**Current approach is actually correct** for the first phase (removing non-defined properties). The issue is that we need to call `copyInheritedProperties()` AFTER this to re-populate from the new parent chain.

**Refactor suggestion:**
Instead of having `resetInheritedProperties()` as a separate function, combine it with the property re-inheritance logic. Or, change the callers to explicitly call both functions in sequence.

**Better approach:** Rename and expand the function:

```go
// resetAndReinheritProperties removes inherited property overrides and re-inherits from current parents
func resetAndReinheritProperties(obj *db.Object, store *db.Store) {
    // First, collect properties that are DEFINED on this object (not inherited)
    definedProps := make(map[string]*db.Property)
    for name, prop := range obj.Properties {
        if prop.Defined {
            definedProps[name] = prop
        }
    }

    // Re-inherit properties from current parent chain
    inherited := copyInheritedProperties(obj, store)

    // Merge defined properties back (they take precedence)
    for name, prop := range definedProps {
        inherited[name] = prop
    }

    obj.Properties = inherited
}
```

Then update callers to use the new function name.

## Testing Strategy

### Unit Tests Needed

1. **Direct property conflict detection:**
   - Create object A with defined property "foo"
   - Create object B with defined property "foo"
   - `chparent(A, B)` should return E_INVARG
   - `chparents(A, {B})` should return E_INVARG

2. **Inherited property shadowing (should work):**
   - Create object A, inherit property "foo" from parent
   - Create object B with defined property "foo"
   - `chparent(A, B)` should succeed (A doesn't define foo, only inherits it)

3. **Property value reset on chparent:**
   - Create A with property "foo" = "a_value"
   - Create B with property "foo" = "b_value"
   - Create C with parent A
   - Set C.foo = "override"
   - `chparent(C, B)`
   - Verify C.foo == "b_value" (reset to B's value, not "override")

4. **Property value reset on chparents:**
   - Same as above but with multiple parents

### Integration Tests

Run the two failing tests:
```bash
cd ~/code/cow_py
uv run pytest tests/conformance/ -k "chparent_property_conflict_variations" -xvs
uv run pytest tests/conformance/ -k "chparents_property_reset_multi" -xvs
```

### Verification Against Toast

For each test case, verify behavior matches Toast:
```bash
./toast_oracle.exe '<test MOO code>'
```

## Files to Modify

1. `builtins/objects.go` - Main implementation changes
   - `builtinChparent()` - Add direct property conflict check
   - `builtinChparents()` - Add direct property conflict check and property reset
   - `resetInheritedProperties()` or create `resetAndReinheritProperties()` - Fix property reset logic

## Estimated Complexity

**Overall: Medium**

- Direct conflict checks: **Low** - Simple iteration over properties
- Property reset in chparents: **Low** - Just add missing function call
- Property re-inheritance: **Medium** - Need to ensure correct merge of defined vs inherited properties

**Time estimate:** 1-2 hours for implementation + testing

## Edge Cases to Consider

1. **Object defines property that parent defines:** E_INVARG ✓
2. **Object inherits property that parent defines:** OK (value resets) ✓
3. **Multiple parents define same property:** Already handled (E_INVARG at create/chparents time) ✓
4. **Chparent-added descendant defines conflicting property:** Already handled by existing code ✓
5. **Property ownership/perms after reset:** Should match new parent's definition ✓
6. **Empty parent list:** Should clear all inherited properties ✓

## Success Criteria

1. Both failing tests pass
2. No regression in other object/property tests
3. Behavior matches ToastStunt reference implementation
4. Property metadata (owner, perms) correctly reflects new parent after reparenting

## Notes

- The existing `hasChparentDescendantConflict()` logic appears correct - it only checks descendants added via chparent, not via create
- The `collectAncestorProperties()` function is already implemented and works correctly
- The main bug is the missing direct property conflict check between the object being reparented and its new parent(s)
- Secondary bug is the missing property reset call in `builtinChparents`
