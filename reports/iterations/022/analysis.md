# Iteration 022 - Chr Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 021: 22 failed, 11313 passed, 126 skipped
- Target: `chr_call_shapes`
- Target failures: 4

## Target

Fix `chr_call_shapes` next, because it is the largest remaining failure family.

## Changes

- Matched Toast's error taxonomy for `chr()` by returning `E_INVARG` for non-integer/list/string/map arguments.

## Verification

- Targeted `chr_call_shapes` before: 4 failed, 11457 deselected.
- Targeted `chr_call_shapes` after: 4 passed, 11457 deselected.
- Full managed Barn conformance after: 18 failed, 11317 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 4 full-suite failures from `chr_call_shapes`.
- Remaining largest clusters are `read_stdin`, `exec`, and `file_handle_call_shapes` with 3 failures each.
