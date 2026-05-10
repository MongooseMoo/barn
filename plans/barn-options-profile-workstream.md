# Barn Options And Profile Workstream

Goal: make Barn a profile-aware Toast conformance target by adding typed
server options, explicit `.conf` startup configuration, runtime feature
metadata, named profile manifests, and harness gates that reject invalid
Toast/Barn comparisons.

This workstream is the prerequisite for continuing
`barn-toast-mongoose-convergence-workstreams.md` past the first execution slice.
The immediate blocker there is that Barn cannot represent
`option.OUTBOUND_NETWORK` as a supported profile feature.

## Control Rules

- Do not run Barn-vs-Toast conformance comparisons for option-sensitive tests
  until both targets emit matching feature metadata.
- A Barn profile is supported only when it has an explicit config file, fixture
  identity, feature manifest, and implementation ref.
- Defaults may exist for normal Barn operation, but managed conformance profiles
  must not rely on implicit defaults.
- Disabled and absent are different states. A disabled option is a real feature
  value; a missing builtin is a different feature.
- Barn-only extension behavior must live under `barn_extension.*` feature keys
  and must not count as Toast conformance.
- Config parsing must fail closed: unknown option names, malformed values, and
  invalid profile manifests abort the run.
- Do not add harness machinery without at least one concrete profile/test that
  needs it.
- Do not edit conformance tests to match Barn. First expose Barn metadata, then
  run existing or Toast-verified tests against matching profiles.
- Generated manifests and run logs are diagnostics unless explicitly promoted.

## Dependency Order

The executable order is:

1. WS0 Current Surface Inventory
2. WS1 Typed Barn Options Model
3. WS2 `.conf` Parser And CLI Integration
4. WS3 Runtime Option Plumbing
5. WS4 `OUTBOUND_NETWORK` Behavior
6. WS5 Feature Manifest Emission
7. WS6 Barn Profile Registry
8. WS7 Managed Run Integration
9. WS8 Harness Metadata Gate
10. WS9 Toast/Barn Profile Smoke Runs
11. WS10 Promotion Into Convergence Plan
12. WS11 Coverage Expansion For Option-Sensitive Tests

WS4 depends on WS1-WS3. WS5 depends on WS1-WS4. WS6 depends on WS5. WS8 depends
on WS5-WS7 and on the conformance harness loading target metadata. WS9 starts
only after WS8 can reject a deliberately mismatched profile pair.

## Target Architecture

Barn gets one typed options package and one profile surface:

- `config.Options`: typed runtime options with explicit defaults.
- `.conf` files: simple `KEY = VALUE` syntax for server startup.
- `--config`: Barn CLI flag naming the config file for managed runs.
- `--profile-id`: Barn CLI flag naming the run profile.
- `--profile-manifest`: Barn CLI flag naming where to write JSON metadata.
- Runtime option accessors used by builtins and server subsystems.
- Feature manifest JSON with stable keys such as
  `option.OUTBOUND_NETWORK`.
- Named profile files that bind profile ID, config path, fixture ID, platform,
  and support status.

Initial option:

```conf
OUTBOUND_NETWORK = 1
```

```conf
OUTBOUND_NETWORK = 0
```

Initial supported profile IDs:

- `barn-linux-testdb-outbound-on`
- `barn-linux-testdb-outbound-off`
- `barn-linux-mongoose-outbound-on`
- `barn-linux-mongoose-outbound-off`

Initial diagnostic profile IDs:

- `barn-windows-testdb-outbound-on`
- `barn-windows-testdb-outbound-off`
- `barn-windows-mongoose-outbound-on`
- `barn-windows-mongoose-outbound-off`

Windows profiles remain diagnostic until paired with a matching Windows Toast
oracle or a test declares itself platform-insensitive.

## File Ownership Plan

Expected Barn-owned source paths:

- `config/` or `server/config/`: typed options model and parser.
- `cmd/barn/main.go`: CLI flags, config loading, manifest output path.
- `server/server.go`: carry options into server construction.
- `server/scheduler.go` and VM creation sites if task contexts need options.
- `types/context.go`: only if builtin access should flow through task context.
- `builtins/registry.go` and network builtin files: option-aware builtin
  behavior.
- `profiles/barn/*.conf`: committed profile config templates.
- `profiles/barn/*.json` or `profiles/barn/*.yaml`: committed profile
  registry entries, if Barn owns registry files.
- Focused unit tests beside the new config/profile code.

Expected conformance-owned paths, if harness work is required:

- `../moo-conformance-tests/src/moo_conformance/...`
- Profile registry or target metadata loader files in that repo.
- Focused harness unit tests for feature requirements and mismatch rejection.

