# Iteration 004 - Create Call Shapes

## Baseline

- Date: 2026-07-09
- Barn command: `uv run --project ..\moo-conformance-tests moo-conformance --server-command "C:/Users/Q/code/barn/barn.exe -db {db} -port {port}" --tb=no -q --junitxml .tmp\latest-conformance-barn-after-toast-green.xml`
- Barn result: 1143 failed, 10192 passed, 126 skipped
- Toast oracle result: 11314 passed, 147 skipped

## Failure Ordering

1. `create_call_shapes`: 780
2. `is_member_call_shapes`: 84
3. `task_stack_call_shapes`: 55
4. `unlisten_call_shapes`: 54
5. `file_stat_call_shapes`: 36

## Target

Fix `create_call_shapes` first. Toast has already passed the full suite with the WSL oracle, so these rows are confirmed Barn divergences unless a focused repro proves the harness is selecting a different behavior.

## Root Cause

Barn validated parent existence before parsing optional argument shapes. Toast reports malformed optional argument types first, even when the parent object is invalid. This made invalid-parent plus bad-optional-argument rows return `E_INVARG` where Toast returns `E_TYPE`.

## Fix

Move parent existence, duplicate-parent, and duplicate-property validation after optional argument parsing in `builtinCreate`, while keeping first-argument structural type checks first.

## Verification

- Targeted: `create_call_shapes` passed, 2802 passed and 8659 deselected.
- Full managed Barn run: 363 failed, 10972 passed, 126 skipped.
- Net: 780 failures fixed, no observed regressions.

## Next Target

`is_member_call_shapes`: 84 failures.
