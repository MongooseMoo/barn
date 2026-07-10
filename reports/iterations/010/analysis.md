# Iteration 010 - Background Threads

## Baseline

- Date: 2026-07-09
- Barn result after iteration 009: 102 failed, 11233 passed, 126 skipped
- Target: `background_threads`
- Target failures: 9

## Target

Fix `background_threads` next, because it is the largest remaining failure family.

## Changes

- Added per-task `ThreadMode` defaulting to Toast's enabled state.
- Matched `set_thread_mode()` reporting current mode while `set_thread_mode(arg)` updates mode and returns `0`.
- Matched `thread_pool("INIT", "MAIN", n)` wizard-only validation and success shape for non-negative sizes.
- Matched `threads()` wizard-only integer-handle reporting for queued/running/suspended tasks.
- Implemented `background_test(str, delay)` as immediate echo for zero delay or disabled thread mode, and as an async suspended task for positive delay.
- Cleared the indefinite-suspend scheduling sentinel when async completion requeues a task through `CompleteExec`, which also fixed the existing exec conformance failures.

## Verification

- Targeted `background_threads` before: 9 failed, 3 passed, 11449 deselected.
- Targeted thread groups after: 13 passed, 11448 deselected.
- Full managed Barn conformance after: 90 failed, 11245 passed, 126 skipped.
- `go test ./...` is blocked by existing scheduler test `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`.

## Result

- Fixed 12 full-suite failures: 9 from `background_threads` plus 3 from `exec`.
- Remaining largest clusters are 7-failure groups: `connection_input_call_shapes`, `url_curl`, `optional_extensions`, `add_property_call_shapes`, and `disassemble_call_shapes`.
