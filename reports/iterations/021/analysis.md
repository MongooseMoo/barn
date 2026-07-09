# Iteration 021 - Read Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 020: 27 failed, 11308 passed, 126 skipped
- Target: `read_call_shapes`
- Target failures: 5

## Target

Fix `read_call_shapes` next, because it is the largest remaining failure family.

## Changes

- Matched `read(player, nonblocking)` validation order by checking that the target player is connected before returning the nonblocking no-input value.
- Kept permission checks before the connection check.

## Verification

- Targeted `read_call_shapes` before: 5 failed, 2 passed, 11454 deselected.
- Targeted `read_call_shapes` after: 7 passed, 11454 deselected.
- Full managed Barn conformance after: 22 failed, 11313 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 5 full-suite failures from `read_call_shapes`.
- Remaining largest cluster is `chr_call_shapes` with 4 failures.
