# Iteration 008 - File Stat Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 007: 170 failed, 11165 passed, 126 skipped
- Target: `file_stat_call_shapes`
- Target failures: 36

## Target

Fix file stat/listing call-shape divergences next, because it is the largest remaining failure family.

## Root Cause

Barn's shared file-stat parser returned `E_TYPE` for non-handle/non-path values, and `file_stat()`/`file_type()` still manually required strings despite Toast registering these builtins as `TYPE_ANY`.

## Fix

Return `E_INVARG` from the shared file-stat parser for unsupported value shapes, route `file_stat()` through that parser, and route `file_type()` through it while preserving the existing missing-path `0` result.

## Verification

- Targeted: `file_stat_call_shapes` passed, 42 passed and 11419 deselected.
- Full managed Barn run: 134 failed, 11201 passed, 126 skipped.
- Net: 36 failures fixed, no observed regressions.

## Next Target

`slice_call_shapes`: 32 failures.
