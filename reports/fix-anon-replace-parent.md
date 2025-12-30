# Fix: Anonymous Object Type Reporting

## Problem

Test `replace_parent_does_not_corrupt_anonymous` fails with:
- Expected: `typeof(a) == OBJ` returns 1 (true)
- Actual: `typeof(a) == OBJ` returns 0 (false)

Where `a` is an anonymous object created with `create(p, 1)`.

## Investigation

### Current Barn Behavior

```bash
# Anonymous object typeof()
; a = create($nothing, 1); return typeof(a);
=> 12  # TYPE_ANON

# Regular object typeof()
; a = create($nothing, 0); return typeof(a);
=> 1   # TYPE_OBJ

# Test comparison
; a = create($nothing, 1); return typeof(a) == OBJ;
=> 0   # false

; a = create($nothing, 1); return typeof(a) == ANON;
=> 1   # true
```

### Conformance Test Evidence

From `cow_py/tests/conformance/builtins/create.yaml`:

```yaml
- name: second_arg_1_creates_anonymous
  code: "typeof(create($nothing, 1))"
  expect:
    value: 12  # TYPE_ANON

- name: second_arg_0_creates_object
  code: "typeof(create($nothing, 0))"
  expect:
    value: 1  # TYPE_OBJ
```

These tests PASS in barn, confirming that:
- Anonymous objects return TYPE_ANON (12)
- Regular objects return TYPE_OBJ (1)

### Toast Behavior (Unexpected)

When testing against ToastStunt server:

```bash
; a = create($nothing, 1); return typeof(a);
=> *anonymous*  # Returns a string representation, not a type code!

; a = create($nothing, 0); return typeof(a);
=> #129  # Returns the object itself, not a type code!

; return typeof(123);
=> 0  # TYPE_INT - works correctly for integers
```

**This suggests Toast may have verb overrides or a different typeof() implementation for objects.**

### Type Code Definitions

From `types/typecode.go`:
```go
const (
    TYPE_INT   TypeCode = 0
    TYPE_OBJ   TypeCode = 1
    TYPE_STR   TypeCode = 2
    ...
    TYPE_ANON  TypeCode = 12
)
```

From `types/obj.go`:
```go
func (o ObjValue) Type() TypeCode {
    if o.anonymous {
        return TYPE_ANON  // Returns 12
    }
    return TYPE_OBJ  // Returns 1
}
```

## Root Cause Analysis

There are three possible interpretations:

### Hypothesis 1: Test Bug
The test expectation is wrong. It should expect:
```yaml
return typeof(a) == ANON;  # Not OBJ
expect:
  value: 1
```

**Evidence FOR:**
- All create.yaml tests expect TYPE_ANON (12) for anonymous objects
- Barn's implementation matches this expectation
- The test name mentions "replace_parent" but no parent replacement occurs in the code
- Test may have been incorrectly translated from Ruby original

**Evidence AGAINST:**
- Test was explicitly written with this expectation
- Multiple conformance tests in cow_py repository use this pattern

### Hypothesis 2: Barn Bug
Anonymous objects should report as TYPE_OBJ (1), not TYPE_ANON (12).

**Evidence FOR:**
- ToastStunt's `is_object()` method considers both TYPE_OBJ and TYPE_ANON as objects
- MOO semantics may treat anonymous objects as a subtype of objects

**Evidence AGAINST:**
- create.yaml tests explicitly expect TYPE_ANON (12)
- Those tests pass in barn
- TYPE_ANON exists specifically to distinguish anonymous objects
- Toast server returned strange results (string/object instead of type codes)

### Hypothesis 3: typeof() Should Normalize
The `typeof()` builtin should return TYPE_OBJ for BOTH regular and anonymous objects, but some internal type-checking mechanism needs TYPE_ANON.

**Evidence FOR:**
- Would make the failing test pass
- Aligns with "objects are objects" philosophy

**Evidence AGAINST:**
- Contradicts all the create.yaml tests
- Removes the ability to distinguish anonymous objects via typeof()
- No documentation supports this

## Recommended Fix

**Fix the test, not the implementation.**

The test appears to have incorrect expectations. Based on:
1. All other conformance tests expect TYPE_ANON for anonymous objects
2. Barn's implementation correctly distinguishes object types
3. The test name suggests it should test parent replacement (which doesn't happen)

### Option A: Fix Test Expectation

```yaml
- name: replace_parent_does_not_corrupt_anonymous
  permission: wizard
  description: "Replacing parent object doesn't corrupt anonymous child state"
  statement: |
    p = create($nothing);
    a = create(p, 1);
    return typeof(a) == ANON;  # Changed from OBJ
  expect:
    value: 1
```

### Option B: Fix Test Code

Add actual parent replacement:

```yaml
- name: replace_parent_does_not_corrupt_anonymous
  permission: wizard
  description: "Replacing parent object doesn't corrupt anonymous child state"
  statement: |
    p1 = create($nothing);
    p2 = create($nothing);
    a = create(p1, 1);
    chparents(a, {p2});  # Actually replace parent
    return typeof(a) == ANON && valid(a);
  expect:
    value: 1
```

## Alternative: If Barn Implementation Needs Change

If further evidence shows that `typeof()` should return OBJ for anonymous objects:

### Changes Required

**File: `types/obj.go`**
```go
// Type returns the MOO type
// Both regular and anonymous objects return TYPE_OBJ (1)
func (o ObjValue) Type() TypeCode {
    return TYPE_OBJ  // Always return TYPE_OBJ, regardless of anonymous flag
}
```

**But this would break create.yaml tests:**
- `second_arg_1_creates_anonymous` expects 12
- `third_arg_1_creates_anonymous` expects 12

**Would need new builtin or property to check anonymity:**
```go
func builtinIsAnonymous(ctx *types.TaskContext, args []types.Value) types.Result {
    if len(args) != 1 {
        return types.Err(types.E_ARGS)
    }
    obj, ok := args[0].(types.ObjValue)
    if !ok {
        return types.Err(types.E_TYPE)
    }
    if obj.IsAnonymous() {
        return types.Ok(types.IntValue{Val: 1})
    }
    return types.Ok(types.IntValue{Val: 0})
}
```

## Decision

Based on the evidence, **the test is wrong**. The implementation should NOT be changed.

Barn correctly implements:
- `typeof(regular_object)` → 1 (TYPE_OBJ)
- `typeof(anonymous_object)` → 12 (TYPE_ANON)

This matches the conformance tests in create.yaml and provides a clear way to distinguish anonymous objects from regular objects.

## Verification

Ran all anonymous-related conformance tests against barn:
- **69 tests PASSED**
- **4 tests FAILED** (including `replace_parent_does_not_corrupt_anonymous`)

Key passing tests:
- `second_arg_1_creates_anonymous`: expects `typeof(create($nothing, 1))` → 12 ✓
- `second_arg_0_creates_object`: expects `typeof(create($nothing, 0))` → 1 ✓
- `third_arg_1_creates_anonymous`: expects `typeof(create($nothing, #1, 1))` → 12 ✓

These confirmthat barn correctly returns TYPE_ANON (12) for anonymous objects.

## Status

**No code changes made to barn.**

The test `replace_parent_does_not_corrupt_anonymous` has incorrect expectations and should be fixed in the cow_py conformance test suite.

### Recommended Test Fix

```yaml
- name: replace_parent_does_not_corrupt_anonymous
  permission: wizard
  description: "Anonymous objects maintain correct type"
  statement: |
    p = create($nothing);
    a = create(p, 1);
    return typeof(a) == ANON;  # Changed from OBJ to ANON
  expect:
    value: 1
```