Barn commits and conformance commits must remain separate.

## WS0 Current Surface Inventory

Purpose: confirm the current code surfaces before source edits.

Checks:

- Read `cmd/barn/main.go` and list all startup flags.
- Read `server.NewServer` and server construction call sites.
- Read builtin registration and `open_network_connection()` implementation.
- Read `types.TaskContext` and VM/evaluator construction paths.
- Confirm whether any existing config package exists.
- Confirm whether `moo-conformance-tests` already has target profile metadata
  support.

Done when:

- The exact files for WS1-WS4 are named.
- There is no existing Barn config surface that the work would accidentally
  bypass.
- If an existing harness primitive is present, the plan says to use it instead
  of adding a duplicate.

## WS1 Typed Barn Options Model

Purpose: create one typed runtime option model for Barn.

Required design:

- `Options` struct with typed fields.
- `DefaultOptions()` for normal operation.
- `Validate()` that rejects impossible combinations.
- `FeatureMap()` that returns stable conformance feature keys.
- No map-driven runtime lookups for known options in production code.

Initial shape:

```go
type Options struct {
    OutboundNetwork bool
}
```

Initial feature output:

```json
{
  "option.OUTBOUND_NETWORK": true,
  "builtin.open_network_connection": "present"
}
```

Rules:

- `builtin.open_network_connection` remains `present` for both outbound-on and
  outbound-off profiles if Barn implements the builtin in both cases.
- `option.OUTBOUND_NETWORK = false` means the builtin raises `E_PERM`, not that
  the builtin disappears.
- Defaults are allowed in `DefaultOptions()`, but managed profiles must pass
  explicit config files and include their checksums in manifests.

Tests:

- Defaults produce the intended normal-operation state.
- `FeatureMap()` reports true/false correctly.
- `Validate()` rejects unknown future invalid states once they exist.

Done when:

- Options have a typed API.
- No production code needs to parse string option names to inspect
  `OUTBOUND_NETWORK`.

## WS2 `.conf` Parser And CLI Integration

Purpose: make managed Barn runs explicit.

Required `.conf` grammar:

- Blank lines allowed.
- `#` comments allowed.
- `KEY = VALUE` entries.
- Keys are case-sensitive and must be known.
- Boolean values accept only `0` or `1` for Toast-style option parity.
- Duplicate keys are errors.
- Unknown keys are errors.
- Malformed lines are errors with file path and line number.

Required CLI behavior:

- Add `--config <path>` to `cmd/barn`.
- Add `--profile-id <id>` for managed/profile runs.
- Add `--profile-manifest <path>` to write JSON metadata before server start.
- For normal operation without `--config`, use `DefaultOptions()`.
- For managed conformance profiles, the harness must always pass `--config`.

Config examples:

```conf
# Barn Toast-compatible outbound-enabled profile.
OUTBOUND_NETWORK = 1
```

```conf
# Barn Toast-compatible outbound-disabled profile.
OUTBOUND_NETWORK = 0
```

Tests:

- Parses `OUTBOUND_NETWORK = 1`.
- Parses `OUTBOUND_NETWORK = 0`.
- Rejects `OUTBOUND_NETWORK = true`.
- Rejects `OUTBOUND_NETWORK = yes`.
- Rejects duplicate `OUTBOUND_NETWORK`.
- Rejects unknown `FOO`.
- Reports malformed line numbers.

Done when:

- Barn can start with `--config profiles/barn/outbound-on.conf`.
- Barn can start with `--config profiles/barn/outbound-off.conf`.
- Bad config exits before database load or network listen.

## WS3 Runtime Option Plumbing

Purpose: make options available to all code that needs them without global
string parsing.

Implementation choices to evaluate in WS0:

- Server owns `Options`, scheduler/evaluator receive immutable options.
- Builtins receive options through registry construction.
- Builtins receive options through `TaskContext`.

Preferred shape:

- `server.NewServer(..., options config.Options)`.
- Scheduler and evaluator carry immutable options.
- Builtin registry is constructed with immutable options or an option provider.
- Test-only evaluators use `DefaultOptions()` unless tests pass explicit
  options.

Rules:

- Avoid mutable global option state for profile-sensitive behavior.
- Avoid hidden environment-variable overrides.
- Do not thread raw config file paths through VM/builtin code.

Tests:

- Server construction preserves options.
- Evaluator/registry created by the server sees those options.
- Existing tests that construct evaluators directly keep current defaults.

Done when:

- Production builtin behavior can read `Options.OutboundNetwork`.
- There is no package-level `var outboundNetwork` controlling conformance
  behavior.

