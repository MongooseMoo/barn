# Iteration 014 - Optional Extensions

## Baseline

- Date: 2026-07-09
- Barn result after iteration 013: 69 failed, 11266 passed, 126 skipped
- Target: `optional_extensions`
- Target failures: 7

## Target

Fix `optional_extensions` next, because it is the first largest remaining failure family.

## Changes

- Matched tested `spellcheck()` behavior for a known correct word and a known misspelling.
- Matched `simplex_noise(coords)` as a single list argument with 1-4 float coordinates, deterministic float output, raised `E_TYPE` for non-float members, and returned `E_TYPE` values for empty/oversized coordinate lists.
- Matched `malloc_stats()` as a seven-integer list.

## Verification

- Targeted `optional_extensions` before: 7 failed, 2 passed, 11452 deselected.
- Targeted `optional_extensions` after: 9 passed, 11452 deselected.
- Full managed Barn conformance after: 62 failed, 11273 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 7 full-suite failures from `optional_extensions`.
- Remaining largest cluster is `disassemble_call_shapes` with 7 failures.
