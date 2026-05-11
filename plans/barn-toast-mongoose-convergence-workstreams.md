# Barn/Toast/Mongoose Convergence Workstreams

Goal: make Barn converge toward ToastStunt behavior by discovering real
Barn-vs-Toast differences, capturing each confirmed Toast behavior as a
conformance test, and fixing Barn against those tests.

This plan treats ToastStunt as the oracle and `mongoose.db.new` as the
real-world stress database. Discovery happens through a dual interactive bridge;
durable convergence happens through `../moo-conformance-tests`.

## Control Rules

- Toast behavior must be verified before any Barn-side debugging for a disputed
  behavior.
- Toast is an oracle only at a named profile point: implementation build,
  runtime OS, database fixture, and feature/config values. A result from one
  profile is not universal MOO behavior.
- Conformance means Barn matches Toast at the same profile point. If the profile
  metadata differs, the comparison is invalid and the harness must abort or mark
  the test inapplicable; it must not silently rewrite expectations.
- Configurable behavior must remain configurable. Do not collapse a Toast build
  option, optional library, platform behavior, or Barn extension into a global
  Barn semantic.
- Barn may be a Toast-compatible superset, but every Barn-only behavior must be
  behind an explicit Barn extension profile. Barn extensions are not Toast
  conformance tests.
- If Barn does not support a Toast profile, mark that profile `unsupported` with
  a reason. Unsupported profiles are visible coverage debt, not passing
  conformance and not Barn semantic failures.
- The Toast option set must be extracted from the actual upstream build inputs
  and generated build output before any option-sensitive test is trusted.
- Verification means an actual `moo-conformance-tests` test run against Toast,
  not an interactive probe transcript, shell experiment, manual eval, source
  inspection, or plausible inference.
- For every Barn/Toast divergence, the mandatory order is:
  1. distill the smallest conformance test for the intended behavior;
  2. run that test on Toast and record the exact passing command/result;
  3. run the same test on Barn and observe the pre-fix failure;
  4. only then inspect or change Barn production code;
  5. rerun the test on Barn and keep the fix only if it passes.
- Interactive Mongoose/Barn/Toast probes are discovery tools only. They may
  identify candidate differences, but they do not authorize Barn source edits.
- A Barn source change made before the Toast-pass/Barn-fail conformance gate is
  invalid work for this workstream and must be reverted or abandoned before
  continuing.
- The source Mongoose DB is immutable input. Every server run gets its own
  disposable DB copy.
- No manual conformance repro replaces the managed harness. Manual server runs
  are only for explicitly approved interactive discovery.
- Conformance tests are not edited to match Barn. They are added or corrected
  only after Toast confirms the behavior.
- Conformance tests may declare feature requirements or profile-matrix
  expectations. They may not hide a profile mismatch by pretending an option is
  absent, disabled, or enabled without target metadata proving it.
- Discovery artifacts are diagnostics. Do not commit transcripts, screenshots,
  generated logs, or DB copies unless explicitly requested.
- Dependency metadata must not pin local paths. Local checkouts can be used by
  commands, but committed dependencies must resolve from a clean checkout or a
  pushed remote revision.

## Dependency Order

The executable order is:

1. WS0 Repository And Artifact Hygiene
2. WS1 Oracle Runtime Setup
3. WS1A Profile And Feature Inventory
4. WS1B Managed Build/Profile Matrix
5. WS2 Dual MOO Bridge
6. WS3 Probe Library
7. WS4 Diff Classifier
8. WS5 Conformance Test Factory
9. WS6 Harness Extensions
10. WS7 Barn Fix Loop
11. WS8 Coverage Expansion
12. WS9 Regression Dashboard

WS6 can start once WS3 exposes a missing primitive, but it must not invent
harness features speculatively. WS7 starts only after WS5 has a Toast-verified
test or an existing test already proves the behavior.

