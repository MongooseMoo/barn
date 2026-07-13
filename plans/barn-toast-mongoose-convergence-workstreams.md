# Barn/ToastStunt/Mongoose Convergence Plan

## Goal

Make Barn run the real Mongoose workload with exact Toast-compatible behavior,
and keep it compatible by converting every discovered delta into a durable
conformance test.

The required loop is:

1. observe a concrete Mongoose failure or divergence;
2. reduce it to the smallest useful conformance scenario;
3. prove the unchanged scenario passes the WSL Toast oracle;
4. prove it fails on Barn before changing Barn;
5. make the smallest Barn production change that closes the delta;
6. prove the unchanged scenario passes on Barn;
7. run the relevant managed suite and commit the conformance and Barn slices in
   their respective repositories.

Tests in their final state pass on both Toast and Barn. Before the Barn fix, the
new test must pass on Toast and fail on Barn. If Barn already passes a genuine
candidate test, keep it as coverage and do not invent a Barn source change.

This is a plan only. No current Mongoose regression was reproduced while
writing it; reproducing the present break through the managed harness is an
explicit execution milestone.

## Repositories And Authority

- `C:/Users/Q/code/barn` owns Barn implementation, runtime profiles, local
  diagnostics, and Barn-side verification.
- `C:/Users/Q/code/moo-conformance-tests` owns durable behavioral truth.
- `C:/Users/Q/code/mongoose` supplies the real workload and its database/client
  behavior. It is a discovery source, not a substitute for conformance.
- WSL Toast is the behavioral oracle. Do not substitute a Windows Toast binary.

For ordinary Toast behavior, use:

- `/root/src/toaststunt/build-release/moo`
- stock Toast commit `aecc51e9449c6e7c95272f0f044b5ba38948459e`

For Mongoose `PROMOTE_NUMBERS` behavior, use the pinned WSL Mongoose build:

- `/root/src/toaststunt-mongoose/build-release/moo`
- detached worktree commit `72e3c7f96ce7a41fdeba793aef8818dc4408072e`

The Barn-local WSL oracle procedure is documented in
`reports/toast-oracle-wsl.md`. The managed wrapper is
`scripts/run_toast_wsl.sh`.

## Recovered Records

The useful prior records are:

- `reports/toast-oracle-wsl.md`: canonical WSL oracle path and database notes;
- `notes/mongoose-differential-2026-07-01.md`: the detailed successful July 1
  Mongoose differential campaign;
- `notes-mongoose-promote-and-login.md`: `PROMOTE_NUMBERS`, trusted-PROXY, and
  `read()`-based login findings;
- `notes/observability-slog-2026-07-11.md`: the structured logging, metrics, and
  pprof implementation record;
- `src/moo_conformance/_tests/builtins/yin_semantics.yaml`: durable `yin()`
  coverage in the conformance repository;
- `src/moo_conformance/_tests/builtins/connection_name_semantics.yaml`: durable
  basic connection-name coverage;
- `src/moo_conformance/_tests/audit/connection_lifecycle_toast_oracle.yaml`:
  partial trusted-PROXY and login lifecycle coverage.

The existing live-login tools are part of the recovered authority, not examples
to recreate:

- `cmd/moo_client/main.go` and the built `moo_client.exe` drive a real socket,
  preserve partial prompt bytes, and provide `-banner-wait`, `-inter-cmd`, and
  idle `-timeout` controls;
- `reports/read-login-verifier.md` records the proven Mongoose settings
  `-banner-wait 3000 -inter-cmd 2500` and the exact trusted-PROXY -> account ->
  password interaction;
- `notes/mongoose-differential-2026-07-01.md` records the July 1 fresh database
  fetch and repeated live use of `moo_client.exe`;
- `cmd/toast_oracle` and `scripts/wsl_oracle.sh` are emergency-expression tools.
  They do not replace `moo_client.exe` for login, connection, or room-render
  behavior.

Do not scrape notes to reconstruct a client, replace `moo_client.exe` with an
ad hoc socket script, or infer the login conversation from a nearby database.
Run the existing tool and observe the selected fixture first.

The July 1 campaign landed six Barn fixes but only two corresponding conformance
suites. The real trusted-PROXY -> account login -> room render path, startup
responsiveness, burst input, and most gameplay behavior were not captured as a
single durable cross-engine gate.

## Current Profile And Process Gaps

### Mongoose profiles do not enable Mongoose semantics

`profiles/barn/profiles.json` names Mongoose profiles, but their config files
only set `OUTBOUND_NETWORK`. They do not set `PROMOTE_NUMBERS = 1`.

