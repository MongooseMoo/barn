# Iteration 009 - Slice Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 008: 134 failed, 11201 passed, 126 skipped
- Target: `slice_call_shapes`
- Target failures: 32

## Target

Fix `slice()` call-shape divergences next, because it is the largest remaining failure family.

## Root Cause

Barn returned `E_TYPE` for unsupported `slice()` start specifier shapes. Toast reports unsupported start specifiers as `E_INVARG`; the default value argument remains `TYPE_ANY`.

## Fix

Return `E_INVARG` from `builtinSlice` for unsupported start specifier types.

## Verification

- Targeted: `slice_call_shapes` passed, 57 passed and 11404 deselected.
- Full managed Barn run: 102 failed, 11233 passed, 126 skipped.
- Net: 32 failures fixed, no observed regressions.

## Next Target

`background_threads`: 9 failures.
