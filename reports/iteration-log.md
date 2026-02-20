# Iteration Log

## Baseline
- Total: 2756 | Pass: 2548 | Fail: 25 | Skip: 183

### Failure Clusters

**Cluster A: Error traceback (12 failures)**
- error_traceback::caught_exception_has_four_elements
- error_traceback::exception_fourth_element_is_list
- error_traceback::exception_stack_has_frames
- error_traceback::exception_stack_frame_has_six_fields
- error_traceback::exception_stack_contains_error_verb
- error_traceback::exception_stack_verb_order_in_chain
- error_traceback::exception_stack_frame_this_field
- error_traceback::exception_stack_frame_vloc_field
- error_traceback::exception_stack_frame_player_field
- error_traceback::exception_stack_line_number_is_positive_int
- error_traceback::exception_stack_line_number_correct
- error_traceback::exception_traceback_has_two_frames

**Cluster B: Task stack (7 failures)**
- task_management::task_stack_current_task_is_invalid
- task_management::task_stack_suspended_returns_list
- task_management::task_stack_suspended_has_frames
- task_management::task_stack_frame_has_five_elements
- task_management::task_stack_with_line_numbers_frame_has_six_elements
- task_management::task_stack_frame_verb_name
- task_management::task_stack_line_number_value

**Cluster C: Call stack / callers (3 failures)**
- call_stack::callers_three_deep_verb_names
- call_stack::callers_line_numbers_are_positive_integers
- call_stack::callers_line_number_reflects_call_site

**Cluster D: Standalone (3 failures)**
- math::ctime_with_int_arg_is_invarg
- dump_persistence::inherited_override_survives_dump_and_restart
- fork_timing::fork_zero_delay_executes

---

## 001 - 2026-02-20
- Start: 25 failures
- Targets: Cluster A (error traceback), Cluster B (task_stack), Cluster C (callers)
- Result: 8 failures (17 fixed, 0 regressions)
- Commits: 111cfd8
- Fixed: All 12 error_traceback, all 3 call_stack, 1 task_stack_current_task, 1 fork_timing
- Remaining: 6 task_stack fork tests, 1 ctime, 1 dump_persistence

---

## 002 - 2026-02-20
- Start: 8 failures
- Targets: Remaining task_stack fork tests (6), fork scheduling bugs
- Result: 2 failures (6 fixed, 0 regressions)
- Commits: 3a6a831
- Fixed: All 6 task_stack fork tests (double-scheduling, initial frame, line numbers, frame order)
- Remaining: math::ctime_with_int_arg_is_invarg (platform), dump_persistence::inherited_override_survives_dump_and_restart (persistence)
- Final: 2571 passed, 2 failed, 183 skipped

---

## 003 - 2026-02-20
- Start: 2 failures
- Targets: dump_persistence::inherited_override_survives_dump_and_restart
- Root causes: (1) dump_database() was a stub (never called checkpoint), (2) writer used stale PropOrder instead of recomputing from parent chain, (3) add_property didn't update PropOrder/PropDefsCount
- Fixes: Wire dump_database() to server checkpoint, recompute property order from parent chain in writer, update PropOrder in add_property
- Result: 1 failure (1 fixed, 0 regressions)
- Remaining: math::ctime_with_int_arg_is_invarg (test bug: Toast source shows ctime accepts optional INT arg, test incorrectly expects E_INVARG for ctime(0))
- Final: 2572 passed, 1 failed, 183 skipped
