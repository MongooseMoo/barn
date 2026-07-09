# Iteration 018 - Callers Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 017: 43 failed, 11292 passed, 126 skipped
- Target: `callers_call_shapes`
- Target failures: 6

## Target

Fix `callers_call_shapes` next, because it is the largest remaining failure family.

## Changes

- Matched Toast's optional `callers(arg)` behavior by using MOO truthiness for any value instead of requiring an integer.
- Kept arity and frame construction unchanged.

## Verification

- Targeted `callers_call_shapes` before: 6 failed, 11455 deselected.
- Targeted `callers_call_shapes` after: 6 passed, 11455 deselected.
- Full managed Barn conformance after: 37 failed, 11298 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 6 full-suite failures from `callers_call_shapes`.
- Remaining largest clusters are 5-failure groups: `is_clear_property_call_shapes`, `listeners_call_shapes`, and `read_call_shapes`.
