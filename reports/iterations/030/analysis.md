# Iteration 030 - Clear Property Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 029: 1 failed, 11334 passed, 126 skipped
- Target: `clear_property_call_shapes`
- Target failures: 1

## Target

Fix `clear_property_call_shapes`, the final remaining conformance failure.

## Changes

- Masked missing-property errors with `E_PERM` for non-wizard non-owners.
- Moved `clear_property()` write-permission validation before the local-defined check, matching Toast's error precedence for programmer calls on `#0`.

## Verification

- Targeted `clear_property_call_shapes` after: 12 passed, 11449 deselected.
- Full managed Barn conformance after: 11335 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed the final full-suite failure from `clear_property_call_shapes`.
- Barn managed conformance is fully green.
