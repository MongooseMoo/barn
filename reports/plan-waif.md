# Waif Nested Map Indexing Fix Plan

## Executive Summary

**Problem**: Two conformance tests fail with `E_PROPNF` when accessing nested maps via waif properties.

**Root Cause**: Waif property assignment creates a new waif (copy-on-write) but discards it instead of updating the variable. When code later tries to access the property, it's not found.

**Solution**: Modify `assignWaifProperty()` to return the new waif, and update `assignProperty()` to store it back to the variable.

**Complexity**: MEDIUM (2.5-3.5 hours estimated)

**Impact**: Fixes 2 failing tests, enables nested map indexing on waif properties.

---

## Investigation Date
2025-12-30

## Failing Tests

### 1. `waif::nested_waif_map_indexes`
**Location**: `C:\Users\Q\code\cow_py\tests\conformance\language\waif.yaml:200-211`

**Test Code**:
```moo
waif_class = create($waif);
add_property(waif_class, ":data", [], {player, ""});
w = waif_class:new();
w.data = ["outer" -> ["inner" -> "value"]];
return w.data["outer"]["inner"];
```

**Expected**: `"value"`
**Actual**: `E_PROPNF` (Property Not Found)

### 2. `waif::deeply_nested_waif_map_indexes`
**Location**: `C:\Users\Q\code\cow_py\tests\conformance\language\waif.yaml:213-224`

**Test Code**:
```moo
waif_class = create($waif);
add_property(waif_class, ":data", [], {player, ""});
w = waif_class:new();
w.data = ["a" -> ["b" -> ["c" -> 42]]];
return w.data["a"]["b"]["c"];
```

**Expected**: `42`
**Actual**: `E_PROPNF` (Property Not Found)

## Verification

### Python Implementation (cow_py) - PASSES
Both tests pass when run with `--transport direct` using the Python MOO interpreter:
```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ -k "nested_waif_map" --transport direct -xvs
# Result: 2 passed
```

### Barn Implementation - FAILS
Both tests fail when run against Barn server with `E_PROPNF`:
```bash
uv run pytest tests/conformance/ -k "nested_waif_map" --transport socket --moo-port 9300 -xvs
# Result: 1 failed with E_PROPNF
```

## Root Cause Analysis

### Expected Behavior
When evaluating `w.data["outer"]["inner"]`:
1. Parse into AST: `IndexExpr(IndexExpr(PropertyExpr(w, "data"), "outer"), "inner")`
2. Evaluate outer `IndexExpr`:
   - Evaluate its `Expr` field: `IndexExpr(PropertyExpr(w, "data"), "outer")`
   - This should evaluate the inner `IndexExpr`:
     - Evaluate `PropertyExpr(w, "data")` → returns map value `["outer" -> ["inner" -> "value"]]`
     - Index that map with `"outer"` → returns map `["inner" -> "value"]`
   - Result: map `["inner" -> "value"]`
3. Index that result with `"inner"` → returns `"value"`

### Current Barn Behavior
Barn returns `E_PROPNF` which indicates "Property Not Found". This error code is returned by property access operations, not indexing operations.

### Critical Insight
The error `E_PROPNF` strongly suggests that Barn is treating the chained indexing as a property access rather than a map index operation. This could happen if:

1. **Hypothesis 1**: The parser is incorrectly parsing `w.data["outer"]` as something other than `IndexExpr(PropertyExpr(w, "data"), "outer")`

2. **Hypothesis 2**: There's a special case in property evaluation that doesn't properly handle when a property value is immediately indexed

3. **Hypothesis 3**: The waif property read path (`waifProperty()`) is not returning the actual value, but some proxy or reference that cannot be indexed

### Code Investigation

#### Relevant Files
- `C:\Users\Q\code\barn\vm\properties.go` - Property access for waifs
- `C:\Users\Q\code\barn\vm\indexing.go` - Collection indexing
- `C:\Users\Q\code\barn\types\waif.go` - Waif value type
- `C:\Users\Q\code\barn\parser\ast.go` - AST node definitions

