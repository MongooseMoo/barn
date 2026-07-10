# Iteration 011 - Connection Input Call Shapes

## Baseline

- Date: 2026-07-09
- Barn result after iteration 010: 90 failed, 11245 passed, 126 skipped
- Target: `connection_input_call_shapes`
- Target failures: 7

## Target

Fix `connection_input_call_shapes` next, because it is the first largest remaining failure family.

## Changes

- Matched Toast's generated `flush_input` signature by accepting one or two arguments.
- Kept the optional `show_messages` argument as `TYPE_ANY`; current behavior ignores it and returns the same success value for the no-connection shape covered here.

## Verification

- Targeted `connection_input_call_shapes` before: 7 failed, 7 passed, 11447 deselected.
- Targeted `connection_input_call_shapes` after: 14 passed, 11447 deselected.
- Full managed Barn conformance after: 83 failed, 11252 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 7 full-suite failures from `connection_input_call_shapes`.
- Remaining largest clusters are 7-failure groups: `add_property_call_shapes`, `url_curl`, `disassemble_call_shapes`, and `optional_extensions`.