The critical gate between WS4/WS5 and WS7 is non-negotiable: Barn is never fixed
from a probe transcript alone. The transcript must be distilled into a
conformance test, Toast must pass it at the named profile, and Barn must fail it
first at the matching profile.

The critical gate between WS1A/WS1B and all later conformance work is also
non-negotiable: a server binary/config pair with unknown or unverified feature
metadata is not an oracle.

## WS0 Repository And Artifact Hygiene

Purpose: make later work safe in the current dirty multi-repo environment.

Owned surfaces:

- `C:/Users/Q/code/barn`
- `C:/Users/Q/code/moo-conformance-tests`
- `C:/Users/Q/code/mongoose/bridge` as read-only reference unless explicitly
  promoted
- A local ignored artifact area, preferably `.tmp/mongoose-oracle/`

Deliverables:

- Inventory of dirty tracked files in each repo.
- Written list of task-owned paths before every edit slice.
- Ignored artifact layout for DB downloads, disposable run dirs, transcripts,
  and comparison output.
- A short "current source DB identity" record: file size, timestamp, source
  command, and checksum.

Issues to resolve:

- `barn`, `moo-conformance-tests`, and `mongoose/bridge` are all dirty.
- Barn has many untracked binaries, logs, DBs, and diagnostic files.
- `moo-conformance-tests` has many untracked reports and test files.
- `mongoose/bridge` has uncommitted channel/GMCP changes that may be useful
  but are not cleanly packaged.

Done when:

- The work can name exactly which paths it owns for the next slice.
- No source edit relies on an uncommitted diagnostic artifact.
- The source Mongoose DB and disposable DB copies are clearly separated.

## WS1 Oracle Runtime Setup

Purpose: reproducibly start Barn and Toast against equivalent DB copies.

Target architecture:

- Download `root@mongoose.world:~/mongoose/mongoose.db.new` into an ignored
  source-artifact location.
- For each run, copy that source DB into separate Barn and Toast working dirs.
- Start Barn with native Windows command support.
- Start ToastStunt through WSL when the Windows build is not suitable.
- Capture stdout/stderr, process IDs, ports, DB copy paths, and exit status.
- Stop processes and leave the immutable source DB untouched.

Required command abstractions:

- `fetch-source-db`
- `prepare-run`
- `start-barn`
- `start-toast`
- `stop-run`
- `describe-run`

Issues to prove:

- WSL path translation: `C:\...` to `/mnt/c/...`.
- Toast command shape in the chosen build: input DB, output/checkpoint DB,
  port, and flags.
- Toast readiness signal: log line or socket accept, not a guessed sleep.
- Barn checkpoint behavior: it writes in place and writes a sibling `.new`.
- Toast database-embedded listeners may open extra ports. The harness must
  record this rather than guessing.
- IPv4/IPv6 localhost behavior can differ between Windows and WSL.

Done when:

- A single managed command can start Barn and Toast from fresh DB copies.
- A health probe can connect to both.
- Shutdown leaves no live child processes for the run.
- The run manifest proves which DB copy each server used.

## WS1A Profile And Feature Inventory

Purpose: make the oracle configuration explicit before any behavior is called
correct.

Profile identity fields:

- `profile_id`: stable name such as `toast-linux-testdb-outbound-on`.
- `implementation`: `toast` or `barn`.
- `implementation_ref`: git SHA plus local dirty-tracked status.
- `binary_path`: exact executable used.
- `build_system`: Make/CMake/Go command and environment.
- `runtime_os`: `linux`, `windows`, or `wsl-linux`.
- `arch_bits`: `32` or `64`.
- `database_fixture`: `toast-testdb`, `mongoose-db-new`, or named reduced
  fixture.
- `database_checksum`: checksum of immutable source fixture.
- `config_file`: committed template or ignored local config path.
- `features`: machine-readable feature map.
- `support_status`: `supported`, `unsupported`, or `diagnostic-only`.
- `unsupported_reason`: required unless `support_status: supported`.

Toast feature inventory:

