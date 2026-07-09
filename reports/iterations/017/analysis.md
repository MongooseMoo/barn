# Iteration 017 - Listappend Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 016: 49 failed, 11286 passed, 126 skipped
- Target: `listappend_call_shapes`
- Target failures: 6

## Target

Fix `listappend_call_shapes` next, because it is the first largest remaining failure family.

## Changes

- Matched Toast's accepted append boundary by allowing `listappend(list, value, length(list)+1)`.
- Kept negative and larger out-of-range indices returning `E_RANGE`.

## Verification

- Targeted `listappend_call_shapes` before: 6 failed, 5 passed, 11450 deselected.
- Targeted `listappend_call_shapes` after: 11 passed, 11450 deselected.
- Full managed Barn conformance after: 43 failed, 11292 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 6 full-suite failures from `listappend_call_shapes`.
- Remaining largest cluster is `callers_call_shapes` with 6 failures.