#### Key Function: `waifProperty()` (properties.go:417-468)
```go
func (e *Evaluator) waifProperty(waif types.WaifValue, node *parser.PropertyExpr, ctx *types.TaskContext) types.Result {
    // ... property name resolution ...

    // Check waif's own properties first
    if val, ok := waif.GetProperty(propName); ok {
        return types.Ok(val)  // Returns the actual value
    }

    // Fall back to class object's properties
    classID := waif.Class()
    classObj := e.store.Get(classID)
    if classObj == nil {
        return types.Err(types.E_PROPNF)
    }

    prop, errCode := e.findProperty(classObj, propName, ctx)
    if errCode != types.E_NONE {
        return types.Err(errCode)
    }

    return types.Ok(prop.Value)  // Returns the actual value
}
```

This function DOES return the actual property value, not a reference or proxy.

#### Key Function: `index()` (indexing.go:12-66)
```go
func (e *Evaluator) index(node *parser.IndexExpr, ctx *types.TaskContext) types.Result {
    // Evaluate the expression being indexed
    exprResult := e.Eval(node.Expr, ctx)
    if !exprResult.IsNormal() {
        return exprResult
    }

    expr := exprResult.Val

    // Get collection length for $ and ^ resolution in sub-expressions
    length := getCollectionLength(expr)
    if length < 0 {
        return types.Err(types.E_TYPE) // Not a collection
    }

    // ... evaluate index ...

    // Dispatch based on collection type
    switch coll := expr.(type) {
    case types.ListValue:
        return listIndex(coll, index)
    case types.StrValue:
        return strIndex(coll, index)
    case types.MapValue:
        return mapIndex(coll, index)
    default:
        return types.Err(types.E_TYPE)
    }
}
```

This looks correct - it evaluates the expression (which could be a PropertyExpr), gets the value, and indexes into it.

### The Mystery Deepens

The code path looks correct:
1. `PropertyExpr` is evaluated via `waifProperty()` and returns the actual map value
2. `IndexExpr` takes that map value and indexes into it
3. The result can be indexed again

But we're getting `E_PROPNF`. Where is this error coming from?

### New Hypothesis: Parser Issue?

Let me check if the parser might be treating `w.data["outer"]` differently. The key is whether the parser creates:
- Correct: `IndexExpr(PropertyExpr(w, "data"), "outer")`
- Wrong: Something else that would trigger `E_PROPNF`

Actually, looking at the parser code isn't shown in the files I've read. Let me think about what could cause `E_PROPNF` specifically:

1. **From `waifProperty()`** - Line 458-465: Returns `E_PROPNF` if property not found on waif or class
2. **From `property()`** - Line 71: Returns `E_PROPNF` if property not found
3. **From `findProperty()`** - Returns `E_PROPNF` if property doesn't exist

### Critical Realization

Wait! I need to re-examine the test failure. It says the test returns `E_PROPNF`, but I haven't traced through what happens when we try to READ (not assign) `w.data["outer"]`.

Let me think about the evaluation order again for `return w.data["outer"]["inner"];`:

1. Outer `IndexExpr`: `expr=IndexExpr(PropertyExpr(w, "data"), "outer")`, `index="inner"`
2. Call `index()` on the outer IndexExpr
3. Evaluate `node.Expr` which is the inner IndexExpr
4. Call `index()` on inner IndexExpr with `expr=PropertyExpr(w, "data")`, `index="outer"`
5. Evaluate `node.Expr` which is `PropertyExpr(w, "data")`
6. Call `property()` → `waifProperty()`
7. Returns the map `["outer" -> ["inner" -> "value"]]`
8. Back in step 4, index that map with "outer"
9. Returns the map `["inner" -> "value"]`
10. Back in step 2, index that map with "inner"
11. Returns `"value"`

This should work!

### Testing Hypothesis

I need to create a minimal test case to see exactly where the error occurs. Let me check if simpler cases work:
- `w.data` - Does this work?
- `m = w.data; m["outer"]` - Does this work?
- `w.data["outer"]` - Does this work?