- Extract all compile/config options from the actual latest upstream Toast
  source used for the build.
- Required source inputs:
  - `src/include/options.h`
  - `src/include/version.h`
  - `src/include/version_opt_gen.pl`
  - build-system definitions and compiler flags
  - generated `version_options.h` or equivalent build output
- Required runtime confirmation:
  - `server_version()` / `server_options()` or equivalent MOO-visible output
    where available
  - targeted probes for options whose runtime state is not exposed directly
- The extracted option list is committed as generated profile metadata only
  when intentionally promoted; otherwise it remains a diagnostic artifact.

Initial feature keys:

- `option.OUTBOUND_NETWORK`
- `builtin.open_network_connection`
- `builtin.listen`
- `builtin.curl`
- `builtin.url_encode`
- `builtin.url_decode`
- `builtin.sqlite`
- `builtin.pcre_match`
- `builtin.pcre_replace`
- `builtin.fileio`
- `platform.path_separator`
- `platform.backslash_is_path_separator`
- `platform.memory_usage_statm`
- `platform.ctime_timezone_style`
- `runtime.arch_bits`

This list is a seed, not the final truth. The final feature schema must be
generated from Toast's complete option inventory plus observed optional builtins.

Barn feature inventory:

- Barn must have a Go-native config model with a `.conf` file interface before
  any Toast option-sensitive comparison is accepted.
- The config file must be explicit for every managed run. Defaults are allowed
  for normal operation, but conformance profiles must not rely on defaults.
- Barn feature metadata comes from the `.conf`, runtime platform detection, and
  MOO-visible introspection/probes.
- Barn extension features use the namespace `barn_extension.*` and are never
  treated as Toast conformance unless Toast exposes the same feature.

Barn `.conf` seed:

```conf
OUTBOUND_NETWORK = 1
```

```conf
OUTBOUND_NETWORK = 0
```

Support accounting:

- `supported`: Barn claims this profile should match Toast.
- `unsupported`: Barn does not claim support. The profile is explicit debt and
  does not count as pass/fail conformance.
- `diagnostic-only`: useful to run, but not comparable to a Toast profile
  because platform, feature set, or fixture differs.
- `invalid-comparison`: produced by the harness when a Toast/Barn pair has
  mismatched feature metadata.

Issues to prove:

- Some Toast features are compile-time options; Barn should prefer runtime
  `.conf` options when practical, but the profile metadata must still identify
  the matching Toast build.
- Optional library presence can create or remove builtins. Tests must gate on
  builtin presence separately from option values.
- Platform behavior is part of the profile. Linux Toast vs Windows Barn is
  invalid for path, time-zone, process, memory, and filesystem semantics unless
  the test declares itself platform-insensitive.
- A disabled feature can have real, correct behavior. That behavior belongs to
  the disabled profile only.
- An enabled feature can expose extra Barn behavior. That is allowed only under
  a supported matching profile or a declared Barn extension profile.

Done when:

- The complete Toast option inventory for the chosen upstream SHA is recorded.
- Each managed Toast and Barn target emits a profile manifest before tests run.
- The harness can reject a Barn/Toast comparison with mismatched feature values.
- `OUTBOUND_NETWORK` is represented as a real profile feature for both Toast and
  Barn, not as an accidental skip or hardcoded behavior.

## WS1B Managed Build/Profile Matrix

Purpose: build and run every supported oracle profile deliberately.

Required initial profile set:

- `toast-linux-testdb-outbound-on`
- `toast-linux-testdb-outbound-off`
- `barn-linux-testdb-outbound-on`
- `barn-linux-testdb-outbound-off`
- `toast-linux-mongoose-outbound-on`
- `toast-linux-mongoose-outbound-off`
- `barn-linux-mongoose-outbound-on`
- `barn-linux-mongoose-outbound-off`

Diagnostic profiles:

