# Conformance Zero Plan

## Conformance Guardrails

1. Use only the documented managed conformance command unless the user explicitly approves a different path.
2. Do not manually launch Barn or Toast for conformance work unless the user explicitly asks for a manual repro.
3. Do not manually run against tracked database files. If a manual repro is explicitly required, use a disposable copy only.
4. Verify uncertain, surprising, or order-dependent behavior against Toast before Barn-side debugging.
5. If the managed harness command fails, report the exact command, working directory, expected result, and actual failure before improvising.

## Goal

Reduce Barn's updated `moo-conformance-tests` gap from `44` failures to `0`.

Baseline on `2026-04-02`:

- `2742 passed`
- `139 skipped`
- `44 failed`

Primary baseline log:

- `C:\Users\Q\code\moo-conformance-tests\reports\barn_updated_suite_20260402.log`

## Current Failure Clusters

1. Network/listener/admin behavior is incomplete.
2. `player_huh` command routing is missing.
3. `read_http()` is still a stub.
4. SQLite builtins are placeholders, not real behavior.
5. Argon2 builtins do not match the expected contract.
6. Dump/restart persistence is losing runtime-added inherited property state.
7. Startup-repair / canned-DB compatibility is mostly unimplemented.
8. Two failures appear order-dependent and should be re-evaluated after the above:
   - `disassemble_with_index_operators`
   - `do_command_return_true_stops_parsing`

## Milestones

### Milestone 1: Real Listener / Connection Admin Semantics

Target failures:

- `server_admin::connection_name_lookup_records_result`
- `server_admin::listeners_listen_unlisten_roundtrip`
- `server_admin::buffered_output_length_increases_after_notify`
- `server_admin::boot_player_disconnects_open_connection`

Deliverables:

- `listen()` supports dynamic listeners including ephemeral port allocation.
- `unlisten()` removes dynamic listeners and closes their sockets.
- `listeners()` reports both the primary listener and runtime listeners.
- `open_network_connection()` creates a tracked outbound connection object instead of returning `0`.
- `boot_player()` disconnects tracked outbound/open connections.
- `connection_name_lookup()` returns and optionally stores the rewritten name.
- Buffered output accounting works on those tracked connections.

Commit slices:

1. Introduce listener/outbound-connection bookkeeping in the connection manager.
2. Wire builtins to real listener/outbound state and fix return shapes.
3. Add or update focused Go tests for listeners, outbound connections, and boot/lookup behavior.
4. Re-run targeted conformance tests for `server_admin`.

### Milestone 2: Command Dispatch Option Semantics

Target failures:

- `command_parsing::huh_calls_player_verb_with_player_huh_enabled`
- `command_parsing::location_huh_disabled_with_player_huh_enabled`

Deliverables:

- Scheduler honors `$server_options.player_huh`.
- Unknown-command routing prefers `player:huh` when enabled and suppresses location fallback.

Commit slices:

1. Add server-option lookup in the unknown-command path.
2. Add focused tests for `player_huh` enabled and disabled behavior.
3. Re-run targeted `command_parsing` conformance tests.

### Milestone 3: Argon2 Contract Parity

Target failures:

- `argon2::argon2_non_wizard_denied`
- `argon2::argon2_iterations_type_error`
- `argon2::argon2_known_vector`
- `argon2::argon2_verify_non_wizard_denied`

Deliverables:

- Wizard-only enforcement matches expected behavior.
- Supported arity and argument validation match the tests.
- Known-vector output matches the suite.
- Verify path matches the same contract.

Commit slices:

1. Expand builtin signature and validation logic.
2. Add permission gating and known-vector compatibility.
3. Add focused builtin tests.
4. Re-run targeted Argon2 conformance tests.

### Milestone 4: HTTP Parser Implementation

Target failures:

- All `http::*` failures in the baseline run.

Deliverables:

- `read_http("request")` and `read_http("response")` parse forced input correctly.
- Invalid inputs return the expected structured error payloads.
- Header parsing, folded headers, `Content-Length`, chunked bodies, and chopped-up input work.

Commit slices:

1. Implement a parser independent of task suspension using held/forced input first.
2. Add request-line and response-line validation/error mapping.
3. Add header parsing, folding, and body handling.
4. Add focused parser unit tests.
5. Re-run targeted HTTP conformance tests.

### Milestone 5: Real SQLite Compatibility Layer

Target failures:

- All `sqlite::*` failures in the baseline run.

Deliverables:

- Real handle lifecycle.
- `sqlite_info()` and `sqlite_handles()` match expected maps/lists.
- `sqlite_query()` and `sqlite_execute()` return Toast-compatible row shapes.
- `sqlite_last_insert_row_id()` works.
- `sqlite_limit()` supports string and numeric categories with prior-value semantics.
- `sqlite_interrupt()` aborts long-running work.

Commit slices:

1. Replace the in-memory shim with real SQLite-backed handles.
2. Implement query/execute result-shape compatibility.
3. Implement limits and interrupt semantics.
4. Add focused tests.
5. Re-run targeted SQLite conformance tests.

### Milestone 6: Dump / Restart Property Persistence

Target failures:

- `dump_persistence::inherited_override_survives_dump_and_restart`

Deliverables:

- Runtime-added propdefs persist across `dump_database()` and restart.
- Child overrides on inherited properties survive reload.

Commit slices:

1. Trace and fix propdef serialization for runtime-added properties.
2. Add a focused persistence regression test in Go.
3. Re-run targeted dump-persistence conformance tests.

### Milestone 7: Startup Repair and Canned DB Compatibility

Target failures:

- `startup_repair_anon1`
- `startup_repair_anon2`
- `startup_repair_anon6`
- `startup_repair_broken1`
- `startup_repair_broken2`
- `startup_repair_broken3`
- `startup_repair_broken4`
- `startup_repair_broken5`

Deliverables:

- Barn can load the canned DB fixtures used by the conformance suite.
- Pending-finalization state is preserved or repaired as expected.
- Invalid references/types/cycles/inconsistencies are repaired on startup.
- Repair actions are logged with the expected messages.
- Startup-repair dumps produce the expected `.new` files when required.

Commit slices:

1. Add DB-format compatibility needed for the canned fixtures Barn currently rejects.
2. Implement pending-finalization reading/writing.
3. Implement startup repair passes for invalid refs/types/cycles/inconsistencies.
4. Emit repair logs and output DBs in the expected places.
5. Add focused canned-DB tests.
6. Re-run targeted startup-repair conformance tests.

### Milestone 8: Full-Suite Cleanup

Target failures:

- Any residual failures after milestones 1-7.

Deliverables:

- Re-run full suite.
- Re-check previously order-dependent failures in full-suite context.
- Fix any remaining stragglers until the suite is green.

Commit slices:

1. Run full suite and classify residual failures.
2. Fix residual regressions one cluster at a time.
3. Re-run full suite until failure count is `0`.

## Execution Rules

1. After every passing substantial targeted conformance run, reread this plan and continue with the next unchecked milestone.
2. After every passing full-suite run, reread this plan before reporting status.
3. Do not declare completion until the updated conformance suite reports `0 failed`.

## First Active Step

Start with Milestone 1. The outbound/listener/admin path is currently underimplemented and also likely contaminates later suite behavior.
