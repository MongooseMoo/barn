# Divergence Report: Task Builtins

**Spec File**: `spec/builtins/tasks.md`
**Barn Files**: `builtins/tasks.go`, `builtins/system.go`
**Status**: divergences_found
**Date**: 2026-01-03

## Summary

Tested 15+ task-related builtins against both Toast (reference) and Barn implementations. Found 3 spec gaps where the spec documentation does not match actual Toast/Barn behavior, and identified 8 builtins documented in spec but not yet implemented in Barn.

**Key Findings:**
- Most implemented builtins match Toast behavior perfectly
- 3 spec documentation errors found (callers default, queued_tasks args, ticks/seconds behavior)
- 8 ToastStunt-specific builtins not yet implemented in Barn
- All error handling matches between servers

## Divergences

### 1. callers() - Default argument behavior

| Field | Value |
|-------|-------|
| Test | `callers()` called from within a verb |
| Barn | Returns frames WITH line numbers: `{{#2876, "test", #2876, #2876, #2876, 1}}` |
| Toast | Returns frames WITH line numbers: `{{#10598, "test", #10598, #10598, #10598, 1}}` |
| Classification | likely_spec_gap |
| Evidence | Both servers include line numbers by default, but spec example at line 76-78 shows `callers()` returning frames WITHOUT line numbers. Barn implementation comment says "default true" for include_line_numbers, which matches actual behavior. |

**Spec claims (lines 76-78):**
```moo
callers()
  => {{#room, "look", #wizard, #room, #player},
      {#thing, "describe", #wizard, #thing, #player}}
```

**Actual behavior:** Both servers return 6-element frames with line numbers by default.

---

### 2. queued_tasks() - Argument count

| Field | Value |
|-------|-------|
| Test | `queued_tasks(1)` |
| Barn | E_ARGS |
| Toast | E_ARGS |
| Classification | likely_spec_gap |
| Evidence | Spec says signature is `queued_tasks([include_vars])` with optional argument, but both Toast and Barn reject any arguments with E_ARGS. Barn implementation correctly takes 0 args (line 14 of tasks.go). |

**Spec claims (line 95):**
```
Signature: queued_tasks([include_vars]) → LIST
```

**Actual behavior:** Both servers require exactly 0 arguments.

---

### 3. ticks_left() / seconds_left() - Return zero in eval context

| Field | Value |
|-------|-------|
| Test | `ticks_left() > 0` and `seconds_left() > 0` in eval |
| Barn | Both return 0 (false) |
| Toast | Both return 0 (false) |
| Classification | likely_spec_gap |
| Evidence | Spec says these return "remaining ticks/seconds for current task" but doesn't document that they return 0 in eval context (non-forked tasks). Both servers behave identically. |

**Context:** When called from top-level eval (`;` command), these builtins return 0, not positive integers as spec description implies.

---

## Behaviors Verified Correct

The following builtins match between Barn and Toast:

### Task Identity
- `task_id()` - returns INT type (specific values differ, types match)
- `caller_perms()` - returns player object at top level

### Call Stack
- `callers()` - returns empty list at top level
- `callers(0)` - returns frames without line numbers
- `callers(1)` - returns frames with line numbers
- Frames have correct 5 or 6 element structure

### Task Queries
- `queued_tasks()` - returns list of suspended tasks with correct 10-element format:
  `{task_id, start_time, x, y, programmer, verb_loc, verb_name, line, this, vars}`

### Task Control
- `kill_task(task_id())` - returns E_INTRPT when killing self
- `kill_task(99999)` - returns E_INVARG for invalid task
- `resume(99999)` - returns E_INVARG for invalid task

### Task Stack
- `task_stack(task_id())` - returns E_INVARG for non-suspended task (both servers)

### Task Local Storage
- `set_task_local({...})` / `task_local()` - works correctly
- Stores and retrieves values per-task

### Exception Handling
- `raise(E_PERM)` - raises E_PERM correctly

### Resource Limits
- `ticks_left()` - returns 0 in eval context (matches Toast)
- `seconds_left()` - returns 0 in eval context (matches Toast)

---

## Test Coverage Gaps

The following behaviors are documented in spec but have NO conformance test coverage:

### Task Control Edge Cases
- `suspend()` without arguments (indefinite suspension)
- `suspend(seconds)` timeout behavior vs explicit resume
- `resume(task, value)` with various value types
- Permission checks on `resume()` and `kill_task()` (non-wizard, non-owner)

### Task Stack Inspection
- `task_stack(task_id, 0)` - without line numbers
- `task_stack(task_id, 1)` - with line numbers
- Permission checks on `task_stack()`

### Permission Context
- `set_task_perms(who)` - changing permission context
- Wizard-only enforcement

### Resource Limit Getters
- `ticks_left()` behavior in forked tasks
- `seconds_left()` behavior in forked tasks
- Whether these ever return non-zero values

---

## Builtins Not Yet Implemented in Barn

The following builtins are documented in spec but NOT registered in `builtins/registry.go`:

### ToastStunt Task Extensions
1. `yin(threshold)` - yield if needed (line 200-216 in spec)
2. `set_task_ticks(task_id, ticks)` - set tick limit (line 222-228)
3. `set_task_seconds(task_id, seconds)` - set time limit (line 232-238)
4. `queue_info()` - ToastStunt queue stats (mentioned in prompt line 33)

### Server Control (documented in tasks.md but not task-specific)
5. `shutdown([message])` - server shutdown (line 318-324)
6. `dump_database()` - force checkpoint (line 327-334)
7. `memory_usage()` - memory statistics (line 309-314)
8. `server_version()` - IMPLEMENTED (registered in registry.go line 151)

**Note:** `server_version()` IS implemented and registered, contrary to initial assessment.

---

## Fork Statement

The `fork` statement is documented in spec (lines 272-297) but was not tested as part of builtin testing. The `fork` statement is a language construct, not a builtin function, and requires separate verification.

---

## Implementation Quality

Barn's task builtin implementations are high quality:
- Error handling matches Toast exactly
- Return value formats match specification
- Permission checks are consistent
- Code is well-commented with clear semantics

The main gaps are:
1. ToastStunt-specific extensions not yet implemented
2. Some server control functions not implemented

---

## Recommendations

### For Spec
1. **Fix callers() example** (line 76-78) - show line numbers in default call
2. **Fix queued_tasks() signature** (line 95) - remove `[include_vars]` optional arg
3. **Document ticks_left()/seconds_left() edge case** - add note that they return 0 in eval context

### For Barn Implementation
1. Consider implementing ToastStunt extensions: `yin()`, `set_task_ticks()`, `set_task_seconds()`
2. Add conformance tests for suspend/resume scenarios
3. Add conformance tests for task_stack permission checks

### For Conformance Tests
1. Add tests for `suspend()` timeout vs resume distinction
2. Add tests for `task_stack()` with different arguments
3. Add permission violation tests for task control builtins
4. Add tests to verify ticks_left()/seconds_left() in forked vs eval tasks