## WS4 `OUTBOUND_NETWORK` Behavior

Purpose: match Toast's enabled/disabled outbound network profile behavior.

Target behavior:

- If `OUTBOUND_NETWORK = 1`, wizard callers can reach existing
  `open_network_connection()` behavior.
- If `OUTBOUND_NETWORK = 0`, every call to `open_network_connection()` raises
  `E_PERM`.
- Non-wizard callers still raise `E_PERM` in both profiles.
- Argument validation must not leak through before the disabled-profile
  permission result if Toast raises `E_PERM` first for the disabled option.

Toast verification required before finalizing order-sensitive behavior:

- Run a focused Toast outbound-off conformance test or managed probe that calls
  `open_network_connection()` as a wizard with valid arguments.
- Run a focused Toast outbound-off probe for invalid argument shapes only if the
  error ordering is uncertain.
- Encode the confirmed behavior in `moo-conformance-tests` before changing
  Barn if no suitable test already exists.

Tests:

- Barn unit test: disabled option returns `E_PERM`.
- Barn unit test: enabled option preserves current argument validation and
  connection-manager behavior.
- Managed Barn conformance run: disabled profile matches Toast-verified test.

Done when:

- `open_network_connection()` is profile-sensitive.
- The builtin remains present in function metadata for both profiles, unless
  Toast verification proves otherwise for the selected profile.

## WS5 Feature Manifest Emission

Purpose: make Barn's target metadata machine-readable.

Manifest fields:

- `profile_id`
- `implementation`: `barn`
- `implementation_ref`: git SHA plus dirty tracked status
- `binary_path`
- `build_system`
- `runtime_os`
- `arch_bits`
- `database_fixture`
- `database_checksum`
- `config_file`
- `config_checksum`
- `features`
- `support_status`
- `unsupported_reason`

Initial feature keys:

- `option.OUTBOUND_NETWORK`
- `builtin.open_network_connection`
- `runtime.arch_bits`
- `platform.path_separator`
- `platform.backslash_is_path_separator`

Rules:

- Emit manifest before executing tests or accepting player connections in
  managed mode.
- Unknown feature state must be absent only when the manifest marks the profile
  unsupported or diagnostic-only with a reason.
- Dirty tracked status must be explicit; untracked diagnostics do not make the
  implementation ref dirty.

Tests:

- Manifest includes config checksum.
- Manifest changes when config changes.
- Manifest reports `option.OUTBOUND_NETWORK` accurately.
- Manifest rejects missing `--profile-id` when `--profile-manifest` is used.

Done when:

- A Barn managed run writes a complete JSON manifest.
- The manifest is enough for the harness to compare feature values with Toast.

## WS6 Barn Profile Registry

Purpose: define known profile bundles.

Profile registry entry fields:

- `profile_id`
- `implementation`
- `runtime_os`
- `database_fixture`
- `config_file`
- `support_status`
- `unsupported_reason`
- `expected_features`
- `command_template`

Required initial entries:

- `barn-linux-testdb-outbound-on`
- `barn-linux-testdb-outbound-off`
- `barn-linux-mongoose-outbound-on`
- `barn-linux-mongoose-outbound-off`

Diagnostic entries:

- `barn-windows-testdb-outbound-on`
- `barn-windows-testdb-outbound-off`
- `barn-windows-mongoose-outbound-on`
- `barn-windows-mongoose-outbound-off`

Rules:

- Registry expected features must be checked against runtime manifest output.
- A registry mismatch aborts the run.
- Unsupported profiles remain visible and do not count as pass/fail
  conformance.

Tests:

- Registry loads all required profile IDs.
- Runtime manifest feature mismatch is rejected.
- Missing config path is rejected.
- Unsupported profile is reported separately from failed tests.

Done when:

- One command can list Barn profiles and support status.
- One command can start a selected Barn profile with the expected config.

## WS7 Managed Run Integration

Purpose: make profile-aware Barn startup usable by the conformance harness.

Required managed command shape:

```powershell
.\barn.exe `
  --db <disposable-db-copy> `
  --listen tcp://127.0.0.1:<port> `
  --checkpoint-interval 0 `
  --config profiles/barn/outbound-on.conf `
  --profile-id barn-windows-testdb-outbound-on `
  --profile-manifest <run-dir>\barn-profile.json
```

Linux/WSL command shape:

```bash
./barn \
  --db <disposable-db-copy> \
  --listen tcp://127.0.0.1:<port> \
  --checkpoint-interval 0 \
  --config profiles/barn/outbound-on.conf \
  --profile-id barn-linux-testdb-outbound-on \
  --profile-manifest <run-dir>/barn-profile.json
```

