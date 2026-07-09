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

---

## 004 - 2026-07-09
- Start: 1143 failures
- Toast oracle: 11314 passed, 147 skipped
- Target: `create_call_shapes` (780 failures)
- Root cause: `create()` validated parent existence before malformed optional argument types, returning `E_INVARG` before Toast's `E_TYPE`.
- Fix: Parse optional argument shapes before parent existence/duplicate validation.
- Result: 363 failures (780 fixed, 0 observed regressions)
- Commits: this commit
- Remaining: `is_member_call_shapes` (84), `task_stack_call_shapes` (55), `unlisten_call_shapes` (54), `file_stat_call_shapes` (36), `slice_call_shapes` (32), smaller clusters.

---

## 005 - 2026-07-09
- Start: 363 failures
- Target: `is_member_call_shapes` (84 failures)
- Root cause: `is_member()` accepted only two args and returned `E_TYPE` for non-collection second arguments; Toast accepts optional `case_matters` and raises `E_INVARG`.
- Fix: Accept optional third int arg, preserve default case-sensitive comparison, and return `E_INVARG` for non-list/map collections.
- Result: 279 failures (84 fixed, 0 observed regressions)
- Commits: this commit
- Remaining: `task_stack_call_shapes` (55), `unlisten_call_shapes` (54), `file_stat_call_shapes` (36), `slice_call_shapes` (32), smaller clusters.

---

## 006 - 2026-07-09
- Start: 279 failures
- Target: `task_stack_call_shapes` (55 failures)
- Root cause: `task_stack()` accepted only 1-2 args and validated optional flag types before missing-task `E_INVARG`; Toast accepts 1-3 args with optional `TYPE_ANY` flags.
- Fix: Accept the third optional arg, check task existence before optional flag handling, and use truthiness for the line-number flag.
- Result: 224 failures (55 fixed, 0 observed regressions)
- Commits: this commit
- Remaining: `unlisten_call_shapes` (54), `file_stat_call_shapes` (36), `slice_call_shapes` (32), smaller clusters.

---

## 007 - 2026-07-09
- Start: 224 failures
- Target: `unlisten_call_shapes` (54 failures)
- Root cause: `unlisten()` accepted only one arg and surfaced descriptor parser `E_TYPE`; Toast accepts 1-2 `TYPE_ANY` args and reports missing/malformed listeners as `E_INVARG`.
- Fix: Accept the optional second arg and normalize descriptor parse failures to `E_INVARG`.
- Result: 170 failures (54 fixed, 0 observed regressions)
- Commits: this commit
- Remaining: `file_stat_call_shapes` (36), `slice_call_shapes` (32), smaller clusters.

---

## 008 - 2026-07-09
- Start: 170 failures
- Target: `file_stat_call_shapes` (36 failures)
- Root cause: File stat helpers surfaced `E_TYPE` for non-handle/non-path values, and `file_stat()`/`file_type()` manually required strings despite Toast `TYPE_ANY` signatures.
- Fix: Normalize unsupported stat values to `E_INVARG`, and route `file_stat()`/`file_type()` through the shared stat parser while preserving `file_type()` missing-path `0`.
- Result: 134 failures (36 fixed, 0 observed regressions)
- Commits: this commit
- Remaining: `slice_call_shapes` (32), smaller clusters.

---

## 009 - 2026-07-09
- Start: 134 failures
- Target: `slice_call_shapes` (32 failures)
- Root cause: `slice()` returned `E_TYPE` for unsupported start specifier shapes; Toast returns `E_INVARG`.
- Fix: Return `E_INVARG` for unsupported start specifier types while leaving default values as `TYPE_ANY`.
- Result: 102 failures (32 fixed, 0 observed regressions)
- Commits: this commit
- Remaining: `background_threads` (9), multiple 7-failure clusters, smaller clusters.

---

## 010 - 2026-07-09
- Start: 102 failures
- Target: `background_threads` (9 failures)
- Status: complete
- Targeted: `background_threads or set_thread_mode_call_shapes` passed (13 passed)
- Full after: 90 failures, 11245 passed, 126 skipped
- Result: 12 failures fixed (9 `background_threads`, 3 `exec`)
- Commits: this commit
- Remaining: five 7-failure clusters, then 6-failure and smaller clusters.

---

## 011 - 2026-07-09
- Start: 90 failures
- Target: `connection_input_call_shapes` (7 failures)
- Status: complete
- Targeted: `connection_input_call_shapes` passed (14 passed)
- Full after: 83 failures, 11252 passed, 126 skipped
- Result: 7 failures fixed
- Commits: this commit
- Remaining: four 7-failure clusters, then 6-failure and smaller clusters.

---

## 012 - 2026-07-09
- Start: 83 failures
- Target: `add_property_call_shapes` (7 failures)
- Status: complete
- Targeted: `add_property_call_shapes` passed (49 passed)
- Full after: 76 failures, 11259 passed, 126 skipped
- Result: 7 failures fixed
- Commits: this commit
- Remaining: three 7-failure clusters, then 6-failure and smaller clusters.