- `barn-windows-testdb-outbound-on`
- `barn-windows-testdb-outbound-off`
- `barn-windows-mongoose-outbound-on`
- `barn-windows-mongoose-outbound-off`

The Windows Barn profiles are diagnostic until there is a matching Windows Toast
oracle or the test declares itself platform-insensitive. If Barn cannot run a
Linux profile yet, that profile is `unsupported` and must be reported as such.

Profile-pairing rules:

- `toast-linux-testdb-outbound-on` compares only to
  `barn-linux-testdb-outbound-on`.
- `toast-linux-testdb-outbound-off` compares only to
  `barn-linux-testdb-outbound-off`.
- `toast-linux-mongoose-outbound-on` compares only to
  `barn-linux-mongoose-outbound-on`.
- `toast-linux-mongoose-outbound-off` compares only to
  `barn-linux-mongoose-outbound-off`.
- Diagnostic profiles do not contribute to Toast conformance totals unless
  promoted to supported matching profiles.

Build artifacts:

- Each Toast profile gets its own build directory or a recorded build flag set
  that proves the option state.
- Each Barn profile gets its own `.conf` file and run manifest.
- Build outputs include SHA, build command, option metadata, and binary path.
- Reusing a binary across profiles is allowed only when profile differences are
  runtime config values and the manifest proves that.

Done when:

- A single managed command can list all profiles and their support status.
- A single managed command can build or verify every supported profile.
- A single managed command can run a selected Toast/Barn profile pair and abort
  on metadata mismatch.
- No conformance run depends on an unnamed Toast build.

## WS2 Dual MOO Bridge

Purpose: let an AI agent operate on Toast/Mongoose and Barn/Mongoose in parallel
and see differences directly.

Target architecture:

- One bridge session per server, wrapped by one controller.
- Commands:
  - send to Toast only
  - send to Barn only
  - send to both
  - read Toast
  - read Barn
  - read both
  - mark transcript
  - export transcript
- Every output line is tagged with server, connection, timestamp, and source
  channel.
- Raw bytes are retained for debugging; normalized text is shown to the agent.

Reference material from `../mongoose/bridge`:

- `telnet_connection.py`: telnet IAC parsing and GMCP subnegotiation.
- `mud_connection.py`: timeout-based raw MUD reads.
- `bridge.py`: prompt-driven login, async buffering, modal editor awareness,
  channel/page normalization, and agent-facing output batching.
- `config.py`: named world/session config.

Do not copy blindly:

- Claude/Codex backend orchestration is not the core requirement for this repo.
- Mongoose-specific channel display rules should be optional normalizers.
- Bridge credentials and local config must not enter committed files.

Issues to prove:

- The bridge is the connectivity proof. Do not split off a standalone raw-socket
  banner check as a workstream milestone; that creates a manual/debug side path
  instead of advancing the managed controller.
- Mongoose login is prompt-driven: username/email, password, character select.
- Output is asynchronous; command-response pairing is often approximate.
- GMCP/OOB output may duplicate plain text and must be normalized or tagged.
- Modal editor input needs explicit handling if probes use `@edit`,
  `@program`, `@notedit`, or similar commands.
- The agent needs both summarized differences and raw transcripts.

Done when:

- The agent can log into both Toast and Barn on disposable Mongoose DB copies.
- A `send-both "look"` style action produces tagged output from both servers.
- The bridge can preserve raw transcripts while showing normalized diffs.
- Disconnect/reconnect behavior is observable without losing transcript context.

## WS3 Probe Library

Purpose: make exploratory behavior checks replayable.

Probe format:

- Name and target behavior.
- Required login state and permissions.
- Ordered input events.
- Wait/read policy after each input.
- Normalizers to apply.
- Expected comparison mode: exact, contains, regex, value, error, or
  human-review.
- Cleanup commands if the probe mutates the DB.

Initial probe families:

