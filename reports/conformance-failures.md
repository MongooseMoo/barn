# Conformance Test Failures Report

**Date:** 2025-12-30
**Server:** Barn (Go MOO Server)
**Database:** Test.db
**Test Suite:** cow_py conformance tests

## Summary

- **Total Tests:** 1478
- **Passed:** 1198 (81.1%)
- **Failed:** 161 (10.9%)
- **Skipped:** 119 (8.0%)
- **Pass Rate:** 88.1% (excluding skipped tests)

## Test Execution

```bash
# Build and start server
cd /c/Users/Q/code/barn
go build -o barn_test.exe ./cmd/barn/
./barn_test.exe -db Test.db -port 9300 > server.log 2>&1 &

# Run conformance tests
cd /c/Users/Q/code/cow_py
uv run pytest tests/conformance/ --transport socket --moo-port 9300 -v
```

## Failure Analysis by Category

### 1. Caller Permissions (1 failure)

**caller_perms::caller_perms_top_level_eval**
- Issue: caller_perms() behavior at top-level eval

### 2. Garbage Collection (22 failures)

All GC-related tests are failing, indicating GC functionality is not implemented:

- `gc::run_gc_requires_wizard_perms` - GC permission check
- `gc::run_gc_allows_wizard` - GC execution
- `gc::gc_stats_requires_wizard_perms` - GC stats permission
- `gc::gc_stats_allows_wizard` - GC stats access
- `gc::gc_stats_returns_map` - GC stats structure
- `gc::gc_stats_has_purple_key` - GC stats content
- `gc::gc_stats_has_black_key` - GC stats content
- `gc::gc_stats_purple_is_int` - GC stats types
- `gc::gc_stats_black_is_int` - GC stats types
- `gc::nested_list_no_possible_root` - GC collection behavior
- `gc::nested_map_no_possible_root` - GC collection behavior
- `gc::run_gc_doesnt_crash_with_anonymous` - GC with anonymous objects
- `gc::single_cyclic_self_reference_basic` - Cyclic reference handling
- `gc::single_cyclic_self_reference_with_recycle` - Cyclic with recycle
- `gc::dual_cyclic_self_references` - Multiple cyclic refs
- `gc::dual_cyclic_self_references_with_recycle` - Multiple cyclic with recycle
- `gc::cyclic_references_through_list` - Cyclic via collections
- `gc::cyclic_references_through_list_with_recycle` - Cyclic collection with recycle
- `gc::cyclic_references_through_map` - Cyclic via maps
- `gc::cyclic_references_through_map_with_recycle` - Cyclic map with recycle
- `gc::empty_list_is_green` - GC color marking
- `gc::empty_map_is_green` - GC color marking

**Common Error:** E_VERBNF (verb not found) - indicates `run_gc()` and `gc_stats()` builtins are missing

### 3. HTTP (6 failures)

HTTP functionality not implemented:

- `http::non_wizard_cannot_call_no_arg_version` - HTTP permission check
- `http::read_http_no_args_fails` - Argument validation
- `http::read_http_invalid_type_foobar` - Type validation
- `http::read_http_invalid_type_empty_string` - Type validation
- `http::read_http_type_arg_not_string` - Type checking
- `http::read_http_connection_arg_not_obj` - Type checking

**Common Error:** E_VERBNF - indicates `read_http()` builtin is missing

### 4. JSON (4 failures)

- `json::generate_json_escape_tab` - Tab character escaping
- `json::generate_json_anon_obj` - Anonymous object serialization
- `json::generate_json_anon_obj_common` - Anonymous object (common mode)
- `json::generate_json_anon_obj_embedded` - Anonymous object (embedded mode)

### 5. Map Operations (5 failures)

