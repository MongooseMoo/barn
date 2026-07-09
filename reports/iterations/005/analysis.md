# Iteration 005 - Is Member Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 004: 363 failed, 10972 passed, 126 skipped
- Target: `is_member_call_shapes`
- Target failures: 84

## Target

Fix `is_member()` call-shape divergences next, because it is the largest remaining failure family after `create_call_shapes`.

## Root Cause

Barn's `is_member()` accepted exactly two arguments and returned `E_TYPE` for non-list/map collection arguments. Toast's generated signature is `TYPE_ANY, TYPE_ANY, TYPE_INT` with 2-3 arguments, and non-collection second arguments raise `E_INVARG`.

## Fix

Accept the optional third `case_matters` integer argument, preserve existing two-argument case-sensitive behavior, and return `E_INVARG` when the second argument is not a list or map.

## Verification

- Targeted: `is_member_call_shapes` passed, 98 passed and 11363 deselected.
- Full managed Barn run: 279 failed, 11056 passed, 126 skipped.
- Net: 84 failures fixed, no observed regressions.

## Next Target

`task_stack_call_shapes`: 55 failures.
