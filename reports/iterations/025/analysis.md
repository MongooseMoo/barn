# Iteration 025 - File Handle Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 024: 11 failed, 11324 passed, 126 skipped
- Target: `file_handle_call_shapes`
- Target failures: 3

## Target

Fix `file_handle_call_shapes` next, because it was the largest remaining failure family.

## Changes

- Matched Toast's handle-based `file_count_lines(FHANDLE)` behavior.
- Matched Toast's handle-based `file_grep(FHANDLE, STR [, INT])` behavior.
- Invalid literal handle `0` now reaches `getFileHandle()` and returns `E_INVARG`.
- Kept unreadable handles rejected with `E_INVARG`.

## Verification

- Targeted `file_handle_call_shapes` after: 4 passed, 11457 deselected.
- Full managed Barn conformance after: 8 failed, 11327 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 3 full-suite failures from `file_handle_call_shapes`.
- Remaining largest clusters are `delete_verb_call_shapes`, `mapvalues_call_shapes`, and `network_matrix` with 2 failures each.
