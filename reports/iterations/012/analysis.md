# Iteration 012 - Add Property Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 011: 83 failed, 11252 passed, 126 skipped
- Target: `add_property_call_shapes`
- Target failures: 7

## Target

Fix `add_property_call_shapes` next, because it is the first largest remaining failure family.

## Changes

- Normalized `add_property()` invalid typed object references from store `E_INVIND` to Toast-compatible `E_INVARG`.
- Kept non-object first arguments returning `E_TYPE` and left property/value/info validation order otherwise unchanged.

## Verification

- Targeted `add_property_call_shapes` before: 7 failed, 42 passed, 11412 deselected.
- Targeted `add_property_call_shapes` after: 49 passed, 11412 deselected.
- Full managed Barn conformance after: 76 failed, 11259 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 7 full-suite failures from `add_property_call_shapes`.
- Remaining largest clusters are 7-failure groups: `url_curl`, `optional_extensions`, and `disassemble_call_shapes`.