Rules:

- Every managed run uses a disposable DB copy.
- Manifest paths are run artifacts, not committed output.
- Server startup aborts if manifest write fails.

Tests:

- Managed startup writes manifest before readiness.
- Failed config prevents listener startup.
- Disposable DB path is reflected in manifest checksum.

Done when:

- The harness can start Barn with a named profile and read the profile manifest.

## WS8 Harness Metadata Gate

Purpose: reject invalid Toast/Barn comparisons before tests execute.

Required gate behavior:

- Load Toast manifest.
- Load Barn manifest.
- Compare required feature keys for the selected profile pair.
- Abort or mark `invalid-comparison` when required features differ.
- Report `unsupported-profile` when Barn profile support status is unsupported.
- Distinguish skipped tests from unsupported profiles and invalid comparisons.

Initial required match keys:

- `option.OUTBOUND_NETWORK`
- `database_fixture`
- `runtime_os`, unless test declares platform-insensitive

Rules:

- Unknown feature metadata fails closed.
- A disabled feature is not a skip.
- A missing builtin can skip only tests that explicitly require that builtin to
  be present.

Tests:

- Toast outbound-on versus Barn outbound-off is rejected before test execution.
- Toast outbound-off versus Barn outbound-off is accepted.
- Unsupported Barn profile is reported as unsupported, not passed.
- Missing feature key aborts option-sensitive tests.

Done when:

- A deliberately mismatched `OUTBOUND_NETWORK` pair cannot run tests.
- A matched pair reaches test execution.

## WS9 Toast/Barn Profile Smoke Runs

Purpose: prove the new profile machinery on small fixtures before returning to
Mongoose.

Required smoke sequence:

1. Build Toast outbound-on and outbound-off profiles.
2. Build Barn.
3. Prepare disposable `Test.db` copies.
4. Start Toast outbound-on and capture manifest.
5. Start Barn outbound-on and capture manifest.
6. Run a minimal health probe against each.
7. Repeat for outbound-off.
8. Run mismatch gate: Toast on versus Barn off must abort before tests.
9. Run matched gate: Toast off versus Barn off must execute the selected test.

Health probe:

- Connect to the server socket.
- Capture banner or first prompt bytes.
- Stop the server.
- Verify no live child process remains.

Done when:

- Both matched profile pairs can be started and stopped.
- The mismatch gate is proven.
- The run record names the Toast and Barn manifests used.

## WS10 Promotion Into Convergence Plan

Purpose: unblock the main Barn/Toast/Mongoose convergence workflow.

Required updates:

- Update the first execution slice status with the new Barn profile support.
- Re-run the source DB identity check.
- Re-run Toast profile build verification.
- Run the first matching Toast/Barn profile pair against disposable DB copies.
- Capture banners with a minimal raw socket client.

Rules:

- Do not skip back to WS7 Barn Fix Loop until profile metadata gates are in
  place.
- Do not treat local smoke success as conformance unless it used the managed
  harness and matching metadata.

Done when:

- `barn-toast-mongoose-convergence-workstreams.md` can proceed past item 8 of
  its first execution slice.

## WS11 Coverage Expansion For Option-Sensitive Tests

Purpose: use the new profile machinery to add real conformance coverage.

Initial candidates:

- `open_network_connection()` enabled profile behavior.
- `open_network_connection()` disabled profile behavior.
- `function_info("open_network_connection")` in both profiles.
- `server_options()` / `server_version()` option exposure if Barn implements
  equivalent MOO-visible metadata.
- Builtin presence versus disabled option semantics.

Rules:

- Every test must pass Toast first at the named profile.
- Barn must be run at the matching profile.
- Do not edit Barn from a probe transcript alone.

Done when:

- At least one Toast-verified option-sensitive conformance test runs against
  both outbound-on and outbound-off profiles.

## Completion Criteria

This workstream is complete when:

- Barn has typed runtime options.
- Barn starts with explicit `.conf` files.
- `OUTBOUND_NETWORK` controls `open_network_connection()` behavior.
- Barn emits complete profile manifests.
- Required Barn profiles are listed with accurate support status.
- The harness rejects mismatched Toast/Barn feature metadata.
- A matched outbound-on and matched outbound-off profile pair can run managed
  tests.
- The main convergence plan can resume from the profile-pair smoke run without
  violating its oracle/profile gate.

## Non-Goals

- Do not implement every Toast option in the first slice.
- Do not add Barn-only extensions to Toast conformance totals.
- Do not add Mongoose login bridge behavior in this workstream.
- Do not create local dependency pins.
- Do not commit generated run manifests unless explicitly requested.