Barn already publishes `option.PROMOTE_NUMBERS` in profile manifests. However,
the conformance profile gate currently requires only
`option.OUTBOUND_NETWORK`. A strict-mode target can therefore be accepted as a
Mongoose comparison.

### Active oracle guidance has drifted

`CLAUDE.md` still contains a Windows Toast server command even though the WSL
oracle report and current project instructions require WSL Toast. The correct
procedure must live in tracked active instructions, not only in an untracked
side report.

### Existing PROXY coverage is incomplete

The current conformance row proves that a trusted `PROXY` prelude is cleared
before `do_login_command` runs. It does not prove:

- adoption of the announced client address;
- the Mongoose account-login conversation;
- successful room entry and rendering;
- input delivery while the Mongoose world is busy;
- burst input through `hold-input` and `read()`;
- reconnect and disconnect hooks in the real connection contract.

## Authoritative Fixture Identity

The earlier plan incorrectly treated `mongoose.db` matching a recorded checksum
as proof that it was fresh. A checksum proves identity, not provenance. All
July 13 managed results using this file are rejected:

- rejected file: `C:/Users/Q/code/barn/mongoose.db`;
- size: `100959239` bytes;
- SHA-256: `a9d167861eab56d62e9bd12ae1d47c5e6a858530020a5dcf174a0b104fb23db9`;
- rejection reason: no proof that the file was a fresh upstream
  `mongoose.db.new` rather than a Barn-written or otherwise evolved copy.

The recovered known-good fixture is the file Claude fetched from
`mongoose@mongoose.world:~/mongoose/mongoose.db.new` on July 1:

- local path: `C:/Users/Q/code/barn/mongoose_fresh2.db`;
- size: `101244108` bytes;
- SHA-256: `33201970097d3d2d2bfc0d5f875f087d587601bf8255ef31ef19b416d65ac925`;
- provenance record: `notes/mongoose-differential-2026-07-01.md`;
- matching untouched WSL discovery copy: `/tmp/mg_in.db`.

Use this exact snapshot for the current convergence row. Re-hash it immediately
before every authoritative comparison, copy it to a disposable run location,
and never run Barn or Toast directly against the source file. A future live
`mongoose.db.new` is a new fixture identity and must be fetched, recorded, and
baselined on Toast before it replaces this snapshot.

## Corrected Execution State (2026-07-13)

Milestones 0 and 1 are committed. The first Milestone 2 attempts are invalid
because they used the rejected fixture and a reconstructed login invocation.
They are not evidence that Mongoose fails on Toast and must not be used to
justify a Barn change.

The recovered Claude workflow was rerun successfully on July 13 with:

- WSL Mongoose Toast source commit
  `72e3c7f96ce7a41fdeba793aef8818dc4408072e`;
- `/root/src/toaststunt-mongoose/build-release/moo`, executable SHA-256
  `a748a93644fe2b973cc85dfed902454a0a56c8b368afdc8104161ec76154d098`;
- a disposable copy of `mongoose_fresh2.db`;
- `moo_client.exe -banner-wait 3000 -inter-cmd 2500 -timeout 15`;
- the trusted-PROXY prelude followed by the account and password prompts.

The result was a successful connection as player `#249` and a complete room
render. Stable observed anchors were:

- `[Toint Town; Zemilda's Tea House]`;
- `The interior of the tea house is warm and close`;
- `You can go northwest.`

A later `Confunc failed: This database is not open.` traceback occurred after
the room render in optional sound/database code. It does not negate successful
login or room control, but it is a separate candidate delta only after it is
reproduced through the Toast-first row.

## Structured Observability

Barn now has the diagnostics needed for this campaign:

- every run writes readable stderr and structured JSONL;
- `logs/latest.jsonl` is the stable current-run path and older runs rotate;
- records carry fields such as `conn_id`, `task_id`, `player`, `this`, `verb`,
  `error`, `err`, `traceback`, `frames`, and `go_stack`;
- an uncaught MOO error is one record containing the player-visible traceback,
  structured frames, and source lines;
- `barn_logs` filters a run, expands tracebacks and Go stacks, and exits nonzero
  if the selected records include an error;
- `/debug/vars` exposes task, connection, GC, panic, exception, and checkpoint
  counters;
- `/debug/pprof/` exposes profiles on a localhost-only endpoint;
- `/debug/loglevel` changes the running server's log level without restarting
  it.

These are diagnostic tools, not an oracle. A plausible JSON record, counter, or
profile does not replace a Toast-passing conformance test.

Some log message text is already a conformance contract. Preserve asserted text
byte-for-byte and add structured attributes rather than rewording it.

## Git Accountability

Before editing in either repository:

1. record branch, HEAD, and tracked-file status;
2. identify the exact intended files for the active slice;
3. do not combine multiple experimental source slices in one worktree;
4. finish each slice as either a committed kept improvement or a full revert;
5. commit conformance changes in `moo-conformance-tests` and Barn changes in
   `barn` separately;
6. stage exact paths only and preserve unrelated untracked files.

Experiment records required by this plan are repository records, not chat-only
summaries.

## Milestone 0: Repair The Durable Control Surface

Reconcile the useful untracked Mongoose/oracle records into this tracked plan
and the active project instructions.

Required work:

- replace stale Windows Toast commands with the WSL-managed workflow;
- record the exact Barn SHA, both Toast SHAs, selected database checksum, and
  connection contract for each run;
- document the environment variable used to provide the uncommitted login
  script without recording credentials;
- remove or clearly supersede conflicting process instructions;
- commit the documentation-only correction before behavioral source work.

Acceptance criteria:

- another agent can start the exact managed oracle workflow using tracked files
  alone;
- no active instruction directs Barn conformance work to Windows Toast;
- the selected fixture and engine identities are explicit.

## Milestone 1: Make Profiles Truthful

Add explicit Mongoose profiles rather than reusing strict profiles with a
Mongoose name.

Required work:

- add Mongoose config files with `PROMOTE_NUMBERS = 1` and an explicit outbound
  networking value;
- include `option.PROMOTE_NUMBERS` in each Mongoose profile's expected features;
- require promotion compatibility in the conformance profile gate;
- define stock WSL Toast and WSL Mongoose Toast oracle manifests;
- fail closed when promotion metadata is missing or mismatched.

Use two validation lanes:

1. WSL Toast versus WSL Barn for OS-matched semantic conformance;
2. Windows Barn against the same tests and fixture for the real deployment
   proof.

This avoids lying about `runtime_os` while still proving the Windows target.

Acceptance criteria:

- a strict Barn profile cannot masquerade as Mongoose-compatible;
- a missing or mismatched promotion feature prevents tests from starting;
- the selected database checksum and runtime options appear in both manifests.

## Milestone 2: Capture The Present Break

Start from the proven Claude tool and fixture, then transfer that exact behavior
into the managed conformance harness. Do not begin with a reconstructed login
script or an unproven database. Do not modify the conformance transport: it is
an established measurement surface, and no transport defect has been shown.

The discovery proof and the durable gate are distinct required artifacts:

1. **Known-good live proof.** Run `moo_client.exe` against the pinned WSL
   Mongoose Toast build using a disposable copy of `mongoose_fresh2.db`,
   `-banner-wait 3000`, `-inter-cmd 2500`, and `-timeout 15`. Send the
   trusted-PROXY prelude, then answer the account and password prompts exactly
   as the selected fixture presents them. Do not add `connect`, `login`, or a
   character-selection line unless the live prompt for this exact fixture asks
   for it.
2. **Managed conformance proof.** Use the same freshly hashed source fixture via
   the managed disposable lifecycle. Supply the already-prepared, uncommitted
   login conversation through the existing login-script environment mechanism;
   do not scrape a note to synthesize it and do not commit or print credentials.

The July 13 `moo_client.exe` proof completed step 1. The earlier managed runs do
not satisfy step 2 because they used the rejected `mongoose.db` fixture.

The first scenario should exercise the actual connection contract:

1. freshly verify `mongoose_fresh2.db` at
   `33201970097d3d2d2bfc0d5f875f087d587601bf8255ef31ef19b416d65ac925`;
2. start WSL Mongoose Toast through the managed harness with a disposable copy;
3. send the trusted `PROXY` prelude;
4. complete the exact account login observed with `moo_client.exe`;
5. prove post-login eval and command control;
6. run the unchanged scenario against current Barn.

Stable post-login assertions should include:

- the announced client IP is reflected by `connection_name`;
- `option.PROMOTE_NUMBERS` is enabled;
- mixed integer/float arithmetic follows Mongoose Toast behavior;
- `look` reaches at least two of the observed stable room-render anchors rather
  than guessed or volatile world state;
- the server responds within measured, oracle-derived deadlines.

The test must pass WSL Toast first. If Barn already passes, keep the test as
coverage and do not patch Barn. If Barn fails, record the exact failed assertion
as the active behavior row.

Acceptance criteria:

- the exact `moo_client.exe` proof command shape, WSL Toast identity, fresh
  fixture identity, and successful connection/room anchors are recorded without
  credentials;
- the exact managed Toast command, profile manifest, fixture checksum, focused
  test name, and pass result are recorded;
- the same unchanged test produces a concrete Barn result;
- `src/moo_conformance/transport.py` has no change for this row;
- no Barn production change precedes the Toast result and Barn red proof.

