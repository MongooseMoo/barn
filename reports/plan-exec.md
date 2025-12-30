# Plan: Fix 4 Failing Exec Conformance Tests

## Executive Summary

All 4 failing tests have the same root cause: **fork variables are not accessible in the parent scope after a fork statement when executed via eval commands (`;` commands)**.

The tests fail with `E_VARNF` (variable not found) or `E_VERBNF` (verb not found, actually a symptom of trying to call a function with an undefined variable) because the fork variable (e.g., `go`, `go1`, `go2`) is never assigned in the parent's environment.

## Failing Tests

### 1. `kill_task_works_on_suspended_exec`
```moo
task_id = 0;
fork go (0)
  exec({"sleep", "5"});
endfork
suspend(0);
result = kill_task(go);  // E_VARNF: 'go' not found
return result;
```
**Expected:** `result = 0` (task killed successfully)
**Actual:** `E_VARNF` when trying to reference `go`

### 2. `kill_task_fails_on_already_killed`
```moo
task_id = 0;
fork go (0)
  exec({"sleep", "5"});
endfork
suspend(0);
kill_task(go);           // E_VARNF: 'go' not found
return kill_task(go);
```
**Expected:** `E_INVARG` (trying to kill already-killed task)
**Actual:** `E_VARNF` on first `kill_task(go)` call

