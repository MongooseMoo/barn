# Iteration 007 - Unlisten Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 006: 224 failed, 11111 passed, 126 skipped
- Target: `unlisten_call_shapes`
- Target failures: 54

## Target

Fix `unlisten()` call-shape divergences next, because it is the largest remaining failure family.

## Root Cause

Barn accepted only one `unlisten()` argument and propagated descriptor parser `E_TYPE` errors. Toast's generated signature accepts 1-2 `TYPE_ANY` arguments, and missing or malformed listener descriptors on this surface raise `E_INVARG`.

## Fix

Accept the optional second argument and normalize listener descriptor parse failures to `E_INVARG` before attempting removal.

## Verification

- Targeted: `unlisten_call_shapes` passed, 56 passed and 11405 deselected.
- Full managed Barn run: 170 failed, 11165 passed, 126 skipped.
- Net: 54 failures fixed, no observed regressions.

## Next Target

`file_stat_call_shapes`: 36 failures.
