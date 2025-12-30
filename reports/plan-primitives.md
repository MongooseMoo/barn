# Fix Plan: Primitives Conformance Tests

## Executive Summary

Two conformance tests are failing due to incomplete implementations of `queued_tasks()` and `callers()` when used with primitive prototype method calls:

1. **queued_tasks_includes_this_map**: `queued_tasks()` returns only 6 fields instead of required 10 fields
2. **callers_includes_this_list**: `callers()` returns object ID instead of primitive value for 'this' field

Both issues relate to how primitive prototype calls are tracked in task information and call stacks.

---

## Test 1: queued_tasks_includes_this_map

### What the Test Does

```moo
t = ["one" -> 1, "two" -> 2]:suspend();
q = queued_tasks();
kill_task(t);
return typeof(q[1][5]);
```

- Creates a forked task via `fork` inside map prototype's `:suspend()` verb
- Calls `queued_tasks()` to get task information
- Expects `q[1][5]` to be type 1 (OBJ) - this is the programmer field
- **Test currently fails with E_VARNF** because Barn only returns 6 fields, not 10

### Root Cause

**File:** `C:\Users\Q\code\barn\task\task.go`
**Function:** `ToQueuedTaskInfo()` (lines 349-364)

Current implementation returns only 6 fields:
```go
return types.NewList([]types.Value{
    types.NewInt(t.ID),              // [1] task_id
    types.NewInt(t.QueueTime.Unix()), // [2] start_time
    types.NewInt(0),                  // [3] x (unused)
    types.NewInt(0),                  // [4] y (unused)
    types.NewInt(0),                  // [5] z (unused)
    types.NewObj(t.Owner),            // [6] programmer
})
```

**Expected format** (based on ToastStunt src/tasks.cc:2365-2390):
```go
// 10-element list (11 if include_variables is true)
[1]  task_id       (INT)
[2]  start_time    (INT) - Unix timestamp
[3]  0             (INT) - obsolete clock ID
[4]  30000         (INT) - DEFAULT_BG_TICKS, obsolete
[5]  programmer    (OBJ)
[6]  verb_loc      (OBJ) - where verb is defined
[7]  verb_name     (STR)
[8]  line_number   (INT)
[9]  this          (OBJ/VALUE) - can be primitive!
[10] bytes         (INT) - memory usage estimate
[11] variables     (MAP, optional) - runtime variables
```

### Missing Information in Task Struct

The Task struct (lines 113-153) has some fields but needs access to the **top frame** of the call stack:

**Available in Task:**
- ID, QueueTime, Owner → OK
- VerbName, VerbLoc → available but for initial task, not current frame

**Available in top ActivationFrame (if call stack exists):**
- Verb, VerbLoc, LineNumber, ThisValue

**Missing completely:**
- Bytes (memory usage) - need to calculate or estimate

### Specific Changes Required

**File:** `C:\Users\Q\code\barn\task\task.go`
**Function:** `ToQueuedTaskInfo()`

```go
func (t *Task) ToQueuedTaskInfo() types.Value {
    t.mu.RLock()
    defer t.mu.RUnlock()

    // Get information from the top frame if call stack exists
    var verbName string
    var verbLoc types.ObjID
    var lineNumber int
    var thisVal types.Value
    var programmer types.ObjID

    if len(t.CallStack) > 0 {
        topFrame := t.CallStack[len(t.CallStack)-1]
        verbName = topFrame.Verb
        verbLoc = topFrame.VerbLoc
        lineNumber = topFrame.LineNumber
        programmer = topFrame.Programmer

        // Use ThisValue if set (primitive prototype call), else This (object ID)
        if topFrame.ThisValue != nil {
            thisVal = topFrame.ThisValue
        } else {
            thisVal = types.NewObj(topFrame.This)
        }
    } else {
        // Fallback if no call stack
        verbName = t.VerbName
        verbLoc = t.VerbLoc
        lineNumber = 1
        programmer = t.Owner
        thisVal = types.NewObj(t.This)
    }

    // Estimate bytes (can be 0 for now, or calculate if needed)
    bytes := int64(0) // TODO: Implement memory usage calculation

    return types.NewList([]types.Value{
        types.NewInt(t.ID),                    // [1] task_id
        types.NewInt(t.QueueTime.Unix()),      // [2] start_time
        types.NewInt(0),                        // [3] obsolete clock ID
        types.NewInt(30000),                    // [4] DEFAULT_BG_TICKS (obsolete)
        types.NewObj(programmer),               // [5] programmer
        types.NewObj(verbLoc),                  // [6] verb_loc
        types.NewStr(verbName),                 // [7] verb_name
        types.NewInt(int64(lineNumber)),        // [8] line_number
        thisVal,                                // [9] this (OBJ or primitive value)
        types.NewInt(bytes),                    // [10] bytes
    })
}
```

---

## Test 2: callers_includes_this_list

### What the Test Does

