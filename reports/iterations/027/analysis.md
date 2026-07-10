# Iteration 027 - Mapvalues Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 026: 6 failed, 11329 passed, 126 skipped
- Target: `mapvalues_call_shapes`
- Target failures: 2

## Target

Fix `mapvalues_call_shapes` next, because it was one of the largest remaining failure families.

## Changes

- Removed pre-validation of variadic `mapvalues()` lookup keys as storable map keys.
- Let composite lookup keys miss normally and report `E_RANGE`, matching Toast's `TYPE_ANY` variadic key behavior.
- Added direct builtin and registry dispatch coverage for list and map lookup keys.

## Verification

- Targeted `mapvalues_call_shapes` after: 6 passed, 11455 deselected.
- Full managed Barn conformance after: 4 failed, 11331 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 2 full-suite failures from `mapvalues_call_shapes`.
- Remaining largest cluster is `network_matrix` with 2 failures.
