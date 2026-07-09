# Iteration 023 - read_stdin

## Baseline

- Date: 2026-07-09
- Barn result after iteration 022: 18 failed, 11317 passed, 126 skipped
- Target: `read_stdin`
- Target failures: 3

## Target

Fix `read_stdin` next, because it was tied for the largest remaining failure family.

## Changes

- Added a zero-argument `read_stdin` signature override for `function_info()`.
- Implemented process-stdin suspension and resume through the registry host.
- Matched the Toast example extension's line transformation: newline becomes `X`, and `a*` input returns `E_NACC`.
- Corrected the conformance `read_stdin` catch syntax in `moo-conformance-tests` from `except (err)` to `except err (ANY)`; the WSL Toast oracle skips this optional extension, and Toast source grammar does not define catch-variable shorthand in the code-list position.

## Verification

- Targeted `read_stdin` before: 3 failed, 2 passed, 11456 deselected.
- Targeted `read_stdin` after: 5 passed, 11456 deselected.
- Full managed Barn conformance after: 15 failed, 11320 passed, 126 skipped.
- `go test ./builtins ./server` passed.

## Result

- Fixed 3 full-suite failures from `read_stdin`.
- Remaining largest clusters are `exec` and `file_handle_call_shapes` with 3 failures each.