- `map::mapdelete_removes_entry` - Delete operation
- `map::mapdelete_chain` - Chained delete operations
- `map::first_last_index` - Index operations
- `map::ranged_set_invalid_range_2` - Range validation
- `map::ranged_set_invalid_range_3` - Range validation
- `map::ranged_set_merge_existing_key` - Range set with merge
- `map::inverted_ranged_set_in_loop` - Inverted range operations

### 6. Object Creation/Management (10 failures)

- `objects::create_invalid_owner_invarg` - Owner validation
- `objects::create_invalid_owner_ambiguous` - Owner resolution
- `objects::create_invalid_owner_failed_match` - Owner matching
- `objects::create_invalid_owner_invarg_as_programmer` - Owner validation (programmer)
- `objects::create_invalid_parent_ambiguous` - Parent resolution
- `objects::create_invalid_parent_failed_match` - Parent matching
- `objects::create_list_invalid_ambiguous` - Parent list validation
- `objects::create_list_invalid_failed_match` - Parent list matching
- `objects::renumber_basic` - Object renumbering
- `objects::verb_cache_basic` - Verb cache operations

**Common Error:** E_VERBNF - indicates `renumber()` and `verb_cache_stats()` builtins missing

### 7. Primitives (4 failures)

- `primitives::queued_tasks_includes_this_map` - Task info structure
- `primitives::callers_includes_this_list` - Caller info structure
- `primitives::inheritance_with_prototypes` - Prototype inheritance
- `primitives::pass_works_with_prototypes` - Pass with prototypes

### 8. Properties (13 failures)

Property operation validation issues:

- `properties::add_property_invalid_owner` - Owner validation
- `properties::add_property_invalid_perms` - Permission checking
- `properties::add_property_recycled_object` - Recycled object handling
- `properties::add_property_builtin_name` - Builtin name collision
- `properties::add_property_defined_on_descendant` - Descendant property checks
- `properties::add_property_not_owner` - Owner enforcement
- `properties::delete_property_recycled_object` - Delete on recycled
- `properties::is_clear_property_works` - Clear status checking
- `properties::is_clear_property_recycled_object` - Clear on recycled
- `properties::is_clear_property_builtin` - Clear on builtin
- `properties::is_clear_property_with_read_permission` - Permission interaction
- `properties::is_clear_property_wizard_bypasses_read` - Wizard bypass
- `properties::clear_property_recycled_object` - Clear recycled
- `properties::clear_property_builtin` - Clear builtin
- `properties::clear_property_on_definer` - Clear on definer
- `properties::property_info_recycled_object` - Info on recycled
- `properties::set_property_info_recycled_object` - Set info recycled
- `properties::properties_recycled_object` - List properties recycled

### 9. Recycle (2 failures)

- `recycle::recycle_invalid_already_recycled_object` - Double recycle check
- `recycle::recycle_invalid_already_recycled_anonymous` - Anonymous double recycle

### 10. Switch Player (2 failures)

- `switch_player::non_wizard_gets_E_PERM` - Permission check
- `switch_player::programmer_cannot_switch_player` - Programmer restriction

**Common Error:** E_VERBNF - indicates `switch_player()` builtin is missing

### 11. Task Local (8 failures)

Task-local storage not implemented:

- `task_local::fork_and_suspend_case` - Fork with task locals
- `task_local::command_verb` - Command context
- `task_local::server_verb` - Server context
- `task_local::across_verb_calls` - Cross-verb persistence
- `task_local::across_verb_calls_with_intermediate` - Multi-level calls
- `task_local::suspend_between_verb_calls` - Suspend interaction
- `task_local::read_between_verb_calls` - Read after suspend
- `task_local::nonfunctional_kill_task` - Kill task interaction

**Common Error:** E_VERBNF - indicates task-local builtins missing

### 12. Verbs (16 failures)

Verb operation validation:

- `verbs::add_verb_basic` - Basic add
- `verbs::add_verb_invalid_owner` - Owner validation
- `verbs::add_verb_invalid_perms` - Permission check
- `verbs::add_verb_invalid_args` - Argument validation
- `verbs::add_verb_recycled_object` - Recycled object handling
- `verbs::add_verb_with_write_permission` - Write permission
- `verbs::add_verb_wizard_bypasses_write` - Wizard bypass
- `verbs::add_verb_not_owner` - Owner enforcement
- `verbs::add_verb_is_owner` - Owner check
- `verbs::add_verb_wizard_sets_owner` - Wizard ownership
- `verbs::delete_verb_recycled_object` - Delete recycled
- `verbs::verb_info_recycled_object` - Info recycled
- `verbs::verb_args_basic` - Basic args
- `verbs::verb_args_recycled_object` - Args recycled
- `verbs::verb_code_recycled_object` - Code recycled
- `verbs::set_verb_info_recycled_object` - Set info recycled
- `verbs::set_verb_args_recycled_object` - Set args recycled
- `verbs::set_verb_code_recycled_object` - Set code recycled
- `verbs::verbs_recycled_object` - List verbs recycled

### 13. Anonymous Objects (1 failure)

- `anonymous::recycle_invalid_anonymous_no_crash` - Anonymous recycle safety

### 14. Index and Range (2 failures)

- `index_and_range::range_list_single` - Single-element range
- `index_and_range::decompile_with_index_operators` - Decompiler output

### 15. Waif (2 failures)

- `waif::nested_waif_map_indexes` - Nested waif indexing
- `waif::deeply_nested_waif_map_indexes` - Deep nesting

### 16. Exec (6 failures)

- `exec::exec_with_sleep_works` - Exec with sleep
- `exec::exec_rejects_invalid_binary_string` - Binary string validation
- `exec::kill_task_works_on_suspended_exec` - Kill suspended
- `exec::kill_task_fails_on_already_killed` - Double kill
- `exec::resume_fails_on_suspended_exec` - Resume suspended
- `exec::suspended_exec_task_stack_matches_suspended_task` - Stack matching

### 17. Limits (39 failures)

**Critical:** Most limit tests failing with E_VERBNF, suggesting limit enforcement infrastructure issues:

**List/Set Operations:**
- `limits::setadd_checks_list_max_value_bytes_small`
- `limits::setadd_checks_list_max_value_bytes_exceeds`
- `limits::setadd_fails_if_value_too_large`
- `limits::listinsert_checks_list_max_value_bytes`
- `limits::listinsert_fails_if_value_too_large`
- `limits::listappend_checks_list_max_value_bytes`
- `limits::listappend_fails_if_value_too_large`
- `limits::listset_fails_if_value_too_large`
- `limits::setremove_fails_if_result_too_large`
- `limits::listdelete_fails_if_result_too_large`

**List/Map Indexing:**
- `limits::appending_to_list_checks_max_value_bytes`
- `limits::prepending_to_list_checks_max_value_bytes`
- `limits::indexset_on_list_checks_max_value_bytes`
- `limits::rangeset_on_list_checks_max_value_bytes`
- `limits::assigning_list_fails_if_value_too_large`
- `limits::decode_binary_checks_list_max_value_bytes`
- `limits::mapdelete_fails_if_result_too_large`
- `limits::indexset_on_map_checks_max_value_bytes`
- `limits::rangeset_on_map_checks_max_value_bytes`

**Literals:**
- `limits::list_literal_checks_max_value_bytes`
- `limits::map_literal_checks_max_value_bytes`

**String Operations:**
- `limits::string_concat_limit`
- `limits::string_concat_exceeds_limit`
- `limits::tostr_exceeds_limit`
- `limits::toliteral_limit`
- `limits::toliteral_exceeds_limit`
- `limits::strsub_limit`
- `limits::strsub_exceeds_limit`
- `limits::encode_binary_limit`
- `limits::encode_binary_exceeds_limit`
- `limits::substitute_limit`
- `limits::substitute_exceeds_limit`
- `limits::encode_base64_limit`
- `limits::encode_base64_exceeds_limit`
- `limits::random_bytes_within_limit`
- `limits::random_bytes_exceeds_limit`

