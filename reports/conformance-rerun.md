# Conformance Test Results - Post-Limits Fix

**Date**: 2025-12-30
**Server**: Barn (Go MOO Server)
**Database**: Test.db
**Port**: 9300

## Summary Statistics

| Metric | Count | Previous | Change |
|--------|-------|----------|--------|
| **Passed** | 1,222 | 1,189 | +33 |
| **Failed** | 128 | 161 | -33 |
| **Skipped** | 128 | 128 | 0 |
| **Total** | 1,478 | 1,478 | 0 |
| **Pass Rate** | 90.5% | 87.9% | +2.6% |

## Impact of Limits Registration Fix

The fix to register the `limits` package in `builtins/init.go` resolved **33 test failures**, improving the pass rate from 87.9% to 90.5%.

**Fix Applied**: Added `limits.Register(registry)` to builtins initialization, ensuring all limits-related builtins are properly registered and discoverable.

## Remaining Failures (128 tests)

### By Category

| Category | Failed Tests | Example Failures |
|----------|--------------|------------------|
| **gc** (Garbage Collection) | 23 | `run_gc_requires_wizard_perms`, `gc_stats_returns_map`, `cyclic_references_through_list` |
| **verbs** | 18 | `add_verb_basic`, `add_verb_invalid_owner`, `verb_info_recycled_object` |
| **properties** | 18 | `add_property_invalid_owner`, `is_clear_property_works`, `clear_property_builtin` |
| **objects** | 11 | `create_invalid_owner_invarg`, `renumber_basic`, `chparent_property_conflict_variations` |
| **limits** | 9 | `setadd_checks_list_max_value_bytes_exceeds`, `encode_binary_limit` |
| **stress_objects** | 9 | `object_bytes_permission_denied`, `chparents_property_reset_multi` |
| **task_local** | 8 | `fork_and_suspend_case`, `command_verb`, `across_verb_calls` |
| **exec** | 7 | `exec_with_sleep_works`, `kill_task_works_on_suspended_exec` |
| **http** | 6 | `non_wizard_cannot_call_no_arg_version`, `read_http_invalid_type_foobar` |
| **map** | 5 | `mapdelete_empty_list_key`, `ranged_set_invalid_range_2` |
| **json** | 4 | `generate_json_escape_tab`, `generate_json_anon_obj` |
| **primitives** | 4 | `queued_tasks_includes_this_map`, `inheritance_with_prototypes` |
| **switch_player** | 2 | `non_wizard_gets_E_PERM`, `programmer_cannot_switch_player` |
| **recycle** | 2 | `recycle_invalid_already_recycled_object`, `recycle_invalid_already_recycled_anonymous` |
| **waif** | 2 | `nested_waif_map_indexes`, `deeply_nested_waif_map_indexes` |
| **anonymous** | 1 | `recycle_invalid_anonymous_no_crash` |
| **index_and_range** | 2 | `range_list_single`, `decompile_with_index_operators` |
| **caller_perms** | 1 | `caller_perms_top_level_eval` |

## Detailed Analysis

### 1. Garbage Collection (23 failures)

**Status**: All GC builtins returning `E_VERBNF` (Verb Not Found)

**Root Cause**: GC builtins (`run_gc`, `gc_stats`) are not implemented or not registered.

**Examples**:
- `run_gc_requires_wizard_perms` - Expected `E_PERM`, got `E_VERBNF`
- `run_gc_allows_wizard` - Expected success, got `E_VERBNF`
- `gc_stats_returns_map` - Expected map return, got `E_VERBNF`
- All cyclic reference tests fail with `E_VERBNF`

**Action Required**: Implement and register GC builtins (`run_gc`, `gc_stats`).

### 2. Verb Management (18 failures)

**Pattern**: Tests for `add_verb`, `delete_verb`, `verb_info`, etc. on recycled objects and permission checks.

**Examples**:
- `add_verb_basic` - Core verb creation functionality
- `add_verb_invalid_owner` - Permission validation
- `verb_info_recycled_object` - Handling recycled object references

**Action Required**: Review verb management implementation, particularly recycled object handling and permission checks.

