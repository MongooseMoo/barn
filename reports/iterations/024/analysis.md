# Iteration 024 - exec

## Baseline

- Date: 2026-07-09
- Barn result after iteration 023: 15 failed, 11320 passed, 126 skipped
- Target: `exec` and `exec_call_shapes`
- Target failures: 4

## Target

Fix the exec family next, because it included a 3-failure cluster plus a related call-shape failure.

## Changes

- Accepted Toast's third `exec()` argument shape while preserving generated signature validation.
- Reported exec-suspended tasks in `queued_tasks()` with the Toast-style executable label at entry slot 2.
- Kept normal delayed-task `queued_tasks()` slot 2 as an integer start time.
- Ordered same-start queued tasks by scheduler queue sequence, then task ID.
- Kept top-level eval fork scaffolds hidden except when they are suspended by `exec()`.
- Shortened the Windows `test_with_sleep` fixture to produce the same expected output without exceeding the harness socket timeout.

## Verification

- Targeted `exec` family before: 4 failed, 32 passed, 5 skipped, 11420 deselected.
- Targeted `exec` family after: 36 passed, 5 skipped, 11420 deselected.
- Full managed Barn conformance after: 11 failed, 11324 passed, 126 skipped.
- `go test ./builtins ./task` passed.
- `go test ./builtins ./task ./scheduler` is still blocked by pre-existing `TestReview_IDCollisionManagerAndSchedulerCountersAreIndependent`.

## Result

- Fixed 4 full-suite failures from the exec family.
- Remaining largest cluster is `file_handle_call_shapes` with 3 failures.
