# Iteration 028 - Network Matrix

## Baseline

- Date: 2026-07-09
- Barn result after iteration 027: 4 failed, 11331 passed, 126 skipped
- Target: `network_matrix`
- Target failures: 2

## Target

Fix `network_matrix` next, because it was the largest remaining failure family.

## Changes

- Carried the `ipv6` listener option through listener specs, descriptors, filtering, and removal keys.
- Honored `unlisten(desc, ipv6)` by applying the second argument to integer descriptors.
- Bound runtime listeners with explicit `tcp4`/`tcp6` networks based on the listener IPv6 option.
- Recorded outbound endpoint metadata when `open_network_connection()` pairs the accepted loopback connection.
- Reported outbound source/destination fields and `outbound == 1` from `connection_info()` for those connections.
- Narrowed `buffered_output_length()`'s prompt-floor compatibility behavior to implicit current-player calls.

## Verification

- Targeted `network_matrix` after: 3 passed, 2 skipped, 11456 deselected.
- Full managed Barn conformance after: 2 failed, 11333 passed, 126 skipped.
- `go test ./builtins ./server` passed.

## Result

- Fixed 2 full-suite failures from `network_matrix`.
- Remaining failures are `clear_property_call_shapes::clear_property_arg_obj` and `respond_to_call_shapes::respond_to_invalid_object`.
