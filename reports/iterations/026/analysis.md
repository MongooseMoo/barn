# Iteration 026 - Delete Verb Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 025: 8 failed, 11327 passed, 126 skipped
- Target: `delete_verb_call_shapes`
- Target failures: 2

## Target

Fix `delete_verb_call_shapes` next, because it was one of the largest remaining failure families.

## Changes

- Accepted integer verb descriptors in `delete_verb()`, matching the same name-or-index descriptor shape used by other verb builtins.
- Normalized invalid object references with otherwise valid descriptors to `E_INVARG`.
- Preserved `E_TYPE` for descriptor values that are neither string nor integer.

## Verification

- Targeted `delete_verb_call_shapes` after: 49 passed, 11412 deselected.
- Full managed Barn conformance after: 6 failed, 11329 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 2 full-suite failures from `delete_verb_call_shapes`.
- Remaining largest clusters are `mapvalues_call_shapes` and `network_matrix` with 2 failures each.