## Milestone 3: Diagnose The Red Barn Run

Run Barn through the managed harness with its structured log directed to a
stable run directory.

Diagnostic sequence:

1. run `barn_logs -level error`;
2. correlate `conn_id`, `task_id`, player, verb, traceback, frames, and source;
3. snapshot `/debug/vars` before the PROXY prelude, during login, and after the
   observed timeout or failure;
4. if the server is alive but stalled, collect pprof from the already-running
   process;
5. raise the running server to debug only when a named decision requires more
   detail, then turn it back down.

Add instrumentation only when current evidence cannot distinguish two concrete
hypotheses. Extend existing `slog` or metric choke points; do not introduce a
new logging facade, sender, adapter, or service container.

Acceptance criteria:

- the failing ownership boundary is supported by evidence;
- diagnostics are preserved with the run record;
- logs and profiles are not presented as substitutes for the conformance row.

## Milestone 4: Close One Delta At A Time

Every behavior row follows this state machine:

`Mongoose observation -> reduced test -> Toast green -> Barn red -> Barn fix -> Barn green -> full gate`

Rules:

- prefer a reduced `Test.db` test when reduction preserves the behavior;
- retain a real-Mongoose integration test when reduction would erase the
  behavior or workload condition;
- patch the real Barn ownership boundary, not scattered symptoms;
- run focused Go tests, the focused conformance row, the relevant managed suite,
  and `git diff --check`;
- commit the conformance test and Barn fix separately before selecting another
  source slice;
- if two consecutive slices on the active target produce no kept improvement,
  stop and report that result instead of widening the search.

The first behavior family is boot, login, and connection behavior because it
gates every later Mongoose action:

1. startup responsiveness under restored background tasks;
2. trusted-PROXY metadata rewrite;
3. account-login `read()` sequence;
4. burst input and `hold-input` behavior;
5. reconnect and disconnect hooks.

After that family closes, proceed one family at a time:

- `look`, movement, contents, exits, and room rendering;
- parser shortcuts, `huh`, and command dispatch;
- `@who`, `@display`, `@props`, `@verbs`, and object inspection;
- telnet negotiation and packet-boundary behavior;
- MCP, GMCP, and out-of-band traffic;
- task scheduling, suspended reads, queued tasks, restart, and persistence;
- remaining promotion-sensitive comparison, sorting, map, and collection
  semantics.

For raw telnet behavior, split the significant bytes across separate
`send_bytes` steps so parser state across reads is part of the regression.

## Milestone 5: Make "Amazingly" Measurable

Functional compatibility comes first. Once the boot/login gate is durable,
measure the same fixture and script on WSL Mongoose Toast and Barn.

Baseline metrics:

- database load to listening;
- PROXY prelude to first banner;
- complete login time;
- command latency during startup jobs;
- `look` and movement latency;
- settled CPU and memory;
- task and connection liveness;
- checkpoint duration.

Do not choose performance targets before measuring Toast. Record the baseline,
then define the Barn acceptance threshold in the repository before optimizing.

For each performance slice:

1. name one metric and one hypothesis;
2. capture the before measurement and relevant profile;
3. change one production surface;
4. rerun the same benchmark and conformance gate;
5. commit a measured improvement or fully revert the slice.

A profiler-looking result is not proof of a bottleneck. A performance change is
not kept unless it improves the named metric while preserving behavior.

## Completion Criteria

The workstream is complete only when:

- the real Mongoose boot/login/look gate passes WSL Mongoose Toast and Barn on
  disposable copies of the same provenance-proven fixture;
- the current snapshot is
  `mongoose_fresh2.db@33201970097d3d2d2bfc0d5f875f087d587601bf8255ef31ef19b416d65ac925`,
  or a later freshly fetched `mongoose.db.new` whose new identity and Toast
  baseline are committed before use;
- every accepted semantic delta has a Toast-passing conformance test;
- Barn passes the expanded managed suite under truthful strict and Mongoose
  profiles;
- Windows Barn passes the deployment lane on the same database checksum;
- trusted-PROXY metadata, account login, burst input, and connection lifecycle
  are durably covered;
- gameplay and persistence behavior families have no open accepted rows;
- performance targets derived from Toast are met or explicitly deferred by the
  user;
- no accepted behavior exists only in chat, memory, untracked notes, an old
  log, or an ad hoc reconstructed command;
- every kept source slice and required experiment record is committed.

Milestones 0 and 1 are complete. The active next action is the Milestone 2
managed Toast gate using `mongoose_fresh2.db`, after the successful Claude-tool
live proof recorded above. No Barn source patch should precede the managed
Toast-green result and unchanged Barn-red result.
