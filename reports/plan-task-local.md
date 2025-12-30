# Plan: Fix task_local Conformance Test Failures

## Executive Summary

7 task_local conformance tests are failing. Root cause analysis reveals that task_local storage is not being properly preserved and restored when tasks suspend, resume, or call verbs. The current implementation stores task_local in the Task object but doesn't properly manage it during task state transitions.

## Failing Tests Summary

### 1. `fork_and_suspend_case` (HIGH PRIORITY)
**Test Code:**
```moo
set_task_local({1, 2});
id = task_id();
fork (1)
  set_task_local({"foo", "bar", 3, 4, 5});
  suspend(1);
  resume(id, task_local());
endfork
return {task_local(), suspend()};
```

**Expected:** `[[1, 2], ["foo", "bar", 3, 4, 5]]`
**Actual:** `[[1, 2], 0]`

**Problem:** The forked child's task_local is set correctly, but when it resumes from suspend and calls `task_local()` to pass to `resume()`, it returns 0 instead of the stored value.

**Root Cause:** When a task suspends and later resumes, the task_local value stored in the Task object is not being restored into the execution context properly.

### 2. `command_verb` (MEDIUM PRIORITY)
**Test:** Calls a command verb that sets task_local, then calls another verb that reads it.

**Expected:** Verb reads argstr from task_local
**Actual:** E_INVIND (object #-1 doesn't exist)

**Problem:** Test infrastructure issue - the test expects object #-1 to exist with verbs, but it doesn't. This is a test setup issue, not a task_local bug per se, but reveals that task_local isn't being tested with actual verb calls.

### 3. `server_verb` (MEDIUM PRIORITY)
**Test:** Sets task_local, then calls recycle() which triggers a verb that reads task_local.

**Expected:** Verb reads task_local value
**Actual:** E_INVIND (object #-1 doesn't exist)

**Same issue as command_verb** - test setup problem.

### 4. `across_verb_calls` (HIGH PRIORITY)
**Test Code:**
```moo
#-1:foo(1.234);  // Sets task_local
return #-1:bar(); // Reads task_local
```

**Expected:** `1.234`
**Actual:** E_INVIND (object #-1 doesn't exist)

**Problem:** Once test setup is fixed, this will test whether task_local persists across synchronous verb calls. The current implementation likely fails this because CallVerb doesn't preserve task_local properly.

### 5. `across_verb_calls_with_intermediate` (HIGH PRIORITY)
**Test Code:**
```moo
return #-1:baz(23.456); // Calls foo, then bar
```

**Expected:** `23.456`
**Actual:** E_INVIND

**Same category as #4** - tests task_local across multiple verb call layers.

### 6. `suspend_between_verb_calls` (HIGH PRIORITY)
**Test:** Calls verb that sets task_local, suspends multiple times, then reads task_local.

**Expected:** `97.65625` (result of 2.5^5)
**Actual:** E_INVIND

**Problem:** Tests task_local persistence across suspend/resume cycles within verb calls.

### 7. `read_between_verb_calls` (MEDIUM PRIORITY)
**Test:** Like #6 but uses `read()` instead of `suspend()`.

**Expected:** `97.65625`
**Actual:** E_INVIND

**Problem:** Tests task_local persistence when task blocks on input.

## Root Cause Analysis

### Current Implementation Issues

1. **Task Object vs TaskContext Confusion**
   - `task.Task` has `TaskLocal` field (correctly stores the value)
   - `types.TaskContext` also has `TaskLocal` field (fallback)
   - Builtins check `ctx.Task` first, then fall back to `ctx.TaskLocal`
   - This dual storage creates inconsistency

2. **Fork Correctly Copies task_local**
   - `scheduler.go:329` - `t.TaskLocal = parent.GetTaskLocal()`
   - Fork implementation is CORRECT

3. **Suspend/Resume Doesn't Restore task_local to Context**
   - When task suspends, `task.TaskLocal` is preserved in Task object
   - When task resumes, the Task object is reused BUT the builtins may be reading from wrong location
   - The issue: builtins read from `ctx.Task.GetTaskLocal()` which should work, but something in the execution flow is breaking this

4. **Verb Calls May Not Preserve task_local**
   - `vm.CallVerb` doesn't do anything special with task_local
   - It relies on the Task object being attached to ctx
   - If ctx.Task isn't properly set during verb calls, task_local will be lost

### ToastStunt Reference Implementation

From `toaststunt/src/tasks.cc`:
- `current_local` is a global variable that gets swapped in/out when switching tasks
- Stored in VM structure: `vmstruct.local` (line 241)
- When task suspends: saved to `the_vm->local`
- When task resumes: restored from `the_vm->local` to `current_local`
- Builtins read/write `current_local` directly

**Key insight:** In Toast, task_local is **thread-local state** that must be explicitly saved/restored during task switches.

## Architectural Changes Required

### Option A: Global Task-Local State (Toast Model)
Store task_local in a global/thread-local variable that gets swapped when switching tasks.

**Pros:**
- Matches Toast exactly
- Simple builtin implementation

**Cons:**
- Not idiomatic Go (globals are problematic)
- Doesn't work well with goroutine-based execution
- Testing becomes harder

### Option B: Context Propagation (Current Model - FIX IT)
Keep task_local in Task object, ensure it's always accessible via ctx.Task.

**Pros:**
- Idiomatic Go
- Already mostly implemented
- Thread-safe by design

**Cons:**
- Need to ensure ctx.Task is ALWAYS set correctly
- Need to ensure builtins always check ctx.Task first

**RECOMMENDATION: Option B** - Fix the existing model rather than rewriting.

## Implementation Plan

### Phase 1: Fix Core task_local Access (LOW COMPLEXITY)

**Files to change:**
- `builtins/system.go:54-77` (builtinTaskLocal)
- `builtins/system.go:82-104` (builtinSetTaskLocal)

**Changes:**
1. Remove fallback to `ctx.TaskLocal` - it causes confusion
2. ALWAYS require `ctx.Task` to be set
3. Return E_INVARG if ctx.Task is nil (shouldn't happen in normal execution)

**Expected outcome:** Builtins consistently use Task object for storage.

### Phase 2: Ensure ctx.Task is Set Everywhere (MEDIUM COMPLEXITY)

**Files to audit:**
- `server/scheduler.go` - All task execution paths
- `vm/verbs.go:223` - CallVerb function
- `vm/eval_expr.go` - Any verb call expressions
- `server/connection.go` - Command processing

**Changes:**
1. Verify ctx.Task is set before calling any verb
2. In CallVerb, ensure child verb inherits parent's Task object
3. In scheduler.runTask, ensure ctx.Task is set before execution starts

**Critical locations:**
- `scheduler.go:137-150` - Task execution setup (ctx.Task should be set here)
- `scheduler.go:361` - CallVerb for server hooks (line 361: `ctx.Task = t` - GOOD)
- `verbs.go:236` - CallVerb activation frame push (uses ctx.Task - GOOD)

**Verification:**
```go
// Add assertion at start of task_local builtin
if ctx.Task == nil {
    panic("task_local called without ctx.Task set - this is a bug")
}
```

### Phase 3: Fix Suspend/Resume Lifecycle (LOW COMPLEXITY)

**Files to change:**
- `server/scheduler.go` - Resume handling
- `task/manager.go` - Task state transitions

**Current flow:**
1. Task calls suspend() → task.Suspend() sets state
2. Another task calls resume() → task.Resume() sets state to Queued
3. Scheduler picks up queued task → runs it

**The problem:** When scheduler resumes the task, does it reuse the existing Task object with its TaskLocal? YES, it should - need to verify.

**Check:** `scheduler.go:506-519` ResumeTask - it reuses existing task object, which should preserve TaskLocal.

**Likely issue:** The resumed task's Context might not have Task pointer set. Need to verify ctx.Task is set when resuming.

### Phase 4: Test Infrastructure Fixes (LOW COMPLEXITY)

**Problem:** Tests expect object #-1 to exist with verbs.

**Options:**
1. Update Test.db to include #-1 with test verbs
2. Create test setup in YAML that creates object and verbs dynamically
3. Use a different object (like #2 or #3) that exists

**Recommendation:** Option 2 - Add setup block to task_local.yaml that creates test object and verbs.

**Implementation:**
```yaml
setup:
  permission: wizard
  code:
    - "test_obj = create(#3);"
    - "add_verb(test_obj, {player, \"xd\", \"foo\"}, {\"this\", \"none\", \"this\"});"
    - "set_verb_code(test_obj, \"foo\", {\"{value} = args;\", \"set_task_local(value);\"});"
    - "add_verb(test_obj, {player, \"xd\", \"bar\"}, {\"this\", \"none\", \"this\"});"
    - "set_verb_code(test_obj, \"bar\", {\"return task_local();\"});"
```

Then update test cases to use `test_obj` instead of `#-1`.

**Complexity:** Need to verify cow_py test framework supports this setup style.

## Step-by-Step Implementation

### Step 1: Add Diagnostics (1 hour)
**Goal:** Understand current behavior

1. Add logging to builtinTaskLocal/builtinSetTaskLocal
2. Log whether ctx.Task is nil
3. Log task ID and task_local value
4. Run failing tests, collect logs

### Step 2: Fix Builtin Access (2 hours)
**Goal:** Eliminate fallback path

1. Edit `builtins/system.go`
2. Remove `ctx.TaskLocal` fallback in both functions
3. Add error if `ctx.Task` is nil
4. Run simple tests (simple_eval_case, eval_case_with_suspend)

### Step 3: Audit Context Setup (4 hours)
**Goal:** Ensure ctx.Task is always set

1. Search all calls to EvalStmt/CallVerb
2. Verify ctx.Task is set before each call
3. Add missing ctx.Task assignments
4. Special attention to:
   - Verb calls from verbs
   - Server hooks
   - Resume path

### Step 4: Fix Scheduler Resume (2 hours)
**Goal:** Ensure resumed tasks have proper context

1. Check `scheduler.go` resume path
2. Ensure ctx.Task points to resumed Task object
3. Verify Task.Context is reused (not recreated)
4. Test fork_and_suspend_case

### Step 5: Create Test Fixtures (4 hours)
**Goal:** Fix test infrastructure

1. Create helper functions in cow_py for dynamic verb creation
2. Add setup block to task_local.yaml
3. Update verb tests to use setup-created objects
4. Verify tests can create/call verbs

### Step 6: Test Verb Call Preservation (2 hours)
**Goal:** Verify task_local works across verb calls

1. Run across_verb_calls test
2. If fails, debug CallVerb to ensure Task is preserved
3. May need to pass Task through call chain explicitly

### Step 7: Integration Testing (2 hours)
**Goal:** All tests pass

1. Run all 7 failing tests
2. Fix any remaining issues
3. Run full task_local test suite
4. Run broader conformance tests to check for regressions

## Files That Need Changes

### Definitely Need Changes
1. **builtins/system.go** - Fix task_local/set_task_local builtins
2. **server/scheduler.go** - Verify ctx.Task setup in all paths
3. **C:\Users\Q\code\cow_py\tests\conformance\builtins\task_local.yaml** - Add test fixtures

### Might Need Changes
4. **vm/verbs.go** - Ensure CallVerb preserves Task context
5. **task/manager.go** - Verify resume logic
6. **types/context.go** - Consider removing TaskLocal field to eliminate confusion

## Estimated Complexity

| Component | Complexity | Time Estimate |
|-----------|-----------|---------------|
| Builtin fixes | LOW | 2 hours |
| Context audit | MEDIUM | 4 hours |
| Resume fixes | LOW | 2 hours |
| Test infrastructure | MEDIUM | 4 hours |
| Verb call fixes | MEDIUM | 2 hours |
| Testing/debug | MEDIUM | 4 hours |
| **TOTAL** | **MEDIUM** | **18 hours** |

## Success Criteria

1. All 7 failing tests pass
2. No regressions in other task tests
3. task_local persists correctly across:
   - Suspend/resume cycles
   - Fork operations (already works)
   - Synchronous verb calls
   - Server hook invocations

## Risks and Mitigations

### Risk 1: Breaking Existing Task Code
**Mitigation:** Remove fallback gradually, add logging first to identify all code paths.

### Risk 2: Test Infrastructure Limitations
**Mitigation:** Verify cow_py supports dynamic verb creation before relying on it. May need to update Test.db instead.

### Risk 3: Goroutine Concurrency Issues
**Mitigation:** Use existing Task mutex for all TaskLocal access (already in place).

## Notes

- The fork implementation (scheduler.go:329) is CORRECT - it copies parent's task_local
- The core storage mechanism (task.Task.TaskLocal) is CORRECT
- The problem is ACCESS, not STORAGE
- Focus on ensuring builtins always read from the right place
- ToastStunt's global variable approach is simpler but not idiomatic Go

## References

- `C:\Users\Q\src\toaststunt\test\tests\test_task_local.rb` - Reference test implementation
- `C:\Users\Q\src\toaststunt\src\tasks.cc:238` - ToastStunt task_local implementation
- `C:\Users\Q\code\cow_py\tests\conformance\builtins\task_local.yaml` - Test definitions
- `C:\Users\Q\code\barn\task\task.go:125` - Task.TaskLocal field
- `C:\Users\Q\code\barn\builtins\system.go:54` - task_local builtin
