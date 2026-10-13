# Issue 14 and 16 managed Toast failure investigation

## Current findings

- The full managed run reports eight dynamic listener capability admission failures and three `PROMOTE_NUMBERS` option admission failures.
- A fresh documented managed Toast selection ran three representative dynamic-listener cases successfully and failed only the representative `PROMOTE_NUMBERS` case.
- Fresh managed result: 3 passed, 1 failed, 11,562 deselected.
- `PROMOTE_NUMBERS` failure is deterministic on a clean server: both `server_version("options.PROMOTE_NUMBERS")` and `server_version("options/PROMOTE_NUMBERS")` are unavailable, producing the fail-closed `CapabilityProbeError`.
- The listener probes work on a clean server, so their full-order failures demonstrate state/order contamination rather than intrinsic Toast incompatibility.
- The connectable-listener probe chooses `listeners()[1]`, so it depends on mutable listener ordering and the semantics of whichever listener earlier suites leave first.
- The ephemeral-listener probe uses `listen(player, 0, ...)`, so it depends on mutable live player/listener state.
- Replaying all 114 audit-prefix cases before the representative connectable probe did not reproduce contamination: 92 passed, 23 declared fork skips, and the subsequent probe remained healthy.
- Replaying only the current `listen_call_shapes.yaml` before `open_network_connection_call_shapes.yaml` reproduced the exact `connectable_listener_port` `E_INVARG`: 12 passed, one expected option skip, then the admission probe failed.
- Therefore `listen_call_shapes.yaml` is a sufficient contaminator. Broader audit state and `network_matrix.yaml` are not required for the failure.

## Current theories

1. `listen_call_shapes` changes listener state or ordering; the connectable probe later selects `listeners()[1]`, which is no longer a safe assumption and receives `E_INVARG`.
2. The same preceding listener state may make the player-based ephemeral probe invalid, while a clean server succeeds.
3. `PROMOTE_NUMBERS` is enabled behaviorally in pinned Toast but is not exported through the probed `server_version` option namespace.

## Latest managed evidence

- The exact sufficient predecessor is `listen_two_args_accepts_integer_description`: it executes `listen(#0, 0)` and its declared cleanup executes `unlisten(0)`.
- Immediately after that case, `open_network_connection_enabled_three_args_accepts_options_map` fails with Toast `E_INVARG`.
- The same predecessor causes the selected `ephemeral_listen` admissions to fail with `E_INVARG`.
- The current in-progress capability snapshot makes admission deterministic, but it does not restore Toast runtime state. Re-running the exact two-case order against the latest tree produced one pass followed by one body failure: `open_network_connection_enabled_three_args_accepts_options_map` returned `E_INVARG`.
- Therefore snapshotting capabilities alone is insufficient. The actual conformance cases need a pristine managed-server isolation boundary after listener-mutating coverage.

## Current state and resolved mechanism

- Current source adds a session-scoped snapshot of `connectable_listener_port`, `ephemeral_listen`, and behavioral `PROMOTE_NUMBERS`, and prepares a suite's selected DB before admission.
- `prepare_suite_environment()` only restarts when the requested source DB path changes. Both reproducing builtins suites use the default DB, so no restart occurs between them.
- Canonical WSL Toast source shows that `make_listener()` binds descriptor `0` but records the reported port as the descriptor value, `bf_listen()` returns the descriptor, `listeners()` exposes the descriptor as `"port"`, and new listeners prepend to the list. Toast therefore never exposes the kernel-assigned ephemeral port for descriptor `0`.
- If a descriptor-0 listener remains registered, consumers of `listeners()[1]["port"]` attempt outbound port 0 and receive `E_INVARG`; a later `listen(..., 0, ...)` receives `E_INVARG` as a duplicate descriptor. This identifies the roundtrip's failing first-step subexpression as `listen(#0, 0, ...)`, not its two `listeners()` count expressions.
- The latest current source allocates a real free nonzero port for the two-argument listen call-shape case. A managed ordered selection containing that predecessor, all existing listener shape/filter cases, the roundtrip, and the duplicate-listener case passed 10/10.
- This disproves irreversible global poisoning and removes the need for broad per-suite server restarts. The defect is descriptor-0 leakage/order contamination.
- No source or test edits were made by this investigation.

## Next action

- Keep the nonzero allocated descriptor fix and require a managed ordered regression with that call-shape case before open-network, connection-name, and listener-roundtrip families. Run the full managed Toast suite; do not add broad suite restarts unless a distinct current failure proves they are necessary.