- Login and account selection.
- `look`, room rendering, contents, exits, and confunc errors.
- Command parser surfaces: quotes, semicolon eval, shortcut verbs, `huh`.
- Object inspection commands: `@display`, `@props`, `@verbs`, `@dump`.
- WAIF and property surfaces observed in Mongoose rooms.
- Connection lifecycle: connect, disconnect, reconnect, boot, listener hooks.
- Task behavior: fork, suspend, resume, queued tasks, timeouts.
- Persistence: dump/checkpoint/restart observations.
- Modal editing and verb programming workflows.

Issues to prove:

- Some probes are inherently stateful and must run on fresh DB copies.
- Some differences are database-content differences, not server differences.
- Some output differs because one side emits extra diagnostics.
- Timing-sensitive probes need retry or event-driven reads, not blind sleeps.

Done when:

- Each probe can be run against Toast, Barn, or both.
- Probe output includes the exact input sequence and run manifest.
- A failed probe can be replayed without reconstructing manual steps.

## WS4 Diff Classifier

Purpose: turn probe transcripts into actionable suspected divergences.

Difference classes:

- Output text difference.
- MOO value difference.
- MOO error difference.
- Traceback shape or line-number difference.
- Missing/extra notification.
- Timing/order difference.
- Persistence difference after checkpoint/restart.
- Server log difference.
- Harness artifact.
- Database-content difference.

Normalizers:

- Telnet negotiation bytes.
- ANSI where irrelevant.
- Line endings.
- Ports and connection IDs.
- Timestamps and durations.
- Checkpoint filenames and temp paths.
- Known channel/page duplicate forms.

Issues to prove:

- Over-normalization can hide real behavior.
- Under-normalization creates noise the agent cannot use.
- Toast may have nondeterministic ordering for some async events.
- Barn may produce extra debug output; decide whether user-visible, log-only, or
  diagnostic-only.

Done when:

- A probe run produces a ranked list of candidate differences.
- Each candidate links to raw Toast transcript, raw Barn transcript, and the
  normalizer decisions applied.
- Harness artifacts can be dismissed with an explicit reason.
- No candidate is marked actionable for Barn implementation until WS5 has
  produced a Toast-passing conformance test.

## WS5 Conformance Test Factory

Purpose: convert confirmed Toast behavior into durable YAML tests.

Target architecture:

- For every accepted divergence, first write the smallest managed conformance
  test that proves expected behavior at a named profile point.
- Add a focused YAML test in `../moo-conformance-tests`.
- The test must pass on Toast through the conformance harness with matching
  profile metadata.
- The test must then be run unchanged on Barn and should fail before any Barn
  fix at the matching Barn profile, unless Barn already matches and the test is
  pure coverage.
- The Toast pass and Barn pre-fix result are the handoff artifact for WS7.
- Commit conformance test changes in the conformance repo independently from
  Barn fixes.

Test design rules:

- Prefer small behavior-specific tests over full Mongoose transcripts.
- Use Mongoose DB only when the behavior cannot be reproduced in `Test.db` or a
  small fixture.
- Use feature requirements for optional and configurable behavior. Do not encode
  one profile's result as global behavior.
- Use `requires.features` for mandatory feature states.
- Use `matrix` when one behavior has multiple valid expectations across
  profiles.
- Use `skip_if: "missing builtin.foo"` only for genuine builtin absence. Do not
  use it to hide a disabled option or missing profile.
- Use managed server mode for lifecycle and persistence tests.
- Use raw `command:` and secondary connection steps for player-facing behavior.

Feature-gated examples:

```yaml
requires:
  features:
    builtin.url_encode: present
    option.OUTBOUND_NETWORK: true
```

```yaml
requires:
  features:
    builtin.url_encode: present
    option.OUTBOUND_NETWORK: false
expect:
  error: E_PERM
```

Matrix example:

```yaml
matrix:
  - when:
      option.OUTBOUND_NETWORK: true
    expect:
      value: "a%20b%2Fc"
  - when:
      option.OUTBOUND_NETWORK: false
    expect:
      error: E_PERM
```

