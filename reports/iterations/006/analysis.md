# Iteration 006 - Task Stack Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 005: 279 failed, 11056 passed, 126 skipped
- Target: `task_stack_call_shapes`
- Target failures: 55

## Target

Fix `task_stack()` call-shape divergences next, because it is the largest remaining failure family.

## Root Cause

Barn accepted only 1-2 arguments and validated the optional line-number flag before checking whether the task id referred to a suspended task. Toast's generated signature accepts 1-3 arguments with optional `TYPE_ANY` flags, and missing/current task ids raise `E_INVARG` before optional flag semantics matter.

## Fix

Accept the third optional argument, move task-id lookup and permission validation before optional flag handling, and interpret the existing line-number flag with MOO truthiness.

## Verification

- Targeted: `task_stack_call_shapes` passed, 56 passed and 11405 deselected.
- Full managed Barn run: 224 failed, 11111 passed, 126 skipped.
- Net: 55 failures fixed, no observed regressions.

## Next Target

`unlisten_call_shapes`: 54 failures.
