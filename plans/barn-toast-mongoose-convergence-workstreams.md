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
- Discovery artifacts are diagnostics. Do not commit transcripts, screenshots,
  generated logs, or DB copies unless explicitly requested.
- Dependency metadata must not pin local paths. Local checkouts can be used by
  commands, but committed dependencies must resolve from a clean checkout or a
  pushed remote revision.

## Dependency Order

The executable order is:

1. WS0 Repository And Artifact Hygiene
2. WS1 Oracle Runtime Setup
3. WS2 Dual MOO Bridge
4. WS3 Probe Library
5. WS4 Diff Classifier
6. WS5 Conformance Test Factory
7. WS6 Harness Extensions
8. WS7 Barn Fix Loop
9. WS8 Coverage Expansion
10. WS9 Regression Dashboard

WS6 can start once WS3 exposes a missing primitive, but it must not invent
harness features speculatively. WS7 starts only after WS5 has a Toast-verified
test or an existing test already proves the behavior.

The critical gate between WS4/WS5 and WS7 is non-negotiable: Barn is never fixed
from a probe transcript alone. The transcript must be distilled into a
conformance test, Toast must pass it, and Barn must fail it first.

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
  test that proves expected behavior.
- Add a focused YAML test in `../moo-conformance-tests`.
- The test must pass on Toast through the conformance harness.
- The test must then be run unchanged on Barn and should fail before any Barn
  fix, unless Barn already matches and the test is pure coverage.
- The Toast pass and Barn pre-fix result are the handoff artifact for WS7.
- Commit conformance test changes in the conformance repo independently from
  Barn fixes.

Test design rules:

- Prefer small behavior-specific tests over full Mongoose transcripts.
- Use Mongoose DB only when the behavior cannot be reproduced in `Test.db` or a
  small fixture.
- Use `skip_if: "missing builtin.foo"` for Toast extension surfaces when
  appropriate.
- Use managed server mode for lifecycle and persistence tests.
- Use raw `command:` and secondary connection steps for player-facing behavior.

Issues to prove:

- The local `../moo-conformance-tests` checkout is ahead of Barn's pinned
  GitHub dependency. Promotion requires a pushed commit or dependency update to
  a non-local revision.
- Some desired tests need harness primitives before YAML can express them.
- Tests that require credentials or live Mongoose state are not acceptable as
  normal conformance tests; they need distilled fixtures.

Done when:

- Every new test has a Toast verification command and result recorded.
- The test is committed in `moo-conformance-tests`.
- Barn's expected result is known: fail-before-fix or coverage-only pass.
- If the behavior was discovered interactively on Mongoose, the transcript is
  linked only as discovery evidence; the conformance test is the authority.

## WS6 Harness Extensions

Purpose: add only the primitives needed to express confirmed behavior.

Candidate extensions:

- Dual-server oracle mode for comparison runs.
- WSL-aware managed server command wrapper.
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

Done when:

- The new primitive is covered by harness unit tests.
- At least one Toast-verified conformance test uses it.
- Existing conformance tests still run through the managed server flow.

## WS7 Barn Fix Loop

Purpose: fix Barn against Toast-verified tests.

Loop:

1. Read the conformance test and the exact Toast pass command/result.
2. Run the same test on Barn through the managed command.
3. If Barn does not fail, stop and classify the test as coverage-only or stale;
   do not edit Barn.
4. Inspect Barn code only after the Barn pre-fix failure is observed.
5. Make the smallest production change.
6. Run the focused Barn test and verify it now passes.
7. Run the relevant broader suite.
8. Commit the Barn source edit atomically.
9. Record the behavior family as closed.

Violation recovery:

- If a Barn source edit was made before the conformance gate, revert or abandon
  that edit first.
- Then add the conformance test, prove Toast pass, prove Barn fail, and reapply
  only the minimal fix required by that test.
- Do not count manual probes, DB inspectors, ad hoc Go helpers, or source
  analysis as substitutes for the Toast-pass/Barn-fail gate.

Issues to manage:

- Barn currently has many untracked files; commits must be path-limited.
- Existing generated diagnostics may be modified by runs and must not be
  committed accidentally.
- Some fixes touch shared VM/server behavior and need broader verification.
- If a reported issue is already fixed or not reproducible, stop and report the
  premise change before writing code.

Done when:

- The new conformance test passes on Barn.
- No existing conformance tests regress in the relevant suite.
- The Barn commit references the conformance behavior it closes.

## WS8 Coverage Expansion

Purpose: add hundreds of meaningful Toast-verified tests after the bridge and
factory are operating.

Priority batches:

1. HTTP parsing and connection interaction.
2. Command parser and `huh` dispatch gaps.
3. SQLite behavioral suite.
4. URL, `curl`, Argon2, and related extension behavior.
5. Listener/network/connection state behavior.
6. Canned DB startup repair and finalization behavior.
7. Background/thread surfaces.
8. Smaller extension builtins: `read_stdin`, `spellcheck`, `simplex_noise`.
9. Mongoose-discovered real-world WAIF/property/verb/task cases.

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

Done when:

- The suite grows by large batches of Toast-passing tests.
- Each batch has a coverage report and Barn result.
- Shared failures, Barn-only failures, and skipped optional-extension tests are
  separated.

## WS9 Regression Dashboard

Purpose: make long-running convergence measurable.

Tracked rows:

- Run ID.
- Barn commit.
- Conformance repo commit.
- Source DB checksum.
- Toast command and version.
- Barn command.
- Test selector.
- Toast pass/fail/skip counts.
- Barn pass/fail/skip counts.
- Barn-only failures.
- Shared failures.
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
- The dashboard separates `Test.db` conformance from Mongoose-probe discovery.
- A regression can be traced to a commit and test batch.

## First Execution Slice

The first slice should not touch Barn behavior.

1. Create ignored artifact directories.
2. Fetch or verify the source `mongoose.db.new` artifact from
   `root@mongoose.world`.
3. Build or locate Toast in WSL and record the exact command shape.
4. Build Barn.
5. Start both against disposable DB copies.
6. Connect with a minimal raw socket client and capture banners.
7. Write the run manifest.

Stop conditions:

- SSH fetch fails.
- Toast cannot be built or launched in WSL.
- Either server cannot start from a disposable DB copy.
- The bridge cannot distinguish server startup failure from login/protocol
  failure.

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
