# Iteration 016 - Server Version Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 015: 55 failed, 11280 passed, 126 skipped
- Target: `server_version_call_shapes`
- Target failures: 6

## Target

Fix `server_version_call_shapes` next, because it is the first largest remaining failure family.

## Changes

- Matched generated `server_version([TYPE_ANY])` behavior by returning the full version-info list for non-string detail arguments.
- Kept string key behavior unchanged, including `server_version("")` returning the full list and invalid string paths returning `E_INVARG`.

## Verification

- Targeted `server_version_call_shapes` before: 6 failed, 1 passed, 11454 deselected.
- Targeted `server_version_call_shapes` after: 7 passed, 11454 deselected.
- Full managed Barn conformance after: 49 failed, 11286 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 6 full-suite failures from `server_version_call_shapes`.
- Remaining largest clusters are 6-failure groups: `listappend_call_shapes` and `callers_call_shapes`.
