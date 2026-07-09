# Iteration 019 - Is Clear Property Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 018: 37 failed, 11298 passed, 126 skipped
- Target: `is_clear_property_call_shapes`
- Target failures: 5

## Target

Fix `is_clear_property_call_shapes` next, because it is the first largest remaining failure family.

## Changes

- Normalized `is_clear_property(non_object, name)` from `E_TYPE` to Toast-compatible `E_INVARG`.
- Kept object and property-name handling unchanged.

## Verification

- Targeted `is_clear_property_call_shapes` before: 5 failed, 1 passed, 11455 deselected.
- Targeted `is_clear_property_call_shapes` after: 6 passed, 11455 deselected.
- Full managed Barn conformance after: 32 failed, 11303 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 5 full-suite failures from `is_clear_property_call_shapes`.
- Remaining largest clusters are 5-failure groups: `listeners_call_shapes` and `read_call_shapes`.