```moo
c = {1, 2, "three", 4.0}:foo();
return c[1][1];
```

- Calls `:foo()` on a list via list_proto, which calls `:bar()`
- `:bar()` returns `callers()`
- Expects `c[1][1]` (the 'this' field of first frame) to be the list `{1, 2, "three", 4.0}`
- **Test currently fails:** returns '#12' (object ID) instead of the list

### Root Cause

**The `ThisValue` field is NOT being set** when primitive prototype methods are called.

Looking at the code flow:

**File:** `C:\Users\Q\code\barn\vm\verbs.go`
**Function:** `verbCall()` (lines 11-206)

Lines 19-35 correctly detect primitive calls:
```go
var primitiveValue types.Value
isPrimitive := false

objVal, ok := objResult.Val.(types.ObjValue)
if ok {
    objID = objVal.ID()
} else {
    protoID := e.getPrimitivePrototype(objResult.Val)
    if protoID == types.ObjNothing {
        return types.Err(types.E_TYPE)
    }
    objID = protoID
    isPrimitive = true
    primitiveValue = objResult.Val  // Save the primitive value
}
```

Lines 123-125 correctly set ThisValue in the frame:
```go
frame := task.ActivationFrame{
    This:            defObjID,
    ThisValue:       primitiveValue,  // Store primitive value
    ...
}
```

**However**, when the primitive method calls ANOTHER method (`:foo()` calls `:bar()`), the second call goes through `verbCall()` again with a DIFFERENT context.

### The Problem: Context Propagation

When `:foo()` is called on the list:
1. `verbCall()` correctly sets `ThisValue` to the list value
2. `:foo()` executes and calls `:bar()` via another `verbCall()`
3. The **second** `verbCall()` evaluates `this` (line 13), which is now an **object** (the prototype), not the list
4. So `isPrimitive` is false, and `ThisValue` is not set

The issue is that once inside the prototype method, `this` refers to the prototype object, not the original primitive. But according to MOO semantics, `this` should STILL be the primitive value inside ALL nested calls within the prototype.

### Investigation Needed

Check how `this` is evaluated inside prototype methods:

**File:** `C:\Users\Q\code\barn\vm\eval.go` (or wherever `parser.ThisExpr` is handled)

When evaluating `this` inside a prototype method:
- Current: Returns the prototype object ID from `ctx.ThisObj`
- Expected: Should return the primitive value if we're in a primitive prototype call chain

### Possible Solutions

#### Option A: Track Primitive Value in TaskContext

Add a field to `types.TaskContext`:
```go
type TaskContext struct {
    ...
    ThisObj      ObjID   // The object 'this' refers to (might be prototype)
    ThisValue    Value   // The actual 'this' value (primitive or nil for objects)
    ...
}
```

When evaluating `this`:
```go
func (e *Evaluator) Eval(node parser.Node, ctx *types.TaskContext) types.Result {
    case *parser.ThisExpr:
        if ctx.ThisValue != nil {
            return types.Ok(ctx.ThisValue)
        }
        return types.Ok(types.NewObj(ctx.ThisObj))
}
```

When calling nested verbs, preserve `ThisValue` through the context.

#### Option B: Store Primitive Value in Top Frame

When entering a verb on a primitive prototype:
1. Push frame with `ThisValue` set to primitive
2. Update `ctx.ThisValue` for the duration of the call
3. Restore `ctx.ThisValue` when exiting

This requires:
- Setting `ctx.ThisValue` in `verbCall()` after pushing the frame
- Restoring it before popping the frame
- Using `ctx.ThisValue` when evaluating `this` expressions

### Recommended Solution: Option A

**Option A is cleaner** because:
- TaskContext already tracks execution context
- `ThisValue` naturally belongs with `ThisObj`
- Easier to propagate through nested calls
- Matches the mental model: "this" has both an object location and a value

---

## Implementation Steps

### Step 1: Extend TaskContext with ThisValue

**File:** `C:\Users\Q\code\barn\types\types.go`
**Location:** In `TaskContext` struct definition

```go
type TaskContext struct {
    TaskID       int64
    Player       ObjID
    Programmer   ObjID
    ThisObj      ObjID   // Object 'this' refers to (might be prototype for primitives)
    ThisValue    Value   // Actual value of 'this' (primitive value, or nil for objects)
    Verb         string
    IsWizard     bool
    ServerInitiated bool
    Task         interface{} // *task.Task
}
```

### Step 2: Set ThisValue in verbCall()

**File:** `C:\Users\Q\code\barn\vm\verbs.go`
**Function:** `verbCall()`
**Location:** After pushing the frame (line ~135)

```go
frame := task.ActivationFrame{
    This:            defObjID,
    ThisValue:       primitiveValue,
    ...
}
t.PushFrame(frame)
framePushed = true

// Update context for primitive calls
oldThisValue := ctx.ThisValue
if isPrimitive {
    ctx.ThisValue = primitiveValue
}
defer func() {
    ctx.ThisValue = oldThisValue
}()
```

