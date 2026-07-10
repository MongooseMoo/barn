# Iteration 015 - Disassemble Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 014: 62 failed, 11273 passed, 126 skipped
- Target: `disassemble_call_shapes`
- Target failures: 7

## Target

Fix `disassemble_call_shapes` next, because it is the largest remaining failure family.

## Changes

- Matched `disassemble()` validation order for invalid objects:
  descriptor type is checked before object existence.
- Normalized valid descriptor requests on invalid objects from store `E_INVIND` to Toast-compatible `E_INVARG`.

## Verification

- Targeted `disassemble_call_shapes` before: 7 failed, 42 passed, 11412 deselected.
- Targeted `disassemble_call_shapes` after: 49 passed, 11412 deselected.
- Full managed Barn conformance after: 55 failed, 11280 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 7 full-suite failures from `disassemble_call_shapes`.
- Remaining largest clusters are 6-failure groups: `server_version_call_shapes`, `callers_call_shapes`, and `listappend_call_shapes`.
