# Iteration 013 - URL and Curl

## Baseline

- Date: 2026-07-09
- Barn result after iteration 012: 76 failed, 11259 passed, 126 skipped
- Target: `url_curl`
- Target failures: 7

## Target

Fix `url_curl` next, because it is the first largest remaining failure family.

## Changes

- Replaced `url_decode()`'s query-string decoder with Toast-compatible percent decoding:
  literal `+`, literal malformed `%` sequences, binary byte preservation, and NUL truncation.
- Matched the adopted `curl(url[, include_headers[, timeout]])` shape:
  the second argument is any truthy header flag, the third argument must be an integer timeout, and unsupported/request-failure cases return a nonempty error map.

## Verification

- Targeted `url_curl` before: 7 failed, 15 passed, 11439 deselected.
- Targeted `url_curl` after: 22 passed, 11439 deselected.
- Full managed Barn conformance after: 69 failed, 11266 passed, 126 skipped.
- `go test ./builtins` passed.

## Result

- Fixed 7 full-suite failures from `url_curl`.
- Remaining largest clusters are 7-failure groups: `optional_extensions` and `disassemble_call_shapes`.