## ACTUAL Root Cause

After re-examining the error, I realize I need to look at the ASSIGNMENT path, not just the read path! The test does:
```moo
w.data = ["outer" -> ["inner" -> "value"]];
```

Then it reads:
```moo
return w.data["outer"]["inner"];
```

If the ASSIGNMENT is failing or not working correctly, then the property won't have the right value, and subsequent reads would fail.

But wait - the error is `E_PROPNF`, not `E_RANGE` or `E_TYPE`. So the property itself isn't found, not that indexing failed.

### Testing Simple Property Assignment

Looking at the test output from my earlier attempt:
```
; waif_class = create($waif);
{1, 0}
; add_property(waif_class, ":data", [], {player, ""});
{2, {E_VARNF, "", 0}}
```

The `add_property` failed with `E_VARNF` (variable not found) because `waif_class` wasn't persisted between commands! This is a connection/state management issue, not a code logic issue.

### Real Root Cause Found!

The conformance test uses a **setup** block that runs first, then a **statement** block. Looking at the test again:

```yaml
setup:
  code: |
    waif_class = create($waif);
    add_property(waif_class, ":data", [], {player, ""});
statement: |
  w = waif_class:new();
  w.data = ["outer" -> ["inner" -> "value"]];
  return w.data["outer"]["inner"];
```

So:
1. Setup creates the waif class and adds the property
2. Statement creates a waif instance, assigns to `.data`, then reads nested

If the error is `E_PROPNF`, it means either:
- The property `:data` wasn't added correctly
- The waif doesn't have the property
- Something about reading `w.data` is failing

### Aha! The Real Issue

Looking at `waifProperty()` again at line 449-452:
```go
// Check waif's own properties first
if val, ok := waif.GetProperty(propName); ok {
    return types.Ok(val)
}
```

This checks the waif's instance properties (set via `w.data = ...`).

Then at line 454-467:
```go
// Fall back to class object's properties
classID := waif.Class()
classObj := e.store.Get(classID)
if classObj == nil {
    return types.Err(types.E_PROPNF)
}

prop, errCode := e.findProperty(classObj, propName, ctx)
if errCode != types.E_NONE {
    return types.Err(errCode)
}

return types.Ok(prop.Value)
```

This falls back to the class's property **definition** but returns the class's property **value**, not the instance's value!

**THIS IS THE BUG!** When a waif doesn't have an instance property set, it should return the class's **default value** from the property definition, but the instance should be able to override it. However, the code is correct here - it checks instance first, then class.

Wait, let me re-read this. Actually, if the waif has the property set (via `w.data = ...`), then line 450-451 should return it. So that's correct.

### Let Me Test The Actual Expression

Actually, I think I need to test this more carefully. Let me check if the issue is in how Barn handles waif property ASSIGNMENT vs READING when indexed.

Looking back at the assignment code in `assignWaifProperty()` at properties.go:473, this handles `w.data = value`.

But what about `w.data["outer"]`? This is a READ operation that returns a value, which is then indexed.

**AHA! FOUND IT!**

When we do `return w.data["outer"]["inner"]`, the parsing is:
```
IndexExpr(
  IndexExpr(
    PropertyExpr(w, "data"),
    "outer"
  ),
  "inner"
)
```

This is correct. But here's the problem: **What if the PropertyExpr returns something that CAN'T be indexed?**

Let me check the WaifValue.GetProperty() implementation:

From waif.go:61-65:
```go
// GetProperty returns a property value by name
func (w WaifValue) GetProperty(name string) (Value, bool) {
    val, ok := w.properties[name]
    return val, ok
}
```

This returns the value if it exists in the waif's instance properties map. If the waif has `data` set to a map, this should return that map.

### Final Theory: Testing Simple vs Complex Maps

Maybe the issue is with how complex/nested maps are being stored or retrieved from waifs? Let me check if there's any special handling needed.

Actually, I think I need to just run a proper debug trace through Barn to see where exactly `E_PROPNF` is being returned. But I can infer based on the code that the issue is likely:

**The waif's property is not being found when accessed.** This could be because:
1. The property wasn't set correctly during `w.data = [...]`
2. The property name resolution is failing
3. There's an issue with how colon-prefixed properties (`:data`) are handled

Let me check if there's special handling for `:data` vs `data`:

Looking at the test:
```moo
add_property(waif_class, ":data", [], {player, ""});
```

The property is defined with a colon prefix `:data`. But when accessing it:
```moo
w.data["outer"]
```

We use `data` without the colon. This is standard MOO behavior - the colon indicates it's defined on the class, but accessed without the colon.

### ACTUAL FINDING

After all this analysis, I believe the issue is:

**Barn is not correctly resolving class-defined properties for waif instances when those properties are accessed and then indexed in a chained expression.**

The property `:data` is defined on the waif CLASS (via `add_property`), but when we access `w.data`, Barn needs to:
1. Check if the waif instance has an override for `data`
2. If not, check if the waif's class has a property definition for `:data`
3. Return the waif's instance value if set, or the class's default value if not

The bug is likely in step 2 or 3 - Barn may not be correctly looking up class-defined properties when the waif instance hasn't explicitly set them yet.

But wait - the test DOES set the property:
```moo
w.data = ["outer" -> ["inner" -> "value"]];
```

So the waif instance SHOULD have `data` in its properties map after this assignment.

### Back to Assignment

Let me check `assignWaifProperty()` at line 473-533:

```go
func (e *Evaluator) assignWaifProperty(waif types.WaifValue, node *parser.PropertyExpr, value types.Value, ctx *types.TaskContext) types.Result {
    // ... property name resolution ...

    // Check if trying to set built-in properties
    switch propName {
    case "owner", "class", "wizard", "programmer":
        return types.Err(types.E_PERM)
    }

    // Check for circular references (waif containing itself)
    if containsValue(value, waif) {
        return types.Err(types.E_RECMOVE)
    }

    // Set the property (copy-on-write)
    newWaif := waif.SetProperty(propName, value)

    // ... update variable with new waif ...
}
```

The key line is `newWaif := waif.SetProperty(propName, value)`. This creates a new waif with the property set.

But then what? The function needs to UPDATE THE VARIABLE that holds the waif! Let me see the rest of the function...

Actually, I realize I need to look at the full function. Let me read it:

## Files to Examine More Carefully

1. `C:\Users\Q\code\barn\vm\properties.go` - Lines 473-533 for `assignWaifProperty()`
2. Check how the waif variable is updated after property assignment

## Implementation Plan

### Phase 1: Identify Exact Failure Point (HIGH PRIORITY)
**Estimated Complexity: LOW**

1. Add debug logging to `waifProperty()` to see what property name is being looked up
2. Add debug logging to show what properties the waif instance has
3. Add debug logging to show what properties the waif's class has
4. Run the failing test and examine logs to see exactly where `E_PROPNF` is returned

**Files**:
- `vm/properties.go` - Add debug prints in `waifProperty()`

### Phase 2: Fix Property Assignment for Waifs (MEDIUM PRIORITY)
**Estimated Complexity: MEDIUM**

The issue may be that when we do `w.data = value`, the assignment creates a new waif but doesn't update the variable `w` correctly.

Looking at the assignWaifProperty code, I need to see how it updates the variable. The problem is that waifs are immutable (copy-on-write), so:
```go
newWaif := waif.SetProperty(propName, value)
```

This creates a NEW waif, but the old waif in the variable `w` still has the old properties. The code must update the variable somehow.

**Files**:
- `vm/properties.go` - Lines 473-533, need to see full `assignWaifProperty()` implementation
- Check if there's a mechanism to update the variable holding the waif

### Phase 3: Fix Nested Indexing (if needed) (LOW PRIORITY)
**Estimated Complexity: LOW**

If the property assignment is working correctly but nested indexing still fails, then:

1. Check if `index()` properly handles values returned from property access
2. Ensure that chained indexing works for all value types, not just variables

**Files**:
- `vm/indexing.go` - `index()` function

