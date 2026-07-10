# Iteration 020 - Listeners Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 019: 32 failed, 11303 passed, 126 skipped
- Target: `listeners_call_shapes`
- Target failures: 5

## Target

Fix `listeners_call_shapes` next, because it is the first largest remaining failure family.

## Changes

- Matched `listeners(filter)` tolerance by returning an empty listener list for unsupported or malformed filters instead of raising.
- Kept object, integer port, and valid descriptor-map filtering unchanged.

## Verification

- Targeted `listeners_call_shapes` before: 5 failed, 1 passed, 11455 deselected.
- Targeted `listeners_call_shapes` after: 6 passed, 11455 deselected.
- Full managed Barn conformance after: 27 failed, 11308 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 5 full-suite failures from `listeners_call_shapes`.
- Remaining largest cluster is `read_call_shapes` with 5 failures.