Issues to prove:

- The local `../moo-conformance-tests` checkout is ahead of Barn's pinned
  GitHub dependency. Promotion requires a pushed commit or dependency update to
  a non-local revision.
- Some desired tests need harness primitives before YAML can express them.
- Tests that require credentials or live Mongoose state are not acceptable as
  normal conformance tests; they need distilled fixtures.
- Every option-sensitive test must prove which profile it targets. A test that
  depends on an option but lacks feature requirements is incomplete.
- A Barn-only extension test belongs in an extension suite/profile and must not
  be counted as Toast conformance.

Done when:

- Every new test has a Toast verification command, profile ID, feature manifest,
  and result recorded.
- The test is committed in `moo-conformance-tests`.
- Barn's expected result at the matching profile is known: fail-before-fix,
  coverage-only pass, unsupported profile, or invalid comparison.
- If the behavior was discovered interactively on Mongoose, the transcript is
  linked only as discovery evidence; the conformance test is the authority.

## WS6 Harness Extensions

Purpose: add only the primitives needed to express confirmed behavior.

Candidate extensions:

- Dual-server oracle mode for comparison runs.
- WSL-aware managed server command wrapper.
- Profile manifest loading and validation.
- Feature requirement evaluation for YAML tests.
- Matrix expectation selection keyed by profile features.
- Build-profile registry for Toast and Barn targets.
- Barn `.conf` generation/selection for managed runs.
- Login script support for prompt-driven databases.
- Explicit transcript assertion steps.
- Per-test DB fixture selection where suite-level `server_db` is too coarse.
- DB output assertions for checkpoint/startup-repair behavior.
- Better event-driven reads for async output.

Existing primitives already present:

- Managed server lifecycle.
- Temp DB copies.
- `server_db` fixture selection.
- Raw `command:` steps.
- Secondary connections.
- Raw byte sends.
- Restart server step.
- Log/file assertions.

Issues to prove:

- Do not add harness machinery until a real probe/test needs it.
- Dual-server discovery mode may belong in Barn tooling while single-server
  assertions remain in `moo-conformance-tests`.
- WSL path handling must be explicit and tested.
- Harness diagnostics are not convergence progress unless they produce tests.
- Feature evaluation must fail closed. Unknown feature metadata means the test
  is not runnable for that target.
- A disabled feature and an absent builtin are different states.
- A platform mismatch is not a skip; it is either diagnostic-only or an invalid
  comparison unless the test declares platform independence.
- Harness summaries must separate `passed`, `failed`, `skipped`,
  `unsupported-profile`, and `invalid-comparison`.

Done when:

- The new primitive is covered by harness unit tests.
- At least one Toast-verified conformance test uses it.
- Existing conformance tests still run through the managed server flow.
- A run against mismatched `OUTBOUND_NETWORK` profiles aborts before executing
  option-sensitive tests.
- A run against a supported matching profile pair executes the enabled and
  disabled network expectations in the correct profile only.

## WS7 Barn Fix Loop

Purpose: fix Barn against Toast-verified tests.

Loop:

1. Read the conformance test, profile ID, feature manifest, and exact Toast
   pass command/result.
2. Select the matching Barn profile. If the profile is unsupported, record
   unsupported coverage debt and do not edit Barn.
3. Run the same test on Barn through the managed command at the matching
   profile.
4. If Barn does not fail, stop and classify the test as coverage-only or stale;
   do not edit Barn.
5. Inspect Barn code only after the Barn pre-fix failure is observed.
6. Make the smallest production change.
7. Run the focused Barn test and verify it now passes at the matching profile.
8. Run the relevant broader suite for that profile.
9. Commit the Barn source edit atomically.
10. Record the behavior family as closed for that profile.

Violation recovery:

- If a Barn source edit was made before the conformance/profile gate, abandon
  the experiment branch or hard-reset it to the last valid base. Do not produce
  revert-noise commits for bad speculative work unless the user explicitly asks.