### 3. Property Management (18 failures)

**Pattern**: Tests for `add_property`, `delete_property`, `is_clear_property`, `clear_property` with various edge cases.

**Examples**:
- `add_property_invalid_owner` - Permission validation
- `is_clear_property_works` - Basic clear property detection
- `clear_property_builtin` - Builtin name conflicts

**Action Required**: Review property management implementation, focusing on clear property semantics and recycled object handling.

### 4. Object Management (11 failures)

**Pattern**: Tests for `create()`, `renumber()`, `chparent()` with invalid arguments and edge cases.

**Examples**:
- `create_invalid_owner_invarg` - Expected `E_INVARG` for invalid owner
- `renumber_basic` - Basic object renumbering functionality
- `chparent_property_conflict_variations` - Property conflict handling during parent changes

**Action Required**: Improve object creation/management error handling and validation.

### 5. Limits System (9 failures)

**Status**: Despite fixing registration, some limit checks still fail.

**Examples**:
- `setadd_checks_list_max_value_bytes_exceeds` - List value size limit enforcement
- `listinsert_checks_list_max_value_bytes` - Insert operation limit checks
- `encode_binary_limit` - Binary encoding size limits

**Action Required**: Review limit enforcement in collection operations. Limits are registered but enforcement may be incomplete.

### 6. Task Management (8 failures)

**Pattern**: Tests for `task_local()` builtin across various execution contexts.

**Examples**:
- `fork_and_suspend_case` - Task-local data across fork/suspend
- `command_verb` - Task locals in command execution
- `across_verb_calls` - Task local preservation across verb calls

**Action Required**: Implement or fix `task_local()` builtin.

### 7. Exec Security (7 failures)

**Pattern**: Tests for `exec()` builtin with suspended tasks and task management.

**Examples**:
- `exec_with_sleep_works` - Exec with suspend operations
- `kill_task_works_on_suspended_exec` - Task killing on exec'd code
- `suspended_exec_task_stack_matches_suspended_task` - Task stack introspection

**Action Required**: Review exec implementation and interaction with task management.

### 8. HTTP Builtins (6 failures)

**Pattern**: Tests for HTTP-related builtins (likely `read_http()`).

**Examples**:
- `non_wizard_cannot_call_no_arg_version` - Permission checks
- `read_http_invalid_type_foobar` - Type validation

**Action Required**: Implement HTTP builtins or mark as intentionally unsupported.

### 9. Caller Permissions (1 failure)

**Test**: `caller_perms_top_level_eval`

**Issue**: Expected `#3`, got `#-1`

**Action Required**: Fix `caller_perms()` behavior for top-level eval context (should return object that set eval's permissions, not `#-1`).

## Next Steps

### High Priority (Quick Wins)

1. **Implement GC builtins** (23 tests) - `run_gc()`, `gc_stats()`
2. **Fix caller_perms eval context** (1 test) - Should return `#3` not `#-1`
3. **Fix HTTP builtin registration/implementation** (6 tests)

### Medium Priority (Core Functionality)

4. **Review verb management** (18 tests) - Recycled object handling
5. **Review property management** (18 tests) - Clear property semantics
6. **Fix object creation validation** (11 tests) - Error handling

### Lower Priority (Advanced Features)

7. **Implement task_local()** (8 tests)
8. **Review exec task management** (7 tests)
9. **Complete limits enforcement** (9 tests) - Already registered, needs enforcement
10. **Review waif indexing** (2 tests)

## Test Execution Details

```bash
# Server startup
cd /c/Users/Q/code/barn
./barn_test.exe -db Test.db -port 9300 > server.log 2>&1 &

# Test execution
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -q

# Results
128 failed, 1222 passed, 128 skipped in 12.43s
```

## Comparison to Previous Run

**Before Fix** (with limits not registered):
- 161 failed
- 1,189 passed
- 87.9% pass rate

**After Fix** (with limits registered):
- 128 failed (-33)
- 1,222 passed (+33)
- 90.5% pass rate (+2.6%)

**Improvement**: The single-line fix adding `limits.Register(registry)` resolved 33 test failures, demonstrating the importance of proper builtin registration.
