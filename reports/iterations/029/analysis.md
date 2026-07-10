# Iteration 029 - Respond To Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 028: 2 failed, 11333 passed, 126 skipped
- Target: `respond_to_call_shapes`
- Target failures: 1

## Target

Fix `respond_to_call_shapes` next, because it was one of the two remaining singleton failures.

## Changes

- Normalized invalid object references in `respond_to()` to `E_INVARG`.
- Preserved `E_TYPE` for non-object first arguments.

## Verification

- Targeted `respond_to_call_shapes` after: 7 passed, 11454 deselected.
- Full managed Barn conformance after: 1 failed, 11334 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 1 full-suite failure from `respond_to_call_shapes`.
- Remaining failure is `clear_property_call_shapes::clear_property_arg_obj`.