- Then add the conformance test, prove Toast pass, prove Barn fail, and reapply
  only the minimal fix required by that test at the matching profile.
- Do not count manual probes, DB inspectors, ad hoc Go helpers, or source
  analysis as substitutes for the Toast-pass/Barn-fail gate.
- If the mistake came from a misbuilt Toast profile, discard all conclusions
  drawn from that profile and rebuild the profile matrix before continuing.

Issues to manage:

- Barn currently has many untracked files; commits must be path-limited.
- Existing generated diagnostics may be modified by runs and must not be
  committed accidentally.
- Some fixes touch shared VM/server behavior and need broader verification.
- If a reported issue is already fixed or not reproducible, stop and report the
  premise change before writing code.
- Extra Barn behavior is allowed only if it does not alter Toast-compatible
  behavior at a supported profile point, or if it is behind an explicit
  `barn_extension.*` profile.

Done when:

- The new conformance test passes on Barn.
- No existing conformance tests regress in the relevant suite.
- The Barn commit references the conformance behavior it closes.
- The run record says which Toast profile and Barn profile were compared.

## WS8 Coverage Expansion

Purpose: add hundreds of meaningful Toast-verified tests after the bridge and
factory are operating.

Priority batches:

1. Profile/feature matrix coverage for Toast options and optional builtins.
2. HTTP parsing and connection interaction.
3. Command parser and `huh` dispatch gaps.
4. SQLite behavioral suite.
5. URL, `curl`, Argon2, and related extension behavior.
6. Listener/network/connection state behavior.
7. Canned DB startup repair and finalization behavior.
8. Background/thread surfaces.
9. Smaller extension builtins: `read_stdin`, `spellcheck`, `simplex_noise`.
10. Mongoose-discovered real-world WAIF/property/verb/task cases.

Known gap data:

- `../moo-conformance-tests/reports/toaststunt-gap-analysis-2026-04-01.md`
  estimates roughly 260-450 useful tests from the first five batches.
- `../moo-conformance-tests/reports/barn-toast-deviation-conformance-checklist.md`
  shows many earlier Barn/Toast deviation families already covered, so new work
  should avoid re-adding those cases.

Issues to prove:

- Some Toast extension behavior may depend on build-time optional libraries.
- Some surfaces are hard to test portably on Windows plus WSL.
- Some tests require reliable cleanup to avoid poisoning later tests.
- Canned DB tests need careful fixture ownership.
- Profile coverage is not the same as semantic coverage. A behavior tested only
  with `OUTBOUND_NETWORK=false` is not covered for `OUTBOUND_NETWORK=true`.
- Barn extension coverage is useful but must be reported separately from Toast
  conformance.

Done when:

- The suite grows by large batches of Toast-passing tests.
- Each batch has a coverage report and Barn result.
- Shared failures, Barn-only failures, skipped tests, unsupported profiles,
  invalid comparisons, and Barn-extension results are separated.

## WS9 Regression Dashboard

Purpose: make long-running convergence measurable.

Tracked rows:

- Run ID.
- Barn commit.
- Conformance repo commit.
- Source DB checksum.
- Toast profile ID.
- Barn profile ID.
- Toast feature manifest checksum.
- Barn feature manifest checksum.
- Profile comparison status: `matched`, `unsupported`, `diagnostic-only`, or
  `invalid-comparison`.
- Toast command and version.
- Barn command.
- Test selector.
- Toast pass/fail/skip counts.
- Barn pass/fail/skip counts.
- Barn-only failures.
- Shared failures.
- Unsupported-profile count.
- Invalid-comparison count.
- Barn-extension count.
- New tests added since previous run.
- Runtime and timeout count.

Artifacts:

- Machine-readable summary JSON.
- Human-readable trend report.
- Barn-only failure list.
- Shared failure list.
- Recently added test list.
- Links to run logs.

Issues to prove:

- Dashboard data must not be confused with source changes.
- Historical reports in this repo are stale and should not be treated as current
  truth without rerunning.
- Counts from pinned and local conformance repos are not directly comparable.

Done when:

- One command can produce a current Barn-vs-Toast conformance summary.
- The dashboard separates `Test.db` conformance, Mongoose-probe discovery,
  unsupported profile debt, invalid comparisons, and Barn extension results.
- A regression can be traced to a commit and test batch.

## First Execution Slice

The first slice should not touch Barn behavior. It must establish the profile
matrix before any conformance behavior is trusted.

1. [x] Create ignored artifact directories.
2. [x] Fetch or verify the source `mongoose.db.new` artifact from
   `root@mongoose.world`.
3. [x] Reset or abandon any experiment branches containing work derived from an
   unverified or misconfigured Toast profile.
4. [x] Extract the complete Toast option inventory from latest upstream source and
   generated build output.
5. [x] Build the required Toast profiles, starting with
   `toast-linux-testdb-outbound-on` and `toast-linux-testdb-outbound-off`.
6. [x] Add or verify Barn `.conf` files for the matching Barn profiles.
7. [x] Build Barn profiles or mark unsupported profiles explicitly.
8. [x] Start one matching Toast/Barn profile pair against disposable DB copies.
9. [ ] Build the first managed bridge connection path and use it to capture
   tagged Toast and Barn banners from the managed profile pair.
10. [x] Write the run manifest, including profile IDs and feature manifests.

Current status:

- Source Mongoose DB identity verified from
  `.tmp/mongoose-oracle/source/mongoose.db.new`:
  `0F90CA1D766523E50C719ABCAEE9D949EDDAFC28E4D047FB662CE6A1C5BE1273`.
- Toast Linux `OUTBOUND_NETWORK` on/off builds exist at upstream commit
  `7e6e4a5e17a86b78f14bdbce00c234034e6e61a7`.
- Barn now has explicit profile configs, profile manifests, a profile registry,
  and runtime `OUTBOUND_NETWORK` behavior.
- Metadata gate rejects Toast-on versus Barn-off before test execution, and
  accepts Toast-off versus Barn-off.
- Toast-off and Barn-off both pass
  `open_network_connection_disabled_returns_perm` through the conformance
  harness.
- The remaining unchecked first-slice work is the first managed bridge
  connection path, not a standalone raw-socket/manual banner probe.

Stop conditions:

- SSH fetch fails.
- Toast cannot be built or launched in WSL.
- Toast option extraction is incomplete or disagrees with runtime behavior.
- Barn lacks a `.conf`/profile mechanism for an option-sensitive comparison and
  the profile has not been marked unsupported.
- Either server cannot start from a disposable DB copy.
- The bridge cannot distinguish server startup failure from login/protocol
  failure.
- A conformance run is attempted without matching profile metadata.

## Open Design Questions

- Should dual-server discovery live in Barn tooling, `moo-conformance-tests`, or
  a small separate tool? Current bias: separate discovery tool or Barn-side
  script, with durable assertions promoted into `moo-conformance-tests`.
- Should the bridge be interactive first or probe-file first? Current bias:
  probe-file core with an interactive shell on top.
- How much Mongoose-specific login knowledge belongs in committed config?
  Current bias: committed schema and examples only; secrets and live account
  data remain local.
- Should Mongoose-derived tests use a reduced fixture DB? Current bias: yes
  whenever the behavior can be distilled.
- Which Toast options should Barn intentionally not support? Current rule: every
  unsupported Toast profile must be named with a reason and reported as coverage
  debt, not hidden by skips.
- Which Barn extensions should be first-class? Current rule: extensions are
  allowed as a superset only under `barn_extension.*` profiles and never count
  toward Toast conformance totals.
- Should profile manifests live in `moo-conformance-tests` or Barn tooling?
  Current bias: target/profile registry in `moo-conformance-tests`; Barn owns
  `.conf` files and runtime option exposure.