### 3. `resume_fails_on_suspended_exec`
```moo
task_id = 0;
fork go (0)
  exec({"sleep", "5"});
endfork
suspend(0);
return resume(go);       // E_VARNF: 'go' not found
```
**Expected:** `E_INVARG` (can't resume exec task)
**Actual:** `E_VARNF` when trying to reference `go`

### 4. `suspended_exec_task_stack_matches_suspended_task`
```moo
task_id1 = 0;
task_id2 = 0;
fork go1 (0)
  suspend(5);
endfork
fork go2 (0)
  exec({"sleep", "5"});
endfork
suspend(0);
stack1 = task_stack(go1);  // E_VERBNF because go1 is undefined
stack2 = task_stack(go2);
return stack1 == stack2;
```
**Expected:** `1` (stacks should match)
**Actual:** `E_VERBNF` when trying to call `task_stack(go1)`

## Root Cause Analysis

The bug exists in **two separate execution paths**:

### Path 1: Eval Commands (The Bug Location)

When code is executed via eval commands (`;` prefix):

1. **Entry Point:** `server/scheduler.go:EvalCommand()` (line 434)
2. Creates a lightweight task without `ForkCreator` set (line 469)
3. Creates a NEW evaluator: `eval := vm.NewEvaluatorWithStore(s.store)` (line 478)
4. Calls `eval.EvalStatements(stmts, ctx)` (line 479)
5. **In `vm/eval_stmt.go:EvalStatements()`** (line 14):
   - When fork statement encountered, returns `FlowFork` result
   - Line 22-23: Just `continue` - **NO fork variable assignment**
   - Fork task is never created
   - Fork variable is never set in parent environment

### Path 2: Scheduled Tasks (Works Correctly)

When code runs as a scheduled task (verb execution or queued task):

1. **Entry Point:** `server/scheduler.go:runTask()` (line 127)
2. Task has `ForkCreator` set to scheduler (line 259, 285)
3. Uses task's evaluator if set, else scheduler's evaluator (line 144-149)
4. Executes statements in loop (line 189)
5. **When fork encountered** (line 208-219):
   - Creates child task via `t.ForkCreator.CreateForkedTask()` (line 211)
   - **Assigns fork variable:** `evaluator.GetEnvironment().Set(varName, childID)` (line 215)
   - Parent continues execution

**The discrepancy:** Fork variable assignment happens ONLY in the scheduled task path, NOT in the eval command path.

## Why This Matters

Eval commands are how conformance tests execute code:
- Tests send code via socket as `;task_id = 0; fork go (0) ...`
- Multi-line test code is flattened to single line (cow_py/tests/conformance/transport.py:560)
- Barn receives this as eval command and processes via `EvalCommand()`
- Fork variables are never assigned, causing `E_VARNF`

## Files Requiring Changes

### 1. `vm/eval_stmt.go` (Primary Fix)
**Function:** `EvalStatements()` (line 14)

**Current behavior:**
```go
if result.Flow == types.FlowFork {
    continue  // Just skip to next statement
}
```

**Required behavior:**
- Check if `ctx.Task` has a `ForkCreator`
- If yes: Create child task and assign fork variable
- If no: Continue as before (fork is no-op)

### 2. `server/scheduler.go` (Configuration Fix)
**Function:** `EvalCommand()` (line 434)

**Current code:**
```go
t := &task.Task{
    Owner:      player,
    Programmer: player,
    CallStack:  make([]task.ActivationFrame, 0),
    TaskLocal:  types.NewEmptyMap(),
}
```

**Required change:**
```go
t.ForkCreator = s  // Add this line to enable fork support in eval commands
```

### 3. `task/task.go` (Possible - Interface Definition)
May need to verify `ForkCreator` interface is exported and usable from vm package.

## Implementation Plan

### Step 1: Enable Fork Support in Eval Commands (Low Complexity)
**File:** `server/scheduler.go:EvalCommand()`
- Add `t.ForkCreator = s` after task creation (after line 474)
- This makes the scheduler available for fork creation during eval execution

### Step 2: Handle Fork in EvalStatements (Medium Complexity)
**File:** `vm/eval_stmt.go:EvalStatements()`
- When `result.Flow == types.FlowFork` (line 22):
  - Check if `ctx.Task` exists and has `ForkCreator`
  - If yes:
    - Call `ctx.Task.ForkCreator.CreateForkedTask(ctx.Task, result.ForkInfo)`
    - Store returned child ID
    - If `result.ForkInfo.VarName != ""`:
      - Assign to evaluator environment: `e.env.Set(varName, childID)`
  - Continue to next statement

**Key consideration:** Need to handle case where `ctx.Task` might be nil or not have ForkCreator (backward compatibility).

### Step 3: Type Safety Check (Low Complexity)
**File:** `task/task.go`
- Verify `ForkCreator` interface is properly defined
- Check if vm package can access it without circular imports
- Current code uses `t.ForkCreator.CreateForkedTask()` in scheduler.go, so interface should be fine

### Step 4: Test the Fix (Low Complexity)
Run the 4 failing tests:
```bash
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ -k "kill_task_works_on_suspended_exec or kill_task_fails_on_already_killed or resume_fails_on_suspended_exec or suspended_exec_task_stack_matches" --transport socket --moo-port 9300 -v
```

### Step 5: Regression Testing (Medium Complexity)
Run full exec test suite to ensure no regressions:
```bash
uv run pytest tests/conformance/server/exec.yaml --transport socket --moo-port 9300 -v
```

Run related fork/task tests:
```bash
uv run pytest tests/conformance/ -k "fork or suspend or task" --transport socket --moo-port 9300 -v
```

## Estimated Complexity

### Overall: **MEDIUM**

**Why not LOW:**
- Need to handle fork creation in evaluator (outside scheduler context)
- Must maintain backward compatibility for cases without ForkCreator
- Need to ensure environment variable assignment works correctly
- Cross-package concerns (vm accessing task package)

**Why not HIGH:**
- Root cause is clearly identified
- Solution pattern already exists in `scheduler.go:runTask()`
- Only 2 files need modification (plus possible interface check)
- No algorithm changes needed - just code reuse

### By Component:
- **Step 1 (Enable fork in eval):** LOW - Single line addition
- **Step 2 (Handle fork in EvalStatements):** MEDIUM - Logic replication with safety checks
- **Step 3 (Type safety):** LOW - Verification only, likely no changes needed
- **Step 4 (Test fix):** LOW - Run existing tests
- **Step 5 (Regression test):** MEDIUM - May uncover edge cases

## Potential Gotchas

### 1. Circular Import Risk
If `vm/eval_stmt.go` needs to import `task` package to access `ForkCreator`, might create circular dependency since `task` likely imports `vm` types.

**Mitigation:** Use interface from `types` package instead, or cast via interface{}.

### 2. Environment Lifetime
The evaluator environment in `EvalStatements` must persist across all statements in the eval command.

**Verification:** Check that `e.env` is the same instance throughout the statement loop.

### 3. Child Task Execution
The forked child task needs to actually execute `exec()`. Verify that:
- Child task is queued by scheduler
- Child has correct permissions (wizard)
- exec() builtin can actually be called

**Mitigation:** Check that `CreateForkedTask()` properly queues the task with all necessary context.

### 4. Suspend/Resume Interaction
Tests call `suspend(0)` which should suspend the current task. Verify:
- Eval command tasks can be suspended
- Scheduler properly manages suspended eval tasks
- Fork variables remain accessible after suspend

**Potential issue:** `suspend()` might not work in eval commands if they're not properly integrated with scheduler's task queue.

## Success Criteria

1. All 4 failing tests pass
2. No regressions in other exec tests
3. No regressions in fork/suspend/task tests
4. Fork variables accessible in eval commands
5. Fork tasks are actually created and executed

## Alternative Approaches Considered

### Alternative 1: Make EvalCommand use runTask()
**Idea:** Instead of calling `EvalStatements()` directly, create a full task and queue it.

**Pros:**
- Would automatically get fork support
- Consistent code path for all execution

**Cons:**
- Eval commands are synchronous - would need to wait for task completion
- More complex scheduler integration
- Potential performance impact

**Verdict:** REJECTED - Too invasive for the problem scope.

### Alternative 2: Duplicate fork logic in EvalStatements
**Idea:** Copy-paste the fork handling from `runTask()` into `EvalStatements()`.

**Pros:**
- Self-contained fix
- No dependency on external ForkCreator

**Cons:**
- Code duplication
- Maintenance burden
- Violates DRY principle

**Verdict:** CONSIDERED - This is essentially the chosen approach, but with proper dependency injection via `ctx.Task.ForkCreator` rather than duplication.

### Alternative 3: Move fork handling to Evaluator
**Idea:** Make Evaluator responsible for creating fork tasks.

**Pros:**
- Single location for fork logic
- Cleaner separation of concerns

**Cons:**
- Evaluator would need scheduler reference
- Breaks current architecture where scheduler owns task lifecycle
- Would require significant refactoring

**Verdict:** REJECTED - Too large a change for this bug fix.

## Implementation Notes

### Code Pattern to Follow
The fix should mirror the existing logic in `scheduler.go:runTask()` lines 208-219:

```go
if result.Flow == types.FlowFork {
    if result.ForkInfo != nil {
        // Get ForkCreator from task context if available
        var forkCreator task.ForkCreator
        if ctx.Task != nil {
            forkCreator = ctx.Task.GetForkCreator()  // May need to add this method
        }

        if forkCreator != nil {
            childID := forkCreator.CreateForkedTask(ctx.Task, result.ForkInfo)

            // Assign fork variable in parent's environment
            if result.ForkInfo.VarName != "" {
                e.env.Set(result.ForkInfo.VarName, types.NewInt(childID))
            }
        }
    }
    continue
}
```

### Testing Strategy
1. **Unit test:** Verify fork variable assignment in isolated evaluator
2. **Integration test:** Run failing conformance tests
3. **Regression test:** Run full test suite
4. **Manual test:** Use moo_client to verify interactive fork behavior

### Rollback Plan
If the fix causes regressions:
1. Revert changes to `eval_stmt.go`
2. Revert changes to `scheduler.go:EvalCommand()`
3. Tests will return to current state (4 failures)

## Related Issues

This fix may also resolve or improve:
- Fork behavior in any eval command context
- Interactive testing of fork statements
- Debug/development workflow where forks are tested interactively

## References

- Test definitions: `C:\Users\Q\code\cow_py\tests\conformance\server\exec.yaml`
- Fork implementation: `C:\Users\Q\code\barn\vm\eval_stmt.go:723` (forkStmt function)
- Task execution: `C:\Users\Q\code\barn\server\scheduler.go:127` (runTask function)
- Eval command: `C:\Users\Q\code\barn\server\scheduler.go:434` (EvalCommand function)