**Memory:**
- `limits::quota_errors_do_not_leak_memory`

**Common Error:** E_VERBNF (expected E_QUOTA or success) - indicates limit checking code is calling missing verbs/functions

### 18. Stress Objects (8 failures)

- `stress_objects::chparents_property_reset_multi` - Multi-parent property reset
- `stress_objects::object_bytes_permission_denied` - Permission check
- `stress_objects::object_bytes_wizard_allowed` - Wizard access
- `stress_objects::object_bytes_type_int` - Type handling (int)
- `stress_objects::object_bytes_type_float` - Type handling (float)
- `stress_objects::object_bytes_type_string` - Type handling (string)
- `stress_objects::object_bytes_recycled_object` - Recycled object
- `stress_objects::object_bytes_created_objects` - Created objects

**Common Error:** E_VERBNF - indicates `object_bytes()` builtin is missing

## Common Error Patterns

### E_VERBNF (Verb Not Found)

**Most common error** - indicates missing builtin functions:

- `run_gc()` / `gc_stats()` - GC functionality
- `read_http()` - HTTP support
- `renumber()` - Object renumbering
- `verb_cache_stats()` - Verb cache inspection
- `switch_player()` - Player switching
- Task-local storage builtins
- `object_bytes()` - Memory inspection
- Various limit-checking infrastructure functions

### Validation Issues

Tests expect proper error codes (E_INVARG, E_PERM, E_RECMOVE) but Barn returns E_VERBNF or crashes:

- Recycled object handling
- Owner/permission validation
- Ambiguous object resolution
- Builtin name collision detection

### Feature Gaps

Several ToastStunt extensions not implemented:

- Garbage collection (22 tests)
- HTTP support (6 tests)
- Task-local storage (8 tests)
- Anonymous object edge cases (4 tests)
- Waif advanced features (2 tests)

## Next Steps

### Priority 1: Missing Builtins (High Impact)

1. **Limit Checking Infrastructure** (39 failures)
   - Root cause appears to be limit-checking code calling missing functions
   - Need to trace why E_VERBNF instead of E_QUOTA or success

2. **Object Management** (18 failures)
   - Implement `renumber()`
   - Implement `object_bytes()`
   - Implement `verb_cache_stats()`

3. **Player Management** (2 failures)
   - Implement `switch_player()`

### Priority 2: Validation & Error Handling (Medium Impact)

1. **Recycled Object Handling** (~20 failures)
   - Add proper E_INVARG checks for recycled objects
   - Affects properties, verbs, and object operations

2. **Owner/Permission Validation** (~10 failures)
   - Improve owner resolution (ambiguous, failed match)
   - Add proper E_PERM/E_INVARG responses

3. **Map Operations** (5 failures)
   - Fix mapdelete
   - Fix range operations

### Priority 3: Extended Features (Lower Impact)

1. **Garbage Collection** (22 failures)
   - Implement `run_gc()` and `gc_stats()`
   - Add cyclic reference handling

2. **HTTP Support** (6 failures)
   - Implement `read_http()`

3. **Task-Local Storage** (8 failures)
   - Implement task-local storage builtins

4. **JSON Edge Cases** (4 failures)
   - Fix tab escaping
   - Fix anonymous object serialization

## Test Logs

- Full output: `C:\Users\Q\code\barn\conformance_full.log`
- Summary: `C:\Users\Q\code\barn\conformance_summary.log`
- Server log: `C:\Users\Q\code\barn\server.log`

## Notes

- Server ran successfully on port 9300
- No crashes detected during test execution
- Most core functionality (arithmetic, strings, basic objects) working correctly
- 81% pass rate on attempted tests shows solid foundation
- Main gaps are advanced/extension features and edge case handling