### Step 3: Use ThisValue when evaluating 'this'

**File:** `C:\Users\Q\code\barn\vm\eval.go`
**Function:** `Eval()` or wherever `parser.ThisExpr` is handled

```go
case *parser.ThisExpr:
    // For primitive prototype calls, use the actual primitive value
    if ctx.ThisValue != nil {
        return types.Ok(ctx.ThisValue)
    }
    // For normal object calls, use the object ID
    return types.Ok(types.NewObj(ctx.ThisObj))
```

### Step 4: Fix queued_tasks() format

**File:** `C:\Users\Q\code\barn\task\task.go`
**Function:** `ToQueuedTaskInfo()`

Replace lines 349-364 with the full 10-element format shown in "Test 1: Root Cause" section above.

### Step 5: Verify callers() uses ThisValue

**File:** `C:\Users\Q\code\barn\task\task.go`
**Function:** `ActivationFrame.ToList()`

Already correct (lines 72-88) - it checks `ThisValue` and uses it if set.

---

## Testing Plan

### Unit Tests

1. Test `queued_tasks()` returns 10 elements
2. Test `queued_tasks()[1][9]` is the primitive value for primitive calls
3. Test `callers()[1][1]` is the primitive value for primitive calls
4. Test nested primitive method calls maintain ThisValue

### Integration Tests

```moo
# Test 1: queued_tasks with map primitive
t = ["one" -> 1, "two" -> 2]:suspend();
q = queued_tasks();
kill_task(t);
assert(typeof(q[1][5]) == 1);  # programmer is OBJ
assert(length(q[1]) == 10);     # 10 elements

# Test 2: callers with list primitive
c = {1, 2, "three", 4.0}:foo();
assert(c[1][1] == {1, 2, "three", 4.0});  # this is the list

# Test 3: nested calls preserve primitive
# In prototype method that calls another method:
c = {1, 2, 3}:foo();  # foo() calls bar()
assert(c[1][1] == {1, 2, 3});     # first frame (bar)
assert(c[2][1] == {1, 2, 3});     # second frame (foo)
```

### Run Full Conformance Suite

```bash
cd ~/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9350 -k "primitives" -v
```

---

## Files to Modify

| File | Function/Location | Change |
|------|-------------------|--------|
| `types/types.go` | `TaskContext` struct | Add `ThisValue Value` field |
| `vm/verbs.go` | `verbCall()` | Set/restore `ctx.ThisValue` for primitive calls |
| `vm/eval.go` | `Eval()` for `ThisExpr` | Use `ctx.ThisValue` if set, else `ctx.ThisObj` |
| `task/task.go` | `ToQueuedTaskInfo()` | Return 10 elements instead of 6, include `ThisValue` from top frame |

**No changes needed:**
- `task/task.go::ActivationFrame.ToList()` - already handles `ThisValue` correctly
- `builtins/tasks.go::builtinCallers()` - already calls `ToList()` which handles it

---

## Estimated Complexity

**Low to Medium**

- **Low risk:** Changes are localized to a few files
- **Medium complexity:** Need to understand context propagation through nested calls
- **Well-defined:** Clear reference implementation (ToastStunt)
- **Good test coverage:** Conformance tests will validate the fix

### Estimated Time

- Step 1-3 (ThisValue propagation): 30-45 minutes
- Step 4 (queued_tasks format): 15-20 minutes
- Testing and debugging: 30-45 minutes
- **Total: 1.5-2 hours**

---

## Potential Issues

### Issue 1: Memory Usage Calculation

`queued_tasks()[1][10]` should return bytes (memory usage). Currently returning 0.

**Impact:** Low - tests don't check this field
**Fix:** Can implement later as enhancement

### Issue 2: Include Variables Parameter

`queued_tasks()` can take 2 optional parameters:
- `queued_tasks(include_variables)` → adds 11th element (variable map)
- `queued_tasks(include_variables, return_count)` → returns count instead of list

**Impact:** Low - current tests don't use these parameters
**Fix:** Can implement later as enhancement

### Issue 3: Context Mutation

Setting `ctx.ThisValue` modifies the context during execution. Need to ensure:
- It's restored properly on all exit paths (use defer)
- It doesn't leak between independent verb calls
- It works correctly with nested calls, fork, suspend/resume

**Impact:** Medium - need careful testing
**Fix:** Use defer to restore old value, test thoroughly

---

## Success Criteria

1. Both tests pass: `queued_tasks_includes_this_map` and `callers_includes_this_list`
2. All other primitives tests continue to pass
3. No regressions in other conformance tests (especially tasks and builtins)
4. Manual testing confirms:
   - `queued_tasks()` returns 10-element lists
   - `callers()` returns primitive values for 'this' in prototype calls
   - Nested primitive method calls maintain the primitive value throughout the call chain