### Phase 4: Add Test Coverage (LOW PRIORITY)
**Estimated Complexity: LOW**

1. Add Go unit tests for waif property assignment
2. Add Go unit tests for nested map indexing on waif properties
3. Ensure conformance tests pass

**Files**:
- Create `vm/waif_test.go` or similar

## CONFIRMED Root Cause

**FOUND AT**: `vm/properties.go:502-507`

The `assignWaifProperty()` function contains this code:

```go
// NOTE: This doesn't actually persist the change in barn's current architecture
// because waifs are immutable values. The calling code needs to handle updating
// the variable that holds the waif.
// For now, we'll just return success - the actual persistence will be handled
// by the assignment expression evaluator.
_ = waif.SetProperty(propName, value)

return types.Ok(value)
```

**The problem**: The new waif with the property set is DISCARDED (notice `_ =`). The function returns success, but the waif remains unchanged.

When the test does:
```moo
w.data = ["outer" -> ["inner" -> "value"]];
```

The assignment appears to succeed, but the waif `w` still has no `data` property set. Later, when we try to read:
```moo
return w.data["outer"]["inner"];
```

Barn tries to access `w.data`, which doesn't exist, so it returns `E_PROPNF`.

### Why This Happens

Waifs in Barn use immutable value semantics (copy-on-write). When you set a property:
```go
newWaif := waif.SetProperty(propName, value)
```

This creates a NEW waif with the property set. The original waif is unchanged.

For property assignment to work, the code must:
1. Create the new waif with the property set
2. Update the variable that holds the waif with the new value

Currently, step 1 happens but the result is discarded. Step 2 never happens.

### The Fix

The `assignProperty()` function needs to detect when it's assigning to a waif stored in a variable, and update that variable with the new waif value.

For `w.data = value` where `w` is a variable:
1. Evaluate `w` → get waif value
2. Create new waif with `data` property set
3. Store new waif back to variable `w`

The challenge is that `node.Expr` could be:
- `IdentifierExpr` (simple variable like `w`) - Easy, update the variable
- `PropertyExpr` (like `obj.member`) - Complex, need to update nested structure
- `IndexExpr` (like `list[1]`) - Complex, need copy-on-write update

For the failing tests, `node.Expr` is an `IdentifierExpr`, so the fix is straightforward.

## Summary

### Root Cause (CONFIRMED)
Waif property assignment (`w.data = value`) creates a new waif but **discards it** instead of updating the variable. The variable still holds the old waif without the property set, causing `E_PROPNF` when the property is accessed later.

### Next Steps
1. **Fix `assignProperty()` to handle waif variable updates** (CRITICAL)
2. Verify fix with conformance tests
3. Ensure nested cases work (if `node.Expr` is complex)

### Estimated Total Complexity: MEDIUM

The fix requires:
1. Detecting when `node.Expr` is an `IdentifierExpr` (variable)
2. Creating the new waif with the property set
3. Updating the variable with the new waif

For more complex cases (nested properties, indexed access), additional work may be needed, but those aren't required for the failing tests.

## Detailed Implementation Plan

### Step 1: Modify `assignWaifProperty()` to Return the New Waif
**File**: `vm/properties.go:473-510`
**Complexity**: LOW

Change the function signature to return both the result AND the new waif:
```go
func (e *Evaluator) assignWaifProperty(waif types.WaifValue, node *parser.PropertyExpr, value types.Value, ctx *types.TaskContext) (types.WaifValue, types.Result)
```

Change line 507 from:
```go
_ = waif.SetProperty(propName, value)
return types.Ok(value)
```

To:
```go
newWaif := waif.SetProperty(propName, value)
return newWaif, types.Ok(value)
```

### Step 2: Update `assignProperty()` to Handle Waif Updates
**File**: `vm/properties.go:194-204`
**Complexity**: MEDIUM

Currently:
```go
// Check if result is a waif
if waifVal, ok := objResult.Val.(types.WaifValue); ok {
    return e.assignWaifProperty(waifVal, node, value, ctx)
}
```

Change to:
```go
// Check if result is a waif
if waifVal, ok := objResult.Val.(types.WaifValue); ok {
    newWaif, result := e.assignWaifProperty(waifVal, node, value, ctx)
    if !result.IsNormal() {
        return result
    }

    // Update the variable that holds the waif
    // For now, only handle simple identifier case: w.data = value
    if ident, ok := node.Expr.(*parser.IdentifierExpr); ok {
        e.env.Set(ident.Name, newWaif)
    }
    // TODO: Handle complex cases (nested properties, indexed access)

    return result
}
```

### Step 3: Test the Fix
**Complexity**: LOW

Run the failing conformance tests:
```bash
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9300 > test_waif_nested.log 2>&1 &
sleep 2

cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ -k "nested_waif_map" --transport socket --moo-port 9300 -xvs
```

Expected: Both tests should pass.

### Step 4: Handle Complex Cases (Future Work)
**Complexity**: HIGH

The simple fix above only works for `w.data = value` where `w` is a variable.

For complex cases like:
- `list[1].data = value` - Waif stored in a list
- `obj.member.data = value` - Waif stored as a property
- `map["key"].data = value` - Waif stored in a map

These require copy-on-write propagation back up the chain, similar to how `nestedAssign()` works in `indexing.go:317-416`.

However, these complex cases are NOT needed for the failing tests, so they can be deferred.

### Step 5: Handle `waifPropertyIndexedAssign()` (Related Issue)
**File**: `vm/properties.go:535-700`
**Complexity**: MEDIUM

This function has a similar issue - it creates a modified waif but doesn't update the variable. The same fix pattern applies:

1. Return the new waif from the function
2. Update the variable in the caller

However, this is a separate issue from the failing tests and can be addressed later.

## Files That Need Changes

### Primary Files
1. **`vm/properties.go`** (CRITICAL)
   - Line 473-510: Modify `assignWaifProperty()` signature and implementation
   - Line 194-204: Update `assignProperty()` to handle waif variable updates

### Test Files
2. **Run conformance tests** to verify the fix

### Documentation
3. **This plan document** (already updated)

## Testing Strategy

### Unit Testing
Create a simple Go test to verify waif property assignment:
```go
func TestWaifPropertyAssignment(t *testing.T) {
    // Create waif
    waif := types.NewWaif(types.ObjID(100), types.ObjID(2))

    // Set property
    waif = waif.SetProperty("data", types.NewList([]types.Value{
        types.NewInt(1), types.NewInt(2),
    }))

    // Verify property is set
    val, ok := waif.GetProperty("data")
    if !ok {
        t.Fatal("Property not found")
    }

    list, ok := val.(types.ListValue)
    if !ok {
        t.Fatal("Property is not a list")
    }

    if list.Len() != 2 {
        t.Fatalf("Expected list length 2, got %d", list.Len())
    }
}
```

### Integration Testing
Run the conformance tests:
```bash
uv run pytest tests/conformance/ -k "waif" --transport socket --moo-port 9300 -v
```

Expected results:
- `nested_waif_map_indexes` - PASS
- `deeply_nested_waif_map_indexes` - PASS
- All other waif tests - Should still pass (no regressions)

## Risk Assessment

### Low Risk Changes
- Modifying `assignWaifProperty()` to return the new waif
- Updating the simple identifier case in `assignProperty()`

### Medium Risk Changes
- Ensuring the fix doesn't break existing waif tests
- Handling edge cases (nil checks, etc.)

### High Risk (Deferred)
- Implementing complex nested waif updates (list[1].data, etc.)

## Estimated Effort

- **Implementation**: 1-2 hours
- **Testing**: 1 hour
- **Documentation**: 30 minutes

**Total**: 2.5-3.5 hours

## Success Criteria

1. Both failing tests pass:
   - `waif::nested_waif_map_indexes`
   - `waif::deeply_nested_waif_map_indexes`

2. No regressions in other waif tests

3. Code is clean and well-commented

4. The fix handles the common case (variable.property = value) correctly
